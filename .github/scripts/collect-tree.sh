#!/usr/bin/env bash
# Copies the recipe data out of a checkout, and nothing else.
#
#   collect-tree.sh SOURCE DESTINATION
#
# In checker.yml, SOURCE is a pull request's checkout, and the job holding a
# token to the private dispel repository runs dispelnet over what this copies.
# So only regular files at recipes/<family>/*.yaml,
# recipes/<family>/*.captured, recipes/_probes/*.yaml and
# recipes/_probes/*.captured cross over. A symlink there could point dispelnet at a
# file outside the tree, and one whose content ends up in an error message would
# print it, so any non-regular file at those paths fails the job instead of
# being skipped.
#
# This script is always run from a trusted checkout (the base branch or a
# release tag), never from the pull request it is reading.
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 SOURCE DESTINATION" >&2
  exit 2
fi

source_dir=${1%/}
destination=$2
recipes_dir="$source_dir/recipes"
status=0

is_family() {
  [[ $1 == _probes || $1 =~ ^[a-z][a-z0-9_]*$ ]]
}

refuse() {
  echo "::error file=$1::$1 $2"
  status=1
}

mkdir -p "$destination"

if [[ ! -d $recipes_dir || -L $recipes_dir ]]; then
  refuse "recipes" "must be a real directory"
  exit "$status"
fi

# A family folder that is itself a symlink would be silently skipped by the
# walk below rather than refused.
while IFS= read -r -d '' entry; do
  name=${entry#"$recipes_dir"/}
  if is_family "$name"; then
    refuse "$name" "is a symlink; a family folder must be a real directory"
  fi
done < <(find "$recipes_dir" -mindepth 1 -maxdepth 1 -type l -print0)

copied=0

while IFS= read -r -d '' entry; do
  relative=${entry#"$recipes_dir"/}
  family=${relative%%/*}

  is_family "$family" || continue

  if [[ -L $entry || ! -f $entry ]]; then
    refuse "$relative" "is not a regular file; recipes, probes and captures must be"
    continue
  fi

  target=$family
  [[ $family == _probes ]] && target=probes
  mkdir -p "$destination/$target"
  cp -- "$entry" "$destination/$target/${relative#*/}"
  copied=$((copied + 1))
done < <(find "$recipes_dir" -mindepth 2 -maxdepth 2 \
  -not -path "$source_dir/.*" \
  \( -name '*.yaml' -o -name '*.captured' \) -print0)

if [[ $copied -eq 0 ]]; then
  echo "::error::no recipe data found under $source_dir"
  status=1
fi

echo "copied $copied recipe and capture files into $destination"
exit "$status"
