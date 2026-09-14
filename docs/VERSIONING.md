# Versioning

Releases of this repository follow [SemVer 2.0](https://semver.org). The
consumer is the `dispelnet` binary and whatever reads the facts it records, so
a version number describes what changes for them, not how much work went in.

A maintainer cuts a release by pushing an annotated tag. CI computes the minimum
bump the changes require and refuses a tag below it. On pull requests the same
computation is reported as a comment or job summary; there it is advisory and
never blocks a merge.

## What each bump means

| Bump | Meaning | Changes |
|---|---|---|
| MAJOR | A client of the previous release could refuse the tree or silently lose something | the tree is refused by the checker pinned at the base release (a new field, `uses:` reader or assertion kind) · a recipe or probe file removed · `family` or `question` renamed · a `fields:` key removed · an `applies` pattern removed |
| MINOR | Adds coverage, or changes what is sent to a device | recipe or probe added (a new family included) · anything that decides what reaches a device changed: a recipe's `command:`, or a step's `run:`, `uses:`, `with:`, `for-each:` or `limit:`; a probe's `transport:`, `command:`, `oid:` or `read:` (compared exactly, whitespace included) · an `applies` pattern added · a `fields:` key added · a probe's `tier:` or `is:` changed |
| PATCH | Same commands, same output shape | any other change to a recipe or probe (template, `select`, `key`, field mapping source, `verified:`, comments) · a `.captured` added or changed · `LICENSE` or `NOTICE` changed (both ship in the tarball) |
| none | Nothing a release ships | `README.md`, docs, `tools/`, `.github/`, `.dispelnet-version` (its effect is caught by the compatibility check) |

Rules on top of the table:

- **The highest bump among all changes wins.** One removed `fields:` key makes a
  release MAJOR, however many template fixes come with it.
- **A changed command is never a patch**, even when it fixes something. A release
  that changes what runs on production equipment must say so, and a patch number
  says nothing changed there.
- **A safety withdrawal is a patch.** Removing a recipe that harms a device must
  not wait for a major. The tag message declares it with one trailer per file:

  ```text
  Withdraws: <path> — <reason>
  ```

  The trailer lowers that file's removal from MAJOR to PATCH, and the release
  notes lead with it. A removal without a trailer is still MAJOR.
- **Backwards compatible is tested, not judged.** The release workflow builds the
  checker pinned in `.dispelnet-version` *at the base release* and runs
  `dispelnet recipes verify` on the new tree. If that older checker refuses the
  tree, the release is MAJOR, whatever the table above would otherwise say. This
  is what catches a new field or reader that an older client cannot parse.

## Tag contract

A `v*` tag is a release request only if all of these hold. Otherwise the release
workflow fails at its first step, before bump selection, the compatibility check
or building, and publishes nothing.

1. **Strict SemVer 2.0 with a `v` prefix.** The tag must pass `semver.IsValid`
   and equal `semver.Canonical(tag)` (`golang.org/x/mod/semver`), which refuses
   the `v1` and `v1.2` shorthands. It must carry no build metadata (`+…`):
   SemVer ignores build metadata for precedence, so two such tags could not be
   ordered.
2. **Annotated.** `git cat-file -t refs/tags/<tag>` must print `tag`. A
   lightweight tag is refused, because the tag message carries the `Withdraws:`
   trailers and the tagger identity.
3. **On main.** `git merge-base --is-ancestor <tag> origin/main` must succeed.
4. **The first release is `v1.0.0`**, or a `v1.0.0-…` pre-release. Clients will
   pin a major, and `v0` would mean nothing to them.

### Base selection

The base is what a new tag is compared against, for both the bump and the
compatibility check.

- Candidates are tags that meet rules 1 and 2 **and have a published release**.
  A tag whose release failed, or never ran, is not a base.
- **Pre-releases are never a base.** The base is the highest stable release whose
  version is lower than the new tag. `v1.1.0-rc.1`, `v1.1.0-rc.2` and `v1.1.0`
  are all compared against, say, `v1.0.3`, so the final release is held to
  everything since the last stable one rather than the last rc.
- No candidate means a first release, and rule 4 applies.

Pre-releases follow the same bump rules, are published as pre-releases, and are
never GitHub's Latest release. Latest goes only to the highest stable version, so
a `v1` patch published after `v2.0.0` does not take it.

## Cutting a release

Check the bump report on the last merged pull request, or compute it locally
against the base release:

```console
$ go -C tools run ./cmd/bump --base v1.1.0 --head HEAD
```

Then tag `main` and push the tag:

```console
$ git switch main && git pull
$ git tag -a v1.2.0 -m "v1.2.0"
$ git push origin v1.2.0
```

A release that withdraws a recipe puts the trailer in the last paragraph of the
tag message. A second `-m` starts a new paragraph:

```console
$ git tag -a v1.2.1 -m "v1.2.1: withdraw a recipe" \
    -m "Withdraws: recipes/cisco_ios/example.yaml — <what it does to the device>"
$ git push origin v1.2.1
```

Use one `Withdraws:` line per file withdrawn.

If the tag bumps too little, the release workflow says which changes require
more and publishes nothing. A published release cannot be changed or replaced,
so fix a mistake with a new version rather than by moving a tag.
