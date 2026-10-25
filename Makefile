GO ?= go
VERSION ?= dev
BINARY ?= bin/kuketeams
GOOS ?= linux
GOARCH ?= amd64

.PHONY: kuketeams test dist
kuketeams:
	$(GO) build -trimpath -ldflags '-X main.version=$(VERSION)' -o $(BINARY) ./cmd/kuketeams

test:
	$(GO) vet ./...
	$(GO) test -race ./...

dist:
	GO='$(GO)' GOOS='$(GOOS)' GOARCH='$(GOARCH)' VERSION='$(VERSION)' bash scripts/build-dist.sh

.PHONY: test-harnesses
test-harnesses:
	$(GO) test ./internal/harnessassets
	python3 -m unittest discover -s harnesses/codex/tests -v
	python3 -m unittest discover -s harnesses/tests -v
	MODE=check PLATFORMS=linux/amd64,linux/arm64 ./harnesses/build-images.sh --print | python3 harnesses/tests/check_plan.py
