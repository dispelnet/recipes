#!/usr/bin/env bash
# Runs dispel's own corpus tests against a collected recipe tree.
#
#   corpus-tests.sh DISPEL_CHECKOUT TREE
#
# `dispelnet recipes verify` does not check three things dispel's
# recipes/recipes_test.go does: that a recipe reads as many rows as its capture
# counts, that duplicated transcripts stay byte-identical, and that nothing
# published carries `quarantine:`. That test imports dispel's internal packages,
# which another module cannot, so it runs inside the dispel checkout with this
# tree in place of dispel's own recipes/.
#
# The only Go code that runs is dispel's, at the pinned commit: TREE holds data
# files alone (see collect-tree.sh).
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 DISPEL_CHECKOUT TREE" >&2
  exit 2
fi

dispel=${1%/}
tree=${2%/}

if [[ ! -f $dispel/recipes/recipes_test.go ]]; then
  echo "::error::the pinned dispel has no recipes/recipes_test.go; the corpus checks must move into \`recipes verify\` before this pin"
  exit 1
fi

find "$dispel/recipes" -mindepth 1 -maxdepth 1 ! -name recipes_test.go -exec rm -rf -- {} +
cp -R -- "$tree"/. "$dispel/recipes/"

go -C "$dispel" test ./recipes/
