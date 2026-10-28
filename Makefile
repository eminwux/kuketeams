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
	MODE=check TAG=v0.0.0-test PLATFORMS=linux/amd64,linux/arm64 ./harnesses/build-images.sh --print | python3 harnesses/tests/check_plan.py
	REGISTRY= TAG=dev MODE=check PLATFORMS=linux/amd64,linux/arm64 ./harnesses/build-images.sh --print | python3 harnesses/tests/check_plan.py

# One installer serves every role and the public role installation targets.
ROLE ?= roles/operator
.PHONY: claude codex opencode-openrouter
claude codex opencode-openrouter:
	@sh scripts/materialize-role.sh '$(ROLE)' '$@'

.PHONY: test-integration
test-integration:
	@test -n "$(KUKETEAMS_INTEGRATION_HOST)" || { echo 'Set KUKETEAMS_INTEGRATION_HOST to a disposable kukeond Unix endpoint' >&2; exit 1; }
	$(GO) test -race ./internal/initialize -run '^TestKukeondIntegration$$' -count=1 -v

.PHONY: test-remote-operator
test-remote-operator: kuketeams
	@test -n "$(KUKETEAMS_REMOTE_HOST)" || { echo 'Set KUKETEAMS_REMOTE_HOST explicitly; see docs/remote-operator-check.md' >&2; exit 1; }
	$(GO) test -race ./internal/launchcheck -run '^TestRemoteOperator$$' -count=1 -v -timeout 10m
