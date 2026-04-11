# How to Generate Expiring Single-Use Download Links with CallFS

Sharing a file securely is harder than it looks. Handing out an API key so a recipient can download one document means that key can be reused indefinitely, shared further, or leaked. CallFS solves this with built-in support for temporary download links: cryptographically signed, time-limited URLs that work exactly once. The recipient downloads the file without presenting any credentials, and the link is permanently consumed the moment the first byte is served. A second attempt returns HTTP 410 Gone.

This tutorial walks through every step: configuring the feature, generating a single-use download link, downloading via that link, and understanding what happens under the hood.

---

## How It Works

Every expiring download link in CallFS is an HMAC-SHA256 signed token. When you call the generation endpoint, the server performs the following steps:

1. **Generates a cryptographically secure 16-byte random token ID** using `crypto/rand`.
2. **Computes an HMAC-SHA256 signature** over the concatenation of the token ID and the target file path, keyed with the secret you configure in `auth.single_use_link_secret`. This binds the token to a specific path — a token cannot be reused against a different file.
3. **Combines the token ID and signature** into a single URL-safe string, separated by a dot: `<tokenID>.<signature>`.
4. **Persists the link record** in the metadata store with the file path, expiry time, and status `active`.
5. **Returns the full download URL** constructed from `server.external_url`, plus the token and its expiry timestamp.

When a recipient requests the download URL:

1. The server looks up the token in the metadata store.
2. It checks that the record exists, that the status is still `active`, and that the expiry time has not passed.
3. It re-verifies the HMAC signature using constant-time comparison to prevent timing attacks.
4. If all checks pass, it **atomically marks the token as `used`** via a conditional update — only succeeding if the status is still `active`. This prevents a race condition where two simultaneous requests both pass the validity check.
5. It streams the file content to the client and records the consuming IP address.

A token that has been used, tampered with, or expired always returns HTTP 410 Gone — never a generic 404 that might encourage enumeration attempts.

---

## Prerequisites

- A running CallFS instance with `auth.single_use_link_secret` and `server.external_url` configured (see the Configuration section below).
- An API key with read access to the file you want to share.
- `curl` for the shell examples. The Python section requires the `requests` library.

---

## Configuration

Two fields in `config.yaml` control the single-use link feature.

```yaml
auth:
  api_keys:
    - "your-api-key-here-at-least-16-chars"
  internal_proxy_secret: "your-internal-secret-change-me"
  single_use_link_secret: "a-long-random-secret-for-signing-links"

server:
  listen_addr: ":8443"
  protocol: "https"
  external_url: "https://files.example.com"
  cert_file: "/etc/callfs/server.crt"
  key_file: "/etc/callfs/server.key"
```

### `auth.single_use_link_secret`

This is the HMAC signing key. CallFS SHA-256-hashes it internally before use, so any length is accepted, but you should treat it like a password: generate it randomly, keep it secret, and never reuse it across environments.

The server will refuse to start if this field is empty or set to the default placeholder `change-me-link-secret`.

### `server.external_url`

This is the base URL that CallFS uses when constructing the `url` field in the link generation response. Set it to the publicly reachable address of your server, including the scheme (`https://` or `http://`). Without this, recipients receive a URL built from the listen address, which is often a local bind address that is not reachable externally.

Both fields accept environment variable overrides with the `CALLFS_` prefix and double-underscore nesting, for example:

```bash
CALLFS_AUTH__SINGLE_USE_LINK_SECRET="..." callfs server --config /etc/callfs/config.yaml
```

---

## Step-by-Step: Generating and Using a Single-Use Download Link

### Step 1 — Upload a file

You need a file in CallFS before you can generate a temporary download link for it. Upload using an authenticated POST request:

```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @agreement.pdf \
  https://files.example.com/v1/files/contracts/agreement.pdf
```

A successful upload returns HTTP 201 Created.

### Step 2 — Generate a temporary download link

Call the generation endpoint with the file path and an expiry duration in seconds. The caller must be authenticated and must have read access to the target path:

```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"path":"/contracts/agreement.pdf","expiry_seconds":3600}' \
  https://files.example.com/v1/links/generate
```

The server validates that `expiry_seconds` is between 1 and 86400 (one second to twenty-four hours), signs the token, persists the link record, and responds with HTTP 201:

```json
{
  "url": "https://files.example.com/download/TOKEN",
  "token": "TOKEN",
  "expires": "2026-04-11T09:00:00Z"
}
```

The `url` field is the complete, ready-to-use download link. The `token` field carries the same value embedded in the URL, which is useful if you need to construct the URL yourself or log just the token for auditing. The `expires` field is the UTC timestamp after which the link will no longer work, even if it has never been used.

### Step 3 — Share the link with the recipient

Send the `url` to whoever needs the file — via email, a ticket system, a chat message, or any other channel. The recipient does not need an account or API key. The URL itself is the credential.

### Step 4 — Recipient downloads the file

The recipient requests the URL directly. No `Authorization` header is required:

```bash
curl -O https://files.example.com/download/TOKEN
```

The `-O` flag tells curl to save the file using the server-provided filename from the `Content-Disposition` header. The server streams the file content and marks the token `used` in the same atomic transaction.

### Step 5 — Second attempt returns HTTP 410 Gone

Any further request to the same URL is rejected:

```bash
curl -v https://files.example.com/download/TOKEN
# HTTP/2 410
# {"error":"link is invalid or has been used"}
```

This response is returned for tokens that have been used, tokens whose signature does not match (indicating tampering), and tokens past their expiry time.

---

## Expiry Limits

The server enforces a hard ceiling on expiry:

| Minimum | Maximum |
|---------|---------|
| 1 second | 86400 seconds (24 hours) |

A request with `expiry_seconds` outside this range is rejected immediately with HTTP 400 and an error message explaining the constraint. There is no server-side default — you must always specify the expiry explicitly.

Choose the shortest expiry that is practical for your use case. A contract link sent by email can often be set to 3600 seconds (one hour). A link embedded in an automated build notification might be set to 300 seconds if delivery is near-instantaneous.

---

## Rate Limiting

CallFS applies per-IP rate limiting independently to the generation and download endpoints:

| Endpoint | Rate limit |
|----------|-----------|
| `POST /v1/links/generate` | 100 requests per second |
| `GET /download/{token}` | 10 requests per second, burst of 5 |

When a limit is exceeded, the server returns HTTP 429 Too Many Requests with a JSON body:

```json
{"code":"RATE_LIMIT_EXCEEDED","message":"Rate limit exceeded"}
```

The tighter limit on the download endpoint prevents automated token enumeration even if an attacker guesses the token format.

---

## Python Example: Generate a Link and Email It

The following script generates a single-use download link for a file and sends it to a recipient by email using Python's standard library. Install the `requests` package if you do not already have it (`pip install requests`).

```python
import smtplib
import requests
from email.message import EmailMessage

CALLFS_API = "https://files.example.com"
API_KEY = "your-api-key-here"

SMTP_HOST = "smtp.example.com"
SMTP_PORT = 587
SMTP_USER = "sender@example.com"
SMTP_PASSWORD = "smtp-password"


def generate_download_link(file_path: str, expiry_seconds: int) -> dict:
    response = requests.post(
        f"{CALLFS_API}/v1/links/generate",
        headers={
            "Authorization": f"Bearer {API_KEY}",
            "Content-Type": "application/json",
        },
        json={"path": file_path, "expiry_seconds": expiry_seconds},
        timeout=10,
    )
    response.raise_for_status()
    return response.json()


def send_link_email(recipient: str, file_name: str, link_data: dict) -> None:
    msg = EmailMessage()
    msg["Subject"] = f"Your download link for {file_name}"
    msg["From"] = SMTP_USER
    msg["To"] = recipient
    msg.set_content(
        f"Your download link is ready.\n\n"
        f"File: {file_name}\n"
        f"Link: {link_data['url']}\n"
        f"Expires: {link_data['expires']}\n\n"
        f"This link can only be used once. It expires after the time shown above."
    )

    with smtplib.SMTP(SMTP_HOST, SMTP_PORT) as server:
        server.starttls()
        server.login(SMTP_USER, SMTP_PASSWORD)
        server.send_message(msg)


def main() -> None:
    file_path = "/contracts/agreement.pdf"
    recipient_email = "client@example.com"
    expiry_seconds = 3600  # 1 hour

    link_data = generate_download_link(file_path, expiry_seconds)
    send_link_email(recipient_email, "agreement.pdf", link_data)
    print(f"Link sent to {recipient_email}")
    print(f"URL: {link_data['url']}")
    print(f"Expires: {link_data['expires']}")


if __name__ == "__main__":
    main()
```

Replace the constants at the top with your actual values. The script requests a link with a one-hour expiry and delivers it immediately by email. Because the link is single-use, forwarding the email does not grant permanent access — only whoever clicks first gets the file.

---

## Background Cleanup

CallFS runs a background worker that periodically removes link records from the metadata store. The worker applies two separate retention policies:

- **Expired active links** — links that were never used but whose expiry time has passed are purged immediately on the next cleanup cycle. There is no grace period.
- **Used links** — links with status `used` are retained for 24 hours after consumption, then removed. The retention window exists so that audit logs and access records (including the consuming IP address) are available for a reasonable investigation period.

The cleanup worker runs on a configurable interval and respects graceful shutdown signals — it will not interrupt a cleanup run mid-way when the server is stopping.

No configuration is required to enable cleanup; it starts automatically alongside the server.

---

## Use Cases

Single-use expiring download links are well-suited to any workflow where you need to deliver a file to a specific person without granting that person ongoing access to your file server.

**Contracts and legal documents** — generate a link for a signed PDF and send it to the counterparty. The link expires after a few hours, and you have a record of when it was downloaded and from which IP address.

**Build artifacts and release packages** — your CI pipeline uploads a build artifact to CallFS and generates a short-lived link to post in a deployment ticket. Engineers download once; the link cannot be reused.

**Invoices and financial documents** — accounting software generates a link for each invoice and includes it in the outgoing email. Recipients download their copy; the server-side record confirms delivery.

**Onboarding documents** — HR systems generate per-person links for onboarding packets. Each new hire receives a unique URL that expires after 24 hours, preventing forwarding to unauthorized parties.

**Sensitive data exports** — data exports generated on demand can be placed behind single-use links with short expiry windows, reducing the risk of stale sensitive data remaining accessible.

In all of these cases, the key property is the same: access is scoped to one download, one moment in time, and one cryptographically verified token. No API key is distributed beyond your own systems.
