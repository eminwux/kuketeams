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
