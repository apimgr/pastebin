# =============================================================================
# Build Stage
# =============================================================================
FROM casjaysdev/go:latest AS builder

ARG TARGETARCH
ARG VERSION=dev
ARG BUILD_DATE
ARG COMMIT_ID

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags "-s -w \
      -X 'main.Version=${VERSION}' \
      -X 'main.CommitID=${COMMIT_ID}' \
      -X 'main.BuildDate=${BUILD_DATE}'" \
    -o /app/pastebin ./src

# =============================================================================
# Runtime Stage
# All OCI image metadata is applied at build time via --annotation flags
# (docker buildx build --annotation ...) — never via LABEL statements.
# =============================================================================
FROM alpine:latest

# Required runtime packages per PART 26: git, curl, bash, tini, tor
RUN apk add --no-cache \
    git \
    curl \
    bash \
    tini \
    tor

# Copy binary and optional rootfs overlay (entrypoint, etc.)
COPY --from=builder /app/pastebin /usr/local/bin/pastebin
COPY docker/rootfs/ /

RUN chmod 755 /usr/local/bin/pastebin

# Runtime defaults — only what the binary cannot infer from the environment.
# Do NOT set ENABLE_TOR here; the binary auto-detects Tor from PATH.
ENV TZ=America/New_York \
    MODE=production

# Internal port (always 80 in container per PART 8 / PART 26).
EXPOSE 80

STOPSIGNAL SIGRTMIN+3

HEALTHCHECK --start-period=10m --interval=5m --timeout=15s --retries=3 \
    CMD /usr/local/bin/pastebin --status || exit 1

ENTRYPOINT ["tini", "-p", "SIGTERM", "--", "/usr/local/bin/entrypoint.sh"]
