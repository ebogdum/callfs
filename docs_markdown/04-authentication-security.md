# Authentication and Security

This page covers who can use CallFS and what they can do: access keys, users, admins, console sign-in, sessions, secrets, TLS, rate limits, security headers and path rules.

Related pages: [Configuration](02-configuration.md), [API reference](03-api-reference.md), [Clustering](07-clustering.md), [Web console](08-web-console.md), [Admin API](10-admin-api.md).

## Overview

| What | How it is checked |
|---|---|
| API calls | `Authorization: Bearer <api key>` header |
| Web console | Sign in with an access key. The console keeps a session cookie and a CSRF token |
| Servers talking to each other | The shared `auth.internal_proxy_secret` |
| Download links | A signed one-time token in the link, no key needed |

All of `/v1` requires a key or a console session. The only exceptions are the health endpoints, `POST /v1/session` (sign in) and `/download/{token}` links.

## Access keys

An access key is a secret string. Each key belongs to one **app user**. The user ID is stored as the `owner` of every file and folder that key creates.

CallFS does not check keys against a user database. You list them in `config.yaml` (or manage them in the console, see [Manage users and keys in the console](#manage-users-and-keys-in-the-console)).

### Define users with `auth.api_key_users`

```yaml
auth:
  api_key_users:
    alice: "alice-secret-key-1234"
    bob: "bob-secret-key-5678"
  admin_users: ["alice"]
```

The map goes from user ID to key. The ID is tied to the name, so you can replace, rotate or reorder keys without changing who owns existing files.

User ID rules for `api_key_users`:

- Must not be empty.
- Console and Admin API: 1 to 64 letters, digits, dots, dashes or underscores.
- `root`, `internal-proxy` and any `api-user-<number>` name are reserved and are rejected.

Set one user from the environment (names are lowercased):

```bash
export CALLFS_AUTH__API_KEY_USERS__ALICE="alice-secret-key-1234"
```

This sets the user `alice`.

### Numbered keys with `auth.api_keys` (older form)

```yaml
auth:
  api_keys:
    - "first-secret-key-1234"
    - "second-secret-key-5678"
```

These keys get a numbered identity based on their position in the list, so files they create are owned by `api-user-0`, `api-user-1` and so on. Removing or reordering an entry changes the identity of every later key, and the key that moves up inherits ownership of the removed user's files. Use `api_key_users` instead. If you keep this form, only add keys at the end of the list. The console shows these keys as "Some keys share a numbered user" and has a **Fix this** action that gives each one its own user ID and moves its files to the new owner (see below).

You can use both forms together while you migrate.

### Key rules (checked at startup)

| Rule | Error |
|---|---|
| At least one key is set in `api_keys` or `api_key_users` | `auth.api_keys or auth.api_key_users must contain at least one key` |
| Keys must not be placeholders (contain `your-`, `change-me` or `a-strong-`) | key rejected as a placeholder |
| The same key must not appear twice across both lists | `key ... is already configured for ...` |
| Reserved user IDs are not allowed | `user ID "root" is reserved` |
| The internal secret must not be used as an API key | `auth.internal_proxy_secret must not be reused as an API key` |
| The link secret must not be used as an API key | `auth.single_use_link_secret must not be reused as an API key` |

Run `callfs config validate` to check a file before you start the server (see [Installation](01-installation.md)).

### Key format

CallFS accepts any non-empty string as a key. Keys that the console or Admin API generates are 32 random bytes encoded as URL-safe base64 (43 characters, for example `Xr3...`). Use at least 32 random characters for keys you create by hand.

CallFS compares keys by a hash, so a key is not compared in plain text.

### Send the key

```bash
curl -H "Authorization: Bearer your-api-key" https://files.example.com:8443/v1/me
```

`GET /v1/me` returns who you are:

```json
{
  "user_id": "alice",
  "is_admin": true,
  "auth_method": "api_key",
  "session_expires_at": null,
  "csrf_token": null,
  "permissions_model": "owner",
  "idle_timeout_seconds": 3600
}
```

With a console session, `auth_method` is `session` and `session_expires_at` and `csrf_token` are filled in. `idle_timeout_seconds` is the value of `ui.idle_timeout` in seconds.

A missing or wrong key returns `401` with:

```json
{"code":"AUTHENTICATION_FAILED","message":"authentication failed"}
```

The response includes `WWW-Authenticate: Bearer`.

### Failed attempts are throttled

If one client address sends too many wrong keys, CallFS answers `429` with `{"code":"RATE_LIMIT_EXCEEDED","message":"Rate limit exceeded"}` and does not check the key. Each address can make 10 wrong attempts in a burst, then 1 more every 6 seconds. Correct keys are not counted. IPv6 clients are grouped by their /64 network.

## App users are not OS users

App users exist only inside CallFS. A user ID such as `alice` has no relation to any operating system user, UID or GID, and CallFS never reads or writes OS accounts. Files stored on disk are written by the user that runs the CallFS process. Ownership in CallFS is only the user ID string stored with each file.

## Permissions

Every file and folder has one owner: the user ID that created it, and a mode (for example `0644`). Access depends on who you are.

| Who | Files | Folders |
|---|---|---|
| Owner | read, write, delete | read, create inside, delete |
| Other user | read if the "other" read bit is set, write if the "other" write bit is set; never delete | list if the "other" read bit is set, create files or folders inside if the "other" write bit is set; never delete |
| Admin | everything, on any path | everything, on any path |

- The mode bits for other users are the last octal digit: `4` is read, `2` is write. A file with mode `0644` can be read but not changed by other users. A file with mode `0600` is private to its owner. A folder with mode `0777` lets every user list it and create items in it; `0755` lets them list it only.
- New folders start with mode `0777` and new files with `0644`. Owners and admins change the mode with `PATCH` on the path (see [API reference](03-api-reference.md)) or in the console file details, where you can also type a symbolic mode such as `u=rw,go=r`.
- Deleting always needs ownership (or admin). The mode never grants delete.
- Mode bits only control what other app users can do inside CallFS. They have no relation to operating system accounts.
- Creating a new path needs write permission on its nearest existing parent folder.
- A path that does not exist returns `404` for reads.
- A denied request returns `403` with `{"code":"PERMISSION_DENIED",...}`.

### Status codes for access problems

| Situation | Status |
|---|---|
| Wrong key on `POST /v1/session` | `401` `AUTHENTICATION_FAILED` |
| Search over paths you cannot read | `403` `PERMISSION_DENIED` |
| Move to another storage or server as a non-admin | `403` `PERMISSION_DENIED` |
| Erasure record of a path does not exist | `404` `FILE_NOT_FOUND` |
| `X-CallFS-Erasure: true` on `PUT` while `erasure.enabled` is off | `501` (body `INTERNAL_ERROR`; the reason is in the server log) |

### Check what a user can do

```bash
curl -H "Authorization: Bearer your-api-key" \
  "https://files.example.com:8443/v1/permissions/check?path=/reports/q1.pdf&perm=write"
```

| Parameter | Required | Meaning |
|---|---|---|
| `path` | yes | File or folder path |
| `perm` | yes | `read`, `write` or `delete` (anything else returns `400`) |
| `user` | no | 1 to 256 characters. Check another user. Only admins can check someone other than themselves; others get `403` with code `ADMIN_REQUIRED` |

Response:

```json
{"path":"/reports/q1.pdf","perm":"write","user":"bob","allowed":false,"reason":"not_owner","owner":"alice","type":"file"}
```

`reason` values:

| Reason | Meaning |
|---|---|
| `owner` | The user owns the path |
| `admin` | The user is an admin |
| `open_directory` | A folder whose mode lets this user in (the "other" bit for the action is set) |
| `parent_open` | The path does not exist yet, and the parent folder allows creating it |
| `not_found` | The path does not exist |
| `not_owner` | The user does not own the path. `allowed` is `true` when the "other" bit of the file mode allows the action, and `false` otherwise |

### Admins

Admins bypass all permission checks and can use `/v1/admin` and the settings, users and cluster pages in the console.

- `root` and `internal-proxy` are always admins.
- Add more with `auth.admin_users`. Every listed ID must be a configured user. Listed users bypass file and folder permissions (read, write, delete) like `root`.
- Non-admins calling an admin route get `403` with `{"code":"ADMIN_REQUIRED","message":"Admin access required"}`.
- Admins see and revoke every user's download links. Other users see only their own.

```yaml
auth:
  admin_users: ["alice"]
```

Changes to `api_keys`, `api_key_users` and `admin_users` apply without a restart.

## Console sign-in and sessions

Open `https://files.example.com:8443/` and enter an access key on the sign-in page (see [Web console](08-web-console.md)). The key is the same one used for API calls. Pasting it with a leading `Bearer ` also works.

![Sign in](../docs/images/console/login.png)

You can also sign in through the API:

```bash
curl -i -X POST https://files.example.com:8443/v1/session \
  -H "Content-Type: application/json" \
  -d '{"api_key":"your-api-key"}'
```

Response (`200`):

```json
{"user_id":"alice","is_admin":true,"expires_at":"2026-01-01T18:00:00Z","csrf_token":"..."}
```

and a `Set-Cookie: callfs_session=...` header. The request body is limited to 4096 bytes (larger returns `413`). A wrong key returns `401`. Sign-in attempts are rate limited per address (`server.rate_limit.session_rps` and `session_burst`, below); excess attempts return `429` with a `Retry-After` header.

Sign out with `DELETE /v1/session` (returns `204` and clears the cookie). Signing out also ends the session on the server, so a copy of the old cookie stops working at once. In a cluster the sign-out applies on every server.

### Session settings

| Key | Type | Default | Allowed | What it does | Applies |
|---|---|---|---|---|---|
| `ui.session_ttl` | duration | `8h` | `5m` to `720h` | How long a session lasts before you sign in again. Applies to new sign-ins | live |
| `ui.idle_timeout` | duration | `1h` | `1m` up to `ui.session_ttl` | The console ends the session after this long with no use, and warns you shortly before. Sign-in and `GET /v1/me` return it as `idle_timeout_seconds` | live |

```yaml
ui:
  session_ttl: 8h
  idle_timeout: 1h
```

### The session cookie

| Property | Value |
|---|---|
| Name | `callfs_session` |
| `HttpOnly` | yes, scripts in the page cannot read it |
| `SameSite` | `Strict` |
| `Secure` | yes, whenever the connection uses TLS |
| `Path` | `/` |
| Lifetime | `ui.session_ttl` |

The cookie is signed with a key derived from `auth.internal_proxy_secret`. Anyone who changes that secret signs out every console user.

A session stops working when the key it was created with is removed or replaced, when the user is removed, when you sign out, or when it expires. After an idle sign-out the sign-in page shows a "session expired" notice.

### CSRF token

Signing in returns a `csrf_token`; `GET /v1/me` returns it again for a session. For requests that change data (any method except `GET`, `HEAD` and `OPTIONS`) that use the cookie, send the token in a header:

```bash
curl -X DELETE https://files.example.com:8443/v1/files/old.txt \
  -b "callfs_session=<cookie>" \
  -H "X-CallFS-CSRF: <csrf_token>"
```

A missing or wrong token returns `403` with `{"code":"CSRF_FAILED","message":"csrf token missing or invalid"}`. The console adds the header for you. Requests that use `Authorization: Bearer` do not need it.

### Failure reasons

Failed authentication on `/v1` is counted by reason in the Prometheus counter `callfs_auth_failures_total{reason}` and listed on the console **Sign-ins** page ([Monitoring](06-monitoring-metrics.md)).

| Reason | Cause |
|---|---|
| `missing_key` | No `Authorization` header and no session cookie |
| `wrong_key` | Key not known, or session cookie invalid, or its key was removed |
| `expired_session` | Session older than `ui.session_ttl`, or signed out |
| `csrf` | Missing or wrong `X-CallFS-CSRF` |
| `bad_delegation` | A bad `X-CallFS-On-Behalf-Of` header (see below) |
| `blocked` | Rejected because the address made too many wrong attempts |

## Internal proxy secret

Servers in a cluster trust each other with one shared secret.

```yaml
auth:
  internal_proxy_secret: "node-shared-secret-at-least-32-chars"
```

| Rule | Detail |
|---|---|
| Required | Must be set |
| Length | At least 16 characters. Use 32 or more |
| Placeholders | `change-me-internal-secret` and values that contain `your-`, `change-me` or `a-strong-` are rejected |
| Reuse | Must differ from `auth.single_use_link_secret` and from every API key |
| Cluster | Must be identical on every node |
| Changing it | Needs a restart. It also signs out every console session |

Another server calls your API with this secret as the Bearer key. That identity is `internal-proxy` and it is an admin. It can send `X-CallFS-On-Behalf-Of: <user id>` to act as a named user; the ID must be 1 to 256 characters with no spaces or control characters, and cannot be `internal-proxy`. Anything else returns `401`.

When one server forwards a call to another, the target endpoint must use `https`. Servers check each other's certificates. If your servers use certificates from a private authority, point `backend.internal_proxy_ca_file` at that authority's PEM file; it is trusted in addition to the system authorities. It applies to forwarded requests, erasure pieces, the node gateway and server health probes, and needs a restart. A missing file or a file with no PEM certificates stops startup (see [Troubleshooting](09-troubleshooting.md)).

```yaml
backend:
  internal_proxy_ca_file: "/etc/callfs/tls/internal-ca.pem"
```

The setting `backend.internal_proxy_skip_tls_verify` (default off) lets servers accept unverified certificates between each other. Leave it off in production, because with it on someone on the network could read the shared secret.

Treat this secret like a root password. Do not give it to users.

## NATS cluster credentials

Cluster traffic between nodes (metadata, locks, node messages, shared sign-outs and limits) uses NATS, either the embedded server or your own (`nats.mode: external`). With the default `nats.auth.mode: derived`, CallFS derives the NATS login from `auth.internal_proxy_secret`, so there is nothing else to configure.

What to keep identical on all nodes:

- `auth.internal_proxy_secret`
- `nats.cluster_name`
- `nats.auth.mode`

Rotate the secret without downtime by setting the old value in `nats.auth.previous_secret` (at least 16 characters, different from the current one) while you roll the new value to every node. Remove it after all nodes run the new secret.

```yaml
nats:
  auth:
    mode: derived
    previous_secret: "old-internal-secret-value"
```

| `nats.auth.mode` | Meaning |
|---|---|
| `derived` (default) | Login derived from `auth.internal_proxy_secret` |
| `nkey` | Use the NKey seed in `nats.auth.nkey_seed_file` |
| `creds` | Use the credentials file in `nats.auth.creds_file`. Only with `nats.mode: external` |

With `nats.mode: external`, only `nkey` and `creds` are allowed. With the embedded server, use `derived` or `nkey`. For your own servers, `nats.external.tls_ca_file` names the authority that signed their certificates.

Encrypt node-to-node traffic with `nats.tls.cert_file`, `nats.tls.key_file` and `nats.tls.ca_file`. With a CA file, servers verify each other's certificates in both directions. When `nats.cluster_listen` is not a loopback address (for example `10.0.0.5:6222`), all three are required. When `nats.client_listen` is not a loopback address, `nats.tls.cert_file` and `nats.tls.key_file` are required. Set `nats.tls.client_cert_file` and `nats.tls.client_key_file` together when the server's own connection to NATS must present a certificate. Use `tls-route://` in `nats.routes` for encrypted routes. All NATS TLS keys need a restart.

```yaml
nats:
  cluster_listen: "10.0.0.5:6222"
  tls:
    cert_file: "/etc/callfs/tls/nats.pem"
    key_file: "/etc/callfs/tls/nats.key"
    ca_file: "/etc/callfs/tls/cluster-ca.pem"
```

If the cluster is reachable by other servers and no `nats.tls.cert_file` is set (or `nats.external.urls` do not use `tls://` and have no `nats.external.tls_ca_file`), the console shows the finding "Cluster messaging is not encrypted": file index changes and locks travel in the clear. The finding links to **Settings > Coordination**. See [Clustering](07-clustering.md) for the rest of the NATS settings.

## TLS

```yaml
server:
  protocol: https
  cert_file: "/etc/callfs/tls/fullchain.pem"
  key_file: "/etc/callfs/tls/privkey.pem"
```

| Key | Default | What it does |
|---|---|---|
| `server.protocol` | `https` | `https`, `http` or `auto` (HTTPS when the certificate files exist, otherwise plain HTTP) |
| `server.cert_file` | `server.crt` | Certificate (PEM, include the chain) |
| `server.key_file` | `server.key` | Private key that matches the certificate |
| `server.enable_quic` | `false` | Also answer HTTP/3 over UDP |
| `server.quic_listen_addr` | `:8443` | Host and UDP port for HTTP/3 |

- With `protocol: https`, both files are required; the server refuses to start without them.
- With `protocol: http`, access keys travel in the clear. Use it only behind a proxy that does TLS or on a trusted local network.
- HTTP/3 needs the certificate files and an open UDP port.
- Connections need TLS 1.2 or newer.
- All of these keys need a restart.
- The console **Settings** page shows the certificate expiry date.

For production, use a certificate from a trusted authority such as Let's Encrypt.

## Download links

A download link gives one person one download of one file without sharing an API key.

```bash
curl -X POST https://files.example.com:8443/v1/links/generate \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"path":"/reports/q1.pdf","expiry_seconds":3600}'
```

```json
{"id":42,"url":"https://files.example.com:8443/download/TOKEN","token":"TOKEN","expires":"2026-01-01T13:00:00Z"}
```

Security properties:

| Property | Behavior |
|---|---|
| Signed | The token is signed with `auth.single_use_link_secret`; a changed token is rejected |
| Expiry | `expiry_seconds` from 1 to 86400 (24 hours). Anything else returns `400` |
| One use | The first successful download uses the link. Later opens return `410` |
| Bound to a path | The link works only for the file it was created for |
| Files only | Links for folders return `400` |
| Needs read access | You can create a link only for a file you can read (otherwise `403`) |
| Owner limit | Each user can have `server.rate_limit.max_active_links_per_user` active links (default 1000; `0` also means 1000). More returns `429` with `too many active download links` |
| Rate limited | Creating links and opening links have their own limits (below) |
| Revocable | The owner or an admin can revoke an active link with `DELETE /v1/links/{id}` (`409` if it is not active) |
| Visibility | `GET /v1/links` shows your own links. Admins see every user's links |
| Statistics | `GET /v1/links/stats` returns counts for your own links. Admins get counts for all links in the cluster. It also reports when the cleanup of old links last ran (`last_cleanup`: time, removed expired, removed used, error) |

Opening a link in a browser (`GET /download/{token}`) shows a confirm page ("This link can be used only once"). The download happens on `POST /download/{token}`, so link previews and scanners that only fetch the page do not use up the link. Unknown tokens return `404`; used or expired tokens return `410`.

The link secret:

```yaml
auth:
  single_use_link_secret: "link-signing-secret-at-least-32-chars"
```

- At least 16 characters (use 32 or more).
- Must not be a placeholder (`change-me-link-secret`, or a value with `your-`, `change-me`, `a-strong-`).
- Must differ from `auth.internal_proxy_secret` and from every API key.
- Needs a restart. Changing it stops all active links.
- Must be identical on every node so any node can open a link.

## Rate limits

Limits apply per client address (IPv6 clients are grouped by /64 network). When a client goes over a limit, CallFS returns `429` with a JSON error and a `Retry-After` header.

| Key | Type | Default | Allowed | Limits |
|---|---|---|---|---|
| `server.rate_limit.session_rps` | number | `1` | above 0 | Sign-in attempts per second |
| `server.rate_limit.session_burst` | integer | `5` | 1 or more | Sign-in burst |
| `server.rate_limit.link_generate_rps` | number | `100` | above 0 | Link creations per second |
| `server.rate_limit.link_generate_burst` | integer | `100` | 1 or more | Link creation burst |
| `server.rate_limit.download_rps` | number | `10` | above 0 | Download link opens per second |
| `server.rate_limit.download_burst` | integer | `5` | 1 or more | Download open burst |
| `server.rate_limit.max_active_links_per_user` | integer | `1000` | 0 or more (`0` means 1000) | Active links per user (restart needed) |

```yaml
server:
  rate_limit:
    session_rps: 1
    session_burst: 5
    link_generate_rps: 100
    link_generate_burst: 100
    download_rps: 10
    download_burst: 5
```

The `*_rps` and `*_burst` keys change live from the console **Settings** page and from the **Cluster > Limits** page, which also shows recent rejections and the busiest client addresses. Wrong-key throttling (above) is separate and fixed. With `nats.counters: true` (restart needed) the limits are counted across all servers instead of per server (see [Clustering](07-clustering.md)).

## Security headers

CallFS adds these headers to every `/v1` response:

| Header | Value |
|---|---|
| `Content-Security-Policy` | `default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'self'; form-action 'self'` |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Permissions-Policy` | geolocation, microphone, camera, payment, usb, magnetometer, gyroscope and accelerometer all disabled |
| `Cross-Origin-Opener-Policy` | `same-origin` |
| `Cross-Origin-Embedder-Policy` | `require-corp` |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains; preload` (sent only on TLS connections) |

## Path security

Every path you send is cleaned before use.

- `.` and empty parts are dropped and `//` is collapsed.
- A path that climbs above the root with `..` (for example `/../etc/passwd` or `a/../../b`) is rejected with an error. It is never rewritten to `/`.
- Windows-style absolute paths are rejected.
- Paths cannot escape the storage root, including through symbolic links.
- Paths in download links and in owner transfers (`prefix`) must not contain `..`.

## Audit and monitoring

- **Sign-ins:** every failed authentication is recorded with time, client address, reason and path. The console **Activity > Sign-ins** page shows the last 24 hours, with counts of wrong keys, client addresses and active users. `GET /v1/admin/auth/failures` returns the same list (`since` as an RFC 3339 time, `limit`). See [Admin API](10-admin-api.md).
- **Audit log:** sign-ins (`session` operations), file operations, link creation and opening, and admin actions are recorded with user, route, status and bytes on **Activity > Audit**.
- **Server log:** sign-ins, key replacement, user creation, user removal and admin changes are logged with the acting user.
- **Metrics:** `callfs_auth_failures_total{reason}` counts failed authentications.
- **Health finding:** "Many failed sign-ins" appears when more than 100 sign-ins fail in 10 minutes.

## Manage users and keys in the console

Open **Settings > Access** (admins only). The page lists every user with admin status, key hint, files owned, last sign-in, requests in the last 24 hours and failed sign-ins.

![Users and keys](../docs/images/console/settings-access.png)

| Action | What it does |
|---|---|
| **Add user** | Enter a user ID and choose Admin or not. CallFS creates the user and shows the new key once. Copy it then. |
| **Replace key** | Creates a new key and stops the old one at once. The new key is shown once. Open console sessions of that user end |
| **Make admin / Remove admin** | Changes admin access. Works for users that have their own key. `root` is always an admin. The last admin cannot be removed (`409`) |
| **Transfer files** | Gives all files of this user (or those under a folder; `/` means all) to another user. Content does not change, only the owner. It runs as a job you can follow on **Activity > Jobs** |
| **Remove user** | Deletes the user and the key. Their files keep the old owner ID. The last admin cannot be removed |
| **Fix this** (numbered keys) | Gives each numbered key its own user ID and, by default, transfers the files that numbered identity owned to the new user |

Changes are saved to the config file and take effect at once, with no restart. `root` and `internal-proxy` are shown as system identities with no key to manage. Files with no owner are flagged, and you can transfer them to a user.

![Security settings](../docs/images/console/settings-security.png)

The same actions exist in the Admin API: `GET/POST /v1/admin/users`, `PATCH/DELETE /v1/admin/users/{id}`, `POST /v1/admin/users/{id}/rotate`, `POST /v1/admin/users/migrate-positional`, `GET /v1/admin/owners` and `POST /v1/admin/owners/reassign` (see [Admin API](10-admin-api.md)).

## Deployment checklist

- Use a different random value of 32 or more characters for each key, `auth.internal_proxy_secret` and `auth.single_use_link_secret`.
- Keep secrets out of the config file you share. Set them with environment variables (`CALLFS_AUTH__INTERNAL_PROXY_SECRET`, `CALLFS_AUTH__SINGLE_USE_LINK_SECRET`, `CALLFS_AUTH__API_KEY_USERS__<USER>`) or a secrets manager.
- Run with `server.protocol: https`.
- Give every person their own user and key, and use `api_key_users` rather than `api_keys`.
- Make only the people who manage the server admins.
- Open only the API port (default `8443`) and, for clusters, the NATS ports between nodes (for example `6222`). The metrics port (default `:9090`) has no sign-in, so bind `metrics.listen_addr` to a private address or firewall it.
- On local storage, make the data directory (for example `/var/lib/callfs`) readable only by the user that runs CallFS. On S3, give CallFS a role limited to `s3:GetObject`, `s3:PutObject`, `s3:DeleteObject` and `s3:ListBucket` on its bucket, and turn on server-side encryption.
- Watch **Activity > Sign-ins** and `callfs_auth_failures_total` for bad keys.
