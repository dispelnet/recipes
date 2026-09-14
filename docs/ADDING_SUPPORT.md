# Adding device and command support

How new family and command support is added to this repository.

## Principle

Do not invent parsing knowledge that already exists.

Before writing a recipe for a new family, command or question, search the
existing parser corpora. Recipes may adapt open-source knowledge into native
recipes when the licence permits.

Current upstream sources:

1. NTC Templates, https://github.com/networktocode/ntc-templates
2. Cisco Genie Parser, https://github.com/CiscoTestAutomation/genieparser

The result is always a native recipe, run by `dispelnet`'s own runtime.
Upstream projects are knowledge sources, not runtime dependencies, and a recipe
has no way to call out to them anyway.

```text
existing recipe support
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
     capture and attribution
              ↓
          validation
              ↓
         native recipe
```

If no suitable implementation exists, write the parser from captured output.

## 1. Start with the question

Device commands are not what `dispelnet` exposes. A recipe answers a question:

```yaml
question: lldp-neighbors
```

Cisco XR may answer that question with `show lldp neighbors`. Another platform
may use a completely different command.

Before adding support, find out:

- which `dispelnet` question is being answered;
- whether that question already exists;
- which fields the question is expected to produce;
- whether the family already has a recipe answering it.

Do not create a new question because an upstream parser uses a different command
name.

The file is named for the question with underscores, `lldp_neighbors.yaml`; the
`question:` inside it uses hyphens.

## 2. Search this repository first

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
4. adjust it for `dispelnet`'s TextFSM implementation where necessary;
5. map the parsed values into `dispelnet` fields;
6. keep the capture, and name the source on the template's first line (§6);
7. validate it with `dispelnet` (§11).

Do not assume `dispelnet`'s TextFSM behaves identically to the one NTC uses. A
template passing upstream tests does not show that it works here; only the
`dispelnet` runtime can.

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
native recipe
    ↓
dispelnet output
    ↓
semantic comparison
```

Reimplement only the parsing the question needs.

## 5. Prefer the smallest parser necessary

Do not copy an entire upstream parser. If an upstream LLDP parser extracts
`neighbor`, `local_interface`, `hold_time`, `capability`, `neighbor_port`,
`management_address` and more, but the question needs only `identity`,
`local_port` and `port`, read those three.

Collect data because `dispelnet` has a use for it, not because it is there.

## 6. Keep the raw output, and name what you did not write

Every recipe and probe keeps the raw output it was written against in this
repository, as a `.captured` file beside it. CI checks the recipe against it;
releases ship only the recipes. Save exactly what the device or the fixture
printed, and do not tidy it: the point of a capture is that it is not written
by hand.

Output taken from an upstream project is used byte for byte, and the project
must be listed in `NOTICE`; add it there in the same pull request if it is new.

A step whose parsing you did not write names its source on the template's first
line, as `owner/repo@<full commit>`, and nothing more. The commit is what the
licence is checked against.

```yaml
    template: |
      # networktocode/ntc-templates@c6dca50ea10fe5a23e750748582fddda263fb053
      Value Required NEIGHBOR (.+?)
```

Comments at the top of a recipe are only for what a reader could not guess: a
device quirk, or why this command rather than an obvious other one.

## 7. Captures are part of support

A recipe without representative captured output is incomplete. Every recipe and
probe needs a capture that exercises its parsing path; `dispelnet recipes verify`
refuses one that has none, or can no longer read the one it has.

Prefer, in order:

1. output captured from real hardware or software;
2. appropriately licensed upstream fixtures;
3. existing captures in this repository.

### One recipe, several output formats

When a command's output differs between releases or platforms, prefer one
template that reads every known format, with one capture per format:

```text
recipes/arista_eos/ospf_neighbors.yaml
recipes/arista_eos/ospf_neighbors.captured               the main capture
recipes/arista_eos/ospf_neighbors.no-instance.captured   another format
```

A label is lower-case letters, digits and hyphens, and says what is different
about that output. `dispelnet recipes verify` requires the recipe to read every
capture, so each format stays checked.

Split a question into several recipes with non-overlapping `applies:` ranges
only when the command itself differs between versions, or when captures show
which versions produce which output. A wrong range leaves those versions
without a recipe, and nothing reports it.

## 8. Map syntax into `dispelnet` semantics

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

is `dispelnet` knowledge. Map into the fields the question already uses rather
than leaking upstream field names into the model. Upstream names are
implementation details; `dispelnet`'s fields are the contract.

Which fields a question has, and what kind of value each one holds, is written
down in `recipes/_questions/<question>.json`. Read it before mapping: it says
which fields are required, and what a value is expected to be — an address, a
MAC, an interface name, an AS number. A new question needs a schema in the same
pull request.

## 9. Identification is separate

Adding a recipe does not teach `dispelnet` to recognise a family. That is a probe,
in `recipes/_probes/<family>.yaml`, with its own `.captured` beside it.

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

Run from the repository root:

```console
$ make check
```

It downloads the newest [dispelnet](https://github.com/dispelnet/dispelnet)
release, which needs the [GitHub CLI](https://cli.github.com), logged in. To use
a dispelnet you already have, pass `DISPELNET=/path/to/dispelnet`; for a given
release, `DISPELNET_VERSION=v0.1.0`.

- The layout check refuses anything under `recipes/` but recipes and probes,
  each with its capture beside it, as regular files: everything under it but
  the captures ships in the release.
- `dispelnet recipes verify` parses every recipe and probe, runs each against
  every capture, and refuses overlaps and probe conflicts.

To try a recipe against your own device, pass the collected tree to
`dispelnet discover --recipes build/tree`.

Pull request CI runs the same `make check`.

Do not weaken a check to make imported content pass. Adapt the content instead.

## 12. Definition of done: a new command

- [ ] Existing support in this repository was checked.
- [ ] NTC Templates was checked.
- [ ] Genie Parser was checked.
- [ ] The command is read-only.
- [ ] The recipe answers an existing, or deliberately added, question.
- [ ] Parsed values are mapped into `dispelnet` fields.
- [ ] A representative `.captured` is included.
- [ ] Upstream output is used verbatim.
- [ ] Parsing you did not write is attributed as §6 requires.
- [ ] Any new upstream source is listed in `NOTICE`.
- [ ] `make check` passes.
- [ ] No existing family, question or version coverage regresses.

## 13. Definition of done: a new family

Everything above, and:

- [ ] The family identifier follows the existing naming, and is not `probes`,
      which is reserved: probes live in `recipes/_probes/`.
- [ ] `recipes/_probes/<family>.yaml` exists, with its `.captured` beside it.
- [ ] Identification creates no unresolved ambiguity with existing families.
- [ ] At least the minimum useful questions are answered.
- [ ] Relevant NTC and Genie coverage was investigated.
- [ ] The family works end to end: identify, recipes, facts, model.

