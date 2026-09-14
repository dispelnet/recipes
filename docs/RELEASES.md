# Releases

What a release of this repository contains, why it is shaped the way it is,
and how to check one. Version numbers and how to cut a release are in
[VERSIONING.md](VERSIONING.md).

## Assets

Every release is an immutable GitHub release with one asset,
`recipes-X.Y.Z.tar.gz`: the recipe tree `dispelnet` installs. It was checked
with the newest dispelnet release at the time, which the release job's summary
names.

Use this asset, not the "Source code" archives GitHub adds to every release.
Those are generated on request, contain the whole repository rather than the
recipe tree, and are not covered by GitHub's release attestation.

## The tarball

```text
recipes-X.Y.Z/
├── LICENSE
├── NOTICE
├── _probes/
│   └── <family>.yaml          identification probe
├── _questions/
│   └── <question>.json        the fields a question's rows carry
└── <family>/
    └── <question>.yaml        recipe
```

This is the repository's `recipes/` directory at the tagged commit without its
`.captured` files, plus `LICENSE` and `NOTICE`, under one top-level directory.
Nothing else is in it: `make check` runs the layout check on the tagged commit
before the tarball is built, and it refuses anything under `recipes/` that is
not a recipe, a probe, a capture or a question schema.

Why each part is there, or isn't:

- **One top-level directory.** `dispelnet` strips exactly one leading path
  segment from every entry, and refuses entries that would land outside the
  directory it unpacks into. The directory is named for the version, so an
  unpacked release says what it is.
- **Only what `dispelnet` runs.** `dispelnet` parses every `.yaml` it is given
  and refuses fields it does not know, so a workflow, a tool fixture or anything
  else from the repository would break every client. Docs and `.github/`
  stay out.
- **No captures.** A capture is the evidence a recipe is checked against, and
  checking is this repository's job: every pull request and every release runs
  `dispelnet recipes verify` against them, and a recipe that cannot read its
  capture is never released. Clients only run recipes, so the captures would be
  most of the download and nothing would read them. They stay in the
  repository, and a release's captures are at its tag.
- **`LICENSE` and `NOTICE`.** Recipes may adapt parsing from the Apache-2.0
  projects listed in `NOTICE`, and Apache-2.0 §4 requires redistributions to
  carry the licence and the notices. `dispelnet` ignores both files.
- **Built from the tag alone, reproducibly.** The files come from `git archive`
  of the tagged commit, and are packed in name order with the commit's time, no
  owner, and `gzip -n`, so the same tag always builds the same bytes. Nothing
  from the runner's working directory can end up in it.
- **Checked as shipped.** Before anything is published, the release unpacks the
  tarball, lays the tag's captures beside it, and runs `dispelnet recipes verify`
  on the result, so the layout a client receives is the layout that was
  checked.

## Verifying a release

With [GitHub CLI](https://cli.github.com):

```console
$ gh release download vX.Y.Z -R dispelnet/recipes
$ gh release verify vX.Y.Z -R dispelnet/recipes
$ gh release verify-asset vX.Y.Z recipes-X.Y.Z.tar.gz -R dispelnet/recipes
```

- `gh release verify` checks GitHub's attestation that the release is
  immutable: its tag and assets cannot have changed since publishing.
- `gh release verify-asset` checks that the downloaded file is the one that
  release published.
