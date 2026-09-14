// Package corpus reads this repository the way its checks need to: which paths
// are recipes, probes and captures, what a capture block says, and the parts of
// a recipe or probe that decide how big a change to it is.
//
// It does not validate recipes. `dispelnet recipes verify` does that, and a
// second, weaker parser here would only disagree with it. The reads are loose
// on purpose: an unknown key is the checker's to refuse.
package corpus

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

// Kind is what a path in the repository is.
type Kind int

// The kinds of path a repository holds.
const (
	// Other is anything a release does not ship: docs, tools, workflows.
	Other Kind = iota
	Recipe
	Probe
	Capture
	// Shipped is a non-recipe file inside the release tarball.
	Shipped
)

func (k Kind) String() string {
	return [...]string{"other", "recipe", "probe", "capture", "shipped"}[k]
}

// RecipesDir contains all runtime recipe data.
const RecipesDir = "recipes"

// ProbeDir is the directory inside RecipesDir where identification lives. A
// probe has no family until it identifies one, so it has no family folder.
const ProbeDir = "_probes"

// familyName is the shape of a family folder, following netmiko and
// ntc-templates naming.
var familyName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ShippedFiles ship in the release tarball beside the tree.
var ShippedFiles = []string{"LICENSE", "NOTICE"}

// Classify says what a slash-separated repository path is.
func Classify(name string) Kind {
	if slices.Contains(ShippedFiles, name) {
		return Shipped
	}

	dir, file := path.Split(name)
	dir = strings.TrimSuffix(dir, "/")
	parts := strings.Split(dir, "/")

	if len(parts) != 2 || parts[0] != RecipesDir {
		return Other
	}

	family := parts[1]
	if family != ProbeDir && !familyName.MatchString(family) {
		return Other
	}

	switch {
	case strings.HasSuffix(file, ".captured"):
		return Capture
	case !strings.HasSuffix(file, ".yaml"):
		return Other
	case family == ProbeDir:
		return Probe
	default:
		return Recipe
	}
}

// CapturedPath is the evidence beside a recipe or probe.
func CapturedPath(name string) string {
	return strings.TrimSuffix(name, ".yaml") + ".captured"
}

// Match modes for a capture block.
const (
	Exact    = "exact"
	Extract  = "extract"
	Modified = "modified"
)

// CaptureBlock is the provenance a recipe or probe declares in its header.
type CaptureBlock struct {
	Source string
	Commit string
	File   string
	Match  string
	Select string
	Note   string
}

var captureField = regexp.MustCompile(`^#\s+([a-z]+):\s*(.*?)\s*$`)

// ParseCapture reads the capture block from a file's leading comments. It
// returns nil when there is none, and an error when one is started but cannot
// be read.
func ParseCapture(body []byte) (*CaptureBlock, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))

	var (
		block  *CaptureBlock
		inside bool // reading the lines of the block
		seen   = map[string]bool{}
		line   int
	)

	// Only the leading comments are a header; the first line that is not a
	// comment ends the search.
	for scanner.Scan() {
		line++
		text := scanner.Text()

		if !strings.HasPrefix(text, "#") {
			break
		}

		if strings.TrimSpace(text) == "# Capture:" {
			if block != nil {
				return nil, fmt.Errorf("line %d: a second capture block", line)
			}

			block, inside = &CaptureBlock{}, true

			continue
		}

		if !inside {
			continue
		}

		found := captureField.FindStringSubmatch(text)
		if found == nil {
			// The block ends at the first line that is not one of its fields.
			inside = false

			continue
		}

		key, value := found[1], found[2]
		if seen[key] {
			return nil, fmt.Errorf("line %d: capture block repeats %s", line, key)
		}

		seen[key] = true

		switch key {
		case "source":
			block.Source = value
		case "commit":
			block.Commit = value
		case "file":
			block.File = value
		case "match":
			block.Match = value
		case "select":
			block.Select = value
		case "note":
			block.Note = value
		default:
			return nil, fmt.Errorf("line %d: capture block has unknown field %s", line, key)
		}
	}

	return block, scanner.Err()
}

// Doc is what a size-of-change decision needs from a recipe or probe.
// Every slice is in a canonical order so that two Docs compare meaningfully.
type Doc struct {
	Family   string
	Question string
	ID       string
	Verified string

	// Applies are the version patterns a recipe answers for, sorted.
	Applies []string

	// Sends is everything that decides what reaches a device, one entry per
	// setting, in file order: a recipe's command or each step's run, uses and
	// with, for-each and limit; a probe's transport, command, oid and read.
	Sends []string

	// Fields are the output field names a recipe produces, sorted.
	Fields []string

	// Tier and Is decide how a probe identifies a device.
	Tier string
	Is   []string
}

type step struct {
	Run     string         `yaml:"run"`
	Uses    string         `yaml:"uses"`
	With    map[string]any `yaml:"with"`
	ForEach string         `yaml:"for-each"`
	Limit   int            `yaml:"limit"`
	Fields  map[string]any `yaml:"fields"`
}

type document struct {
	Family    string         `yaml:"family"`
	Question  string         `yaml:"question"`
	Applies   []string       `yaml:"applies"`
	Verified  string         `yaml:"verified"`
	Command   string         `yaml:"command"`
	Fields    map[string]any `yaml:"fields"`
	Steps     []step         `yaml:"steps"`
	ID        string         `yaml:"id"`
	Tier      string         `yaml:"tier"`
	Transport string         `yaml:"transport"`
	OID       string         `yaml:"oid"`
	Read      string         `yaml:"read"`
	Is        []any          `yaml:"is"`
}

// ParseDoc reads a recipe or probe loosely.
func ParseDoc(kind Kind, body []byte) (Doc, error) {
	var d document
	if err := yaml.Unmarshal(body, &d); err != nil {
		return Doc{}, err
	}

	doc := Doc{
		Family:   d.Family,
		Question: d.Question,
		ID:       d.ID,
		Verified: d.Verified,
		Tier:     d.Tier,
		Applies:  sorted(d.Applies),
	}

	send := func(key, value string) {
		if value != "" {
			doc.Sends = append(doc.Sends, key+": "+value)
		}
	}

	switch kind {
	case Probe:
		send("transport", d.Transport)
		send("command", d.Command)
		send("oid", d.OID)
		send("read", d.Read)

		for _, assertion := range d.Is {
			doc.Is = append(doc.Is, canonical(assertion))
		}

		slices.Sort(doc.Is)
	default:
		send("command", d.Command)

		names := keys(d.Fields)

		for i, s := range d.Steps {
			prefix := "steps." + strconv.Itoa(i) + "."
			send(prefix+"run", s.Run)
			send(prefix+"uses", s.Uses)

			if len(s.With) > 0 {
				send(prefix+"with", canonical(s.With))
			}

			send(prefix+"for-each", s.ForEach)

			if s.Limit != 0 {
				send(prefix+"limit", strconv.Itoa(s.Limit))
			}

			names = append(names, keys(s.Fields)...)
		}

		doc.Fields = sorted(names)
	}

	return doc, nil
}

// canonical renders a YAML value with sorted keys, so equal values compare
// equal whatever order they were written in.
func canonical(value any) string {
	out, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}

	return string(out)
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}

// sorted returns a sorted copy without duplicates.
func sorted(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)

	return slices.Compact(out)
}
