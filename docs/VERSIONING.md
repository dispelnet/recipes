# Versioning

Releases of this repository follow [SemVer 2.0](https://semver.org). The
consumer is the `dispelnet` binary and whatever reads the facts it records, so
a version number describes what changes for them, not how much work went in.

A maintainer picks the version and cuts the release by pushing an annotated
tag. Nothing computes the bump: the table below is the rule the maintainer
applies.

## What each bump means

| Bump | Meaning | Changes |
|---|---|---|
| MAJOR | A client of the previous release could silently lose something | Recipe or probe file removed; `family` or `question` renamed<br>`fields:` key or `applies` pattern removed<br>Question schema or one of its fields removed<br>Question-schema field type changed, or field made required |
| MINOR | Adds coverage, or changes what is sent to a device | Recipe or probe added (a new family included); question schema added<br>Anything that decides what reaches a device changed: a step's `run:`, `uses:`, `with:`, `for-each:`, or `limit:`; a probe's `transport:`, `command:`, `oid:`, or `read:` (compared exactly, whitespace included)<br>`applies` pattern or `fields:` key added; probe's `tier:` or `is:` changed |
| PATCH | Same commands, same output shape | Other recipe or probe change (template, `select`, `key`, field mapping source, comments)<br>Question-schema description or new optional field<br>`LICENSE` or `NOTICE` changed (all ship in the tarball) |
| none | Nothing a release ships | `.captured` files (checked in CI, not shipped)<br>`README.md`, `Makefile`, docs, `.github/` |

Rules on top of the table:

- **The highest bump among all changes wins.** One removed `fields:` key makes a
  release MAJOR, however many template fixes come with it.
- **A changed command is never a patch**, even when it fixes something. A release
  that changes what runs on production equipment must say so, and a patch number
  says nothing changed there.

## Tag contract

The release workflow refuses a `v*` tag, and publishes nothing, unless it is:

1. **Strict SemVer 2.0 with a `v` prefix**, such as `v1.2.3` or `v1.3.0-rc.1`.
   No shorthand (`v1`, `v1.2`), no leading zeros and no build metadata (`+…`),
   which SemVer cannot order.
2. **Annotated** (`git tag -a`). A lightweight tag records neither who tagged it
   nor when.
3. **On main.**

A pre-release is published as a pre-release and is never GitHub's Latest
release. Latest goes only to the highest stable version, so a `v1` patch
published after `v2.0.0` does not take it.

## Cutting a release

Review what changed since the last stable release, ignoring captures, and pick
the bump from the table:

```console
$ git switch main && git pull
$ git diff --stat v1.1.0 -- recipes ':!*.captured'
```

Then tag and push:

```console
$ git tag -a v1.2.0 -m "v1.2.0"
$ git push origin v1.2.0
```

A published release cannot be changed or replaced, so fix a mistake with a new
version rather than by moving a tag.
