#!/usr/bin/env bash
# Fails if a workflow uses an action by anything but a full commit SHA.
#
# release.yml publishes with these actions, so a movable tag is not an
# acceptable reference. Dependabot proposes new SHAs.
set -euo pipefail

unpinned="$(grep -nE '^[[:space:]]*(-[[:space:]]+)?uses:' .github/workflows/*.yml .github/workflows/*.yaml 2>/dev/null \
  | grep -vE 'uses:[[:space:]]+[^@[:space:]]+@[0-9a-f]{40}([[:space:]]|$)' || true)"

if [[ -n $unpinned ]]; then
  echo "::error::every action must be pinned to a full commit SHA, with its version in a comment"
  echo "$unpinned"
  exit 1
fi
