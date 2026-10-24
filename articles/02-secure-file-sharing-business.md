# Secure File Sharing with CallFS: API Keys, Permissions, and Expiring Links

Sharing files securely across teams or with external clients involves more than just restricting who can log in. You need to control who can delete what, audit access, and hand temporary download URLs to people who should never see your internal credentials.

CallFS addresses this with three distinct security layers: **authentication** through API keys, **authorization** through an owner-based permission model, and **single-use expiring download links** for safe external sharing. This article walks through all three in practical terms, with working configuration, curl examples, and the matching screens of the web console.

---

## Layer 1: Authentication with API Keys

Every request to CallFS must present a valid API key in the `Authorization: Bearer` header, or a console session that was started with one. Keys are defined in the configuration file or managed from the console, which means you provision one key per team member, service account, or integration, with no shared passwords.

### Configuring Multiple API Keys

Bind each key to a named user with `auth.api_key_users`:

```yaml
auth:
  api_key_users:
    alice: "REPLACE_WITH_ALICE_KEY"
    bob: "REPLACE_WITH_BOB_KEY"
    ci-pipeline: "REPLACE_WITH_CI_KEY"
  admin_users: ["alice"]
  internal_proxy_secret: "REPLACE_WITH_SECRET_1"
  single_use_link_secret: "REPLACE_WITH_SECRET_2"
```

Generate each value with `openssl rand -hex 32` and keep keys out of the file if you prefer: `CALLFS_AUTH__API_KEY_USERS__ALICE=...` sets Alice's key from the environment.

CallFS checks these rules at startup and refuses to start if one fails:

- Every key, `internal_proxy_secret` and `single_use_link_secret` must be at least 16 characters.
- None of them may contain placeholder text such as `your-`, `change-me` or `a-strong-`, and the literal `default-api-key` is banned.
- All keys and both secrets must be different from each other.
- User names are 1 to 64 letters, digits, dots, dashes or underscores. `root`, `internal-proxy` and names like `api-user-3` are reserved.
- Every name in `admin_users` must be a configured user. Admins bypass file and folder permissions (read, write, delete) like `root`.

### How Keys Map to Identities

The user name is the identity. When Alice uploads a file, `alice` is recorded as its owner, and that stays true if you add, rotate or remove other keys. This is what the authorization layer uses to decide what a caller may do.

Older configurations used `auth.api_keys`, a plain list where the first key becomes `api-user-0`, the second `api-user-1`, and so on. That mapping is positional: inserting or removing a key shifts every later identity and hands files to the wrong person. It still works, but prefer `api_key_users`. In the console, Settings, "Users and keys" can move numbered keys to named users for you.

Admin users get the administration pages of the console, can see and revoke every user's download links, and can change file owners. The owner rules below apply to everyone, including admins, for reading, writing and deleting.

### Managing Users Without Editing the File

Administrators can add users, rotate keys, grant or remove admin rights and delete users from the console at Settings, "Users and keys". A new key is shown once, at creation. Changes are saved to the config file and take effect at once, with no restart. The same actions exist in the admin API (`/v1/admin/users`); see [Admin API](../docs_markdown/10-admin-api.md).

### Making an Authenticated Request

```bash
curl -X POST \
  -H "Authorization: Bearer alice-api-key" \
  --data-binary @report.pdf \
  https://files.example.com/v1/files/reports/q4.pdf
```

```bash
curl -H "Authorization: Bearer bob-api-key" \
  https://files.example.com/v1/files/reports/q4.pdf \
  -o q4.pdf
```

A request without a valid key receives `401 Unauthorized` with `{"code":"AUTHENTICATION_FAILED","message":"authentication failed"}` before any file operation is attempted.

CallFS also slows down guessing. Each client address may fail authentication about 10 times in a burst and then once every 6 seconds; beyond that, requests with a key get `429 Too Many Requests` until the client waits. The console sign-in has its own limit (`server.rate_limit.session_rps`, default 1 per second with a burst of 5). Failed attempts show up in the console under Activity, "Sign-ins".

### Signing In to the Console

Open `https://files.example.com/ui/`, paste an API key into "Access key", and select "Sign in". The console keeps you signed in with a secure cookie for 8 hours (`ui.session_ttl`) and signs you out after 1 hour of no use (`ui.idle_timeout`; the console reads it from the server). Signing out revokes the session on the server, and in a cluster the revoked session stops working on every node. A wrong key at sign-in returns `401`, and a console that was signed out for expiry shows a notice on the sign-in page. Changing actions in the console carry a per-session CSRF token, so another website cannot act on your behalf. The console is part of the Docker image and of `make build` binaries; see [Installation](../docs_markdown/01-installation.md).

---

## Layer 2: Authorization: The Owner Model

Authentication answers "who are you?". Authorization answers "what are you allowed to do?". CallFS uses an ownership-based model that is straightforward to reason about. The owner is an application user name; it has nothing to do with operating-system accounts.

### The Rules

When a file or directory is created, the caller's user name is recorded as its owner. From that point:

- **The owner and administrators can do everything** with the item: read, write, rename and delete.
- **Everyone else is governed by the "other" bits of the mode.** The read bit lets other users read a file or list a folder. The write bit lets them overwrite a file or create items in a folder. New files get mode `0644` (others can read) and new folders get `0777` (others can list and create).
- **Delete stays with the owner.** If `alice` uploaded `/reports/q4.pdf` with the default mode, then `bob` can download it but cannot overwrite, rename or delete it. Only a folder's owner can delete the folder itself.
- **Only administrators can change an owner**: a `PATCH` with `{"owner": "bob"}` returns `403` with the code `ADMIN_REQUIRED` for everyone else. Owners and administrators change the mode with `{"mode": "0600"}` (octal). In the console, the mode editor also accepts symbolic input such as `u=rw,go=r`.

Mode `0600` hides a file from everyone but the owner and administrators. Mode `0666` lets every user overwrite it. There are no access control lists to maintain, and the mode has nothing to do with operating-system accounts.

### Inspecting Ownership

Every file response includes an `X-CallFS-Owner` header with the owner's user name. A HEAD request checks ownership without downloading the body:

```bash
curl -I \
  -H "Authorization: Bearer bob-api-key" \
  https://files.example.com/v1/files/reports/q4.pdf
```

The response headers include:

```
X-CallFS-Owner: alice
X-CallFS-Size: 204800
X-CallFS-Mode: 0644
X-CallFS-MTime: 2026-04-11T08:00:00Z
```

This is useful for automation: a CI pipeline can verify the owner of a file before deciding whether to proceed.

### Asking Why a Request Would Be Allowed or Denied

The permission check endpoint explains the decision for any path:

```bash
curl -H "Authorization: Bearer bob-api-key" \
  "https://files.example.com/v1/permissions/check?path=/reports/q4.pdf&perm=delete"
```

```json
{"path":"/reports/q4.pdf","perm":"delete","user":"bob","allowed":false,"reason":"not_owner","owner":"alice","type":"file"}
```

`perm` is `read`, `write` or `delete`. The `reason` is `owner`, `admin`, `open_directory`, `parent_open`, `not_found` or `not_owner`. Administrators can add `&user=alice` to check on behalf of another user. In the console, the Files page shows the same answer under "Show permissions".

### What Happens on a Denied Request

If `bob` attempts to delete a file owned by `alice`:

```bash
curl -X DELETE \
  -H "Authorization: Bearer bob-api-key" \
  https://files.example.com/v1/files/reports/q4.pdf
```

The response is:

```
HTTP/1.1 403 Forbidden
{"code":"PERMISSION_DENIED","message":"permission denied"}
```

No ambiguity, no fallthrough.

---

## Layer 3: Single-Use Expiring Download Links

The two layers above protect your API. But sometimes you need to share a file with someone who should not have an API key: an external auditor, a client, a partner. That is what single-use expiring download links are for.

The workflow is:

1. An authenticated user generates a link, through the API or the console.
2. The link is sent to the recipient (email, chat, wherever).
3. The recipient opens the link and confirms the download. No API key is needed.
4. The link is consumed and cannot be used again.

### Step 1: Generate a Link

```bash
curl -X POST \
  -H "Authorization: Bearer alice-api-key" \
  -H "Content-Type: application/json" \
  -d '{"path":"/reports/q4.pdf","expiry_seconds":3600}' \
  https://files.example.com/v1/links/generate
```

The server responds with `201 Created`:

```json
{
  "id": 42,
  "url": "https://files.example.com/download/TOKEN",
  "token": "TOKEN",
  "expires": "2026-04-11T09:00:00Z"
}
```

The rules:

- `expiry_seconds` must be between 1 and 86400 (24 hours); otherwise the request returns `400`.
- `path` must name a file. A folder returns `400` with "links work for files only".
- You need read permission on the file, and a missing file returns `404`.
- The `url` is built from `server.external_url`, so set it to the address your recipients actually reach, such as `https://files.example.com`. Without a `http://` or `https://` prefix, CallFS assumes `https://`.
- One user can hold at most `server.rate_limit.max_active_links_per_user` active links (default 1,000; `0` also means 1,000); the next request returns `429`.
- Link creation is rate limited per client address (`server.rate_limit.link_generate_rps`, default 100 per second, `link_generate_burst` default 100).

In the console, open Files, select the file, choose "Create download link...", pick 15 minutes, 1 hour, 1 day or a custom time, and copy the link (or "Copy as curl"). You can also start from Sharing, "New download link", type the file path and select "Check file".

### Step 2: Send the Link

Pass the `url` value to the recipient. It contains no credentials and no information about your API key or identity. Anyone who has the link can use it once, so send it over a channel you trust.

### Step 3: Download (No Auth Required)

Opening the link in a browser shows a short confirmation page with a "Download file" button. Pressing it starts the download. The extra step means mail scanners and chat previews that merely fetch the address cannot use the link up.

For scripts, send the confirmation yourself with a POST:

```bash
curl -X POST -OJ https://files.example.com/download/TOKEN
```

The file streams back with `Content-Disposition: attachment` and the original file name. A plain GET of the link only shows the confirmation page and does not consume it.

### Step 4: The Link Is Consumed

Once the download starts, the link is marked `used`. Any later request for the same token returns:

```
HTTP/1.1 410 Gone
{"code":"LINK_INVALID","message":"link is invalid or has been used"}
```

The other answers are:

| Situation | Status | Code |
|---|---|---|
| Token does not exist or is malformed | 404 | `LINK_NOT_FOUND` |
| Already used or revoked | 410 | `LINK_INVALID` |
| Past its expiry time | 410 | `LINK_EXPIRED` |
| Too many attempts from one address | 429 | `RATE_LIMIT_EXCEEDED` |

Opening a link uses it up even if the transfer then fails, so recipients should not retry blindly. A background job removes expired and used links every 5 minutes; the console's Sharing page also has "Clean up old links now".

### Seeing and Revoking Links

Every link you create is listed under Sharing in the console, grouped by Active, Used, Expired and Revoked. Select a link to see its timeline, including each open attempt with its result and source address. Choose "Revoke" to stop an active link at once. Administrators see everyone's links; other users see their own.

The same data is available over the API:

```bash
curl -H "Authorization: Bearer alice-api-key" \
  "https://files.example.com/v1/links?status=active&limit=50"

curl -X DELETE -H "Authorization: Bearer alice-api-key" \
  https://files.example.com/v1/links/42
```

`GET /v1/links` accepts `status` (`active`, `used`, `expired`, `revoked`), `path`, `created_by`, `limit` (1 to 1000, default 200) and `cursor`, and returns counts per status. `GET /v1/links/{id}` returns one link with its attempts, `GET /v1/links/stats?window=24h` (`1h`, `24h` or `7d`) summarises creations and opens (cluster-wide for admins, your own links for everyone else, with a `last_cleanup` report), and `DELETE /v1/links/{id}` revokes (`409` if the link is not active, `404` if it is not yours).

### How Tokens Are Secured

Each token has two parts joined by a dot: a random 16-byte ID and a signature made from the ID, the file path and `single_use_link_secret`. This gives two properties:

- **Path-bound**: a token created for `/reports/q4.pdf` cannot be used to download `/reports/q3.pdf`.
- **Unforgeable**: without `single_use_link_secret`, nobody can produce a valid token. Changing that secret invalidates every active link, so change it only when you want that.

The secret never appears in a URL, and the console and logs show only a short token prefix.

---

## Rate Limiting

Per-address rate limiting protects the three entry points that matter most. The limits are settings, and the values below are the defaults:

| Entry point | Setting prefix | Rate | Burst |
|---|---|---|---|
| `POST /v1/links/generate` | `server.rate_limit.link_generate_` | 100 per second | 100 |
| `GET` and `POST /download/{token}` | `server.rate_limit.download_` | 10 per second | 5 |
| `POST /v1/session` (console sign-in) | `server.rate_limit.session_` | 1 per second | 5 |

Each limit has an `_rps` and a `_burst` key, for example `server.rate_limit.download_rps`. They can be changed at runtime from Settings in the console, and Cluster, "Limits" shows rejections and the busiest clients. With `nats.counters: true` in a cluster, the download, link and session rates are counted across all nodes. Each user can also hold at most `server.rate_limit.max_active_links_per_user` active links (default 1000; the next link request returns `429`). A request over the limit receives:

```
HTTP/1.1 429 Too Many Requests
Retry-After: 1
{"code":"RATE_LIMIT_EXCEEDED","message":"Rate limit exceeded"}
```

Limiters are kept per client address (per /64 network for IPv6), idle entries expire after 10 minutes, and at most 100,000 addresses are tracked. If CallFS sits behind a reverse proxy, list the proxy in `server.trusted_proxies` so limits apply to the real client rather than to the proxy.

---

## HTTPS and TLS

In production, CallFS should run with TLS enabled:

```yaml
server:
  listen_addr: ":8443"
  protocol: "https"
  external_url: "https://files.example.com"
  cert_file: "/etc/callfs/server.crt"
  key_file: "/etc/callfs/server.key"
```

With `protocol: https`, both `cert_file` and `key_file` are required and CallFS will refuse to start without them. `protocol: auto` also serves HTTPS and refuses to start without a certificate; it never falls back to plain HTTP. On a TLS connection, the server sets `Strict-Transport-Security` with a one-year max-age, `includeSubDomains` and `preload`.

These security headers are set on every response, API and download alike: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Content-Security-Policy`, `Referrer-Policy`, `Permissions-Policy`, `Cross-Origin-Opener-Policy` and `Cross-Origin-Embedder-Policy`. The console cookie is `HttpOnly`, `SameSite=Strict`, and `Secure` on HTTPS.

For development, set `protocol: "http"`. Do not do that on any network you do not fully trust. The sibling settings `server.enable_quic` and `server.quic_listen_addr` add HTTP/3 over the same certificate.

---

## Practical Example: Sharing a Report with an External Client

Putting all three layers together, here is a complete workflow for delivering a file to someone outside your organization.

### 1. Upload the Report

Alice uploads the Q4 report using her API key:

```bash
curl -X POST \
  -H "Authorization: Bearer alice-api-key" \
  --data-binary @q4-report.pdf \
  https://files.example.com/v1/files/reports/q4-final.pdf
```

CallFS records `alice` as the owner of `/reports/q4-final.pdf`. (If `/reports/` does not exist yet, create it first with a POST to `/v1/files/reports/`.)

### 2. Generate a Time-Limited Link

Alice generates a one-hour download link:

```bash
curl -X POST \
  -H "Authorization: Bearer alice-api-key" \
  -H "Content-Type: application/json" \
  -d '{"path":"/reports/q4-final.pdf","expiry_seconds":3600}' \
  https://files.example.com/v1/links/generate
```

Response:

```json
{
  "id": 43,
  "url": "https://files.example.com/download/Zm9vYmFy....",
  "token": "Zm9vYmFy....",
  "expires": "2026-04-11T09:00:00Z"
}
```

### 3. Send the Link to the Client

Alice emails the `url` to the client. The URL contains no API key, no internal path, and no authentication material.

### 4. Client Downloads the File

The client opens the URL in a browser, reads the confirmation page and presses "Download file", or runs:

```bash
curl -X POST -OJ https://files.example.com/download/Zm9vYmFy....
```

The browser saves the file as `q4-final.pdf`.

### 5. Link Expires

After the download starts, the token is marked as used. If the client or anyone else tries the same URL again, they receive `410 Gone`. If the client waits more than an hour, the link has expired and also returns `410 Gone`. Alice can open Sharing in the console at any time to see that the link was used, when, and from which address, or revoke it before it is used.

At no point did the client have access to the CallFS API, Alice's key, or any other file in the system.

---

## Summary

CallFS provides secure file sharing for business through three concrete mechanisms:

- **API keys** authenticate every caller, with each key bound to a named user. Failed attempts are throttled and recorded, and the console manages users and keys without a restart.
- **Owner-based authorization with mode bits** gives creators full control and exclusive delete access, while the "other" read and write bits of each file or folder decide what everyone else may do. No ACLs to maintain, and a permission check tells you why any request is allowed or denied.
- **Single-use expiring links** let authenticated users generate short-lived, signed, path-bound URLs that work without API keys. Links are consumed on first use, expire on a timer of up to 24 hours, can be revoked, and are listed with their full history under Sharing.

Together these layers cover the two most common secure sharing scenarios: internal team access through API keys and one-time external delivery through expiring links. Read more in [Authentication and security](../docs_markdown/04-authentication-security.md), [API reference](../docs_markdown/03-api-reference.md) and [Web console](../docs_markdown/08-web-console.md).
