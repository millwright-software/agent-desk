.PHONY: build run install uninstall which clean dev release-local test fmt lint ci

BINARY_NAME=agent-desk
BUILD_DIR=./build
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

# Build the binary
build:
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/agent-desk

# Run in development
run:
	go run ./cmd/agent-desk

# Where `go install` puts things: $GOBIN if set, else $GOPATH/bin.
GOBIN_DIR := $(shell go env GOBIN)
ifeq ($(GOBIN_DIR),)
GOBIN_DIR := $(shell go env GOPATH)/bin
endif

# Install to the Go bin directory — the one `go install` already uses, and the
# one a Go developer's PATH already contains. No sudo.
#
# It used to `sudo cp` into /usr/local/bin while `go install` wrote to
# $GOPATH/bin, so a machine could end up with two agent-desk binaries in two
# directories and whichever came first on PATH won. That is not hypothetical:
# it left a month-old binary shadowed behind a current one, and a debugging
# session was spent on symptoms that had been fixed in a build the shell was
# not running. One destination, one answer to "which one am I running".
install:
	go install ./cmd/agent-desk
	@echo "✅ Installed to $(GOBIN_DIR)/$(BINARY_NAME)"
	@echo "   (make sure $(GOBIN_DIR) is on your PATH)"
	@echo "Run 'agent-desk' to start"

# Remove it from the Go bin directory, and report any copy left behind in the
# two places older versions of this Makefile installed to — a stale one there
# silently wins if it sits earlier on PATH.
uninstall:
	rm -f $(GOBIN_DIR)/$(BINARY_NAME)
	@echo "✅ Removed $(GOBIN_DIR)/$(BINARY_NAME)"
	@if [ -e /usr/local/bin/$(BINARY_NAME) ]; then \
		echo "⚠️  Another copy is at /usr/local/bin/$(BINARY_NAME) (needs sudo):"; \
		echo "      sudo rm /usr/local/bin/$(BINARY_NAME)"; \
	fi
	@if [ -e $(HOME)/.local/bin/$(BINARY_NAME) ]; then \
		echo "⚠️  Another copy is at $(HOME)/.local/bin/$(BINARY_NAME):"; \
		echo "      rm $(HOME)/.local/bin/$(BINARY_NAME)"; \
	fi

# Report every agent-desk on this machine and which one the shell would run.
which:
	@echo "PATH would run: $$(command -v $(BINARY_NAME) || echo '<none>')"
	@for d in $(GOBIN_DIR) /usr/local/bin $(HOME)/.local/bin; do \
		[ -e "$$d/$(BINARY_NAME)" ] && echo "  found: $$d/$(BINARY_NAME)"; \
	done; true

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	go clean

# Development with auto-reload
dev:
	@which air > /dev/null || go install github.com/cosmtrek/air@latest
	air

# Run tests (with race detector)
test:
	go test -race -v ./...

# Format code
fmt:
	go fmt ./...

# Lint
lint:
	@which golangci-lint > /dev/null || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run

# Run local CI checks (same as pre-push hook: lint + test + build in parallel)
ci:
	@which lefthook > /dev/null || (echo "ERROR: lefthook not found. Run: brew install lefthook" && exit 1)
	lefthook run pre-push --force --no-auto-install

# Local release using GoReleaser
# Prerequisites: brew install goreleaser
# Required env: GITHUB_TOKEN, HOMEBREW_TAP_GITHUB_TOKEN
release-local:
	@echo "=== Pre-flight checks ==="
	@which goreleaser > /dev/null || (echo "ERROR: goreleaser not found. Run: brew install goreleaser" && exit 1)
	@test -n "$$GITHUB_TOKEN" || (echo "ERROR: GITHUB_TOKEN not set" && exit 1)
	@test -n "$$HOMEBREW_TAP_GITHUB_TOKEN" || (echo "ERROR: HOMEBREW_TAP_GITHUB_TOKEN not set" && exit 1)
	@TAG=$$(git describe --tags --exact-match 2>/dev/null) || (echo "ERROR: HEAD is not tagged. Run: git tag vX.Y.Z" && exit 1); \
	CODE_VERSION=$$(grep 'const Version' cmd/agent-desk/main.go | sed 's/.*"\(.*\)".*/\1/'); \
	TAG_VERSION=$${TAG#v}; \
	if [ "$$TAG_VERSION" != "$$CODE_VERSION" ]; then \
		echo "ERROR: Tag $$TAG ($$TAG_VERSION) != code Version $$CODE_VERSION"; \
		exit 1; \
	fi; \
	echo "Version: $$CODE_VERSION"
	@echo "=== Running tests ==="
	go test -race ./...
	@echo "=== Running GoReleaser ==="
	goreleaser release --clean
	@echo "=== Release complete ==="
	@echo "Verify: gh release view $$(git describe --tags --exact-match) --repo millwright-software/agent-desk"
