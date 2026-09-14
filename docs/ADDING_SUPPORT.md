# Adding device and command support

How new family and command support is added to this repository.

## Principle

Do not invent parsing knowledge that already exists.

Before writing a recipe for a new family, command or question, search the
existing parser corpora. DispelNet may adapt open-source knowledge into native
recipes when the licence permits.

Current upstream sources:

1. NTC Templates, https://github.com/networktocode/ntc-templates
2. Cisco Genie Parser, https://github.com/CiscoTestAutomation/genieparser

The result is always a native DispelNet recipe, run by `dispelnet`'s own
runtime. Upstream projects are knowledge sources, not runtime dependencies, and a
recipe has no way to call out to them anyway.

```text
existing DispelNet support
        │
        ├── exists → extend and test it
        │
        └── missing
              ↓
         NTC Templates
              ↓
         Genie Parser
              ↓
      other suitable corpus
              ↓
      adapt or write the parser
              ↓
      capture and provenance
              ↓
          validation
              ↓
         native recipe
```

If no suitable implementation exists, write the parser from captured output.

## 1. Start with the question

Device commands are not what DispelNet exposes. A recipe answers a question:

```yaml
question: lldp-neighbors
```

Cisco XR may answer that question with `show lldp neighbors`. Another platform
may use a completely different command.

Before adding support, find out:

- which DispelNet question is being answered;
- whether that question already exists;
- which fields the question is expected to produce;
- whether the family already has a recipe answering it.

Do not create a new question because an upstream parser uses a different command
name.

The file is named for the question with underscores, `lldp_neighbors.yaml`; the
`question:` inside it uses hyphens.

## 2. Search DispelNet first

Search this repository before writing anything. Check for:

- the family;
- the question;
- equivalent commands;
- existing templates;
- existing captures;
- the fields other families produce for the same question.

If support already exists, extend the existing recipe or its `applies` range
rather than adding a competing one. Two recipes that could both answer one
question for one version are refused, because there would be nothing to choose
between them.

## 3. Search NTC Templates

If this repository has nothing suitable, search NTC Templates by family, command,
equivalent command names and the concept being collected. For example:

```text
family:   cisco_xr
question: lldp-neighbors
command:  show lldp neighbors
```

Look for the TextFSM template and its test fixtures under `tests/`. NTC is most
useful when the job is CLI text to structured rows.

If a suitable template exists:

1. inspect the template;
2. inspect its fixtures and expected output;
3. adapt the parsing knowledge into the recipe;
4. adjust it for DispelNet's TextFSM implementation where necessary;
5. map the parsed values into DispelNet fields;
6. record provenance (§6);
7. validate it with `dispelnet` (§11).

Do not assume DispelNet's TextFSM behaves identically to the one NTC uses. A
template passing upstream tests does not show that it works here; only the
DispelNet runtime can.

## 4. Search Genie Parser

If NTC has nothing suitable, or its template is insufficient, search Genie
Parser. It may hold richer parsing and schema knowledge than a TextFSM template:
supported commands, regular expressions, schemas, field names, fixtures and
expected structured output.

A Genie parser is Python and may not map onto TextFSM. Do not force a complex
parser into a template to keep it looking similar. Treat Genie as a
specification and test oracle instead:

```text
raw fixture
    ↓
Genie expected structure
    ↓
native DispelNet recipe
    ↓
DispelNet output
    ↓
semantic comparison
```

Reimplement only the parsing the DispelNet question needs.

## 5. Prefer the smallest parser necessary

Do not copy an entire upstream parser. If an upstream LLDP parser extracts
`neighbor`, `local_interface`, `hold_time`, `capability`, `neighbor_port`,
`management_address` and more, but the question needs only `identity`,
`local_port` and `port`, read those three.

Collect data because DispelNet has a use for it, not because it is there.

## 6. Record provenance

Every recipe and probe ships the raw output it was written against, as a
`.captured` file beside it. Every recipe and probe must also say where that
output came from, in exactly one of two ways.

### Upstream capture

A capture taken from an upstream project carries a capture block in the YAML
comments, and CI resolves it against upstream:

```yaml
# Capture:
#   source: networktocode/ntc-templates
#   commit: c6dca50ea10fe5a23e750748582fddda263fb053
#   file:   tests/cisco_ios/show_version/cisco_ios_show_version.raw
#   match:  exact
```

- `source` is the GitHub `owner/repo`.
- `commit` must be the **full 40-character SHA**. A short SHA or a tag can be
  made to point somewhere else; a full SHA cannot.
- `file` is the path inside that repository at that commit.
- `match` says how the `.captured` file relates to it, in one of three modes.

**`match: exact`.** The `.captured` file is the upstream file, byte for byte. CI
fetches `https://raw.githubusercontent.com/<source>/<commit>/<file>` and requires
its SHA-256 to equal the `.captured` file's. This is proven.

**`match: extract`.** The upstream file is a stream of JSON documents, and the
capture is one string value inside it. A `select:` path picks it out: the
document index, then array indexes and object keys, separated by dots. CI
requires that string, byte for byte, to equal the `.captured` file. This is
proven too.

```yaml
# Capture:
#   source: netenglabs/suzieq
#   commit: 75198b5585ebef91f0659b6b1ec3c964cc2cd1ad
#   file:   tests/integration/sqcmds/junos-input/bgp.output
#   match:  extract
#   select: 1.1.data
```

**`match: modified`.** The capture was changed from the upstream file. It
requires a `note:` line saying what was changed and why:

```yaml
# Capture:
#   source: networktocode/ntc-templates
#   commit: c6dca50ea10fe5a23e750748582fddda263fb053
#   file:   tests/<platform>/<command>/<fixture>.raw
#   match:  modified
#   note:   <what was changed, and why>
```

CI proves only that the upstream file exists at that commit, not that the
capture came from it. A modified capture is attested by its contributor and
reviewer, and the README lists it as such. Prefer `exact` or `extract` whenever
the output can be used as published.

A capture taken from upstream declares no `verified:`: nobody here ran it
against a device.

### Lab capture

Output you captured yourself from a real device declares `verified:` with the
device and version it ran on, and carries **no** capture block:

```yaml
verified: frr 10.3.4
```

This is your attestation, checked only by review. CI cannot tell a real device's
output from typed text, so a lab capture is trusted, not proven. Save exactly
what the device printed; do not tidy it.

A recipe or probe with neither a capture block nor `verified:`, or with both, is
refused.

### A new upstream source

Only sources listed in `tools/upstreams.yml` are accepted, and each must be
named in `NOTICE`. A pull request that takes output from a new source adds the
source to both, in the same pull request. The `tools/upstreams.yml` entry names
the licence, the licence file and a single line CI requires in that file at every
commit a capture uses; it is reviewed when it is added.

### Parser derived from upstream

A capture block records where the *output* came from. A template or parser
adapted from upstream also records where the *parsing knowledge* came from, in
a `Parser derived from:` comment:

```yaml
# Physical neighbours, by the name each one calls itself.
#
# Parser derived from:
#   networktocode/ntc-templates
#   ntc_templates/templates/cisco_xr_show_lldp_neighbors.textfsm
#   commit: c6dca50ea10fe5a23e750748582fddda263fb053
#   Apache-2.0
#
# Modified for DispelNet.
```

Use the exact repository, file and commit. Do not write vague attribution such
as `# based on NTC`.

This block is enforced by review, not CI, because only a person can tell derived
from independently written. Comments are not a substitute for repository-level
licence compliance: `NOTICE` must stay consistent with what is incorporated.

## 7. Captures are part of support

A recipe without representative captured output is incomplete. Every recipe and
probe needs a capture that exercises its parsing path; `dispelnet recipes verify`
refuses one that ships none, or can no longer read the one it ships.

Prefer, in order:

1. output captured from real hardware or software;
2. appropriately licensed upstream fixtures;
3. existing captures in this repository.

Never claim a recipe was verified against real equipment when it has only been
tested against an upstream fixture.

## 8. Map syntax into DispelNet semantics

The template produces syntax. The recipe assigns meaning.

```text
Value NEIGHBOR (\S+)
Value LOCAL_PORT (\S+)
Value NEIGHBOR_PORT (\S+)
```

is parser knowledge.

```yaml
fields:
  identity: NEIGHBOR
  port:     NEIGHBOR_PORT
```

is DispelNet knowledge. Map into the fields the question already uses rather than
leaking upstream field names into the model. Upstream names are implementation
details; DispelNet's fields are the contract.

## 9. Identification is separate

Adding a recipe does not teach `dispelnet` to recognise a family. That is a probe,
in `recipes/_probes/<family>.yaml`, with its own `.captured` beside it and its own
provenance.

When adding a new family, decide separately:

- how the family is identified;
- which probe provides the evidence;
- which tier applies (`vendor`, `os` or `generic`);
- whether the probe conflicts with existing families.

A probe performs one small, read-only test. Do not turn probes into small
discovery recipes.

A family is usable only when `dispelnet` can both identify it reliably and answer
useful questions about it.

## 10. Commands must be read-only

Discovery commands run on production equipment, and must only read. Never add a
command that:

- modifies configuration;
- clears state or counters;
- restarts anything;
- writes files;
- invokes a local shell;
- causes any intentional operational change.

A command's presence in NTC, Genie or any other project is not evidence that it
is safe for automatic discovery. If you are unsure, do not add it until it has
been reviewed.

## 11. Validation

Run these from the repository root before opening a pull request:

```console
$ tree=$(mktemp -d)
$ .github/scripts/collect-tree.sh . "$tree"
$ dispelnet recipes verify --from "$tree"
$ go -C tools run ./cmd/provenance ..
$ go -C tools test ./...
```

- `dispelnet recipes verify` parses every recipe and probe, runs each against its
  capture, and refuses overlaps and probe conflicts.
- `provenance` checks every capture block against upstream and every
  `tools/upstreams.yml` licence. Add `--offline` to skip the network fetches; it
  then checks only the structure, and says so.
- `go test` runs the tools' own tests.

To try a recipe against your own device, collect the runtime tree first, then
pass it to `dispelnet discover --recipes "$tree"`.

Pull request CI runs the same checks. The provenance check, the tools tests and
the bump report run straight away. `dispelnet checks`, which runs the checker
pinned in `.dispelnet-version` against your data files, runs at once for a branch
in this repository; for a fork it waits until a maintainer approves it.

Do not weaken a check to make imported content pass. Adapt the content instead.

## 12. Definition of done: a new command

- [ ] Existing support in this repository was checked.
- [ ] NTC Templates was checked.
- [ ] Genie Parser was checked.
- [ ] The command is read-only.
- [ ] The recipe answers an existing, or deliberately added, question.
- [ ] Parsed values are mapped into DispelNet fields.
- [ ] A representative `.captured` is included.
- [ ] The capture has a capture block or `verified:`, not both.
- [ ] An adapted parser carries a `Parser derived from:` block.
- [ ] Any new upstream source is in `tools/upstreams.yml` and `NOTICE`.
- [ ] `dispelnet recipes verify --from "$tree"` passes.
- [ ] `provenance` and the tools tests pass.
- [ ] No existing family, question or version coverage regresses.

## 13. Definition of done: a new family

Everything above, and:

- [ ] The family identifier follows the existing naming.
- [ ] `recipes/_probes/<family>.yaml` exists, with its `.captured` and provenance.
- [ ] Identification creates no unresolved ambiguity with existing families.
- [ ] At least the minimum useful questions are answered.
- [ ] Relevant NTC and Genie coverage was investigated.
- [ ] The family works end to end: identify, recipes, facts, model.

## 14. Support levels

Be precise when describing coverage.

```text
IMPORTED    Upstream parsing knowledge exists and has been adapted.

VALIDATED   The native recipe passes `dispelnet recipes verify` against its
            capture.

VERIFIED    The recipe has been observed working against the real device
            or software family, and declares it with `verified:`.
```

Imported is not verified. A recipe whose capture came from upstream is at most
validated, however well it passes.
