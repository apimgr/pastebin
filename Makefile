# ============================================
# Variables
# ============================================
PROJECTNAME := $(shell git remote get-url origin 2>/dev/null | sed -E 's|.*/([^/]+)(\.git)?$$|\1|' || basename "$$(pwd)")
PROJECTORG  := $(shell git remote get-url origin 2>/dev/null | sed -E 's|.*/([^/]+)/[^/]+(\.git)?$$|\1|' || basename "$$(dirname "$$(pwd)")")
CLIENT_NAME  := pastebin-cli

# VERSION can be overridden: make build VERSION=1.2.3
# Otherwise, read from release.txt or default to 0.0.1
VERSION   ?= $(shell cat release.txt 2>/dev/null || echo "0.0.1")
COMMIT_ID  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -ldflags "-X 'main.Version=$(VERSION)' -X 'main.CommitID=$(COMMIT_ID)' -X 'main.BuildDate=$(BUILD_DATE)' -w -s"

# Detect host OS and architecture
HOSTOS  := $(shell go env GOOS)
HOSTARCH := $(shell go env GOARCH)

# Go build/mod cache dirs (host-mounted for speed)
GOCACHE    ?= $(HOME)/.cache/go-build
GOMODCACHE ?= $(HOME)/go/pkg/mod

REGISTRY := ghcr.io/$(PROJECTORG)/$(PROJECTNAME)

GO_DOCKER := docker run --rm \
	-v $$(pwd):/workspace \
	-v $(GOCACHE):/root/.cache/go-build \
	-v $(GOMODCACHE):/root/go/pkg/mod \
	-w /workspace \
	-e CGO_ENABLED=0 \
	golang:alpine

# ============================================
# Main Targets
# ============================================
.PHONY: build dev release test docker docker-dev clean
.DEFAULT_GOAL := build

# Build all binaries for all platforms
build:
	@echo "Building $(PROJECTNAME) + $(CLIENT_NAME) $(VERSION) for all platforms..."
	@mkdir -p binaries
	@$(GO_DOCKER) sh -c ' \
		apk add --no-cache git binutils > /dev/null 2>&1 && \
		for GOOS in linux darwin freebsd windows; do \
			for GOARCH in amd64 arm64; do \
				ext=""; \
				[ "$$GOOS" = "windows" ] && ext=".exe"; \
				name="$$GOOS-$$GOARCH"; \
				echo "  → $$name"; \
				GOOS=$$GOOS GOARCH=$$GOARCH go build $(LDFLAGS) -o binaries/$(PROJECTNAME)-$$name$$ext ./src || exit 1; \
				GOOS=$$GOOS GOARCH=$$GOARCH go build $(LDFLAGS) -o binaries/$(CLIENT_NAME)-$$name$$ext ./src/client || exit 1; \
				strip binaries/$(PROJECTNAME)-$$name$$ext 2>/dev/null || true; \
				strip binaries/$(CLIENT_NAME)-$$name$$ext 2>/dev/null || true; \
			done; \
		done && \
		echo "  → Host ($(HOSTOS)/$(HOSTARCH))" && \
		GOOS=$(HOSTOS) GOARCH=$(HOSTARCH) go build $(LDFLAGS) -o binaries/$(PROJECTNAME) ./src && \
		GOOS=$(HOSTOS) GOARCH=$(HOSTARCH) go build $(LDFLAGS) -o binaries/$(CLIENT_NAME) ./src/client \
	'
	@echo ""
	@echo "✓ Built $(PROJECTNAME) + $(CLIENT_NAME) $(VERSION)"
	@echo "  Binaries: $$(ls -1 binaries/ | wc -l) files"
	@echo "  Host binaries: binaries/$(PROJECTNAME)  binaries/$(CLIENT_NAME)"

# Quick host-platform dev build into a temp dir
dev:
	@mkdir -p $(GOCACHE) $(GOMODCACHE)
	@mkdir -p "$${TMPDIR:-/tmp}/$(PROJECTORG)" && \
		BUILD_DIR=$$(mktemp -d "$${TMPDIR:-/tmp}/$(PROJECTORG)/$(PROJECTNAME)-XXXXXX") && \
		echo "Building dev binaries..." && \
		$(GO_DOCKER) sh -c " \
			GOOS=$(HOSTOS) GOARCH=$(HOSTARCH) go build -o $$BUILD_DIR/$(PROJECTNAME) ./src && \
			GOOS=$(HOSTOS) GOARCH=$(HOSTARCH) go build -o $$BUILD_DIR/$(CLIENT_NAME) ./src/client \
		" && \
		echo "✓ Built: $$BUILD_DIR/$(PROJECTNAME)" && \
		echo "✓ Built: $$BUILD_DIR/$(CLIENT_NAME)"

# Create GitHub release
release:
	@echo "Creating GitHub release $(VERSION)..."
	@mkdir -p releases
	@echo "Copying platform binaries to releases/..."
	@cp binaries/$(PROJECTNAME)-linux-amd64   releases/ 2>/dev/null || { echo "Error: run 'make build' first"; exit 1; }
	@cp binaries/$(PROJECTNAME)-linux-arm64   releases/
	@cp binaries/$(PROJECTNAME)-darwin-amd64  releases/
	@cp binaries/$(PROJECTNAME)-darwin-arm64  releases/
	@cp binaries/$(PROJECTNAME)-freebsd-amd64 releases/
	@cp binaries/$(PROJECTNAME)-freebsd-arm64 releases/
	@cp binaries/$(PROJECTNAME)-windows-amd64.exe releases/
	@cp binaries/$(PROJECTNAME)-windows-arm64.exe releases/
	@cp binaries/$(CLIENT_NAME)-linux-amd64   releases/
	@cp binaries/$(CLIENT_NAME)-linux-arm64   releases/
	@cp binaries/$(CLIENT_NAME)-darwin-amd64  releases/
	@cp binaries/$(CLIENT_NAME)-darwin-arm64  releases/
	@cp binaries/$(CLIENT_NAME)-freebsd-amd64 releases/
	@cp binaries/$(CLIENT_NAME)-freebsd-arm64 releases/
	@cp binaries/$(CLIENT_NAME)-windows-amd64.exe releases/
	@cp binaries/$(CLIENT_NAME)-windows-arm64.exe releases/
	@echo "Creating source archives (no VCS files)..."
	@git archive --format=tar.gz --prefix=$(PROJECTNAME)-$(VERSION)/ HEAD -o releases/$(PROJECTNAME)-$(VERSION)-src.tar.gz
	@git archive --format=zip    --prefix=$(PROJECTNAME)-$(VERSION)/ HEAD -o releases/$(PROJECTNAME)-$(VERSION)-src.zip
	@echo "Deleting existing release if it exists..."
	@gh release delete $(VERSION) -y 2>/dev/null || true
	@git tag -d $(VERSION) 2>/dev/null || true
	@echo "Creating GitHub release $(VERSION)..."
	@gh release create $(VERSION) ./releases/* \
		--title "$(PROJECTNAME) $(VERSION)" \
		--notes "Release $(VERSION)\n\nCommit: $(COMMIT_ID)\nBuilt: $(BUILD_DATE)\n\n**Binaries**: 8 platforms (Linux, macOS, Windows, FreeBSD — amd64/arm64)\n**Source**: tar.gz and zip archives"
	@echo "✓ Release $(VERSION) created"
	@echo "Auto-incrementing version in release.txt..."
	@echo "$(VERSION)" | awk -F. '{printf "%d.%d.%d\n", $$1, $$2, $$3+1}' > release.txt
	@echo "✓ Version incremented: $(VERSION) → $$(cat release.txt)"

# Run all tests
test:
	@echo "Running tests..."
	@$(GO_DOCKER) sh -c ' \
		go vet ./... && \
		go test -v -timeout 5m ./... \
	'
	@echo "✓ All tests passed"

# Build and push multi-platform Docker images (release)
docker:
	@echo "Building multi-platform Docker images..."
	@docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT_ID=$(COMMIT_ID) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(REGISTRY):latest \
		-t $(REGISTRY):$(VERSION) \
		--push \
		.
	@echo "✓ Docker images pushed to $(REGISTRY):$(VERSION)"

# Build Docker image for development (local only, not pushed)
docker-dev:
	@echo "Building development Docker image..."
	@docker build \
		--build-arg VERSION=$(VERSION)-dev \
		--build-arg COMMIT_ID=$(COMMIT_ID) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(PROJECTNAME):dev \
		.
	@echo "✓ Docker development image built: $(PROJECTNAME):dev"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf binaries/ releases/ coverage.out
	@echo "✓ Clean complete"
