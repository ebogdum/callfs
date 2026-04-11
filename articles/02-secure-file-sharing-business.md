# Secure File Sharing with CallFS: API Keys, Permissions, and Expiring Links

Sharing files securely across teams or with external clients involves more than just restricting who can log in. You need to control who can delete what, audit access, and hand temporary download URLs to people who should never see your internal credentials.

CallFS addresses this with three distinct security layers: **authentication** through API keys, **authorization** through an owner-based permission model, and **single-use expiring download links** for safe external sharing. This article walks through all three in practical terms, with working configuration and curl examples.

---

## Layer 1: Authentication with API Keys

Every request to CallFS must present a valid API key in the `Authorization: Bearer` header. Keys are defined statically in the configuration file, which means you provision one key per team member, service account, or integration — no shared passwords.

### Configuring Multiple API Keys

```yaml
auth:
  api_keys:
    - "alice-key-d3f8a1b2c4e5f6a7"
    - "bob-key-9a2b3c4d5e6f7a8b"
    - "ci-pipeline-f1e2d3c4b5a6f7e8"
  internal_proxy_secret: "internal-secret-change-this-now"
  single_use_link_secret: "link-secret-change-this-too-ok"
```

A few constraints enforced at startup:

- Every key must be at least 16 characters. Shorter keys are rejected with a config validation error.
- The literal string `default-api-key` is explicitly banned — CallFS refuses to start if any key uses the default placeholder.
- The `single_use_link_secret` signs the download tokens (more on this below) and must also be set to a non-default value.

### How Keys Map to Identities

Each position in the `api_keys` list maps to a deterministic app-level identity: the first key becomes `api-user-0`, the second `api-user-1`, and so on. This identity is what the authorization layer uses to decide what the caller is allowed to do.

The mapping is positional, so if you add a key at the start of the list, every subsequent identity shifts by one. To keep identities stable, append new keys to the end of the list rather than inserting them.

Authentication uses constant-time comparison across all registered keys on every request, which prevents timing attacks that could reveal which key position matched.

### Making an Authenticated Request

```bash
# Upload a file using Alice's key
curl -X POST \
  -H "Authorization: Bearer alice-key-d3f8a1b2c4e5f6a7" \
  --data-binary @report.pdf \
  https://files.example.com/v1/files/reports/q4.pdf
```

```bash
# Download a file using Bob's key
curl -H "Authorization: Bearer bob-key-9a2b3c4d5e6f7a8b" \
  https://files.example.com/v1/files/reports/q4.pdf \
  -o q4.pdf
```

Any request without a valid key receives a `401 Unauthorized` response immediately, before any file operation is attempted.

---

## Layer 2: Authorization — The Owner Model

Authentication answers "who are you?". Authorization answers "what are you allowed to do?". CallFS uses an ownership-based model that is straightforward to reason about.

### The Rules

When a file or directory is created, the caller's app identity (e.g. `api-user-0`) is recorded as its owner. From that point:

- **All authenticated users can read any file.** If you are logged in with a valid API key, you can GET any file regardless of who created it.
- **Only the owner can write or delete a file.** If `api-user-0` uploaded `/reports/q4.pdf`, then `api-user-1` can download it but cannot overwrite or delete it.
- **All authenticated users can list directories and create files within them.** Only the directory owner can delete the directory itself.

There are no complex access control lists to maintain. The model is: creator owns, everyone reads, no one else deletes.

### Inspecting Ownership

Every file response includes an `X-CallFS-Owner` header identifying the owner's app identity. You can use a HEAD request to check ownership without downloading the file body:

```bash
curl -I \
  -H "Authorization: Bearer bob-key-9a2b3c4d5e6f7a8b" \
  https://files.example.com/v1/files/reports/q4.pdf
```

The response headers will include:

```
X-CallFS-Owner: api-user-0
X-CallFS-Size: 204800
X-CallFS-Mode: -rw-r--r--
X-CallFS-MTime: 2026-04-11T08:00:00Z
```

This is useful for automation: a CI pipeline can verify the owner of a file before deciding whether to proceed with an operation.

### What Happens on a Denied Request

If `api-user-1` attempts to delete a file owned by `api-user-0`:

```bash
curl -X DELETE \
  -H "Authorization: Bearer bob-key-9a2b3c4d5e6f7a8b" \
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

The two layers above protect your API. But sometimes you need to share a file with someone who should not have an API key — an external auditor, a client, a partner. That is what single-use expiring download links are for.

The workflow is:

1. An authenticated user generates a link via the API.
2. The link is sent to the recipient (email, Slack, wherever).
3. The recipient downloads the file using the link — no API key required.
4. The link is consumed and cannot be used again.

### Step 1: Generate a Link

```bash
curl -X POST \
  -H "Authorization: Bearer alice-key-d3f8a1b2c4e5f6a7" \
  -H "Content-Type: application/json" \
  -d '{"path":"/reports/q4.pdf","expiry_seconds":3600}' \
  https://files.example.com/v1/links/generate
```

The server responds with `201 Created`:

```json
{
  "url": "https://files.example.com/download/TOKEN",
  "token": "TOKEN",
  "expires": "2026-04-11T09:00:00Z"
}
```

The `expiry_seconds` field must be between 1 and 86400 (24 hours). Requests outside that range are rejected. The `url` field is constructed from the `server.external_url` value in your configuration, so it reflects the address your recipients will actually reach.

The caller must have read permission on the file to generate a link for it. You cannot generate a link for a file you cannot read yourself.

### Step 2: Send the Link

Pass the `url` value to the recipient. It contains no credentials, no internal identifiers, and no information about your API key or identity. The link itself is the only thing needed.

### Step 3: Download (No Auth Required)

The recipient downloads the file without any `Authorization` header:

```bash
curl -O https://files.example.com/download/TOKEN
```

The server streams the file content directly. The `Content-Disposition` header is set to trigger a file download in browsers, with the original filename preserved.

### Step 4: Link Expires and is Consumed

Once downloaded, the link status is atomically set to `used` in the metadata store. Any subsequent request for the same token receives:

```
HTTP/1.1 410 Gone
{"code":"LINK_INVALID","message":"link is invalid or has been used"}
```

If the link is accessed after the expiry time, the response is also `410 Gone` with `"link has expired"`. Expired-but-unused links are cleaned up automatically by the background cleanup process.

### How Tokens are Secured

Each token is composed of two parts joined by a dot: a 16-byte cryptographically random ID and an HMAC-SHA256 signature computed over the token ID and the file path, using the `single_use_link_secret` as the key.

This design has two important properties:

- **Path-bound**: A token generated for `/reports/q4.pdf` cannot be reused to download `/reports/q3.pdf`, even if an attacker knows the token structure. The HMAC signature covers the path, and the stored link record is verified against it on every redemption.
- **Unforgeable**: Without knowledge of the `single_use_link_secret`, generating a valid token is not feasible. Verification uses `hmac.Equal` for constant-time comparison to prevent timing attacks.

---

## Rate Limiting

Per-IP rate limiting is applied independently to link generation and file downloads.

**Link generation** (`POST /v1/links/generate`) is limited to **100 requests per second** per IP address. This limit exists to prevent token flooding — an authenticated attacker generating tokens in bulk to exhaust metadata storage.

**Link downloads** (`GET /download/{token}`) are limited to **10 requests per second** per IP address, with a burst allowance of 5. This is intentionally lower because the download endpoint is unauthenticated and therefore more exposed. Requests that exceed the limit receive:

```
HTTP/1.1 429 Too Many Requests
{"code":"RATE_LIMIT_EXCEEDED","message":"Rate limit exceeded"}
```

The per-IP limiter uses TTL-based eviction to prevent memory exhaustion — entries inactive for 10 minutes are removed, and the total number of tracked IPs is capped at 100,000.

---

## HTTPS and TLS

In production, CallFS should run with TLS enabled. The configuration is straightforward:

```yaml
server:
  listen_addr: ":8443"
  protocol: "https"
  external_url: "https://files.example.com"
  cert_file: "/etc/callfs/server.crt"
  key_file: "/etc/callfs/server.key"
```

When `protocol` is set to `https`, both `cert_file` and `key_file` are required — CallFS will refuse to start without them. On a TLS connection, the server automatically sets the `Strict-Transport-Security` header with a one-year max-age and `includeSubDomains; preload`, which instructs browsers to enforce HTTPS for all future visits.

Additional security headers are always set regardless of protocol: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Content-Security-Policy`, and `Permissions-Policy`. These apply to both API and download responses.

For development or environments where you want to avoid managing certificates, set `protocol: "http"`. This is not appropriate for any deployment where traffic crosses a network boundary.

---

## Practical Example: Sharing a Report with an External Client

Putting all three layers together, here is a complete workflow for securely delivering a file to someone outside your organization.

### 1. Upload the Report

Alice uploads the Q4 report using her API key:

```bash
curl -X POST \
  -H "Authorization: Bearer alice-key-d3f8a1b2c4e5f6a7" \
  --data-binary @q4-report.pdf \
  https://files.example.com/v1/files/reports/q4-final.pdf
```

CallFS records Alice (`api-user-0`) as the owner of `/reports/q4-final.pdf`.

### 2. Generate a Time-Limited Link

Alice generates a one-hour download link:

```bash
curl -X POST \
  -H "Authorization: Bearer alice-key-d3f8a1b2c4e5f6a7" \
  -H "Content-Type: application/json" \
  -d '{"path":"/reports/q4-final.pdf","expiry_seconds":3600}' \
  https://files.example.com/v1/links/generate
```

Response:

```json
{
  "url": "https://files.example.com/download/Zm9vYmFy....",
  "token": "Zm9vYmFy....",
  "expires": "2026-04-11T09:00:00Z"
}
```

### 3. Send the Link to the Client

Alice emails the `url` to the client. The URL contains no API key, no internal path structure beyond the token, and no authentication material.

### 4. Client Downloads the File

The client opens the URL in a browser or uses curl:

```bash
curl -O https://files.example.com/download/Zm9vYmFy....
```

The file streams immediately. The browser receives a `Content-Disposition: attachment` header and saves it as `q4-final.pdf`.

### 5. Link Expires

After the download completes, the token is marked as `used`. If the client or anyone else tries the same URL again, they receive a `410 Gone` response. If the client waits more than an hour to download, the link has expired and is also `410 Gone`.

At no point did the client have access to the CallFS API, Alice's key, or any other file in the system.

---

## Summary

CallFS provides secure file sharing for business through three concrete mechanisms:

- **API keys** authenticate every caller, with each key mapping to a distinct app identity. Keys are validated with constant-time comparison to prevent information leakage.
- **Owner-based authorization** gives creators exclusive write and delete access to their files while allowing all authenticated users to read. No ACLs to maintain.
- **Single-use expiring links** let authenticated users generate short-lived, HMAC-signed, path-bound URLs that work without API keys. Links are consumed atomically on first use and expire on a configurable timer up to 24 hours.

Together these three layers cover the two most common secure sharing scenarios: internal team access through API keys and one-time external delivery through expiring links.
