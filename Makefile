GO ?= go
VERSION ?= dev
BINARY ?= bin/kuketeams

.PHONY: kuketeams test
kuketeams:
	$(GO) build -trimpath -ldflags '-X main.version=$(VERSION)' -o $(BINARY) ./cmd/kuketeams

test:
	$(GO) vet ./...
	$(GO) test -race ./...
