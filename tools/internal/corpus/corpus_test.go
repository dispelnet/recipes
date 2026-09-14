package corpus

import (
	"slices"
	"testing"
)

func TestClassify(t *testing.T) {
	for name, want := range map[string]Kind{
		"recipes/cisco_ios/version.yaml":     Recipe,
		"recipes/cisco_ios/version.captured": Capture,
		"recipes/_probes/frr.yaml":           Probe,
		"recipes/_probes/frr.captured":       Capture,
		"LICENSE":                       Shipped,
		"NOTICE":                        Shipped,
		"README.md":                     Other,
		"recipes/cisco_ios/README.md":   Other,
		"docs/example.yaml":             Other,
		"tools/upstreams.yml":           Other,
		"tools/testdata/x.yaml":         Other,
		".github/workflows/ci.yml":      Other,
		"top.yaml":                      Other,
		"Cisco/version.yaml":            Other,
		"recipes/cisco_ios/nested/version.yaml": Other,
		"cisco_ios/version.yaml":               Other,
		"probes/frr.yaml":                      Other,
	} {
		if got := Classify(name); got != want {
			t.Errorf("Classify(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestParseCapture(t *testing.T) {
	body := []byte(`# What a device calls itself.
#
# Capture:
#   source: networktocode/ntc-templates
#   commit: c6dca50ea10fe5a23e750748582fddda263fb053
#   file:   tests/cisco_ios/show_version/cisco_ios_show_version.raw
#   match:  exact
#
# note: this line is prose after the block, not a field

family: cisco_ios
# Capture:
`)

	block, err := ParseCapture(body)
	if err != nil {
		t.Fatal(err)
	}

	want := CaptureBlock{
		Source: "networktocode/ntc-templates",
		Commit: "c6dca50ea10fe5a23e750748582fddda263fb053",
		File:   "tests/cisco_ios/show_version/cisco_ios_show_version.raw",
		Match:  Exact,
	}
	if block == nil || *block != want {
		t.Errorf("ParseCapture = %+v, want %+v", block, want)
	}

	if block, err := ParseCapture([]byte("# no block\nfamily: x\n")); block != nil || err != nil {
		t.Errorf("no block: %+v, %v", block, err)
	}

	for _, bad := range []string{
		"# Capture:\n#   source: a\n#   source: b\n",
		"# Capture:\n#   licence: MIT\n",
		"# Capture:\n#   source: a\n#\n# Capture:\n",
	} {
		if _, err := ParseCapture([]byte(bad)); err == nil {
			t.Errorf("ParseCapture(%q) succeeded", bad)
		}
	}
}

func TestParseDocRecipe(t *testing.T) {
	doc, err := ParseDoc(Recipe, []byte(`family: frr
question: bgp_neighbors
applies: ["10.*", "9.*", "10.*"]
steps:
  - run: vtysh -c "show bgp neighbors json"
    select: "{*}"
    fields:
      identity: remoteRouterId
      as: remoteAs
  - uses: expand
    with: {b: 1, a: 2}
    for-each: peers
    limit: 20
    fields:
      state: {from: bgpState, lower: true}
`))
	if err != nil {
		t.Fatal(err)
	}

	checks := map[string][2][]string{
		"applies": {doc.Applies, {"10.*", "9.*"}},
		"fields":  {doc.Fields, {"as", "identity", "state"}},
		"sends": {doc.Sends, {
			`steps.0.run: vtysh -c "show bgp neighbors json"`,
			"steps.1.uses: expand",
			`steps.1.with: {"a":2,"b":1}`,
			"steps.1.for-each: peers",
			"steps.1.limit: 20",
		}},
	}

	for name, c := range checks {
		if !slices.Equal(c[0], c[1]) {
			t.Errorf("%s = %q, want %q", name, c[0], c[1])
		}
	}
}

func TestParseDocProbe(t *testing.T) {
	doc, err := ParseDoc(Probe, []byte(`id: cisco_ios
tier: vendor
transport: ssh
command: show version
is:
  - {contains: "Cisco IOS Software"}
  - {field: 1, equals: "IOS"}
family: cisco_ios
`))
	if err != nil {
		t.Fatal(err)
	}

	if doc.Tier != "vendor" || !slices.Equal(doc.Sends, []string{"transport: ssh", "command: show version"}) {
		t.Errorf("probe doc = %+v", doc)
	}

	if !slices.Equal(doc.Is, []string{`{"contains":"Cisco IOS Software"}`, `{"equals":"IOS","field":1}`}) {
		t.Errorf("is = %q", doc.Is)
	}
}
