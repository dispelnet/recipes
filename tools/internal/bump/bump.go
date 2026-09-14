// Package bump works out the smallest version bump a set of changes requires.
//
// The rules are docs/VERSIONING.md's. The consumer is the dispelnet binary: a change
// is MAJOR when a client of the previous release could refuse the tree or
// silently lose something, MINOR when it adds coverage or changes what is sent
// to a device, and PATCH when the same commands produce the same shape of
// answer, read better.
package bump

import (
	"bytes"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/dispelnet/recipes/tools/internal/corpus"
	"github.com/dispelnet/recipes/tools/internal/release"
)

// Tree is a repository snapshot: slash-separated path to content.
type Tree map[string][]byte

// Status is what happened to a path between two trees.
type Status string

// The statuses a path can have between two trees.
const (
	Added    Status = "added"
	Removed  Status = "removed"
	Modified Status = "modified"
)

// Change is one path that moved, and the bump it requires.
type Change struct {
	Path   string
	Kind   corpus.Kind
	Status Status
	Level  release.Level
	Reason string
}

// Compare classifies every shipped path that differs between base and head.
// withdrawals maps a removed recipe or probe to the reason it was withdrawn,
// from the tag message; a withdrawal is a patch rather than a major.
func Compare(base, head Tree, withdrawals map[string]string) ([]Change, error) {
	paths := slices.Sorted(maps.Keys(base))
	for path := range head {
		if _, ok := base[path]; !ok {
			paths = append(paths, path)
		}
	}

	slices.Sort(paths)

	var changes []Change

	for _, path := range paths {
		kind := corpus.Classify(path)
		if kind == corpus.Other {
			continue
		}

		old, inBase := base[path]
		updated, inHead := head[path]

		change := Change{Path: path, Kind: kind}

		switch {
		case !inBase:
			change.Status = Added
			change.Level, change.Reason = added(kind)
		case !inHead:
			change.Status = Removed
			change.Level, change.Reason = removed(kind, withdrawals[path])
		case bytes.Equal(old, updated):
			continue
		default:
			change.Status = Modified

			level, reason, err := modified(kind, old, updated)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}

			change.Level, change.Reason = level, reason
		}

		changes = append(changes, change)
	}

	for path := range withdrawals {
		if !slices.ContainsFunc(changes, func(c Change) bool {
			return c.Path == path && c.Status == Removed && (c.Kind == corpus.Recipe || c.Kind == corpus.Probe)
		}) {
			return nil, fmt.Errorf("the tag message withdraws %s, which this release does not remove as a recipe or probe", path)
		}
	}

	return changes, nil
}

func added(kind corpus.Kind) (release.Level, string) {
	switch kind {
	case corpus.Recipe, corpus.Probe:
		return release.Minor, "added"
	case corpus.Capture:
		return release.Patch, "capture added"
	default:
		return release.Patch, "added to the release tarball"
	}
}

func removed(kind corpus.Kind, withdrawal string) (release.Level, string) {
	switch kind {
	case corpus.Recipe, corpus.Probe:
		if withdrawal != "" {
			return release.Patch, "withdrawn: " + withdrawal
		}

		return release.Major, "removed; a client relying on it loses it (withdraw it in the tag message if it is unsafe)"
	case corpus.Capture:
		return release.Patch, "capture removed"
	default:
		return release.Patch, "removed from the release tarball"
	}
}

func modified(kind corpus.Kind, old, updated []byte) (release.Level, string, error) {
	switch kind {
	case corpus.Recipe, corpus.Probe:
		before, err := corpus.ParseDoc(kind, old)
		if err != nil {
			return release.None, "", fmt.Errorf("base does not parse: %w", err)
		}

		after, err := corpus.ParseDoc(kind, updated)
		if err != nil {
			return release.None, "", fmt.Errorf("does not parse: %w", err)
		}

		level, reason := classifyModified(kind, before, after)

		return level, reason, nil
	case corpus.Capture:
		return release.Patch, "capture changed", nil
	default:
		return release.Patch, "changed", nil
	}
}

// classifyModified decides the bump for a recipe or probe that exists in both
// trees with different content.
//
// docs/VERSIONING.md, for a modified file:
//
//	MAJOR  family or question renamed · an applies pattern removed ·
//	       a fields key removed
//	MINOR  anything in Sends changed (what reaches a device) ·
//	       an applies pattern added · a fields key added ·
//	       a probe's Tier or Is changed
//	PATCH  anything else: template, select, key, field sources, verified,
//	       comments
//
// The highest applicable level wins, and its reason is returned: the reason is
// printed on the pull request and in the release notes, so it should name
// what changed ("command changed: steps.0.run").
//
// Doc slices are already canonical: Applies, Fields and Is are sorted and
// deduplicated, so reordering them is not a change; Sends is in file order.
//
// Commands are compared exactly. dispelnet sends a command as written, so a
// change of whitespace is still a change to what reaches a device, and the
// cost of calling it minor is only a version number.
//
// Every reason at the winning level is kept, so the maintainer sees all of
// what forced the bump rather than whichever was found first.
func classifyModified(kind corpus.Kind, old, updated corpus.Doc) (release.Level, string) {
	level := release.Patch

	var reasons []string

	raise := func(to release.Level, format string, args ...any) {
		if to > level {
			level, reasons = to, nil
		}

		if to == level {
			reasons = append(reasons, fmt.Sprintf(format, args...))
		}
	}

	if old.Family != updated.Family {
		raise(release.Major, "family renamed from %s to %s", old.Family, updated.Family)
	}

	if old.Question != updated.Question {
		raise(release.Major, "question renamed from %s to %s", old.Question, updated.Question)
	}

	if added, removed := setDiff(old.Applies, updated.Applies); len(removed) > 0 || len(added) > 0 {
		if len(removed) > 0 {
			raise(release.Major, "no longer applies to %s", strings.Join(removed, ", "))
		}

		if len(added) > 0 {
			raise(release.Minor, "now also applies to %s", strings.Join(added, ", "))
		}
	}

	if added, removed := setDiff(old.Fields, updated.Fields); len(removed) > 0 || len(added) > 0 {
		if len(removed) > 0 {
			raise(release.Major, "no longer produces %s", strings.Join(removed, ", "))
		}

		if len(added) > 0 {
			raise(release.Minor, "now also produces %s", strings.Join(added, ", "))
		}
	}

	if !slices.Equal(old.Sends, updated.Sends) {
		raise(release.Minor, "changes what is sent to devices: %s", sendsChanged(old.Sends, updated.Sends))
	}

	if kind == corpus.Probe {
		if old.Tier != updated.Tier {
			raise(release.Minor, "identification tier changed from %s to %s", old.Tier, updated.Tier)
		}

		if !slices.Equal(old.Is, updated.Is) {
			raise(release.Minor, "identification assertions changed")
		}
	}

	if len(reasons) == 0 {
		return release.Patch, "changed without altering commands, coverage or output fields"
	}

	return level, strings.Join(reasons, "; ")
}

// sendsChanged names the settings that differ, by key ("steps.0.run"), so a
// reason says where to look without repeating whole commands.
func sendsChanged(old, updated []string) string {
	key := func(entry string) string {
		name, _, _ := strings.Cut(entry, ": ")

		return name
	}

	sortedOld, sortedUpdated := slices.Clone(old), slices.Clone(updated)
	slices.Sort(sortedOld)
	slices.Sort(sortedUpdated)

	added, removed := setDiff(sortedOld, sortedUpdated)

	var names []string

	for _, entry := range slices.Concat(removed, added) {
		if name := key(entry); !slices.Contains(names, name) {
			names = append(names, name)
		}
	}

	if len(names) == 0 {
		return "steps reordered"
	}

	slices.Sort(names)

	return strings.Join(names, ", ")
}

// setDiff returns what is in updated but not old, and what is in old but not
// updated. Both inputs must be sorted.
func setDiff(old, updated []string) (added, removed []string) {
	for _, value := range updated {
		if _, found := slices.BinarySearch(old, value); !found {
			added = append(added, value)
		}
	}

	for _, value := range old {
		if _, found := slices.BinarySearch(updated, value); !found {
			removed = append(removed, value)
		}
	}

	return added, removed
}

// Minimum is the largest level among the changes, and the changes that set it.
// incompatible means the checker pinned at the base refused the new tree, which
// makes the release MAJOR whatever the paths say.
func Minimum(changes []Change, incompatible bool) (release.Level, []string) {
	level := release.None

	for _, change := range changes {
		level = max(level, change.Level)
	}

	var reasons []string

	if incompatible {
		level = release.Major
		reasons = append(reasons, "the dispelnet checker pinned at the base release refuses this tree")
	}

	for _, change := range changes {
		if change.Level == level {
			reasons = append(reasons, change.Path+": "+change.Reason)
		}
	}

	return level, reasons
}

var withdrawalLine = regexp.MustCompile(`^Withdraws:\s*(\S+)\s+(?:—|--|-)\s+(.+?)\s*$`)

// ParseWithdrawals reads `Withdraws: <path> — <reason>` trailers from a tag
// message. A Withdraws line that does not parse is an error rather than
// ignored: a withdrawal silently missed would turn a safety patch into a
// refused release, or worse, go unannounced.
func ParseWithdrawals(message string) (map[string]string, error) {
	withdrawals := map[string]string{}

	for n, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Withdraws:") {
			continue
		}

		found := withdrawalLine.FindStringSubmatch(line)
		if found == nil {
			return nil, fmt.Errorf("tag message line %d: want `Withdraws: <path> — <reason>`, got %q", n+1, line)
		}

		if _, repeated := withdrawals[found[1]]; repeated {
			return nil, fmt.Errorf("tag message line %d: %s is withdrawn twice", n+1, found[1])
		}

		withdrawals[found[1]] = found[2]
	}

	return withdrawals, nil
}

// Notes renders release notes from the changes, most important first.
func Notes(changes []Change) string {
	sections := []struct {
		title string
		match func(Change) bool
	}{
		{"⚠ Withdrawn", func(c Change) bool { return strings.HasPrefix(c.Reason, "withdrawn: ") }},
		{"Breaking", func(c Change) bool { return c.Level == release.Major }},
		{"Added", func(c Change) bool { return c.Status == Added && c.Level == release.Minor }},
		{"Changed what is sent to devices", func(c Change) bool { return c.Status == Modified && c.Level == release.Minor }},
		{"Fixes and evidence", func(c Change) bool {
			return c.Level == release.Patch && !strings.HasPrefix(c.Reason, "withdrawn: ")
		}},
	}

	var out strings.Builder

	for _, section := range sections {
		var lines []string

		for _, change := range changes {
			if section.match(change) {
				lines = append(lines, fmt.Sprintf("- `%s`: %s", change.Path, change.Reason))
			}
		}

		if len(lines) == 0 {
			continue
		}

		fmt.Fprintf(&out, "## %s\n\n%s\n\n", section.title, strings.Join(lines, "\n"))
	}

	if out.Len() == 0 {
		return "No changes to recipes, probes or captures.\n"
	}

	return out.String()
}
