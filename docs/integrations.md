# Integrations

Pastebin is a drop-in replacement for [pastebin.com](https://pastebin.com), [microbin](https://microbin.eu), and [lenpaste](https://lenpaste.net). Existing clients and tools that target any of those services work without modification.

## Compatible APIs

| API | Base Path | Description |
|-----|-----------|-------------|
| Native REST | `https://pste.us/api/v1/paste` | Full-featured JSON API |
| pastebin.com | `https://pste.us/api/api_post.php` | Drop-in replacement |
| lenpaste | `https://pste.us/api/v1/new`, `https://pste.us/api/v1/get` | Compatible paste creation/retrieval |
| microbin | `https://pste.us/upload`, `https://pste.us/p/{id}` | Compatible upload and retrieval |
| GraphQL | `https://pste.us/graphql` | Read-only query interface |

## Native REST API

```bash
# Create a paste
curl -X POST https://pste.us/api/v1/paste \
  -H "Content-Type: application/json" \
  -d '{"content":"hello world","language":"text"}'

# Get a paste
curl https://pste.us/api/v1/paste/{id}

# Get raw text
curl https://pste.us/api/v1/paste/{id}/raw

# Delete a paste
curl -X DELETE https://pste.us/api/v1/paste/{id}?token={delete_token}

# List pastes
curl https://pste.us/api/v1/pastes
```

## pastebin.com Compatibility

```bash
curl -X POST https://pste.us/api/api_post.php \
  -d "api_option=paste&api_paste_code=hello+world&api_paste_format=text"
```

## lenpaste Compatibility

```bash
curl -X POST https://pste.us/api/v1/new \
  -d "text=hello+world"
```

## CLI Tool

See the [CLI Reference](cli.md) for the `pastebin-cli` tool.
