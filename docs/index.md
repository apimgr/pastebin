# Pastebin

A fast, full-stack Go web application for creating and sharing text snippets anonymously.
Drop-in replacement for pastebin.com, microbin, and lenpaste.

## Official Site

**[https://pste.us](https://pste.us)** — the official hosted instance. The CLI client
uses it as the default server; use `--server <url>` to target your own instance.

## Features

- Anonymous paste creation via web form, JSON API, raw body, or multipart file upload
- Server-side syntax highlighting via Chroma — no client-side JS required
- Language auto-detection from file extension on upload
- Expiry options: `1h`, `1d`, `1w`, `1m`, `3m`, `6m`, `1y`, `18m`, `2y`, `never`, or custom seconds
- Burn after N reads — paste deleted once view count reaches threshold
- Public and unlisted visibility
- Delete token — cryptographically random, returned once at creation, stored as SHA-256 hash
- QR code generation at `/qr/{id}`
- View count tracking with automatic background cleanup
- Full web frontend (dark/light/auto theme, PWA, mobile-first)
- CLI client (`pastebin-cli`)
- OpenAPI/Swagger docs at `/api/v1/server/swagger`
- GraphQL at `/api/v1/server/graphql` (GraphiQL UI at `/server/docs/graphql`, read-only)
- Full route compatibility with pastebin.com, microbin, and lenpaste
- Single self-contained static binary with embedded SQLite

## Quick Start

```bash
./pastebin
```

On first run the server binds all interfaces on a random port in the 64000-64999 range and prints the URL to the console. Open that URL in your browser, or pass `--port` to pin a specific port.

## Links

- [Official Site](https://pste.us)
- [Installation](installation.md)
- [Configuration](configuration.md)
- [API Reference](api.md)
- [CLI Reference](cli.md)
- [Compatibility](compat.md)
- [Security](security.md)
- [Source on GitHub](https://github.com/apimgr/pastebin)
