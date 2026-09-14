#!/usr/bin/env bash
# Downloads a released dispelnet binary for this machine and checks it.
#
#   fetch-dispelnet.sh OUTPUT [VERSION]
#
# VERSION is a dispelnet release tag such as v0.1.0; without one, the newest
# release, pre-releases included. The binary must match the release's
# checksums.txt and carry its build provenance attestation. Needs the gh CLI,
# authenticated (GH_TOKEN in CI).
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
  echo "usage: $0 OUTPUT [VERSION]" >&2
  exit 2
fi

repo=dispelnet/dispelnet
output=$1
version=${2:-}

if [[ -z $version ]]; then
  version="$(gh release list -R "$repo" --exclude-drafts --limit 1 --json tagName --jq '.[0].tagName // empty')"
  if [[ -z $version ]]; then
    echo "::error::$repo has no releases to check recipes with"
    exit 1
  fi
fi

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) echo "::error::no dispelnet release for $(uname -m)"; exit 1 ;;
esac

asset="dispelnet_${version#v}_${os}_${arch}"
download="$(mktemp -d)"
trap 'rm -rf "$download"' EXIT

gh release download "$version" -R "$repo" -p "$asset" -p checksums.txt -D "$download"

sum=(sha256sum)
command -v sha256sum > /dev/null || sum=(shasum -a 256)
(cd "$download" && grep -E "[[:space:]]$asset\$" checksums.txt | "${sum[@]}" -c -)

gh attestation verify "$download/$asset" -R "$repo"

mkdir -p "$(dirname "$output")"
install -m 0755 "$download/$asset" "$output"
"$output" --version

if [[ -n ${GITHUB_STEP_SUMMARY:-} ]]; then
  echo "dispelnet checker: \`$repo@$version\`" >> "$GITHUB_STEP_SUMMARY"
fi
