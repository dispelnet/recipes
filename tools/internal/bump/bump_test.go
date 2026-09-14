package bump

import (
	"slices"
	"strings"
	"testing"

	"github.com/dispelnet/recipes/tools/internal/corpus"
	"github.com/dispelnet/recipes/tools/internal/release"
)

func TestCompare(t *testing.T) {
	base := Tree{
		"README.md":                          []byte("docs"),
		"LICENSE":                            []byte("apache"),
		"recipes/cisco_ios/version.yaml":     []byte("family: cisco_ios\nquestion: version\n"),
		"recipes/cisco_ios/version.captured": []byte("old output"),
		"recipes/cisco_ios/hostname.yaml":    []byte("family: cisco_ios\nquestion: hostname\n"),
		"recipes/frr/version.yaml":           []byte("family: frr\n"),
	}

	head := Tree{
		"README.md":                          []byte("docs, reworded"),
		"LICENSE":                            []byte("apache, updated year"),
		"recipes/cisco_ios/version.yaml":     []byte("family: cisco_ios\nquestion: version\n"),
		"recipes/cisco_ios/version.captured": []byte("new output"),
		"recipes/frr/lldp_neighbors.yaml":    []byte("family: frr\n"),
		"tools/upstreams.yml":                []byte("x"),
	}

	changes, err := Compare(base, head, map[string]string{"recipes/frr/version.yaml": "runs show tech-support"})
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}
	for _, c := range changes {
		got[c.Path] = c.Level.String() + " " + string(c.Status)
	}

	want := map[string]string{
		"LICENSE":                            "patch modified",
		"recipes/cisco_ios/version.captured": "patch modified",
		"recipes/cisco_ios/hostname.yaml":    "major removed",
		"recipes/frr/lldp_neighbors.yaml":    "minor added",
		"recipes/frr/version.yaml":           "patch removed",
	}

	if len(got) != len(want) {
		t.Errorf("changes = %v, want %v", got, want)
	}

	for path, level := range want {
		if got[path] != level {
			t.Errorf("%s = %q, want %q", path, got[path], level)
		}
	}

	level, reasons := Minimum(changes, false)
	if level != release.Major || !slices.Equal(reasons, []string{"recipes/cisco_ios/hostname.yaml: removed; a client relying on it loses it (withdraw it in the tag message if it is unsafe)"}) {
		t.Errorf("Minimum = %v, %q", level, reasons)
	}
}

func TestAWithdrawalMustRemoveSomething(t *testing.T) {
	tree := Tree{"recipes/frr/version.yaml": []byte("family: frr\n")}

	if _, err := Compare(tree, tree, map[string]string{"recipes/frr/version.yaml": "reason"}); err == nil {
		t.Error("withdrawing a file that is still there succeeded")
	}
}

func TestIncompatibleIsMajor(t *testing.T) {
	level, reasons := Minimum([]Change{{Path: "a/b.captured", Level: release.Patch, Reason: "capture changed"}}, true)
	if level != release.Major || len(reasons) != 1 || !strings.Contains(reasons[0], "refuses this tree") {
		t.Errorf("Minimum = %v, %q", level, reasons)
	}

	if level, _ := Minimum(nil, false); level != release.None {
		t.Errorf("no changes = %v, want none", level)
	}
}

func TestParseWithdrawals(t *testing.T) {
	got, err := ParseWithdrawals("Release v1.0.1\n\nWithdraws: recipes/cisco_ios/bgp_neighbors.yaml — pegs the CPU on 15.2\nWithdraws: recipes/_probes/linux.yaml -- misidentifies SONiC\n")
	if err != nil {
		t.Fatal(err)
	}

	if got["recipes/cisco_ios/bgp_neighbors.yaml"] != "pegs the CPU on 15.2" || got["recipes/_probes/linux.yaml"] != "misidentifies SONiC" {
		t.Errorf("withdrawals = %v", got)
	}

	for _, bad := range []string{
		"Withdraws: recipes/cisco_ios/bgp_neighbors.yaml",
		"Withdraws: a.yaml — x\nWithdraws: a.yaml — y",
	} {
		if _, err := ParseWithdrawals(bad); err == nil {
			t.Errorf("ParseWithdrawals(%q) succeeded", bad)
		}
	}
}

func TestNotes(t *testing.T) {
	notes := Notes([]Change{
		{Path: "recipes/frr/version.yaml", Status: Removed, Level: release.Patch, Reason: "withdrawn: unsafe"},
		{Path: "recipes/frr/lldp_neighbors.yaml", Status: Added, Level: release.Minor, Reason: "added"},
		{Path: "recipes/frr/bgp_neighbors.yaml", Status: Modified, Level: release.Minor, Reason: "command changed: steps.0.run"},
		{Path: "recipes/frr/hostname.captured", Status: Modified, Level: release.Patch, Reason: "capture changed"},
	})

	order := []string{"## ⚠ Withdrawn", "## Added", "## Changed what is sent to devices", "## Fixes and evidence"}
	last := -1

	for _, heading := range order {
		at := strings.Index(notes, heading)
		if at <= last {
			t.Fatalf("%q missing or out of order in:\n%s", heading, notes)
		}

		last = at
	}

	if strings.Count(notes, "recipes/frr/version.yaml") != 1 {
		t.Errorf("a withdrawal is listed once, under Withdrawn:\n%s", notes)
	}
}

func TestSetDiff(t *testing.T) {
	added, removed := setDiff([]string{"a", "b", "c"}, []string{"b", "c", "d"})
	if !slices.Equal(added, []string{"d"}) || !slices.Equal(removed, []string{"a"}) {
		t.Errorf("setDiff = %q, %q", added, removed)
	}
}

// TestClassifyModified holds classifyModified to docs/VERSIONING.md's table.
func TestClassifyModified(t *testing.T) {
	recipe := corpus.Doc{
		Family:   "frr",
		Question: "bgp_neighbors",
		Applies:  []string{"10.*", "9.*"},
		Sends:    []string{`steps.0.run: vtysh -c "show bgp neighbors json"`},
		Fields:   []string{"as", "identity"},
	}

	probe := corpus.Doc{
		ID:    "cisco_ios",
		Tier:  "vendor",
		Sends: []string{"transport: ssh", "command: show version"},
		Is:    []string{`{"contains":"Cisco IOS Software"}`},
	}

	edit := func(doc corpus.Doc, change func(*corpus.Doc)) corpus.Doc {
		doc.Applies = slices.Clone(doc.Applies)
		doc.Sends = slices.Clone(doc.Sends)
		doc.Fields = slices.Clone(doc.Fields)
		doc.Is = slices.Clone(doc.Is)
		change(&doc)

		return doc
	}

	cases := []struct {
		name    string
		kind    corpus.Kind
		old     corpus.Doc
		updated corpus.Doc
		want    release.Level
	}{
		{"nothing a client sees (template, comments)", corpus.Recipe, recipe, recipe, release.Patch},
		{"verified added", corpus.Recipe, recipe, edit(recipe, func(d *corpus.Doc) { d.Verified = "frr 10.3.4" }), release.Patch},
		{"family renamed", corpus.Recipe, recipe, edit(recipe, func(d *corpus.Doc) { d.Family = "sonic" }), release.Major},
		{"question renamed", corpus.Recipe, recipe, edit(recipe, func(d *corpus.Doc) { d.Question = "peers" }), release.Major},
		{"applies pattern removed", corpus.Recipe, recipe, edit(recipe, func(d *corpus.Doc) { d.Applies = []string{"10.*"} }), release.Major},
		{"applies pattern added", corpus.Recipe, recipe, edit(recipe, func(d *corpus.Doc) { d.Applies = []string{"10.*", "11.*", "9.*"} }), release.Minor},
		{"field removed", corpus.Recipe, recipe, edit(recipe, func(d *corpus.Doc) { d.Fields = []string{"identity"} }), release.Major},
		{"field added", corpus.Recipe, recipe, edit(recipe, func(d *corpus.Doc) { d.Fields = []string{"as", "identity", "state"} }), release.Minor},
		{"command changed", corpus.Recipe, recipe, edit(recipe, func(d *corpus.Doc) { d.Sends = []string{`steps.0.run: vtysh -c "show bgp summary json"`} }), release.Minor},
		{"command whitespace changed: still sent differently", corpus.Recipe, recipe, edit(recipe, func(d *corpus.Doc) { d.Sends = []string{`steps.0.run: vtysh -c "show bgp neighbors json" `} }), release.Minor},
		{"steps reordered", corpus.Recipe, edit(recipe, func(d *corpus.Doc) { d.Sends = []string{"steps.0.run: a", "steps.1.run: b"} }), edit(recipe, func(d *corpus.Doc) { d.Sends = []string{"steps.1.run: b", "steps.0.run: a"} }), release.Minor},
		{"step added", corpus.Recipe, recipe, edit(recipe, func(d *corpus.Doc) { d.Sends = append(d.Sends, "steps.1.for-each: peers") }), release.Minor},
		{"command changed and field removed: major wins", corpus.Recipe, recipe, edit(recipe, func(d *corpus.Doc) {
			d.Sends = []string{"steps.0.run: other"}
			d.Fields = []string{"as"}
		}), release.Major},
		{"probe command changed", corpus.Probe, probe, edit(probe, func(d *corpus.Doc) { d.Sends = []string{"transport: ssh", "command: show ver"} }), release.Minor},
		{"probe tier changed", corpus.Probe, probe, edit(probe, func(d *corpus.Doc) { d.Tier = "os" }), release.Minor},
		{"probe assertion changed", corpus.Probe, probe, edit(probe, func(d *corpus.Doc) { d.Is = []string{`{"contains":"IOS"}`} }), release.Minor},
		{"probe verified added", corpus.Probe, probe, edit(probe, func(d *corpus.Doc) { d.Verified = "c3750 15.2" }), release.Patch},
	}

	// Every reason at the winning level is reported, and none below it.
	_, reason := classifyModified(corpus.Recipe, recipe, edit(recipe, func(d *corpus.Doc) {
		d.Question = "peers"
		d.Fields = []string{"identity"}
		d.Sends = []string{"steps.0.run: other"}
	}))
	if !strings.Contains(reason, "question renamed") || !strings.Contains(reason, "no longer produces as") || strings.Contains(reason, "sent to devices") {
		t.Errorf("combined reason = %q", reason)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			level, reason := classifyModified(c.kind, c.old, c.updated)
			if level != c.want {
				t.Errorf("level = %v (%q), want %v", level, reason, c.want)
			}

			if reason == "" {
				t.Error("every classification needs a reason: it is printed on the PR and in the release notes")
			}
		})
	}
}
