# Compatibility

Pastebin is a drop-in replacement for pastebin.com, microbin, and lenpaste.
Existing scripts, CLIs, and integrations targeting any of those services work
against this server without modification.

## pastebin.com Routes

| Route | Behavior |
|-------|----------|
| `GET https://pste.us/{key}` | View paste |
| `GET https://pste.us/raw/{key}` | Raw paste text |
| `GET https://pste.us/archive` | Recent public pastes |
| `GET https://pste.us/trends` | Recent public pastes |
| `GET https://pste.us/u/{username}` | Redirect 302 to `/` |

## pastebin.com API

| Route | Behavior |
|-------|----------|
| `POST https://pste.us/api/api_login.php` | Returns `"ANONYMOUS"` (HTTP 200) |
| `POST https://pste.us/api/api_post.php` | Create, list, delete pastes |
| `POST https://pste.us/api/api_raw.php` | Raw paste text |

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
| `POST https://pste.us/upload` | Create paste (multipart) |
| `GET https://pste.us/upload/{id}` | View paste |
| `GET https://pste.us/p/{id}` | View paste |
| `GET https://pste.us/raw/{id}` | Raw text |
| `GET https://pste.us/url/{id}` | Redirect if content is URL |
| `GET https://pste.us/file/{id}` | Download paste |
| `GET https://pste.us/qr/{id}` | QR code PNG |
| `GET https://pste.us/list` | Recent public pastes |
| `GET https://pste.us/remove/{id}` | Show delete form |
| `POST https://pste.us/remove/{id}` | Delete paste |
| `GET https://pste.us/guide` | Redirect to `/server/help` |

## lenpaste Routes

| Route | Behavior |
|-------|----------|
| `POST https://pste.us/` | Create paste; redirect to `/{id}` |
| `GET https://pste.us/{id}` | View paste |
| `GET https://pste.us/raw/{id}` | Raw text |
| `GET https://pste.us/dl/{id}` | Download |
| `GET https://pste.us/emb/{id}` | Embedded view |
| `POST https://pste.us/api/v1/new` | Create paste (JSON/form) |
| `GET https://pste.us/api/v1/get?id=` | Get paste JSON |
| `GET https://pste.us/api/v1/getServerInfo` | Server info JSON |
| `GET https://pste.us/about` | Redirect to `/server/about` |
| `GET https://pste.us/emb_help/` | Redirect to `/server/help` |

## Auth Stub Routes

These routes exist for compatibility with scripts that probe them.
No authentication is ever performed — they redirect silently.

| Route | Response |
|-------|----------|
| `GET https://pste.us/login` | 302 → `/` |
| `GET https://pste.us/register` | 302 → `/` |
| `POST https://pste.us/login` | 302 → `/` |
| `POST https://pste.us/register` | 302 → `/` |
| `POST https://pste.us/logout` | 302 → `/` |
| `GET https://pste.us/settings` | 302 → `/` |
| `GET https://pste.us/auth/{id}` | 302 → `/{id}` |
| `GET https://pste.us/auth_raw/{id}` | 302 → `/raw/{id}` |
