#!/usr/bin/env bash
# Keep the harness image catalog in lockstep with a release tag.
#
# Every leaf `image:` ref in images.yaml must read `<REGISTRY>/<ref>:<tag>`
# where <tag> is the git release tag CI publishes the images under. Run this
# before tagging a release so the catalog, the published image tag, and the git
# tag all match:
#
#   ./release-images.sh v0.6.0          # rewrite images.yaml in place
#   ./release-images.sh v0.6.0 --check  # verify only (no write); fail on drift
#
# CI runs the --check form on every release-tag push, so a tag whose catalog
# refs don't already match the tag fails before any image is published. The
# rewrite and the check share one render path, so they cannot drift.
#
# REGISTRY (env, default `ghcr.io/eminwux/kuketeams`) overrides the published
# registry/namespace prefix the refs are pinned to.
set -euo pipefail
cd "$(dirname "$0")"

REGISTRY="${REGISTRY:-ghcr.io/eminwux/kuketeams}"
CATALOG="images.yaml"

tag="${1:-}"
mode="write"
case "${2:-}" in
  --check) mode="check" ;;
  "")      mode="write" ;;
  *)       echo "usage: $0 <tag> [--check]" >&2; exit 2 ;;
esac
if [ "$#" -gt 2 ] || [[ ! "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9][a-zA-Z0-9.-]*)?$ ]]; then
  echo "usage: $0 <tag> [--check]" >&2
  exit 2
fi

# Rewrite each leaf entry's `image:` value to <REGISTRY>/<ref>:<tag>, pairing
# each `image:` with the `ref:` that opens its list item and preserving the
# original indentation + column alignment of the `image:` line.
render() {
  awk -v reg="$REGISTRY" -v tag="$tag" '
    /(^|[[:space:]])ref:[[:space:]]/ {
      line = $0
      sub(/^.*ref:[[:space:]]*/, "", line)
      sub(/[[:space:]].*$/, "", line)
      ref = line
    }
    /^[[:space:]]*image:([[:space:]]|$)/ {
      match($0, /^[[:space:]]*image:[[:space:]]*/)
      prefix = substr($0, RSTART, RLENGTH)
      printf "%s%s/%s:%s\n", prefix, reg, ref, tag
      next
    }
    { print }
  ' "$CATALOG"
}

if [ "$mode" = "check" ]; then
  if ! diff -u "$CATALOG" <(render); then
    echo "" >&2
    echo "FATAL  $CATALOG is not in lockstep with tag '$tag'." >&2
    echo "       Run: REGISTRY=$REGISTRY ./release-images.sh $tag" >&2
    echo "       then commit the catalog before tagging the release." >&2
    exit 1
  fi
  echo "ok: $CATALOG refs are pinned to $REGISTRY/<image>:$tag"
  exit 0
fi

rendered="$(render)"
printf '%s\n' "$rendered" > "$CATALOG"
echo "pinned $CATALOG refs to $REGISTRY/<image>:$tag"
