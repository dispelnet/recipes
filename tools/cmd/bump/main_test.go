package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheTagContractAgainstARealRepository walks a release history through git
// itself, because the contract is mostly about git objects: annotated tags,
// ancestry and trees at a tag.
func TestTheTagContractAgainstARealRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	dir := t.TempDir()

	// Isolate from the developer's git configuration: a global tag.gpgSign or
	// a default branch name would change what this test is testing.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	git := func(args ...string) {
		t.Helper()

		command := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	write := func(name, body string) {
		t.Helper()

		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	releases := func(tags ...string) string {
		t.Helper()

		path := filepath.Join(t.TempDir(), "releases")
		if err := os.WriteFile(path, []byte(strings.Join(tags, "\n")+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		return path
	}

	bump := func(want int, contains string, args ...string) string {
		t.Helper()

		var stdout, stderr bytes.Buffer

		code := run(append([]string{"--repo", dir}, args...), &stdout, &stderr)
		output := stdout.String() + stderr.String()

		if code != want || !strings.Contains(output, contains) {
			t.Fatalf("bump %s: exit %d, want %d containing %q\n%s", strings.Join(args, " "), code, want, contains, output)
		}

		return output
	}

	git("init", "-q", "-b", "main")
	write("LICENSE", "apache\n")
	write("README.md", "docs\n")
	write("recipes/cisco_ios/version.yaml", "family: cisco_ios\nquestion: version\n")
	write("recipes/cisco_ios/version.captured", "Cisco IOS Software\n")
	git("add", "-A")
	git("commit", "-qm", "first")

	// The first release must be v1.0.0.
	git("tag", "-a", "v1.2.0", "-m", "too far")
	bump(1, "must be v1.0.0", "--tag", "v1.2.0", "--main", "main", "--releases", releases())
	git("tag", "-d", "v1.2.0")

	git("tag", "-a", "v1.0.0", "-m", "first release")
	bump(0, "base:    (none: first release)", "--tag", "v1.0.0", "--main", "main", "--releases", releases())

	write("recipes/cisco_ios/hostname.yaml", "family: cisco_ios\nquestion: hostname\n")
	write("recipes/cisco_ios/hostname.captured", "router1 uptime is 1 week\n")
	git("add", "-A")
	git("commit", "-qm", "hostname")

	// Malformed and lightweight tags are refused before anything else.
	git("tag", "v1.1")
	bump(1, "shorthand", "--tag", "v1.1", "--releases", releases("v1.0.0"))
	git("tag", "v1.1.0")
	bump(1, "lightweight", "--tag", "v1.1.0", "--releases", releases("v1.0.0"))
	git("tag", "-d", "v1.1", "v1.1.0")

	// A new recipe needs a minor, so a patch tag is refused with the reason.
	git("tag", "-a", "v1.0.1", "-m", "too small")
	output := bump(1, "need at least a minor", "--tag", "v1.0.1", "--main", "main", "--releases", releases("v1.0.0"))

	if !strings.Contains(output, "recipes/cisco_ios/hostname.yaml: added") {
		t.Errorf("the refusal names the change behind it:\n%s", output)
	}

	// v1.0.1's release never published, so it is not v1.1.0's base.
	git("tag", "-a", "v1.1.0", "-m", "adds hostname")
	bump(0, "base:    v1.0.0", "--tag", "v1.1.0", "--main", "main", "--releases", releases("v1.0.0"))

	// A pull request removing a recipe is told it needs a major.
	git("rm", "-q", "recipes/cisco_ios/version.yaml")
	git("commit", "-qm", "remove version")
	bump(0, "minimum: major", "--releases", releases("v1.0.0", "v1.1.0"))

	// Withdrawn in the tag message, the same removal is a patch.
	notes := filepath.Join(t.TempDir(), "notes.md")
	git("tag", "-a", "v1.1.1", "-m", "safety release", "-m", "Withdraws: recipes/cisco_ios/version.yaml — pegs the CPU")
	bump(0, "minimum: patch", "--tag", "v1.1.1", "--main", "main", "--releases", releases("v1.0.0", "v1.1.0"), "--notes", notes)

	body, err := os.ReadFile(notes)
	if err != nil || !strings.Contains(string(body), "## ⚠ Withdrawn") || !strings.Contains(string(body), "pegs the CPU") {
		t.Errorf("release notes lead with the withdrawal:\n%s (%v)", body, err)
	}

	// A tag off main is refused.
	git("checkout", "-qb", "side")
	write("recipes/frr/version.yaml", "family: frr\n")
	git("add", "-A")
	git("commit", "-qm", "side")
	git("tag", "-a", "v1.2.0", "-m", "from a branch")
	bump(1, "is not on main", "--tag", "v1.2.0", "--main", "main", "--releases", releases("v1.0.0", "v1.1.0"))
}
