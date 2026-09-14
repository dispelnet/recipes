# Recipes: CI, versioning, provenance and signed releases

Status: approved design, revised after review, 2026-09-14
Repository: `github.com/dispelnet/recipes`

## Goal

Make this repository trustworthy and contributable enough to publish as step 3
of the launch, before `v0.1.0-alpha.1`:

- every pull request, from a fork or not, is checked against the raw captured
  output beside each recipe and probe;
- every release is an immutable, semver-tagged GitHub release carrying the
  recipe tree as a tarball, a checksum file and a Sigstore signature;
- GitHub's "Latest release" points at the newest stable release;
- every capture taken from an upstream project is checked against that project
  at a fixed commit, not only named.

## Non-goals

- **No change to dispelnet.** `recipes update` keeps fetching
  `codeload.github.com/<repo>/tar.gz/<ref>` unverified. Signatures are published
  so a later dispel change can enforce them; until then they serve people
  checking by hand.
- No registry, index, CDN or website integration.
- No new recipes, families or checks beyond what already exists in
  `dispel/recipes`.

## Decisions

| Topic | Decision |
|---|---|
| Repository | `dispelnet/recipes` (dispel's default source is renamed later, in dispel) |
| `latest` | GitHub's built-in Latest marker on immutable releases; no moving git tag |
| Immutability | GitHub Immutable Releases enabled; releases published draft → assets → publish |
| Versions | Strict SemVer, annotated `vMAJOR.MINOR.PATCH[-PRERELEASE]` tags; MAJOR only when not backwards compatible |
| Compatibility | A dispelnet binary will carry the major it reads (later dispel change) |
| Signing | Sigstore keyless (cosign, GitHub OIDC) plus GitHub's release attestation |
| Checker in CI | dispelnet built from a pinned dispel commit, later a pinned release tag |
| Fork PRs | dispel stays private for now. PR code runs without secrets; the dispelnet checks run in a trusted job that never executes PR code and, for forks, waits for maintainer approval |
| Provenance | Upstream captures resolved against a pinned commit and hash-compared; licences come from a reviewed allowlist |
| Cutting a release | A maintainer pushes an annotated tag; CI computes the minimum bump and refuses a smaller one |
| Bump on PRs | Advisory: reported, never blocking |

## Repository layout

```text
<family>/<question>.yaml        recipe
<family>/<question>.captured    raw device output it was written against
<family>/README.md              optional notes
probes/<family>.yaml            identification probe
probes/<family>.captured
tools/                          Go module: provenance and bump checkers
tools/upstreams.yml             reviewed allowlist of upstream sources
.github/workflows/ci.yml        PR code, no secrets
.github/workflows/checker.yml   dispelnet checks on PR data (pull_request_target)
.github/workflows/release.yml
.dispelnet-version              pinned checker: a dispel commit SHA or tag
LICENSE  NOTICE  README.md  VERSIONING.md  ADDING_SUPPORT.md
docs/superpowers/specs/         design records
```

**No `.yaml` file may exist outside `<family>/` and `probes/`.**
`recipe.FromDir` walks the whole tree and parses every `.yaml`, and the codeload
fetch delivers the whole repository, so a stray `.yaml` (a test fixture, a
config file) breaks both `recipes verify` and every client. Workflows and tool
data use `.yml`; tool fixtures use `.yml` or txtar. The provenance check
enforces this.

## Versioning (content of `VERSIONING.md`)

The consumer is the dispelnet binary and whatever reads the facts it records.

| Bump | Meaning | Changes |
|---|---|---|
| MAJOR | A client of the previous release could refuse the tree or silently lose something | the tree is refused by the checker pinned at the base release (a new field, `uses:` reader or assertion kind) · a recipe or probe file removed · `family` or `question` renamed · a `fields:` key removed · an `applies` pattern removed |
| MINOR | Adds coverage, or changes what is sent to a device | recipe or probe added (a new family included) · a `run:` in any step, or a probe's `command:`/`oid:`, changed · an `applies` pattern added · a `fields:` key added · a probe's `tier:` or `is:` changed |
| PATCH | Same commands, same output shape | any other change to a recipe or probe (template, `select`, `key`, field mapping source, `verified:`, comments) · a `.captured` added or changed · `LICENSE` or `NOTICE` changed (both ship in the tarball) |
| none | Nothing a release ships | `README.md`, docs, `tools/`, `.github/`, `.dispelnet-version` (its effect is caught by the compatibility check) |

Rules on top of the table:

- The highest bump among all changes wins.
- **A changed command is never a patch**, even when it fixes something: a
  release that changes what runs on production equipment must say so.
- **A safety withdrawal is a patch.** Removing a recipe that harms a device must
  not wait for a major. The tag message declares it with one trailer per file,
  `Withdraws: <path> — <reason>`. The trailer lowers that file's removal from
  MAJOR to PATCH, and the release notes lead with it.
- **Backwards compatible is tested, not judged.** The MAJOR check builds the
  checker pinned in `.dispelnet-version` *at the base release* and runs
  `recipes verify` on the new tree. If that older checker refuses the tree, the
  release is MAJOR.

### Tag contract

A `v*` tag is a release request only if all of these hold. Otherwise the release
workflow fails at its first step, before bump selection, compatibility or
building, and publishes nothing.

1. **Strict SemVer 2.0 with a `v` prefix.** It must pass `semver.IsValid` and
   equal `semver.Canonical(tag)` (`golang.org/x/mod/semver`), which refuses the
   `v1` / `v1.2` shorthands. It must carry no build metadata (`+…`), since
   SemVer ignores it for precedence and two such tags could not be ordered.
2. **Annotated.** `git cat-file -t refs/tags/<tag>` is `tag`. A lightweight tag
   is refused, because the tag message carries `Withdraws:` trailers and the
   tagger identity.
3. **On main.** `git merge-base --is-ancestor <tag> origin/main`.
4. **The first release is `v1.0.0` or a `v1.0.0-…` pre-release.** Clients will
   pin a major, and `v0` would mean nothing to them.

**Base selection** — what a new tag is compared against, for both the bump and
the compatibility check:

- Candidates are tags that meet rules 1–2 **and have a published release**. A
  tag whose release failed, or never ran, is not a base.
- **Pre-releases are never a base.** The base is the highest stable release
  whose version is lower than the new tag. `v1.1.0-rc.1`, `v1.1.0-rc.2` and
  `v1.1.0` are all compared against, say, `v1.0.3`, so the final release is held
  to everything since the last stable one rather than the last rc.
- No candidate means a first release, and rule 4 applies.

Pre-releases follow the same bump rules, are published with `--prerelease`, and
are never Latest.

## Provenance

Every recipe and probe must satisfy exactly one of:

1. **Lab capture.** It declares `verified:` and carries no capture block. This
   is the contributor's attestation, checked only by review: CI cannot tell a
   real device's output from a typed one, and the docs say so rather than
   implying proof.
2. **Upstream capture.** It carries a capture block, and CI resolves it:

```yaml
# Capture:
#   source: networktocode/ntc-templates
#   commit: c6dca50ea10fe5a23e750748582fddda263fb053
#   file:   tests/cisco_ios/show_version/cisco_ios_show_version.raw
#   match:  exact
```

- `commit` must be a full 40-hex SHA, because a short SHA or a tag is not
  immutable.
- `match: exact` — CI fetches
  `https://raw.githubusercontent.com/<source>/<commit>/<file>` and requires its
  SHA-256 to equal the `.captured` file's. This is the only mode CI treats as
  proven.
- `match: extract` — the upstream file is a stream of JSON documents, and the
  capture is one string value inside it. A `select:` path picks it out:
  document index, then array indexes and object keys, separated by dots (for
  example `1.1.data`). CI requires that string, byte for byte, to equal the
  `.captured` file. This is proven too.
- `match: modified` — requires a `note:` line saying what was changed and why.
  CI proves the upstream file exists at that commit (HTTP 200) but not that the
  capture came from it. The provenance report and README list every modified
  capture as **reviewer-attested**.

**Licences are not per-file claims.** `tools/upstreams.yml` lists each allowed
source once, reviewed when it is added:

```yml
networktocode/ntc-templates:
  license: Apache-2.0
  license_file: LICENSE
  license_marker: "Licensed under the Apache License, Version 2.0"
netenglabs/suzieq:
  license: Apache-2.0
  license_file: LICENSE
  license_marker: "Version 2.0, January 2004"
```

A marker is a single line, so indentation in the licence file can't break the
check. ntc-templates' `LICENSE` is Apache-2.0's short notice form rather than
the full text, which is why the markers differ.

For every source and commit a capture uses, CI fetches `license_file` at that
commit and requires `license_marker` in it. That proves the stated licence
applied at the commit the capture was taken from. `NOTICE` must name every
source used.

The provenance checker (`tools/cmd/provenance`, no dispel import, needs no
secret) refuses:

- a recipe or probe that is neither a lab capture nor an upstream capture, or is
  both;
- a capture block with a missing field, a short commit, or an unknown `match`;
- a `source` absent from `tools/upstreams.yml`, or not named in `NOTICE`;
- an `exact` capture whose hash differs from upstream, or an upstream file or
  licence file that cannot be fetched;
- a `modified` capture with no `note:`;
- a `.yaml` outside `<family>/` and `probes/`, or a `.captured` with no `.yaml`
  beside it.

Fetched upstream files are cached in CI by `(source, commit, file)`, since the
content at a full SHA cannot change. `--offline` skips fetching for local runs
and says that it did.

A template adapted from upstream additionally carries ADDING_SUPPORT §6's
`Parser derived from:` block. That is enforced by review, not CI, because only a
person can tell derived from independently written.

### Migration, as found on 2026-09-14

- Restore the 57 YAML files from `dispel/recipes`. They differ from this
  checkout only in comments; every capture is byte-identical.
- **35 captures are `exact`**: each is byte-identical to a file in ntc-templates
  at `c6dca50ea10fe5a23e750748582fddda263fb053`, matched by git blob hash
  against a local clone. Their free-text comments become capture blocks with
  those paths. That includes `arista_eos/version.yaml`, which names no source
  today.
- **21 are lab captures** (`frr`, `linux`, `nokia_srlinux` and their probes),
  which already declare `verified:`.
- **1 is `extract`: `juniper_junos/bgp_neighbors.captured`.** Its old comment
  named both suzieq and "ntc-templates at commit c6dca50", which contradict each
  other. It is exactly the `data` string of record `1.1` (host leaf01,
  `show bgp neighbor | display json`) in suzieq's
  `tests/integration/sqcmds/junos-input/bgp.output` at
  `75198b5585ebef91f0659b6b1ec3c964cc2cd1ad`.

`NOTICE` is dispel's, re-pathed for this repository. `LICENSE` is Apache-2.0.

## CI

`dispelnet/dispel` stays private for now, and GitHub passes no secrets to a
workflow triggered by a fork. CI is therefore split by one rule: **code from a
pull request never runs in a job that holds a secret.**

### `ci.yml`: the PR's own code, no secrets (`pull_request`, pushes to main)

Runs immediately for every PR, forks included.

1. **Provenance.** `go run ./tools/cmd/provenance .` (online; upstream fetches
   need no credentials).
2. **Tools.** `go test ./...` in `tools/`.
3. **Bump (PRs only).** `go run ./tools/cmd/bump --base <base> --head HEAD`
   prints the minimum bump and the changes behind it, with base selection as in
   the tag contract. It always writes the job summary. A sticky PR comment is
   posted only for same-repository PRs, since a fork's `GITHUB_TOKEN` is
   read-only. With no base, it reports `v1.0.0 (first release)`.

Permissions: `contents: read` at top level; `pull-requests: write` on the bump
job only.

### `checker.yml`: dispelnet against the PR's data (`pull_request_target`, pushes to main)

`pull_request_target` runs the workflow file **from the base branch**, so a PR
cannot change what this job does. It is used for **every** PR, same-repository
ones included, so that one job, `dispelnet checks`, is the required status
check for all of them. A job skipped by an `if:` counts as passing in branch
protection, so the checks are never split into a fork variant and a branch
variant where the skipped one could satisfy the requirement.

**Approval.** The job's environment is chosen per event:
`environment: ${{ github.event.pull_request.head.repo.full_name != github.repository && 'external-pr' || 'internal' }}`.
`external-pr` requires a maintainer's approval, and GitHub asks again for every
new push to the fork. `internal` requires none, and serves same-repository PRs
and pushes to main. Both environments hold `DISPEL_READ_TOKEN`: a fine-grained
token with read-only `contents` on `dispelnet/dispel` alone. It is not a
repository secret, so no other workflow can read it.

**Steps:**

1. **Pin.** For a fork PR, the pin comes from `.dispelnet-version` on the base
   branch, and a fork PR that changes that file fails with "a checker pin
   change must come from a branch in this repository". For a same-repository
   PR or a push, it comes from the head.
2. **Checker.** `actions/checkout` of `dispelnet/dispel` at the pin into
   `dispel/`, with the token and `persist-credentials: false`, so the token
   stays in that one step and never reaches disk. Then
   `.github/scripts/build-checker.sh`, with the toolchain from dispel's
   `go.mod`. It builds `cmd/dispelnet`, or `cmd/dispel` at commits from before
   the rename, because the release's compatibility check builds the base
   release's pin, which may predate it.
3. **PR data.** `actions/checkout` of
   `github.event.pull_request.head.sha` into `pr/`, with
   `persist-credentials: false`. Copy into a fresh `tree/` only **regular
   files** matching `<family>/*.yaml`, `<family>/*.captured`, `probes/*.yaml`
   and `probes/*.captured`. Any symlink, or any other file type, at those paths
   fails the job. Nothing else from `pr/` is read, and `pr/` is deleted before
   the next step.
4. **Verify.** `./dispelnet recipes verify --from tree`
5. **Corpus tests.** Replace `dispel/recipes/` with `tree/`, keeping dispel's
   own `recipes_test.go`, and run `go test ./recipes/` inside `dispel/`. The
   only Go code that runs is dispel's, at a pinned commit. This keeps the three
   checks `verify` lacks: row count against `Total entries displayed: N`,
   byte-identical duplicate transcripts, and no `quarantine:` key. If the
   pinned dispel has no `recipes/recipes_test.go`, the step fails with "the
   corpus checks must move into `recipes verify` before this pin" rather than
   skipping.

Permissions: `contents: read` only. No write scope, and no `id-token`.

**What this protects against, and what it doesn't.** PR data is still untrusted
input to dispelnet's YAML and TextFSM parsing, under a token that can read
dispel's source. The token is gone from the environment and from disk before
that input is parsed. Approval is still required, so an unknown fork can't make
the job run in a loop. A change to `ci.yml` or `checker.yml` in a PR takes
effect only once merged, since the base branch's workflow file is what runs.

**When dispel becomes public**, the token, both environments and the approval
go away, and the checker steps can move into `ci.yml`. The data-only copy stays,
since it costs nothing.

Pinning the checker is deliberate: what counts as a valid recipe is decided by
the checker, and a floating one would let a merge start failing, or passing,
because something outside this repository changed. Moving the pin is a reviewed
PR from a branch in this repository.

**Actions are pinned the same way.** Every `uses:` in every workflow names a
full commit SHA, with the version in a comment. `checker.yml` hands
`DISPEL_READ_TOKEN` to `actions/checkout`, and `release.yml` runs its actions
with `contents: write` and `id-token: write`, so a tag that can be moved would
put that trust in whoever can move it. `.github/dependabot.yml` proposes new
SHAs weekly as reviewed PRs, and `ci.yml` fails on any `uses:` that is not
pinned.

## Release (`release.yml`, on `v*` tags)

**Repository settings, before the first tag** (manual, listed in README):

- **Immutable Releases enabled** (Settings → Releases → Enable release
  immutability). It only applies to releases published afterwards, so it must be
  on before `v1.0.0`. Once a release is published, its tag cannot move and its
  assets cannot be changed or deleted, by anyone with write access included.
- A tag ruleset on `v*` blocking update and deletion. This covers the window
  before a release is published, and tags that never become one.
- Branch protection on `main` requiring CI.

**Gates** (fail closed, in order):

1. the tag contract;
2. all `ci.yml` and `checker.yml` checks on the tagged commit, with the tree
   read directly from the tag;
3. `bump --base <base> --head <tag> --require <tag>` refuses a tag below the
   computed minimum and prints why;
4. the compatibility check: build the checker pinned at the base and run
   `verify`; a refusal makes MAJOR the minimum.

**Assets:**

```text
recipes-X.Y.Z.tar.gz          git archive of the tag, prefix recipes-X.Y.Z/
                              contents: <family>/, probes/, LICENSE, NOTICE
SHA256SUMS                    sha256 of the tarball
SHA256SUMS.sigstore.json      cosign sign-blob bundle over SHA256SUMS
```

- The tarball has **one top-level directory**, because dispelnet's `strip()`
  always removes the first path segment. It is built with
  `git archive --prefix=recipes-X.Y.Z/ <tag> -- <paths> | gzip -n`, so its
  contents follow from the tag alone.
- LICENSE and NOTICE ship inside it (Apache-2.0 §4(d)); dispelnet's `extract`
  keeps only `.yaml` and `.captured`, so clients ignore them.

**Build and sign.** This runs after the gates and before any draft exists. Every
step is a hard gate: if one fails, the run stops with no release, draft or
upload.

1. **Tarball.** `git archive --prefix=recipes-X.Y.Z/ <tag> -- <paths> | gzip -n
   > recipes-X.Y.Z.tar.gz`. Then `tar -tzf` must show every entry under
   `recipes-X.Y.Z/`, and nothing but family directories, `probes/`, `LICENSE`
   and `NOTICE`.
2. **Checksums.** `sha256sum recipes-X.Y.Z.tar.gz > SHA256SUMS`.
3. **Sign.** Install cosign with `sigstore/cosign-installer`, pinned by commit
   SHA with a pinned cosign version. Then
   `cosign sign-blob --yes --bundle SHA256SUMS.sigstore.json SHA256SUMS`. The
   signing certificate comes from the job's OIDC token (`id-token: write`), and
   no key exists anywhere. A missing token fails here, not later as an unsigned
   release.
4. **Verify what was just signed**, exactly as a user would, but against this
   tag's exact identity rather than the README's pattern:
   `cosign verify-blob SHA256SUMS --bundle SHA256SUMS.sigstore.json
   --certificate-oidc-issuer https://token.actions.githubusercontent.com
   --certificate-identity https://github.com/dispelnet/recipes/.github/workflows/release.yml@refs/tags/<tag>`,
   then `sha256sum -c SHA256SUMS`.
5. **All three files exist and are non-empty.** Only then does publishing begin.

**Publishing: draft, then assets, then publish.** Immutability forbids adding
assets after publication, so:

1. If a published release already exists for the tag, fail. If a **draft**
   exists, from an earlier failed run, delete it; drafts are not immutable.
2. `gh release create <tag> --verify-tag --draft --notes-file notes.md`, with
   notes grouped as withdrawals, added, changed commands, fixes, taken from the
   bump report. Add `--prerelease` for a pre-release tag.
3. `gh release upload <tag>` the three assets, then check that the draft lists
   exactly those three names with the expected sizes.
4. `gh release edit <tag> --draft=false` with `--latest` only when the tag is
   stable and the highest stable version, `--latest=false` otherwise (a v1
   patch after v2, or any pre-release).
5. `gh release verify <tag>` must pass, proving the published release is
   immutable. Otherwise the run fails loudly: a release that published without
   immutability means the repository setting was off.

Publishing an immutable release makes GitHub generate a **release attestation**
covering the tag, commit SHA and assets. That replaces
`actions/attest-build-provenance`; the cosign bundle is kept because it names
the workflow that signed, which the attestation does not.

Permissions: `contents: write` and `id-token: write`, on the release job only.
The job runs in a `release` environment that holds `DISPEL_READ_TOKEN`, for the
checker at the tag's pin and at the base's pin. It checks out dispel the same
way `checker.yml` does. A tag can only be pushed by someone with write access,
so no approval is needed.

**Checking a release by hand** (documented in README):

```console
$ gh release verify vX.Y.Z -R dispelnet/recipes
$ gh release verify-asset vX.Y.Z recipes-X.Y.Z.tar.gz -R dispelnet/recipes
$ cosign verify-blob SHA256SUMS \
    --bundle SHA256SUMS.sigstore.json \
    --certificate-oidc-issuer https://token.actions.githubusercontent.com \
    --certificate-identity-regexp '^https://github\.com/dispelnet/recipes/\.github/workflows/release\.yml@refs/tags/v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'
$ sha256sum -c SHA256SUMS
```

## Bootstrap order

```text
1. push dispelnet/dispel             stays private; no release needed
2. this repo: settings above; environments internal, external-pr (reviewers),
   release, each holding DISPEL_READ_TOKEN; required check `dispelnet checks`;
   pin .dispelnet-version to that SHA; CI green
3. tag v1.0.0 (annotated)            first immutable, signed release, Latest
4. dispel: default source → dispelnet/recipes @ v1.0.0, tag v0.1.0-alpha.1
5. this repo: move the pin from SHA to v0.1.0-alpha.1 (a PR; bump: none)
```

Recipes never wait for alpha.1; alpha.1 needs recipes v1.0.0.

## Documentation

- `README.md`: what is here, fixing the stale `internal/probe/seed/` claim; how
  to add or fix support in five steps; how CI checks it; what provenance proves
  and what it only attests; how to verify a release; required repository
  settings.
- `VERSIONING.md`: the versioning section and tag contract above.
- `ADDING_SUPPORT.md`: moved from dispel's `docs/`, trimmed to the contributor
  path (question → dispel → NTC → Genie → capture → provenance → verify), with
  the capture-block format, `exact` versus `modified`, and the IMPORTED /
  VALIDATED / VERIFIED levels.

## Tooling

A Go module at `tools/` (`github.com/dispelnet/recipes/tools`), because CI
already has Go and the classifier must read YAML the way dispel does. It uses
`goccy/go-yaml` for loose reads of `run`, `command`, `oid`, `applies`, `fields`,
`tier`, `is`, `family` and `question`, and `golang.org/x/mod/semver` for tags. It
does not validate recipes; that is the checker's job.

- `cmd/provenance`: the rules above.
- `cmd/bump`: validates the tag contract, selects the base, diffs two git trees
  (`git ls-tree` / `git show`), classifies each changed path, applies
  `Withdraws:` trailers from the tag message, and prints `minimum: minor`
  followed by one line per reason. `--require <tag>` exits non-zero when the
  tag is below the minimum.
- Table tests cover every row of the versioning table, every tag-contract
  refusal, base selection with pre-releases and failed releases, and every
  provenance refusal. Upstream fetching sits behind an interface, so tests never
  touch the network. Fixtures are `.yml` or txtar.

## Failure modes

| Situation | Behaviour |
|---|---|
| Fork PR opened or updated | `ci.yml` runs at once; `dispelnet checks` stays pending until a maintainer approves `external-pr` |
| Fork PR changes `.dispelnet-version` | `dispelnet checks` fails: pin changes come from branches in this repository |
| PR has a symlink or non-regular file at a recipe or capture path | `dispelnet checks` fails before anything is parsed |
| Pinned dispel SHA not pushed, or token missing or expired | `dispelnet checks` fails at checkout, naming `.dispelnet-version` and `DISPEL_READ_TOKEN` |
| Upstream or licence file unreachable | Provenance fails for that capture; no fallback to `modified` |
| Tag malformed, lightweight, off main, or not a first `v1.0.0` | Release fails at the tag contract; nothing is selected or built |
| Tag bumps too little | Release fails before building assets; nothing is published |
| Old checker unbuildable at base | Release fails; a maintainer fixes the pin rather than skipping compatibility |
| Tarball contents wrong, signing, or self-verification fails | Release fails before a draft is created; nothing is uploaded or published |
| Run fails after the draft is created | Draft remains; a re-run deletes and recreates it |
| Re-run of a published tag | Refused at step 1 of publishing; immutability also blocks asset changes |
| Published but `gh release verify` fails | Run fails loudly: immutability was not enabled |

## Known follow-ups (not in this work)

- dispel: fetch the release asset rather than codeload's generated tarball.
  Generated source archives are not covered by `gh release verify-asset` or by
  the signature, so today's fetch path gets none of this work's guarantees.
- dispel: resolve the newest signed `v<major>.x.y` release, verify the bundle
  and checksums, and record the version as the ref.
- dispel: remove or implement the "body must hash to the name it was fetched
  under" claim in `docs/recipes/registry.md`; no such check exists.
- dispel: move the three corpus tests into `recipes verify` before deleting
  `dispel/recipes/`.
