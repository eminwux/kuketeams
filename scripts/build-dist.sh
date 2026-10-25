#!/usr/bin/env bash
set -euo pipefail

# Called by make dist from the repository root. Keep target settings out of tests.
[[ "$GOOS" == linux && "$GOARCH" =~ ^(amd64|arm64)$ ]]
build_sha=$(git rev-parse HEAD)
output="bin/dist/$GOOS-$GOARCH"
binary="kuketeams-$GOOS-$GOARCH"
mkdir -p "$output"

CGO_ENABLED=0 make kuketeams GO="$GO" GOOS="$GOOS" GOARCH="$GOARCH" \
  VERSION="$VERSION" BINARY="$output/$binary"
jq -n --arg sha "$build_sha" --arg os "$GOOS" --arg arch "$GOARCH" \
  --arg go_version "$("$GO" env GOVERSION)" --arg version "$VERSION" \
  '{sha: $sha, os: $os, arch: $arch, go_version: $go_version, version: $version}' \
  > "$output/build-info.json"
(cd "$output" && sha256sum "$binary" > "$binary.sha256")
GO="$GO" bash scripts/verify-artifact.sh "$output" "$GOARCH" "$build_sha" "$VERSION"
