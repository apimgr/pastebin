# Installation

## Binary (recommended)

Download the latest binary for your platform from [GitHub Releases](https://github.com/apimgr/pastebin/releases).

```bash
# Linux amd64
curl -Lo pastebin https://github.com/apimgr/pastebin/releases/latest/download/pastebin-linux-amd64
chmod +x pastebin
./pastebin
```

Available platforms: `linux-amd64`, `linux-arm64`, `darwin-amd64`, `darwin-arm64`,
`windows-amd64.exe`, `windows-arm64.exe`, `freebsd-amd64`, `freebsd-arm64`.

## Docker

```bash
docker run -d \
  --name pastebin \
  -p 64580:80 \
  -v pastebin-data:/data \
  ghcr.io/apimgr/pastebin:latest
```

## Docker Compose

```bash
cd docker/
docker compose up -d
```

## Build from Source

Requires Docker only — no local Go installation needed.

```bash
git clone https://github.com/apimgr/pastebin
cd pastebin
make build        # all 8 platforms → binaries/
make local        # current platform → binaries/
make dev          # quick dev build → /tmp/apimgr/pastebin-XXXXXX/
```

## First Run

The server works with zero configuration. On first run it creates:

- Config: `/etc/apimgr/pastebin/server.yml` (Linux)
- Data / DB: `/var/lib/apimgr/pastebin/db/server.db` (Linux)
- Logs: `/var/log/apimgr/pastebin/` (Linux)

Open your browser at `http://localhost:3010` (default port when run locally).
