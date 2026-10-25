#!/usr/bin/env bash
set -euo pipefail
export LC_ALL=C

directory=$(cd "$1" && pwd)
arch=$2
expected_sha=$3
expected_version=$4
binary="kuketeams-linux-$arch"
case "$arch" in
  amd64) machine='Advanced Micro Devices X86-64' ;;
  arm64) machine='AArch64' ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

for artifact_file in "$binary" "$binary.sha256" build-info.json; do
  test -s "$directory/$artifact_file"
done
# Require a relative, single-file manifest before checking its contents.
[[ $(cat "$directory/$binary.sha256") =~ ^[0-9a-f]{64}\ \ $binary$ ]]
(cd "$directory" && sha256sum --check "$binary.sha256")
jq -e --arg sha "$expected_sha" --arg arch "$arch" --arg version "$expected_version" \
  '.sha == $sha and .os == "linux" and .arch == $arch and .version == $version
   and (.go_version | test("^go[0-9]+\\.[0-9]+"))' "$directory/build-info.json"

header=$(readelf -h "$directory/$binary")
grep -Eq 'Class: +ELF64' <<< "$header"
grep -Fq "$machine" <<< "$header"
# Embedded Go settings verify Linux and a self-contained, CGO-free build.
settings=$("${GO:-go}" version -m "$directory/$binary")
grep -Fq 'GOOS=linux' <<< "$settings"
grep -Fq "GOARCH=$arch" <<< "$settings"
grep -Fq 'CGO_ENABLED=0' <<< "$settings"

if [[ $(uname -s) == Linux && $(uname -m) == x86_64 && "$arch" == amd64 ]]; then
  temporary=$(mktemp -d)
  trap 'rm -rf "$temporary"' EXIT
  mkdir "$temporary/home"
  # Downloads may lose executable mode. Do not smoke-test bootstrap commands.
  chmod +x "$directory/$binary"
  env -i HOME="$temporary/home" "$directory/$binary" --help > "$temporary/help"
  grep -Fq 'Compose agent teams with kuketeams' "$temporary/help"
  env -i HOME="$temporary/home" "$directory/$binary" version > "$temporary/version"
  printf 'kuketeams %s\n' "$expected_version" > "$temporary/expected-version"
  cmp "$temporary/expected-version" "$temporary/version"
  test -z "$(find "$temporary/home" -mindepth 1 -print -quit)"
  echo 'amd64: help/version passed with an unchanged isolated home'
else
  echo "$arch: cross-built and structurally checked; not runtime-tested"
fi
