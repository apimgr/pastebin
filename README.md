# Pastebin

A fast, full-stack Go web application for creating and sharing text snippets anonymously.
Drop-in replacement for pastebin.com, microbin, and lenpaste — existing scripts, CLIs,
and integrations work without modification.

## Features

- Anonymous paste creation via web form, JSON API, raw body, or multipart file upload
- Server-side syntax highlighting via Chroma (no client-side JS required)
- Language auto-detection from file extension on upload
- Expiry options: `1h`, `1d`, `1w`, `1m`, `3m`, `6m`, `1y`, `18m`, `2y`, `never`, or custom seconds
- Burn after N reads — paste deleted once view count reaches threshold
- Public and unlisted visibility
- Delete token — cryptographically random, returned once at creation, stored as SHA-256 hash
- Raw paste view at `/raw/{id}` and `/{id}/raw`
- Download at `/dl/{id}`
- Embedded view at `/emb/{id}` (iframe-embeddable)
- QR code at `/qr/{id}`
- View count tracking
- Automatic background cleanup of expired and burned pastes
- Full web frontend (server-side Go templates, dark/light/auto theme, PWA, mobile-first)
- Server pages: `/server/about`, `/server/help`, `/server/healthz`, `/server/privacy`, `/server/terms`
- CLI client (`pastebin-cli`)
- OpenAPI/Swagger docs at `/api/v1/server/swagger`
- GraphQL at `/graphql` (read-only queries)
- Full route compatibility with pastebin.com, microbin, and lenpaste
- Single self-contained static binary with embedded SQLite

## Install

Download the latest binary for your platform from [Releases](https://github.com/apimgr/pastebin/releases).

```bash
# Linux amd64
curl -Lo pastebin https://github.com/apimgr/pastebin/releases/latest/download/pastebin-linux-amd64
chmod +x pastebin
./pastebin
```

## Docker

```bash
docker run -d \
  -p 64580:80 \
  -v pastebin-data:/data \
  ghcr.io/apimgr/pastebin:latest
```

## Build from Source

Requires Docker (no local Go toolchain needed):

```bash
git clone https://github.com/apimgr/pastebin
cd pastebin
make build      # all 8 platforms
make local      # current platform only
make dev        # quick development build
make test       # run unit tests
```

## Usage

```bash
# Start server (defaults: 0.0.0.0:3010)
./pastebin

# Custom address and port
./pastebin --address 127.0.0.1 --port 8080

# Show version
./pastebin --version

# Show status
./pastebin --status
```

## CLI Client

```bash
# Create a paste from stdin
echo "Hello, World!" | pastebin-cli create

# Create from file
pastebin-cli create myfile.go --lang go --expiry 1d

# Fetch paste content
pastebin-cli get abc12345

# Delete paste
pastebin-cli delete abc12345 <delete-token>

# List recent pastes
pastebin-cli list --limit 20

# Target custom server
pastebin-cli --server https://paste.example.com create myfile.txt
```

## API

```bash
# Create a paste
curl -X POST https://paste.example.com/api/v1/paste \
  -H 'Content-Type: application/json' \
  -d '{"content":"Hello","language":"text","expires_in":"1d"}'

# Get a paste
curl https://paste.example.com/api/v1/paste/{id}

# Delete a paste
curl -X DELETE https://paste.example.com/api/v1/paste/{id} \
  -H 'Authorization: Bearer <delete-token>'

# List recent pastes
curl https://paste.example.com/api/v1/pastes

# Pipe to paste (raw body)
cat file.txt | curl -X POST https://paste.example.com/api/v1/paste \
  --data-binary @- -H 'Content-Type: text/plain'
```

## Configuration

The server reads `/etc/apimgr/pastebin/server.yml` on Linux (created automatically on first run). All settings can be overridden via CLI flags.

## Environment Variables

### Server (`pastebin`)

| Variable | Default | Description |
|----------|---------|-------------|
| `CONFIG_DIR` | Platform default | Configuration directory path |
| `DATA_DIR` | Platform default | Data directory path |
| `LOGS_DIR` | Platform default | Log directory path |
| `CACHE_DIR` | Platform default | Cache directory path |
| `BACKUP_DIR` | Platform default | Backup directory path |
| `PID_FILE` | Platform default | PID file path |
| `PORT` | Random 64xxx | Listen port (80 in container) |
| `ADDRESS` | `0.0.0.0` | Listen address |
| `BASE_URL` | Auto-detected | Public base URL for link generation |
| `DB_PATH` | `{data_dir}/db/server.db` | SQLite database path |
| `SITE_TITLE` | `Pastebin` | Site title shown in the web UI |
| `THEME` | `dark` | UI theme: `dark`, `light`, or `auto` |
| `MAX_SIZE_BYTES` | `10485760` (10 MiB) | Maximum paste size in bytes |
| `NO_COLOR` | unset | Set to any value to disable ANSI color output |
| `_DAEMON_CHILD` | unset | Internal: set by `--daemon` to mark the child process |

Platform default paths:

| Platform | Config | Data | Logs |
|----------|--------|------|------|
| Linux (root/service) | `/etc/apimgr/pastebin/` | `/var/lib/apimgr/pastebin/` | `/var/log/apimgr/pastebin/` |
| Linux (user) | `~/.config/apimgr/pastebin/` | `~/.local/share/apimgr/pastebin/` | `~/.local/log/apimgr/pastebin/` |
| Container | `/config/pastebin/` | `/data/pastebin/` | `/data/log/pastebin/` |
| macOS (user) | `~/Library/Application Support/apimgr/pastebin/` | same | `~/Library/Logs/apimgr/pastebin/` |
| Windows (user) | `%LocalAppData%\apimgr\pastebin\` | same | same |

### Client (`pastebin-cli`)

| Variable | Default | Description |
|----------|---------|-------------|
| `PASTEBIN_SERVER` | (required) | Server base URL — e.g. `https://paste.example.com` |
| `NO_COLOR` | unset | Set to any value to disable ANSI color output |

## License

MIT — see [LICENSE.md](LICENSE.md)
