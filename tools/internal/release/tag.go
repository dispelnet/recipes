// Package release holds the tag contract: which tags are releases, how they are
// ordered, and which earlier release a new one is compared against.
//
// The rules live in docs/VERSIONING.md. They are code here because a release that
// cannot be taken back must not depend on someone remembering them.
package release

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

// Level is how far a version moved, or how far a change requires it to move.
// The order matters: a larger Level is a larger bump.
type Level int

// The levels, smallest first.
const (
	None Level = iota
	Patch
	Minor
	Major
)

func (l Level) String() string {
	switch l {
	case Patch:
		return "patch"
	case Minor:
		return "minor"
	case Major:
		return "major"
	default:
		return "none"
	}
}

// First is the only version a repository with no releases may start at.
// Clients pin a major, and v0 would give them nothing to pin.
const First = "v1.0.0"

// CheckName reports whether a tag name is a release version: strict SemVer with
// a v prefix, in canonical form, without build metadata.
//
// semver.IsValid alone accepts the v1 and v1.2 shorthands, which is why the
// canonical form is compared too. Build metadata is refused because SemVer
// ignores it for precedence, so two such tags could not be ordered.
func CheckName(tag string) error {
	if !semver.IsValid(tag) {
		return fmt.Errorf("tag %q is not a SemVer version with a v prefix (e.g. v1.2.3)", tag)
	}

	if semver.Build(tag) != "" {
		return fmt.Errorf("tag %q carries build metadata, which SemVer cannot order", tag)
	}

	if semver.Canonical(tag) != tag {
		return fmt.Errorf("tag %q is shorthand; write it in full as %q", tag, semver.Canonical(tag))
	}

	return nil
}

// IsPrerelease reports whether a valid tag is a pre-release.
func IsPrerelease(tag string) bool {
	return semver.Prerelease(tag) != ""
}

// Base picks the release a new tag is compared against: the highest stable
// release lower than the tag. Pre-releases are never a base, so a final
// release is held to everything since the last stable one rather than to its
// last release candidate.
//
// published must already be filtered to tags that meet the contract and have a
// published release; a tag whose release failed is not a base.
func Base(tag string, published []string) (string, bool) {
	var base string

	for _, candidate := range published {
		if CheckName(candidate) != nil || IsPrerelease(candidate) {
			continue
		}

		if semver.Compare(candidate, tag) >= 0 {
			continue
		}

		if base == "" || semver.Compare(candidate, base) > 0 {
			base = candidate
		}
	}

	return base, base != ""
}

// Latest is the highest stable release, which is what a pull request is
// compared against before anyone has chosen a tag.
func Latest(published []string) (string, bool) {
	var latest string

	for _, candidate := range published {
		if CheckName(candidate) != nil || IsPrerelease(candidate) {
			continue
		}

		if latest == "" || semver.Compare(candidate, latest) > 0 {
			latest = candidate
		}
	}

	return latest, latest != ""
}

// Distance is the Level a tag moves from its base: the most significant of
// major, minor and patch that differs. Without a base, the tag must be First
// or one of its pre-releases.
func Distance(base, tag string) (Level, error) {
	if base == "" {
		if core(tag) != First {
			return None, fmt.Errorf("the first release must be %s or a %s pre-release, not %s", First, First, tag)
		}

		return Major, nil
	}

	if semver.Compare(tag, base) <= 0 {
		return None, fmt.Errorf("tag %s is not higher than its base %s", tag, base)
	}

	b, t := numbers(base), numbers(tag)

	switch {
	case t[0] != b[0]:
		return Major, nil
	case t[1] != b[1]:
		return Minor, nil
	default:
		return Patch, nil
	}
}

// core is a version without its pre-release suffix.
func core(tag string) string {
	return strings.TrimSuffix(tag, semver.Prerelease(tag))
}

// numbers is major, minor and patch of a valid, canonical tag.
func numbers(tag string) [3]int {
	var out [3]int

	for i, part := range strings.SplitN(strings.TrimPrefix(core(tag), "v"), ".", 3) {
		out[i], _ = strconv.Atoi(part)
	}

	return out
}
