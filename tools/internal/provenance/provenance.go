// Package provenance checks that every recipe and probe says where its captured
// output came from, and proves it where it can.
//
// Two things are proven: that an upstream capture is byte for byte what the
// upstream project published at a full commit, and that the upstream licence
// applied at that commit. Two things are only attested, and reported as such:
// a lab capture's `verified:` (nothing can tell a device's output from a typed
// one) and a `modified` capture (the upstream file exists; that the capture
// came from it is a reviewer's judgement).
package provenance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/dispelnet/recipes/tools/internal/corpus"
	"github.com/dispelnet/recipes/tools/internal/upstream"
)

// UpstreamsPath is the reviewed allowlist of sources, relative to the root.
const UpstreamsPath = "tools/upstreams.yml"

// Source is one allowed upstream project.
type Source struct {
	License       string `yaml:"license"`
	LicenseFile   string `yaml:"license_file"`
	LicenseMarker string `yaml:"license_marker"`
}

// Problem is one refusal, attached to the path that caused it.
type Problem struct {
	Path    string
	Message string
}

func (p Problem) String() string { return p.Path + ": " + p.Message }

// Report is what a check found.
type Report struct {
	Problems []Problem

	// Files is how many recipes and probes were checked.
	Files int
	// Proven are upstream captures matched against upstream.
	Proven int
	// Unchecked are upstream captures not fetched, in offline mode.
	Unchecked int
	// Attested are `modified` captures, which a reviewer vouches for.
	Attested []string
	// Lab are captures declared `verified:` by a contributor.
	Lab int
}

// Check checks the repository at root. A nil fetcher checks offline: every
// rule that needs no network still applies.
func Check(ctx context.Context, root fs.FS, fetch upstream.Fetcher) (Report, error) {
	var report Report

	refuse := func(path, format string, args ...any) {
		report.Problems = append(report.Problems, Problem{Path: path, Message: fmt.Sprintf(format, args...)})
	}

	sources, err := loadSources(root)
	if err != nil {
		return report, err
	}

	notice, err := fs.ReadFile(root, "NOTICE")
	if err != nil {
		refuse("NOTICE", "cannot be read: %v", err)
	}

	docs, err := layout(root, refuse)
	if err != nil {
		return report, err
	}

	licences := map[string]error{}

	for _, name := range docs {
		report.Files++

		body, err := fs.ReadFile(root, name)
		if err != nil {
			return report, err
		}

		kind := corpus.Classify(name)

		doc, err := corpus.ParseDoc(kind, body)
		if err != nil {
			refuse(name, "does not parse: %v", err)

			continue
		}

		block, err := corpus.ParseCapture(body)
		if err != nil {
			refuse(name, "%v", err)

			continue
		}

		lab := strings.TrimSpace(doc.Verified) != ""

		switch {
		case lab && block != nil:
			refuse(name, "declares both `verified:` and a capture block; a lab capture has no upstream, so keep exactly one")

			continue
		case lab:
			report.Lab++

			continue
		case block == nil:
			refuse(name, "names no provenance: declare `verified:` for output from your own device, or add a `# Capture:` block naming the upstream file")

			continue
		}

		source, ok := validate(name, block, sources, notice, refuse)
		if !ok {
			continue
		}

		if fetch == nil {
			if block.Match == corpus.Modified {
				report.Attested = append(report.Attested, name)
			} else {
				report.Unchecked++
			}

			continue
		}

		key := block.Source + "@" + block.Commit
		if _, done := licences[key]; !done {
			licences[key] = checkLicence(ctx, fetch, block, source)
		}

		if err := licences[key]; err != nil {
			refuse(name, "%v", err)

			continue
		}

		captured, err := fs.ReadFile(root, corpus.CapturedPath(name))
		if err != nil {
			// A missing capture is already refused by layout.
			continue
		}

		if err := resolve(ctx, fetch, block, captured); err != nil {
			refuse(name, "%v", err)

			continue
		}

		if block.Match == corpus.Modified {
			report.Attested = append(report.Attested, name)
		} else {
			report.Proven++
		}
	}

	return report, nil
}

func loadSources(root fs.FS) (map[string]Source, error) {
	body, err := fs.ReadFile(root, UpstreamsPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", UpstreamsPath, err)
	}

	var sources map[string]Source
	if err := yaml.UnmarshalWithOptions(body, &sources, yaml.DisallowUnknownField()); err != nil {
		return nil, fmt.Errorf("reading %s: %w", UpstreamsPath, err)
	}

	for name, source := range sources {
		if source.License == "" || source.LicenseFile == "" || source.LicenseMarker == "" {
			return nil, fmt.Errorf("%s: %s needs license, license_file and license_marker", UpstreamsPath, name)
		}

		if strings.Contains(source.LicenseMarker, "\n") {
			return nil, fmt.Errorf("%s: %s license_marker must be one line", UpstreamsPath, name)
		}
	}

	return sources, nil
}

// layout walks the repository, refuses paths that would break dispelnet or
// strand evidence, and returns the recipes and probes in order.
func layout(root fs.FS, refuse func(string, string, ...any)) ([]string, error) {
	var docs []string

	err := fs.WalkDir(root, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			if name == ".git" {
				return fs.SkipDir
			}

			return nil
		}

		kind := corpus.Classify(name)

		switch {
		case kind == corpus.Recipe || kind == corpus.Probe:
			docs = append(docs, name)

			if _, err := fs.Stat(root, corpus.CapturedPath(name)); err != nil {
				refuse(name, "has no %s beside it", corpus.CapturedPath(name))
			}
		case strings.HasSuffix(name, ".yaml"):
			refuse(name, "is a .yaml outside recipes/<family>/ or recipes/_probes/; name it .yml")
		case kind == corpus.Capture:
			if _, err := fs.Stat(root, strings.TrimSuffix(name, ".captured")+".yaml"); err != nil {
				refuse(name, "is evidence for nothing: no .yaml beside it")
			}
		case strings.HasSuffix(name, ".captured"):
			refuse(name, "is a .captured outside recipes/<family>/ or recipes/_probes/")
		}

		return nil
	})

	slices.Sort(docs)

	return docs, err
}

func validate(
	name string,
	block *corpus.CaptureBlock,
	sources map[string]Source,
	notice []byte,
	refuse func(string, string, ...any),
) (Source, bool) {
	ok := true
	fail := func(format string, args ...any) {
		refuse(name, format, args...)
		ok = false
	}

	for field, value := range map[string]string{"source": block.Source, "commit": block.Commit, "file": block.File, "match": block.Match} {
		if value == "" {
			fail("capture block is missing %s", field)
		}
	}

	if !ok {
		return Source{}, false
	}

	source, known := sources[block.Source]
	if !known {
		fail("capture source %s is not in %s; add it there after review", block.Source, UpstreamsPath)
	}

	if notice != nil && !strings.Contains(string(notice), block.Source) {
		fail("capture source %s is not named in NOTICE", block.Source)
	}

	if !upstream.FullCommit.MatchString(block.Commit) {
		fail("capture commit %q must be a full 40-character SHA; a short SHA or tag can come to name something else", block.Commit)
	}

	switch block.Match {
	case corpus.Exact:
		if block.Select != "" {
			fail("select is only for match: extract")
		}
	case corpus.Extract:
		if block.Select == "" {
			fail("match: extract needs a select path, e.g. 1.1.data")
		}
	case corpus.Modified:
		if block.Note == "" {
			fail("match: modified needs a note saying what was changed and why")
		}

		if block.Select != "" {
			fail("select is only for match: extract")
		}
	default:
		fail("capture match %q must be exact, extract or modified", block.Match)
	}

	return source, ok
}

func checkLicence(ctx context.Context, fetch upstream.Fetcher, block *corpus.CaptureBlock, source Source) error {
	body, err := fetch.Fetch(ctx, block.Source, block.Commit, source.LicenseFile)
	if err != nil {
		return fmt.Errorf("cannot fetch %s's licence at %s: %w", block.Source, short(block.Commit), err)
	}

	if !strings.Contains(string(body), source.LicenseMarker) {
		return fmt.Errorf("%s's %s at %s does not say %q, so %s is not established for that commit",
			block.Source, source.LicenseFile, short(block.Commit), source.LicenseMarker, source.License)
	}

	return nil
}

func resolve(ctx context.Context, fetch upstream.Fetcher, block *corpus.CaptureBlock, captured []byte) error {
	where := fmt.Sprintf("%s@%s:%s", block.Source, short(block.Commit), block.File)

	body, err := fetch.Fetch(ctx, block.Source, block.Commit, block.File)
	if errors.Is(err, upstream.ErrNotFound) {
		return fmt.Errorf("%s does not exist upstream", where)
	}

	if err != nil {
		return fmt.Errorf("cannot fetch %s: %w", where, err)
	}

	switch block.Match {
	case corpus.Exact:
		if got, want := digest(captured), digest(body); got != want {
			return fmt.Errorf("the capture differs from %s (sha256 %s here, %s upstream)", where, got[:12], want[:12])
		}
	case corpus.Extract:
		text, err := upstream.Select(body, block.Select)
		if err != nil {
			return fmt.Errorf("%s: %w", where, err)
		}

		if text != string(captured) {
			return fmt.Errorf("the capture differs from %s at %s", where, block.Select)
		}
	}

	return nil
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)

	return hex.EncodeToString(sum[:])
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}

	return commit
}
