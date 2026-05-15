# ============================================
# Variables
# ============================================
PROJECTNAME = pastebin
PROJECTORG = apimgr
CLIENT_NAME = pastebin-cli

# VERSION can be overridden: make build VERSION=1.2.3
# Otherwise, read from release.txt or default to 0.0.1
VERSION ?= $(shell cat release.txt 2>/dev/null || echo "0.0.1")

COMMIT = $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE = $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS = -ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildDate=$(BUILD_DATE) -w -s"

# Detect host OS and architecture
HOSTOS := $(shell go env GOOS)
HOSTARCH := $(shell go env GOARCH)

# ============================================
# Main Targets
# ============================================
# Go build/mod cache dirs (host-mounted for speed)
GOCACHE   ?= $(HOME)/.cache/go-build
GOMODCACHE ?= $(HOME)/go/pkg/mod

.PHONY: build dev releases test docker clean
.DEFAULT_GOAL := build

# Build all binaries for all platforms
build:
	@echo "Building $(PROJECTNAME) + $(CLIENT_NAME) $(VERSION) for all platforms..."
	@mkdir -p binaries
	@docker run --rm \
		-v $$(pwd):/workspace \
		-v $(GOCACHE):/root/.cache/go-build \
		-v $(GOMODCACHE):/root/go/pkg/mod \
		-w /workspace golang:alpine sh -c ' \
		apk add --no-cache git binutils > /dev/null 2>&1 && \
		for GOOS in linux darwin freebsd windows; do \
			for GOARCH in amd64 arm64; do \
				suffix=""; \
				ext=""; \
				[ "$$GOOS" = "darwin" ] && suffix="macos" || suffix="$$GOOS"; \
				[ "$$GOOS" = "freebsd" ] && suffix="bsd"; \
				[ "$$GOOS" = "windows" ] && ext=".exe"; \
				name="$$suffix-$$GOARCH"; \
				echo "  → $$name"; \
				GOOS=$$GOOS GOARCH=$$GOARCH CGO_ENABLED=0 go build $(LDFLAGS) -o binaries/$(PROJECTNAME)-$$name$$ext ./src || exit 1; \
				GOOS=$$GOOS GOARCH=$$GOARCH CGO_ENABLED=0 go build $(LDFLAGS) -o binaries/$(CLIENT_NAME)-$$name$$ext ./src/client || exit 1; \
				strip binaries/$(PROJECTNAME)-$$name$$ext 2>/dev/null || true; \
				strip binaries/$(CLIENT_NAME)-$$name$$ext 2>/dev/null || true; \
			done; \
		done && \
		echo "  → Host ($(HOSTOS)/$(HOSTARCH))" && \
		GOOS=$(HOSTOS) GOARCH=$(HOSTARCH) CGO_ENABLED=0 go build $(LDFLAGS) -o binaries/$(PROJECTNAME) ./src && \
		GOOS=$(HOSTOS) GOARCH=$(HOSTARCH) CGO_ENABLED=0 go build $(LDFLAGS) -o binaries/$(CLIENT_NAME) ./src/client \
	'
	@echo ""
	@echo "✓ Built $(PROJECTNAME) + $(CLIENT_NAME) $(VERSION)"
	@echo "  Binaries: $$(ls -1 binaries/ | wc -l) files"
	@echo "  Host binaries: binaries/$(PROJECTNAME)  binaries/$(CLIENT_NAME)"

# Create GitHub release
release:
	@echo "Creating GitHub release $(VERSION)..."
	@mkdir -p releases
	@echo "Copying platform binaries to releases/..."
	@cp binaries/$(PROJECTNAME)-linux-amd64 releases/ 2>/dev/null || { echo "Error: Build first with 'make build'"; exit 1; }
	@cp binaries/$(PROJECTNAME)-linux-arm64 releases/
	@cp binaries/$(PROJECTNAME)-windows-amd64.exe releases/
	@cp binaries/$(PROJECTNAME)-windows-arm64.exe releases/
	@cp binaries/$(PROJECTNAME)-macos-amd64 releases/
	@cp binaries/$(PROJECTNAME)-macos-arm64 releases/
	@cp binaries/$(PROJECTNAME)-bsd-amd64 releases/
	@cp binaries/$(PROJECTNAME)-bsd-arm64 releases/
	@echo "Creating source archives (no VCS files)..."
	@git archive --format=tar.gz --prefix=$(PROJECTNAME)-$(VERSION)/ HEAD -o releases/$(PROJECTNAME)-$(VERSION)-src.tar.gz
	@git archive --format=zip --prefix=$(PROJECTNAME)-$(VERSION)/ HEAD -o releases/$(PROJECTNAME)-$(VERSION)-src.zip
	@echo "Deleting existing release if exists..."
	@gh release delete $(VERSION) -y 2>/dev/null || true
	@git tag -d $(VERSION) 2>/dev/null || true
	@echo "Creating GitHub release $(VERSION)..."
	@gh release create $(VERSION) ./releases/* \
		--title "$(PROJECTNAME) $(VERSION)" \
		--notes "Release $(VERSION)\n\nCommit: $(COMMIT)\nBuilt: $(BUILD_DATE)\n\n**Binaries**: 8 platforms (Linux, macOS, Windows, BSD - amd64/arm64)\n**Source**: tar.gz and zip archives (no VCS files)"
	@echo "✓ Release $(VERSION) created with binaries and source archives"
	@echo "Auto-incrementing version in release.txt..."
	@echo "$(VERSION)" | awk -F. '{printf "%d.%d.%d\n", $$1, $$2, $$3+1}' > release.txt
	@echo "✓ Version incremented: $(VERSION) → $$(cat release.txt)"

# Build and run server locally for development (host binary, auto-restarts not included)
dev:
	@echo "Building dev binary..."
	@docker run --rm \
		-v $$(pwd):/workspace \
		-v $(GOCACHE):/root/.cache/go-build \
		-v $(GOMODCACHE):/root/go/pkg/mod \
		-w /workspace golang:alpine sh -c ' \
		CGO_ENABLED=0 go build $(LDFLAGS) -o binaries/$(PROJECTNAME) ./src && \
		CGO_ENABLED=0 go build $(LDFLAGS) -o binaries/$(CLIENT_NAME) ./src/client \
	'
	@echo "✓ Dev build ready — run: ./binaries/$(PROJECTNAME)"

# Run all tests
test:
	@echo "Running tests..."
	@docker run --rm \
		-v $$(pwd):/workspace \
		-v $(GOCACHE):/root/.cache/go-build \
		-v $(GOMODCACHE):/root/go/pkg/mod \
		-w /workspace golang:alpine sh -c ' \
		CGO_ENABLED=0 go test -v -timeout 5m ./... \
	'
	@echo "✓ All tests passed"

# Build and push multi-platform Docker images (release)
docker:
	@echo "Building multi-platform Docker images..."
	@docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t ghcr.io/$(PROJECTORG)/$(PROJECTNAME):latest \
		-t ghcr.io/$(PROJECTORG)/$(PROJECTNAME):$(VERSION) \
		--push \
		.
	@echo "✓ Docker images pushed to ghcr.io/$(PROJECTORG)/$(PROJECTNAME):$(VERSION)"

# Build Docker image for development (local only, not pushed)
docker-dev:
	@echo "Building development Docker image..."
	@docker build \
		--build-arg VERSION=$(VERSION)-dev \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(PROJECTNAME):dev \
		.
	@echo "✓ Docker development image built: $(PROJECTNAME):dev"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf binaries/ releases/ coverage.out
	@go clean
	@echo "✓ Clean complete"
