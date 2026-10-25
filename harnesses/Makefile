# Same static build graph for local Docker loads, validation, and tag publishing.
REGISTRY ?= ghcr.io/eminwux/kuketeams
TAG ?= latest
MODE ?= load
PLATFORMS ?= linux/$(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
IMAGES ?=

.PHONY: images plan check
images:
	@REGISTRY="$(REGISTRY)" TAG="$(TAG)" MODE="$(MODE)" PLATFORMS="$(PLATFORMS)" ./build-images.sh $(IMAGES)

plan:
	@REGISTRY="$(REGISTRY)" TAG="$(TAG)" MODE=check PLATFORMS="$(PLATFORMS)" ./build-images.sh --print $(IMAGES)

check:
	$(MAKE) -C .. test-harnesses
