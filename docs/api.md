# API Reference

Base URL: `https://your-server`  
API version prefix: `/api/v1`

All responses include `Access-Control-Allow-Origin: *`.

## Create Paste

```
POST /api/v1/paste
```

**Request body** (JSON, form, or multipart):

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `content` | string | required | Paste body text |
| `title` | string | `""` | Optional title |
| `language` | string | `text` | Chroma language identifier |
| `visibility` | string | `public` | `public` or `unlisted` |
| `expires_in` | string | `never` | `1h`, `1d`, `1w`, `1m`, `3m`, `6m`, `1y`, `18m`, `2y`, `never`, or seconds |
| `burn_after` | integer | `0` | Delete after N views; `0` = disabled; max `9999` |

**Response** (201 Created):

```json
{
  "id": "abc12345",
  "url": "https://your-server/abc12345",
  "delete_token": "raw-plaintext-token-shown-once-only"
}
```

!!! warning
    The `delete_token` is shown **once only** at creation. Store it securely. Loss of the token means the paste cannot be deleted before natural expiry.

## Get Paste

```
GET /api/v1/paste/{id}
```

**Response** (200 OK):

```json
{
  "id": "abc12345",
  "title": "My Paste",
  "content": "Hello, World!",
  "language": "text",
  "is_public": true,
  "burn_after": 0,
  "expires_at": null,
  "views": 3,
  "created_at": "2025-01-01T00:00:00Z"
}
```

## Delete Paste

```
DELETE /api/v1/paste/{id}
Authorization: Bearer <delete-token>
```

Or via query parameter: `DELETE /api/v1/paste/{id}?token=<delete-token>`

**Response**: 204 No Content on success.

## Raw Paste Text

```
GET /api/v1/paste/{id}/raw
```

Returns plain text.

## List Recent Pastes

```
GET /api/v1/pastes?page=1&limit=20
```

Returns paginated list of public (non-unlisted) pastes.

## Health Check

```
GET /server/healthz
```

Content negotiated: HTML for browsers, JSON for API clients, plain text for CLI.

## OpenAPI / Swagger

Interactive API docs at `/api/v1/server/swagger`.

## GraphQL

Endpoint: `/graphql`

```graphql
# Get a paste
query {
  paste(id: "abc12345") {
    id
    title
    content
    language
    views
    createdAt
  }
}

# List recent pastes
query {
  pastes(page: 1, limit: 20) {
    id
    title
    language
    views
    createdAt
  }
}
```

Mutations are not available via GraphQL — use REST for create and delete.

## Raw Body (curl pipe)

```bash
echo "Hello" | curl -X POST https://your-server/api/v1/paste \
  --data-binary @- -H 'Content-Type: text/plain'
```
