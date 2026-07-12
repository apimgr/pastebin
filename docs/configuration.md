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

termbin:
  enabled: false     # raw-TCP termbin/fiche compat listener (off by default)
  port: 9999
  max_size: 32768    # max payload in bytes
  timeout: "5s"      # idle/read timeout

web:
  csp:
    # frame-ancestors for the embeddable /emb/{id} endpoint only
    # default "*" (any site may iframe embeds); restrict with origins,
    # e.g. "'self' https://example.com"
    embed_frame_ancestors: "*"
```

### Logging (`server.logs`)

Each log file accepts `filename`, `format`, `custom` (only when
`format: custom`), `rotate`, and `keep`.

| Key | Default | Formats |
|-----|---------|---------|
| `logs.level` | `info` | `debug`/`info`/`warn`/`error` (server.log, app.log gate) |
| `logs.access` | `access.log`, `apache`, rotate `monthly`, keep `none` | `apache`, `nginx`, `json`, `custom` |
| `logs.server` | `server.log`, `text`, rotate `weekly,50MB`, keep `none` | `text`, `json` |
| `logs.error` | `error.log`, `text`, rotate `weekly,50MB`, keep `none` | `text`, `json` |
| `logs.app` | `app.log`, `logfmt`, rotate `weekly,50MB`, keep `none` | `logfmt`, `json` |
| `logs.auth` | `auth.log`, `syslog`, rotate `weekly,50MB`, keep `none` | `syslog` (RFC 3164), `json` |
| `logs.audit` | `audit.log`, `json`, rotate `daily`, keep `none`, `compress: true` | `json` only |
| `logs.security` | `security.log`, `fail2ban`, rotate `weekly,50MB`, keep `none` | `fail2ban`, `syslog`, `cef`, `json`, `text` |
| `logs.debug` | `enabled: true`, `debug.log`, `text`, rotate `weekly,50MB`, keep `none` | `text`, `json`; only written when debug mode is active |

**`rotate` values:** `never`, `daily`, `weekly`, `monthly`, `yearly`,
`{N}MB`, `{N}GB`, or a time+size combo like `weekly,50MB` (whichever
hits first). Rotation renames the file to `{name}.YYYY-MM-DD` and
reopens a fresh file.

**`keep` values:** `none` (delete rotated files immediately), `{N}`
(keep newest N rotated files), `{N}d`/`{N}w`/`{N}m` (age-based), or
`forever`.

**`format: custom`** (access log) substitutes these variables in
`custom`: `{time}` `{date}` `{datetime}` `{remote_ip}` `{method}`
`{path}` `{query}` `{status}` `{bytes}` `{latency}` `{latency_ms}`
`{user_agent}` `{referer}` `{request_id}` `{fqdn}` `{protocol}`
`{tls_version}` `{country}` `{asn}`.

All log lines are strict raw text: ANSI escape sequences, control
characters, and emoji are stripped before writing. Invalid `rotate`,
`keep`, or `format` values warn and fall back to defaults — startup
never fails. Rotation is checked on every write (size) and by the
`log_rotation` scheduler task (time + retention pruning).

### Backups (`server.backup`)

Retention keys under `backup.retention`:

| Key | Default | Meaning |
|-----|---------|---------|
| `max_backups` | `30` | Newest N dated backups kept |
| `keep_weekly` | `4` | Sunday backups kept |
| `keep_monthly` | `12` | First-of-month backups kept |
| `keep_yearly` | `3` | Jan 1st backups kept |
| `max_total_size` | `10%` | Hard size cap on retained backups |

`max_total_size` accepts a percent of the backup volume (`10%`) or an
absolute size (`50G`, `500MB`); falsey values (`0`, `off`, `none`)
disable the cap. When over the cap, oldest dated backups are pruned
first — the newest backup is never deleted.

Before each daily backup the server checks free disk space: the backup
is skipped (with a `backup.skipped_disk_full` audit event) when free
space is less than 2x the last backup size or disk usage exceeds
`maintenance.cleanup.disk_threshold` (default 90%).

The `--maintenance setup`, `restore`, and `mode` subcommands require
authorization: allowed on first run (empty database), as root (with
confirmation), or as the service user with the operator password.

## Environment Variables

| Variable | Description |
|----------|-------------|
| `PORT`   | Override listen port (useful in Docker) |
| `TZ`     | Timezone (default: `America/New_York`) |
| `MODE`   | Application mode (`production` or `development`) |
| `TERMBIN_ENABLED` | Enable the raw-TCP termbin/fiche compat listener (default: `false`) |
| `TERMBIN_PORT`    | termbin listener port (default: `9999`) |
| `TERMBIN_MAX_SIZE`| Max termbin payload in bytes (default: `32768`) |
| `TERMBIN_TIMEOUT` | termbin idle/read timeout (default: `5s`) |
