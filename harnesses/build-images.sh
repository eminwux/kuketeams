#!/usr/bin/env bash
# Shared local/CI entrypoint. MODE=check never exports images or credentials;
# MODE=load loads native images into Docker; MODE=push publishes manifests.
set -euo pipefail
cd "$(dirname "$0")/.."
export REGISTRY="${REGISTRY:-ghcr.io/eminwux/kuketeams}"
export TAG="${TAG:-latest}"
native_arch=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
export PLATFORMS="${PLATFORMS:-linux/$native_arch}"
mode="${MODE:-load}"
args=(--file harnesses/docker-bake.hcl)
case "$mode" in
  check) ;;
  load)
    # Docker's classic image store accepts one platform per load.
    if [[ "$PLATFORMS" == *,* ]]; then
      echo 'MODE=load needs one PLATFORMS value; use MODE=check for multi-arch validation.' >&2
      exit 2
    fi
    args+=(--set '*.output=type=docker')
    ;;
  push) args+=(--set '*.output=type=registry') ;;
  *) echo 'MODE must be check, load, or push' >&2; exit 2 ;;
esac
if [[ "${1:-}" == --print ]]; then
  args+=(--print)
  shift
fi
exec docker buildx bake "${args[@]}" "$@"
