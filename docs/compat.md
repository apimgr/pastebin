# Compatibility

Pastebin is a drop-in replacement for pastebin.com, microbin, and lenpaste.
Existing scripts, CLIs, and integrations targeting any of those services work
against this server without modification.

## pastebin.com Routes

| Route | Behavior |
|-------|----------|
| `GET /{key}` | View paste |
| `GET /raw/{key}` | Raw paste text |
| `GET /archive` | Recent public pastes |
| `GET /trends` | Recent public pastes |
| `GET /u/{username}` | Redirect 302 to `/` |

## pastebin.com API

| Route | Behavior |
|-------|----------|
| `POST /api/api_login.php` | Returns `"ANONYMOUS"` (HTTP 200) |
| `POST /api/api_post.php` | Create, list, delete pastes |
| `POST /api/api_raw.php` | Raw paste text |

All `api_dev_key` and `api_user_key` fields are accepted but ignored.

**`api_post.php` options:**

| `api_option` | Action |
|---|---|
| `paste` | Create paste |
| `list` | Return recent public pastes as XML |
| `delete` | Delete paste (token via `api_user_key`) |
| `userdetails` | Returns stub anonymous user record |

## microbin Routes

| Route | Behavior |
|-------|----------|
| `POST /upload` | Create paste (multipart) |
| `GET /upload/{id}` | View paste |
| `GET /p/{id}` | View paste |
| `GET /raw/{id}` | Raw text |
| `GET /url/{id}` | Redirect if content is URL |
| `GET /file/{id}` | Download paste |
| `GET /qr/{id}` | QR code PNG |
| `GET /list` | Recent public pastes |
| `GET /remove/{id}` | Show delete form |
| `POST /remove/{id}` | Delete paste |
| `GET /guide` | Redirect to `/server/help` |

## lenpaste Routes

| Route | Behavior |
|-------|----------|
| `POST /` | Create paste; redirect to `/{id}` |
| `GET /{id}` | View paste |
| `GET /raw/{id}` | Raw text |
| `GET /dl/{id}` | Download |
| `GET /emb/{id}` | Embedded view |
| `POST /api/v1/new` | Create paste (JSON/form) |
| `GET /api/v1/get?id=` | Get paste JSON |
| `GET /api/v1/getServerInfo` | Server info JSON |
| `GET /about` | Redirect to `/server/about` |
| `GET /emb_help/` | Redirect to `/server/help` |

## Auth Stub Routes

These routes exist for compatibility with scripts that probe them.
No authentication is ever performed — they redirect silently.

| Route | Response |
|-------|----------|
| `GET /login` | 302 → `/` |
| `GET /register` | 302 → `/` |
| `POST /login` | 302 → `/` |
| `POST /register` | 302 → `/` |
| `POST /logout` | 302 → `/` |
| `GET /settings` | 302 → `/` |
| `GET /auth/{id}` | 302 → `/{id}` |
| `GET /auth_raw/{id}` | 302 → `/raw/{id}` |
