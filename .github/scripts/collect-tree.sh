#!/usr/bin/env bash
# Checks the layout of recipes/ and copies it to DESTINATION unchanged, which is
# the layout a release ships and dispelnet reads, probes included (_probes/).
#
#   collect-tree.sh SOURCE DESTINATION
#
# Only regular files belong under recipes/: <family>/<name>.yaml with
# <name>.captured beside it, and labelled captures <name>.<label>.captured. The
# release tarball is recipes/ without its captures, so anything else would ship
# unchecked, and a symlink could make dispelnet read a file outside the tree.
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "usage: $0 SOURCE DESTINATION" >&2
  exit 2
fi

recipes=${1%/}/recipes
destination=$2
status=0
checked=0

refuse() {
  echo "::error file=recipes/$1::recipes/$1 $2"
  status=1
}

if [[ ! -d $recipes || -L $recipes ]]; then
  echo "::error::$recipes is not a directory"
  exit 1
fi

family='(_probes|[a-z][a-z0-9_]*)'
file="^$family/([^/.]+)(\.[a-z0-9][a-z0-9-]*)?\.(yaml|captured)$"

while IFS= read -r -d '' entry; do
  relative=${entry#"$recipes"/}

  if [[ -L $entry ]]; then
    refuse "$relative" "is a symlink"
    continue
  fi

  if [[ $relative == probes || $relative == probes/* ]]; then
    refuse "$relative" "uses the reserved name probes; probes live in recipes/_probes/"
    continue
  fi

  # Question schemas: the field types one question's rows carry. dispelnet's
  # recipe loader passes over them, because it reads .yaml, and a release ships
  # them beside the recipes.
  if [[ $relative == _questions || $relative == _questions/* ]]; then
    if [[ -f $entry ]]; then
      if [[ $relative =~ ^_questions/[a-z][a-z0-9-]*\.json$ ]]; then
        checked=$((checked + 1))
      else
        refuse "$relative" "is not a question schema: _questions holds <question>.json"
      fi
    elif [[ $relative != _questions ]]; then
      refuse "$relative" "is not a question schema: _questions holds <question>.json"
    fi

    continue
  fi

  if [[ -d $entry ]]; then
    [[ $relative =~ ^$family$ ]] || refuse "$relative" "is not a family folder"
    continue
  fi

  if [[ ! -f $entry || ! $relative =~ $file ]]; then
    refuse "$relative" "is not recipe data: only <family>/<name>.yaml and its .captured files belong here"
    continue
  fi

  folder=${BASH_REMATCH[1]} name=${BASH_REMATCH[2]} label=${BASH_REMATCH[3]} kind=${BASH_REMATCH[4]}

  if [[ $kind == yaml && -n $label ]]; then
    refuse "$relative" "has a dot in its name, which would read as a capture label"
    continue
  fi

  if [[ $kind == yaml && ! -f $recipes/$folder/$name.captured ]]; then
    refuse "$relative" "has no $folder/$name.captured beside it"
  fi

  if [[ $kind == captured && ! -f $recipes/$folder/$name.yaml ]]; then
    refuse "$relative" "is evidence for nothing: no $folder/$name.yaml beside it"
  fi

  checked=$((checked + 1))
done < <(find "$recipes" -mindepth 1 -print0)

if [[ $checked -eq 0 ]]; then
  echo "::error::no recipe data found under $recipes"
  status=1
fi

# Copied whole, and only once the layout is known good: a refused tree leaves
# nothing half-built for the checker to run on.
if [[ $status -ne 0 ]]; then
  exit "$status"
fi

cp -R -- "$recipes"/. "$destination"/

echo "checked and copied $checked recipe, capture and schema files into $destination"
