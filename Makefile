# ============================================
# Variables
# ============================================
PROJECTNAME := $(shell git remote get-url origin 2>/dev/null | sed -E 's|.*/([^/]+)(\.git)?$$|\1|' || basename "$$(pwd)")
PROJECTORG  := $(shell git remote get-url origin 2>/dev/null | sed -E 's|.*/([^/]+)/[^/]+(\.git)?$$|\1|' || basename "$$(dirname "$$(pwd)")")

# VERSION can be overridden: make build VERSION=1.2.3
# Otherwise, read from release.txt or default to 0.1.0
VERSION   := $(shell [ -f release.txt ] && cat release.txt || echo "$${VERSION:-0.1.0}")

# Build info
COMMIT_ID  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date +"%B %-d, %Y at %H:%M:%S")

# Official site URL (OPTIONAL - never guess or assume)
# Sources (in order of precedence):
#   1. File: site.txt in project root (single line, URL only)
#   2. Environment variable: OFFICIALSITE=https://example.com
#   3. Empty (self-hosted projects - users must use --server flag)
OFFICIALSITE := $(shell [ -f site.txt ] && cat site.txt || echo "$${OFFICIALSITE:-}")

# Linker flags to embed build info
LDFLAGS := -ldflags "-s -w -X 'main.Version=$(VERSION)' -X 'main.CommitID=$(COMMIT_ID)' -X 'main.BuildDate=$(BUILD_DATE)' -X 'main.OfficialSite=$(OFFICIALSITE)'"

# Directories
BINDIR := binaries
RELDIR := releases

# Go directories (persistent across builds)
GODIR   := $(HOME)/.local/share/go
GOCACHE := $(HOME)/.local/share/go/build

# Build matrix
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64 freebsd/amd64 freebsd/arm64

# Docker
REGISTRY ?= ghcr.io/$(PROJECTORG)/$(PROJECTNAME)
GO_DOCKER := docker run --rm \
	-v $$(pwd):/build \
	-v $(GOCACHE):/root/.cache/go-build \
	-v $(GODIR):/go \
	-w /build \
	-e CGO_ENABLED=0 \
	golang:alpine

.PHONY: build local release docker test dev clean lint help
.DEFAULT_GOAL := build

# =============================================================================
# BUILD - Full release build for all platforms (via Docker)
# =============================================================================
build: clean
	@mkdir -p $(BINDIR)
	@mkdir -p $(GOCACHE) $(GODIR)
	@echo "Building $(PROJECTNAME) $(VERSION) for all platforms..."
	@$(GO_DOCKER) go mod tidy
	@$(GO_DOCKER) go mod download

	@echo "Building local binary..."
	@$(GO_DOCKER) sh -c "GOOS=$$(go env GOOS) GOARCH=$$(go env GOARCH) \
		go build $(LDFLAGS) -o $(BINDIR)/$(PROJECTNAME) ./src"

	@for platform in $(PLATFORMS); do \
		OS=$${platform%/*}; \
		ARCH=$${platform#*/}; \
		OUTPUT=$(BINDIR)/$(PROJECTNAME)-$$OS-$$ARCH; \
		[ "$$OS" = "windows" ] && OUTPUT=$$OUTPUT.exe; \
		echo "  → server $$OS/$$ARCH"; \
		$(GO_DOCKER) sh -c "GOOS=$$OS GOARCH=$$ARCH \
			go build $(LDFLAGS) -o $$OUTPUT ./src" || exit 1; \
	done

	@if [ -d "src/client" ]; then \
		echo "Building $(PROJECTNAME)-cli..."; \
		$(GO_DOCKER) sh -c "GOOS=$$(go env GOOS) GOARCH=$$(go env GOARCH) \
			go build $(LDFLAGS) -o $(BINDIR)/$(PROJECTNAME)-cli ./src/client"; \
		for platform in $(PLATFORMS); do \
			OS=$${platform%/*}; \
			ARCH=$${platform#*/}; \
			OUTPUT=$(BINDIR)/$(PROJECTNAME)-cli-$$OS-$$ARCH; \
			[ "$$OS" = "windows" ] && OUTPUT=$$OUTPUT.exe; \
			echo "  → cli $$OS/$$ARCH"; \
			$(GO_DOCKER) sh -c "GOOS=$$OS GOARCH=$$ARCH \
				go build $(LDFLAGS) -o $$OUTPUT ./src/client" || exit 1; \
		done; \
	fi

	@if [ -d "src/agent" ]; then \
		echo "Building $(PROJECTNAME)-agent..."; \
		$(GO_DOCKER) sh -c "GOOS=$$(go env GOOS) GOARCH=$$(go env GOARCH) \
			go build $(LDFLAGS) -o $(BINDIR)/$(PROJECTNAME)-agent ./src/agent"; \
		for platform in $(PLATFORMS); do \
			OS=$${platform%/*}; \
			ARCH=$${platform#*/}; \
			OUTPUT=$(BINDIR)/$(PROJECTNAME)-agent-$$OS-$$ARCH; \
			[ "$$OS" = "windows" ] && OUTPUT=$$OUTPUT.exe; \
			echo "  → agent $$OS/$$ARCH"; \
			$(GO_DOCKER) sh -c "GOOS=$$OS GOARCH=$$ARCH \
				go build $(LDFLAGS) -o $$OUTPUT ./src/agent" || exit 1; \
		done; \
	fi

	@echo ""
	@echo "✓ Built $(PROJECTNAME) $(VERSION)"
	@echo "  Binaries: $$(ls -1 $(BINDIR)/ | wc -l | tr -d ' ') files in $(BINDIR)/"

# =============================================================================
# LOCAL - Fast host-platform build into binaries/ (production test build)
# =============================================================================
local: clean
	@mkdir -p $(BINDIR)
	@mkdir -p $(GOCACHE) $(GODIR)
	@echo "Building local binaries version $(VERSION)..."
	@$(GO_DOCKER) go mod tidy
	@$(GO_DOCKER) go mod download

	@echo "Building $(PROJECTNAME)..."
	@$(GO_DOCKER) sh -c "GOOS=$$(go env GOOS) GOARCH=$$(go env GOARCH) \
		go build $(LDFLAGS) -o $(BINDIR)/$(PROJECTNAME) ./src"

	@if [ -d "src/client" ]; then \
		echo "Building $(PROJECTNAME)-cli..."; \
		$(GO_DOCKER) sh -c "GOOS=$$(go env GOOS) GOARCH=$$(go env GOARCH) \
			go build $(LDFLAGS) -o $(BINDIR)/$(PROJECTNAME)-cli ./src/client"; \
	fi

	@if [ -d "src/agent" ]; then \
		echo "Building $(PROJECTNAME)-agent..."; \
		$(GO_DOCKER) sh -c "GOOS=$$(go env GOOS) GOARCH=$$(go env GOARCH) \
			go build $(LDFLAGS) -o $(BINDIR)/$(PROJECTNAME)-agent ./src/agent"; \
	fi

	@echo "✓ Local build complete: $(BINDIR)/"

# =============================================================================
# RELEASE - Manual local release (stable only)
# =============================================================================
release: build
	@mkdir -p $(RELDIR)
	@echo "Preparing release $(VERSION)..."

	@echo "$(VERSION)" > $(RELDIR)/version.txt

	@for f in $(BINDIR)/$(PROJECTNAME)-*; do \
		[ -f "$$f" ] || continue; \
		strip "$$f" 2>/dev/null || true; \
		cp "$$f" $(RELDIR)/; \
	done

	@tar --exclude='.git' --exclude='.github' --exclude='.gitea' \
		--exclude='$(BINDIR)' --exclude='$(RELDIR)' --exclude='*.tar.gz' \
		-czf $(RELDIR)/$(PROJECTNAME)-$(VERSION)-source.tar.gz .

	@gh release delete $(VERSION) --yes 2>/dev/null || true
	@git tag -d $(VERSION) 2>/dev/null || true
	@git push origin :refs/tags/$(VERSION) 2>/dev/null || true

	@gh release create $(VERSION) $(RELDIR)/* \
		--title "$(PROJECTNAME) $(VERSION)" \
		--notes "Release $(VERSION)" \
		--latest

	@echo "✓ Release $(VERSION) created"
	@echo "Auto-incrementing version in release.txt..."
	@echo "$(VERSION)" | awk -F. '{printf "%d.%d.%d\n", $$1, $$2, $$3+1}' > release.txt
	@echo "✓ Version incremented: $(VERSION) → $$(cat release.txt)"

# =============================================================================
# DOCKER - Build and push multi-platform Docker images
# =============================================================================
docker:
	@echo "Building Docker image $(VERSION)..."
	@docker buildx version > /dev/null 2>&1 || (echo "docker buildx required" && exit 1)
	@docker buildx create --name $(PROJECTNAME)-builder --use 2>/dev/null || \
		docker buildx use $(PROJECTNAME)-builder
	@docker buildx build \
		-f docker/Dockerfile \
		--platform linux/amd64,linux/arm64 \
		--build-arg VERSION="$(VERSION)" \
		--build-arg BUILD_DATE="$(BUILD_DATE)" \
		--build-arg COMMIT_ID="$(COMMIT_ID)" \
		-t $(REGISTRY):$(VERSION) \
		-t $(REGISTRY):latest \
		--push \
		.
	@echo "✓ Docker push complete: $(REGISTRY):$(VERSION)"

# =============================================================================
# TEST - Run all tests with coverage (via Docker)
# =============================================================================
test:
	@echo "Running tests..."
	@mkdir -p $(GOCACHE) $(GODIR)
	@$(GO_DOCKER) go mod download
	@$(GO_DOCKER) go vet ./...
	@$(GO_DOCKER) go test -v -timeout 5m -cover -coverprofile=coverage.out ./...
	@echo "✓ All tests passed"

# =============================================================================
# DEV - Quick build for local development (random temp dir, no ldflags)
# =============================================================================
dev:
	@mkdir -p $(GOCACHE) $(GODIR)
	@$(GO_DOCKER) go mod tidy
	@mkdir -p "$${TMPDIR:-/tmp}/$(PROJECTORG)" && \
		BUILD_DIR=$$(mktemp -d "$${TMPDIR:-/tmp}/$(PROJECTORG)/$(PROJECTNAME)-XXXXXX") && \
		echo "Quick dev build to $$BUILD_DIR..." && \
		$(GO_DOCKER) go build -o $$BUILD_DIR/$(PROJECTNAME) ./src && \
		echo "Built: $$BUILD_DIR/$(PROJECTNAME)" && \
		if [ -d "src/client" ]; then \
			$(GO_DOCKER) go build -o $$BUILD_DIR/$(PROJECTNAME)-cli ./src/client && \
			echo "Built: $$BUILD_DIR/$(PROJECTNAME)-cli"; \
		fi && \
		if [ -d "src/agent" ]; then \
			$(GO_DOCKER) go build -o $$BUILD_DIR/$(PROJECTNAME)-agent ./src/agent && \
			echo "Built: $$BUILD_DIR/$(PROJECTNAME)-agent"; \
		fi && \
		echo "Test:  docker run --rm -v $$BUILD_DIR:/app alpine:latest /app/$(PROJECTNAME) --help"

# =============================================================================
# LINT - Run golangci-lint (via Docker)
# =============================================================================
lint:
	@mkdir -p $(GOCACHE) $(GODIR)
	@$(GO_DOCKER) sh -c "which golangci-lint > /dev/null 2>&1 || \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest && \
		golangci-lint run ./..."
	@echo "✓ Lint passed"

# =============================================================================
# HELP - Print available targets and descriptions
# =============================================================================
help:
	@echo "Available targets:"
	@echo "  build    — Full release build for all 8 platforms (via Docker)"
	@echo "  local    — Fast host-platform build into binaries/"
	@echo "  release  — Create GitHub release with binaries and source archive"
	@echo "  docker   — Build and push multi-platform Docker image"
	@echo "  test     — Run unit tests with coverage (via Docker)"
	@echo "  dev      — Quick development build into a temp directory"
	@echo "  lint     — Run golangci-lint (via Docker)"
	@echo "  clean    — Remove build artifacts"
	@echo ""
	@echo "Version: $(VERSION)  Commit: $(COMMIT_ID)"

# =============================================================================
# CLEAN - Remove build artifacts
# =============================================================================
clean:
	@rm -rf $(BINDIR) $(RELDIR) coverage.out
