package provenance

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/dispelnet/recipes/tools/internal/upstream"
)

const (
	ntc    = "networktocode/ntc-templates"
	commit = "c6dca50ea10fe5a23e750748582fddda263fb053"
)

// fake serves upstream files from a map keyed "source|commit|file".
type fake map[string]string

func (f fake) Fetch(_ context.Context, source, commit, file string) ([]byte, error) {
	body, ok := f[source+"|"+commit+"|"+file]
	if !ok {
		return nil, upstream.ErrNotFound
	}

	return []byte(body), nil
}

func header(match, file string, extra ...string) string {
	lines := []string{
		"# Capture:",
		"#   source: " + ntc,
		"#   commit: " + commit,
		"#   file:   " + file,
		"#   match:  " + match,
	}

	for _, e := range extra {
		lines = append(lines, "#   "+e)
	}

	return strings.Join(lines, "\n") + "\n\n"
}

func base() fstest.MapFS {
	return fstest.MapFS{
		"tools/upstreams.yml": {Data: []byte(ntc + `:
  license: Apache-2.0
  license_file: LICENSE
  license_marker: "Licensed under the Apache License, Version 2.0"
`)},
		"NOTICE": {Data: []byte("includes https://github.com/" + ntc + "\n")},

		"recipes/cisco_ios/version.yaml":     {Data: []byte(header("exact", "tests/show_version.raw") + "family: cisco_ios\nquestion: version\n")},
		"recipes/cisco_ios/version.captured": {Data: []byte("Cisco IOS Software\n")},

		"recipes/frr/version.yaml":     {Data: []byte("family: frr\nquestion: version\nverified: frr 10.3.4\n")},
		"recipes/frr/version.captured": {Data: []byte("FRRouting 10.3.4\n")},

		"recipes/juniper_junos/bgp_neighbors.yaml":     {Data: []byte(header("extract", "bgp.output", "select: 1.1.data") + "family: juniper_junos\n")},
		"recipes/juniper_junos/bgp_neighbors.captured": {Data: []byte("{\"peer\": 1}\n")},

		"recipes/_probes/cisco_ios.yaml":     {Data: []byte(header("modified", "tests/show_version2.raw", "note: trimmed the banner") + "id: cisco_ios\n")},
		"recipes/_probes/cisco_ios.captured": {Data: []byte("Cisco IOS Software\n")},
	}
}

func upstreamFiles() fake {
	return fake{
		ntc + "|" + commit + "|LICENSE":                 "Licensed under the Apache License, Version 2.0 (the \"License\");",
		ntc + "|" + commit + "|tests/show_version.raw":  "Cisco IOS Software\n",
		ntc + "|" + commit + "|tests/show_version2.raw": "banner\nCisco IOS Software\n",
		ntc + "|" + commit + "|bgp.output":              `[{"x": 1}]` + "\n" + `[{"data": "a"}, {"data": "{\"peer\": 1}\n"}]`,
	}
}

func TestAGoodTreePasses(t *testing.T) {
	report, err := Check(context.Background(), base(), upstreamFiles())
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Problems) != 0 {
		t.Fatalf("problems: %v", report.Problems)
	}

	if report.Files != 4 || report.Proven != 2 || report.Lab != 1 || len(report.Attested) != 1 {
		t.Errorf("report = %+v", report)
	}
}

func TestOfflineChecksEverythingButUpstream(t *testing.T) {
	report, err := Check(context.Background(), base(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(report.Problems) != 0 || report.Unchecked != 2 || report.Proven != 0 {
		t.Errorf("report = %+v", report)
	}
}

func TestRefusals(t *testing.T) {
	cases := map[string]struct {
		change func(fstest.MapFS, fake)
		want   string
	}{
		"no provenance": {
			func(tree fstest.MapFS, _ fake) {
				tree["recipes/cisco_ios/version.yaml"] = &fstest.MapFile{Data: []byte("family: cisco_ios\n")}
			},
			"names no provenance",
		},
		"both lab and upstream": {
			func(tree fstest.MapFS, _ fake) {
				tree["recipes/frr/version.yaml"] = &fstest.MapFile{Data: []byte(header("exact", "tests/show_version.raw") + "verified: frr 10\n")}
			},
			"declares both",
		},
		"short commit": {
			func(tree fstest.MapFS, _ fake) {
				body := strings.Replace(string(tree["recipes/cisco_ios/version.yaml"].Data), commit, "c6dca50", 1)
				tree["recipes/cisco_ios/version.yaml"] = &fstest.MapFile{Data: []byte(body)}
			},
			"full 40-character SHA",
		},
		"unknown source": {
			func(tree fstest.MapFS, _ fake) {
				body := strings.Replace(string(tree["recipes/cisco_ios/version.yaml"].Data), ntc, "someone/else", 1)
				tree["recipes/cisco_ios/version.yaml"] = &fstest.MapFile{Data: []byte(body)}
			},
			"is not in tools/upstreams.yml",
		},
		"source missing from NOTICE": {
			func(tree fstest.MapFS, _ fake) {
				tree["NOTICE"] = &fstest.MapFile{Data: []byte("nothing\n")}
			},
			"not named in NOTICE",
		},
		"capture differs": {
			func(tree fstest.MapFS, _ fake) {
				tree["recipes/cisco_ios/version.captured"] = &fstest.MapFile{Data: []byte("edited\n")}
			},
			"differs from",
		},
		"extract differs": {
			func(tree fstest.MapFS, _ fake) {
				tree["recipes/juniper_junos/bgp_neighbors.captured"] = &fstest.MapFile{Data: []byte("{}\n")}
			},
			"at 1.1.data",
		},
		"upstream file missing": {
			func(_ fstest.MapFS, up fake) {
				delete(up, ntc+"|"+commit+"|tests/show_version.raw")
			},
			"does not exist upstream",
		},
		"licence marker missing": {
			func(_ fstest.MapFS, up fake) {
				up[ntc+"|"+commit+"|LICENSE"] = "MIT License"
			},
			"is not established",
		},
		"modified without note": {
			func(tree fstest.MapFS, _ fake) {
				tree["recipes/_probes/cisco_ios.yaml"] = &fstest.MapFile{Data: []byte(header("modified", "tests/show_version2.raw") + "id: cisco_ios\n")}
			},
			"needs a note",
		},
		"extract without select": {
			func(tree fstest.MapFS, _ fake) {
				tree["recipes/juniper_junos/bgp_neighbors.yaml"] = &fstest.MapFile{Data: []byte(header("extract", "bgp.output") + "family: juniper_junos\n")}
			},
			"needs a select",
		},
		"stray yaml": {
			func(tree fstest.MapFS, _ fake) {
				tree["tools/testdata/fixture.yaml"] = &fstest.MapFile{Data: []byte("x: 1\n")}
			},
			"name it .yml",
		},
		"capture without recipe": {
			func(tree fstest.MapFS, _ fake) {
				tree["recipes/cisco_ios/orphan.captured"] = &fstest.MapFile{Data: []byte("x\n")}
			},
			"evidence for nothing",
		},
		"recipe without capture": {
			func(tree fstest.MapFS, _ fake) {
				delete(tree, "recipes/frr/version.captured")
			},
			"has no recipes/frr/version.captured",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			tree, up := base(), upstreamFiles()
			c.change(tree, up)

			report, err := Check(context.Background(), tree, up)
			if err != nil {
				t.Fatal(err)
			}

			for _, problem := range report.Problems {
				if strings.Contains(problem.Message, c.want) {
					return
				}
			}

			t.Errorf("want a problem containing %q, got %v", c.want, report.Problems)
		})
	}
}
