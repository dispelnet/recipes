#!/usr/bin/env bash
# Builds the dispelnet checker from a dispel checkout.
#
#   build-checker.sh DISPEL_CHECKOUT OUTPUT
#
# The command directory was renamed from cmd/dispel to cmd/dispelnet. A release
# checks compatibility with the checker pinned at its base release, which may
# predate the rename, so the build takes whichever the pinned commit has rather
# than assuming the current name.
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 DISPEL_CHECKOUT OUTPUT" >&2
  exit 2
fi

dispel=${1%/}
output=$2

for command in cmd/dispelnet cmd/dispel; do
  if [[ -d $dispel/$command ]]; then
    echo "building $command from $(git -C "$dispel" rev-parse HEAD)"
    go -C "$dispel" build -o "$output" "./$command"
    "$output" --version || true
    exit 0
  fi
done

echo "::error::neither cmd/dispelnet nor cmd/dispel exists at the pinned dispel commit"
exit 1
