# Configuration

The server reads `server.yml` from the config directory on startup. First run creates this file automatically. CLI flags always override config file values.

## Config File Location

| Platform | Path |
|----------|------|
| Linux    | `/etc/apimgr/pastebin/server.yml` |
| macOS    | `/Library/Application Support/apimgr/pastebin/server.yml` |
| Docker   | `/config/server.yml` |

Override with: `--config /path/to/server.yml`

## CLI Flags

```
--help / -h               Show help
--version / -v            Show version
--status                  Show server status and health
--mode {production|development}  Application mode (default: production)
--config DIR              Config directory
--data DIR                Data directory
--log DIR                 Log directory
--cache DIR               Cache directory
--backup DIR              Backup directory
--pid FILE                PID file path
--address ADDR            Listen address (default: 0.0.0.0)
--port PORT               Listen port (default: 3010 local, 80 container)
--baseurl PATH            URL path prefix (default: /)
--debug                   Enable debug mode
--daemon                  Run as daemon
--clean-expired           Delete expired/burned pastes and exit
--service CMD             Service management
--maintenance CMD         Maintenance operations
--update [CMD]            Check/perform updates
```

## server.yml Reference

```yaml
server:
  address: "0.0.0.0"
  port: "3010"
  mode: "production"
  base_url: ""

database:
  type: "sqlite"
  path: ""   # default: {data_dir}/db/server.db

rate_limit:
  enabled: true
  create_per_minute: 30
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `PORT`   | Override listen port (useful in Docker) |
| `TZ`     | Timezone (default: `America/New_York`) |
| `MODE`   | Application mode (`production` or `development`) |
