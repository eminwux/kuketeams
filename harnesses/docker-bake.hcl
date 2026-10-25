// All builds use these static Dockerfiles. Linked contexts keep base results
// inside BuildKit: validation never needs to pull or publish kuketeams bases.
variable "REGISTRY" {
  default = "ghcr.io/eminwux/kuketeams"
}
variable "TAG" {
  default = "latest"
}
variable "PLATFORMS" {
  default = "linux/amd64,linux/arm64"
}

group "default" {
  targets = ["base", "base-user", "base-root", "claude", "claude-root", "claude-android", "codex", "codex-root", "opencode", "opencode-root"]
}

target "_common" {
  args = { REGISTRY = REGISTRY, TAG = TAG }
  platforms = split(",", PLATFORMS)
  output = ["type=cacheonly"]
}

target "base" {
  inherits = ["_common"]
  context = "harnesses/base"
  dockerfile = "Dockerfile"
  tags = ["${REGISTRY}/base:${TAG}"]
}

target "base-user" {
  inherits = ["_common"]
  context = "harnesses/base"
  dockerfile = "Dockerfile.user"
  contexts = { "${REGISTRY}/base:${TAG}" = "target:base" }
  tags = ["${REGISTRY}/base-user:${TAG}"]
}

target "base-root" {
  inherits = ["_common"]
  context = "harnesses/base"
  dockerfile = "Dockerfile.root"
  contexts = { "${REGISTRY}/base:${TAG}" = "target:base" }
  tags = ["${REGISTRY}/base-root:${TAG}"]
}

target "_user" {
  inherits = ["_common"]
  contexts = { "${REGISTRY}/base-user:${TAG}" = "target:base-user" }
}

target "_root" {
  inherits = ["_common"]
  contexts = { "${REGISTRY}/base-root:${TAG}" = "target:base-root" }
}

target "claude" {
  inherits = ["_user"]
  context = "harnesses/claude"
  dockerfile = "Dockerfile"
  tags = ["${REGISTRY}/claude:${TAG}"]
}

target "claude-root" {
  inherits = ["_root"]
  context = "harnesses/claude"
  dockerfile = "Dockerfile.root"
  tags = ["${REGISTRY}/claude-root:${TAG}"]
}

target "claude-android" {
  inherits = ["_user"]
  context = "harnesses/claude"
  dockerfile = "Dockerfile.android"
  tags = ["${REGISTRY}/claude-android:${TAG}"]
}

target "codex" {
  inherits = ["_user"]
  context = "harnesses/codex"
  dockerfile = "Dockerfile"
  tags = ["${REGISTRY}/codex:${TAG}"]
}

target "codex-root" {
  inherits = ["_root"]
  context = "harnesses/codex"
  dockerfile = "Dockerfile.root"
  tags = ["${REGISTRY}/codex-root:${TAG}"]
}

target "opencode" {
  inherits = ["_user"]
  context = "harnesses/opencode"
  dockerfile = "Dockerfile"
  tags = ["${REGISTRY}/opencode:${TAG}"]
}

target "opencode-root" {
  inherits = ["_root"]
  context = "harnesses/opencode"
  dockerfile = "Dockerfile.root"
  tags = ["${REGISTRY}/opencode-root:${TAG}"]
}
