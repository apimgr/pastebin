# API Reference

Base URL: `https://pste.us` (official site) or your self-hosted instance  
API version prefix: `/api/v1`

All responses include `Access-Control-Allow-Origin: *`.

## Create Paste

```
POST https://pste.us/api/v1/paste
```

**Request body** (JSON, form, or multipart):

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `content` | string | required | Paste body text, or (when `is_link` is `true`) the `http://`/`https://` redirect target URL |
| `title` | string | `""` | Optional title |
| `language` | string | `text` | Chroma language identifier; ignored when `is_link` is `true` |
| `is_link` | boolean | `false` | Create as a link paste — `content` must be an absolute `http://` or `https://` URL |
| `visibility` | string | `public` | `public` or `unlisted` |
| `expires_in` | string | `never` | `1h`, `1d`, `1w`, `1m`, `3m`, `6m`, `1y`, `18m`, `2y`, `never`, or seconds |
| `burn_after` | integer | `0` | Delete after N views; `0` = disabled; max `9999` |

**Response** (201 Created):

```json
{
  "id": "abc12345",
  "url": "https://pste.us/abc12345",
  "delete_token": "raw-plaintext-token-shown-once-only"
}
```

!!! warning
    The `delete_token` is shown **once only** at creation. Store it securely. Loss of the token means the paste cannot be deleted before natural expiry.

## Get Paste

```
GET https://pste.us/api/v1/paste/{id}
```

**Response** (200 OK):

```json
{
  "id": "abc12345",
  "title": "My Paste",
  "content": "Hello, World!",
  "language": "text",
  "is_link": false,
  "is_public": true,
  "burn_after": 0,
  "expires_at": null,
  "views": 3,
  "created_at": "2025-01-01T00:00:00Z"
}
```

If the paste is a link (`is_link: true`), the frontend and API view routes (`GET /{id}` and `GET /api/v1/paste/{id}`) respond with a `302 Found` redirect to the target URL instead of the JSON body above. Use the `/raw` endpoint to retrieve the target URL as plain text without redirecting.

## Link Pastes

Create a paste that redirects to a URL instead of rendering text:

```bash
curl -X POST https://pste.us/api/v1/paste \
  --header "Content-Type: application/json" \
  --data '{"content":"https://example.com/some/long/path","is_link":true}'
```

- Only `http://` and `https://` target URLs are accepted; anything else is rejected with `400 VALIDATION_FAILED`.
- Short IDs are auto-generated — there is no custom/vanity slug support.
- The target URL is never fetched server-side.
- `GET /{id}` (web and API) issues a `302 Found` redirect to the target.
- `GET /api/v1/paste/{id}/raw` returns the target URL as plain text, without redirecting.
- Expiry, burn-after-read, visibility, and owner-token deletion all work identically to text pastes.

## Delete Paste

```
DELETE https://pste.us/api/v1/paste/{id}
Authorization: Bearer <delete-token>
```

Or via query parameter: `DELETE https://pste.us/api/v1/paste/{id}?token=<delete-token>`

**Response**: 204 No Content on success.

## Raw Paste Text

```
GET https://pste.us/api/v1/paste/{id}/raw
```

Returns plain text.

## List Recent Pastes

```
GET https://pste.us/api/v1/pastes?page=1&limit=20
```

Returns paginated list of public (non-unlisted) pastes.

## Health Check

```
GET https://pste.us/server/healthz
```

Content negotiated: HTML for browsers, JSON for API clients, plain text for CLI.

## OpenAPI / Swagger

Interactive API docs at `https://pste.us/api/v1/server/swagger`.

## GraphQL

Endpoint: `https://pste.us/api/v1/server/graphql` (unversioned alias: `https://pste.us/api/graphql`). Interactive GraphiQL UI: `https://pste.us/server/docs/graphql`.

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
echo "Hello" | curl -X POST https://pste.us/api/v1/paste \
  --data-binary @- -H 'Content-Type: text/plain'
```
