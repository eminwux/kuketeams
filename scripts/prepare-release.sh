#!/usr/bin/env bash
set -euo pipefail

# Prepare downloaded release binary artifacts; publishing is a separate workflow step.
downloads=$1
output=$2
tag=$3
sha=$4
[[ "$sha" =~ ^[0-9a-f]{40}$ ]]
bash harnesses/release-images.sh "$tag" --check
mkdir -p "$output/assets"
test -z "$(find "$output/assets" -mindepth 1 -print -quit)"

for arch in amd64 arm64; do
  binary="kuketeams-linux-$arch"
  directory="$downloads/$binary-${sha:0:12}"
  bash scripts/verify-artifact.sh "$directory" "$arch" "$sha" "$tag"
  cp "$directory/$binary" "$directory/$binary.sha256" "$output/assets/"
  cp "$directory/build-info.json" "$output/assets/$binary.build-info.json"
done

{
  printf '## Release summary\n\n'
  printf 'kuketeams `%s` from commit `%s`.\n\n' "$tag" "$sha"
  printf '### CLI artifacts\n\n'
  printf 'Linux amd64 and arm64 binaries, each with a `.sha256` checksum and\n'
  printf 'an architecture-specific `.build-info.json` recording source and toolchain.\n'
  printf 'Verify with `sha256sum --check kuketeams-linux-<arch>.sha256`.\n'
  printf 'The amd64 binary is smoke-tested; arm64 is cross-built and structurally verified.\n\n'
  printf '### Harness images\n\n'
  printf 'Published for `linux/amd64` and `linux/arm64`:\n\n'
  awk '/^[[:space:]]*image:/ { printf "- `%s`\n", $2 }' harnesses/images.yaml
  printf '\nBuild intermediates `base`, `base-user`, and `base-root` are published in the same registry namespace and tag.\n'
} > "$output/notes.md"
