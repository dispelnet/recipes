// Command bump reports the smallest version bump the changes since the last
// release require, and, for a release tag, refuses a tag that bumps less.
//
// On a pull request:
//
//	go -C tools run ./cmd/bump [--releases FILE] [--summary FILE] [--comment FILE]
//
// For a release tag:
//
//	go -C tools run ./cmd/bump --tag v1.2.0 --main origin/main --releases FILE \
//	    [--incompatible] [--notes FILE] [--github-output FILE]
//
// --releases lists the tags that have a published release, one per line. A tag
// whose release failed is not a base, and only GitHub knows which those are.
// Without it, every annotated release tag is assumed published.
//
// Exit status: 0 fine, 1 refused, 2 could not run.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/dispelnet/recipes/tools/internal/bump"
	"github.com/dispelnet/recipes/tools/internal/corpus"
	"github.com/dispelnet/recipes/tools/internal/release"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// commentMarker lets the workflow find and update its own PR comment.
const commentMarker = "<!-- dispelnet-recipes-bump -->"

type options struct {
	repo, head, base, tag, main       string
	releases, notes, summary, comment string
	githubOutput                      string
	incompatible, contractOnly        bool
}

// refusal is an error that means "no", as opposed to "could not run".
type refusal struct{ error }

func run(args []string, stdout, stderr io.Writer) int {
	var o options

	flags := flag.NewFlagSet("bump", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&o.repo, "repo", "", "repository to read (default: the enclosing git repository)")
	flags.StringVar(&o.head, "head", "HEAD", "revision to compare; ignored with --tag, which compares the tag")
	flags.StringVar(&o.base, "base", "", "release tag to compare against (default: chosen by the tag contract)")
	flags.StringVar(&o.tag, "tag", "", "release tag being cut: applies the tag contract and refuses a bump that is too small")
	flags.StringVar(&o.main, "main", "", "with --tag, the branch the tag must be on (e.g. origin/main)")
	flags.StringVar(&o.releases, "releases", "", "file listing tags with a published release, one per line")
	flags.BoolVar(&o.incompatible, "incompatible", false, "the checker pinned at the base refused this tree: the release is major")
	flags.BoolVar(&o.contractOnly, "contract-only", false, "with --tag, check the tag contract and choose the base, then stop")
	flags.StringVar(&o.notes, "notes", "", "write release notes to this file")
	flags.StringVar(&o.summary, "summary", "", "append a markdown report to this file (e.g. $GITHUB_STEP_SUMMARY)")
	flags.StringVar(&o.comment, "comment", "", "write a markdown PR comment to this file")
	flags.StringVar(&o.githubOutput, "github-output", "", "append base, minimum, prerelease and latest to this file (e.g. $GITHUB_OUTPUT)")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	if err := execute(o, stdout); err != nil {
		_, _ = fmt.Fprintf(stderr, "bump: %v\n", err)

		var refused refusal
		if errors.As(err, &refused) {
			return 1
		}

		return 2
	}

	return 0
}

func execute(o options, stdout io.Writer) error {
	repo := git{dir: o.repo}
	if repo.dir == "" {
		top, err := (git{dir: "."}).output("rev-parse", "--show-toplevel")
		if err != nil {
			return err
		}

		repo.dir = top
	}

	published, err := publishedTags(repo, o.releases, stdout)
	if err != nil {
		return err
	}

	var (
		base        = o.base
		head        = o.head
		withdrawals map[string]string
		distance    release.Level
	)

	if o.tag != "" {
		if err := contract(repo, o.tag, o.main); err != nil {
			return err
		}

		if base == "" {
			base, _ = release.Base(o.tag, published)
		}

		if distance, err = release.Distance(base, o.tag); err != nil {
			return refusal{err}
		}

		message, err := repo.output("for-each-ref", "--format=%(contents)", "refs/tags/"+o.tag)
		if err != nil {
			return err
		}

		if withdrawals, err = bump.ParseWithdrawals(message); err != nil {
			return refusal{err}
		}

		head = "refs/tags/" + o.tag + "^{commit}"
	} else if base == "" {
		base, _ = release.Latest(published)
	}

	_, _ = fmt.Fprintf(stdout, "base:    %s\n", orNone(base))

	if o.contractOnly {
		return writeOutputs(o, base, "", published)
	}

	headTree, err := readTree(repo, head)
	if err != nil {
		return err
	}

	baseTree := bump.Tree{}
	if base != "" {
		if baseTree, err = readTree(repo, "refs/tags/"+base+"^{commit}"); err != nil {
			return err
		}
	}

	changes, err := bump.Compare(baseTree, headTree, withdrawals)
	if err != nil {
		return refusal{err}
	}

	minimum, reasons := bump.Minimum(changes, o.incompatible)

	_, _ = fmt.Fprintf(stdout, "minimum: %s\n", minimum)

	for _, reason := range reasons {
		_, _ = fmt.Fprintf(stdout, "  %s\n", reason)
	}

	report := markdown(base, minimum, changes, o.incompatible)

	if err := appendFile(o.summary, report); err != nil {
		return err
	}

	if o.comment != "" {
		if err := os.WriteFile(o.comment, []byte(commentMarker+"\n"+report), 0o600); err != nil {
			return err
		}
	}

	if o.notes != "" {
		if err := os.WriteFile(o.notes, []byte(bump.Notes(changes)), 0o600); err != nil {
			return err
		}
	}

	if err := writeOutputs(o, base, minimum.String(), published); err != nil {
		return err
	}

	if o.tag == "" {
		return nil
	}

	_, _ = fmt.Fprintf(stdout, "tag:     %s (%s from %s)\n", o.tag, distance, orNone(base))

	if base != "" && distance < minimum {
		return refusal{fmt.Errorf("%s is a %s bump from %s, but the changes need at least a %s; see docs/VERSIONING.md",
			o.tag, distance, base, minimum)}
	}

	return nil
}

// contract applies the tag contract rules that need git: the name, that the tag
// is annotated, and that it is on the main branch.
func contract(repo git, tag, main string) error {
	if err := release.CheckName(tag); err != nil {
		return refusal{err}
	}

	kind, err := repo.output("cat-file", "-t", "refs/tags/"+tag)
	if err != nil {
		return fmt.Errorf("tag %s: %w", tag, err)
	}

	if kind != "tag" {
		return refusal{fmt.Errorf("tag %s is lightweight; a release tag must be annotated (git tag -a), because its message carries withdrawals", tag)}
	}

	if main != "" {
		if _, err := repo.output("merge-base", "--is-ancestor", "refs/tags/"+tag+"^{commit}", main); err != nil {
			return refusal{fmt.Errorf("tag %s is not on %s", tag, main)}
		}
	}

	return nil
}

// publishedTags is the set of possible bases: tags meeting the contract's name
// and annotation rules that have a published release.
func publishedTags(repo git, file string, stdout io.Writer) ([]string, error) {
	var names []string

	if file != "" {
		body, err := os.ReadFile(file) //nolint:gosec // G304: the file is named by whoever runs the command
		if err != nil {
			return nil, err
		}

		names = strings.Fields(string(body))
	} else {
		listed, err := repo.output("tag", "--list", "v*")
		if err != nil {
			return nil, err
		}

		names = strings.Fields(listed)

		if len(names) > 0 {
			_, _ = fmt.Fprintln(stdout, "note:    no --releases given; treating every annotated release tag as published")
		}
	}

	var published []string

	for _, name := range names {
		if release.CheckName(name) != nil {
			continue
		}

		if kind, err := repo.output("cat-file", "-t", "refs/tags/"+name); err != nil || kind != "tag" {
			continue
		}

		published = append(published, name)
	}

	return published, nil
}

// readTree reads the shipped paths of a revision.
func readTree(repo git, rev string) (bump.Tree, error) {
	listing, err := repo.output("ls-tree", "-r", "-z", rev)
	if err != nil {
		return nil, err
	}

	tree := bump.Tree{}

	for _, entry := range strings.Split(listing, "\x00") {
		if entry == "" {
			continue
		}

		meta, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)

		if !ok || len(fields) != 3 || corpus.Classify(path) == corpus.Other {
			continue
		}

		if fields[0] == "120000" {
			return nil, refusal{fmt.Errorf("%s is a symlink at %s; recipes, probes and captures must be regular files", path, rev)}
		}

		if fields[1] != "blob" {
			continue
		}

		body, err := repo.raw("cat-file", "blob", fields[2])
		if err != nil {
			return nil, err
		}

		tree[path] = body
	}

	return tree, nil
}

func markdown(base string, minimum release.Level, changes []bump.Change, incompatible bool) string {
	var out strings.Builder

	out.WriteString("### Version bump\n\n")

	if base == "" {
		out.WriteString("No release exists yet, so everything here is new. The first release must be `v1.0.0`.\n\n")
	} else {
		fmt.Fprintf(&out, "Compared with `%s`, releasing this needs at least a **%s** version bump.\n\n", base, minimum)
	}

	if incompatible {
		out.WriteString("The dispelnet checker pinned at the base release refuses this tree, so this is a major release.\n\n")
	}

	shown := 0

	for _, change := range changes {
		if base == "" {
			break
		}

		if shown == 0 {
			out.WriteString("| Bump | Path | Why |\n|---|---|---|\n")
		}

		fmt.Fprintf(&out, "| %s | `%s` | %s |\n", change.Level, change.Path, change.Reason)
		shown++
	}

	if base != "" && shown == 0 {
		out.WriteString("Nothing a release ships has changed.\n")
	}

	out.WriteString("\nSee [docs/VERSIONING.md](../blob/main/docs/VERSIONING.md). This is advisory; the release workflow enforces it.\n")

	return out.String()
}

func writeOutputs(o options, base, minimum string, published []string) error {
	if o.githubOutput == "" {
		return nil
	}

	lines := []string{"base=" + base}

	if minimum != "" {
		lines = append(lines, "minimum="+minimum)
	}

	if o.tag != "" {
		prerelease := release.IsPrerelease(o.tag)
		latest, _ := release.Latest(published)
		isLatest := !prerelease && (latest == "" || semver.Compare(o.tag, latest) > 0)

		lines = append(lines, fmt.Sprintf("prerelease=%t", prerelease), fmt.Sprintf("latest=%t", isLatest))
	}

	return appendFile(o.githubOutput, strings.Join(lines, "\n")+"\n")
}

func appendFile(name, content string) error {
	if name == "" {
		return nil
	}

	file, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //nolint:gosec // G304: the file is named by whoever runs the command, e.g. $GITHUB_STEP_SUMMARY
	if err != nil {
		return err
	}

	if _, err := file.WriteString(content); err != nil {
		_ = file.Close()

		return err
	}

	return file.Close()
}

func orNone(base string) string {
	if base == "" {
		return "(none: first release)"
	}

	return base
}

// git runs git in one repository.
type git struct{ dir string }

func (g git) raw(args ...string) ([]byte, error) {
	command := exec.Command("git", append([]string{"-C", g.dir}, args...)...) //nolint:gosec // G204: always git, with arguments built here rather than parsed from a shell

	var stderr bytes.Buffer
	command.Stderr = &stderr

	out, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return out, nil
}

func (g git) output(args ...string) (string, error) {
	out, err := g.raw(args...)

	return strings.TrimSpace(string(out)), err
}
