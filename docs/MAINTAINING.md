# Maintaining

Repository settings the workflows depend on, and how this repository is coupled
to [dispelnet/dispelnet](https://github.com/dispelnet/dispelnet). None of this
is in the repository itself, so it is written down here. Contributors don't need
any of it.

## Repository settings

Set these before the first tag. Release immutability in particular only applies
to releases published after it is turned on.

### Releases and tags

- **Release immutability** on (Settings → General → Releases). A published
  release's tag and assets can then not be changed, by anyone. `release.yml`
  ends with `gh release verify` and fails if this is off.
- **A tag ruleset** for `refs/tags/v*` that blocks updates and deletions.

### Branch protection

A ruleset on `main` requiring a pull request and the `Checks` status check from
`ci.yml`.

### Actions

- Default workflow permissions: read repository contents only. Each workflow
  asks for what it needs.
- Fork pull request workflows require approval for outside contributors.
- Dependabot version updates on: `.github/dependabot.yml` keeps the pinned
  action SHAs current.

## Coupling with dispelnet

- **The checker is the newest dispelnet release**, pre-releases included.
  `.github/scripts/fetch-dispelnet.sh` downloads it, checks it against the
  release's `checksums.txt` and build provenance attestation, and names it in
  the job summary. Nothing is built from source. A new dispelnet release can
  therefore turn this repository's checks red without a change here; fix
  whichever side is wrong. CI fails until dispelnet has a release.
- **Captures are checked only when they are passed.** `dispelnet recipes verify`
  takes captures as positional arguments: given none, it parses the recipes,
  reads no device output and still exits 0. `make verify-tree` therefore passes
  the tree twice, and is the only place that spells the command out — the
  release workflow calls it for the unpacked tarball too.
- **Labelled captures** (`<name>.<label>.captured`) are checked only by a
  dispelnet that reads labelled captures; an older one ignores them.

## First release

1. Release dispelnet, reading probes from `_probes/` and checking labelled
   captures.
2. Apply the settings above.
3. Make sure CI on `main` is green.
4. Tag `v1.0.0` (annotated) and push it, as described in
   [VERSIONING.md](VERSIONING.md#cutting-a-release).
