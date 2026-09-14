package release

import "testing"

func TestCheckName(t *testing.T) {
	for tag, ok := range map[string]bool{
		"v1.0.0":        true,
		"v1.2.3":        true,
		"v1.1.0-rc.1":   true,
		"v2.0.0-beta":   true,
		"1.0.0":         false, // no v prefix
		"v1":            false, // shorthand
		"v1.2":          false, // shorthand
		"v1.0.0+build1": false, // build metadata
		"v01.0.0":       false,
		"latest":        false,
		"vfoo":          false,
	} {
		if err := CheckName(tag); (err == nil) != ok {
			t.Errorf("CheckName(%q) = %v, want ok=%v", tag, err, ok)
		}
	}
}

func TestBase(t *testing.T) {
	published := []string{"v1.0.0", "v1.0.3", "v1.1.0-rc.1", "v1.1.0-rc.2", "v2.0.0", "v1", "junk"}

	cases := []struct {
		tag, want string
		found     bool
	}{
		{"v1.1.0-rc.1", "v1.0.3", true},
		{"v1.1.0-rc.3", "v1.0.3", true}, // earlier rcs are never a base
		{"v1.1.0", "v1.0.3", true},
		{"v1.0.4", "v1.0.3", true},
		{"v2.0.1", "v2.0.0", true},
		{"v1.0.1", "v1.0.0", true}, // a v1 patch after v2 exists
		{"v1.0.0", "", false},
		{"v1.0.0-rc.1", "", false},
	}

	for _, c := range cases {
		got, found := Base(c.tag, published)
		if got != c.want || found != c.found {
			t.Errorf("Base(%q) = %q, %v; want %q, %v", c.tag, got, found, c.want, c.found)
		}
	}
}

func TestDistance(t *testing.T) {
	cases := []struct {
		base, tag string
		want      Level
		fails     bool
	}{
		{"", "v1.0.0", Major, false},
		{"", "v1.0.0-rc.1", Major, false},
		{"", "v1.2.0", None, true},
		{"", "v0.1.0", None, true},
		{"v1.0.3", "v1.0.4", Patch, false},
		{"v1.0.3", "v1.1.0-rc.1", Minor, false},
		{"v1.0.3", "v1.1.0", Minor, false},
		{"v1.9.9", "v2.0.0", Major, false},
		{"v1.0.3", "v1.0.3", None, true},
		{"v1.0.3", "v1.0.2", None, true},
	}

	for _, c := range cases {
		got, err := Distance(c.base, c.tag)
		if (err != nil) != c.fails || (!c.fails && got != c.want) {
			t.Errorf("Distance(%q, %q) = %v, %v; want %v, fails=%v", c.base, c.tag, got, err, c.want, c.fails)
		}
	}
}
