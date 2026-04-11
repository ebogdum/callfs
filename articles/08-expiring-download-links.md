# How to Generate Expiring Download Links for Your Files

Sharing a file securely with someone outside your organization is a problem that most teams solve badly. A shared drive link sent by email has no expiry. A zip file attached to a message sits in someone's inbox indefinitely. An FTP credential handed to a contractor stays active long after the work is done.

The correct model is the temporary download link: a URL that works exactly once, expires on a schedule, and cannot be reused or redirected to a different file. This tutorial walks through how that model works, why it matters, and how to implement it end-to-end using [CallFS](https://github.com/ebogdum/callfs).

---

## When You Need a Temporary or Single-Use Download Link

The use cases are more common than they appear:

**Sharing a contract with a client.** You have finalized an agreement and need to send a PDF to the counterparty for review. Attaching it to an email means a copy exists in mail servers, backups, and forwarded threads forever. A temporary download link that expires after the client downloads it keeps the document contained.

**Sending a build artifact to QA.** Your CI pipeline produces a release candidate and you need QA to test it. A permanent link to the artifact on your file server means anyone with the URL can access it indefinitely, including previous builds that may no longer be valid. A single-use expiring link scoped to the exact artifact solves this without credential management.

**Providing a time-limited download for a purchase.** A customer buys software, a dataset, or a digital asset. They should be able to download it, but the link should not work for a third party who obtains the URL through forwarding or sharing.

**Sharing sensitive documents.** Financial records, medical files, legal documents, or personnel data sometimes need to move between parties who do not share a system. A link that expires in minutes and works once provides a narrow, auditable window of access.

In all of these cases, the common requirement is the same: give the recipient access to a specific file for a specific window of time, and then make that access impossible to reuse or transfer.

---

## The Security Model

Before looking at implementation, it is worth understanding what a well-designed temporary download link actually guarantees.

### HMAC-SHA256 Signed Tokens

A token is not a password. It is a cryptographic proof that the server generated it for a specific file. In CallFS, each token is composed of two parts joined by a period:

```
<tokenID>.<signature>
```

The `tokenID` is 16 bytes of cryptographically random data, base64-encoded. The `signature` is an HMAC-SHA256 computed over the concatenation of the token ID and the target file path, using a secret key known only to the server.

When a download request arrives, the server re-derives the expected signature from the token ID and the stored file path, then compares it to the provided signature using a constant-time comparison. If the signature does not match, the request is rejected immediately — no database lookup required to detect a forged token.

### Time-Limited

Every token has an expiry timestamp stored server-side at generation time. The server checks this timestamp on every download attempt. A token that has passed its expiry returns HTTP 410 Gone, regardless of whether it has been used. The maximum allowed expiry is 86,400 seconds (24 hours).

### Single-Use (Consumed After First Download)

Tokens have a status field that starts as `active`. When a token is consumed, the server performs an atomic conditional update: the status transitions to `used` only if it is currently `active`. If two requests arrive simultaneously for the same token, only one can win the update. The other sees the token in a non-active state and is rejected.

This is the critical distinction from time-limited-only links. A time-limited link with a one-hour expiry can be forwarded to ten people and downloaded ten times within that hour. A single-use link can be downloaded exactly once, regardless of who has the URL or when.

### Path-Bound

The HMAC signature is computed over the combination of token ID and file path. A token generated for `/contracts/acme-2026.pdf` cannot be used to download `/contracts/acme-2027.pdf`, even if an attacker knows the token ID. Substituting a different path invalidates the signature.

---

## Full Walkthrough with CallFS

### Step 1: Configure the Link Secret

Before generating any links, set the secret key used to sign tokens. In `config.yaml`:

```yaml
auth:
  api_keys:
    - "your-api-key-here-at-least-16-chars"
  internal_proxy_secret: "your-internal-secret-here-16ch"
  single_use_link_secret: "your-link-secret-here-16chars"

server:
  listen_addr: ":8443"
  protocol: "https"
  external_url: "files.example.com"
  cert_file: "server.crt"
  key_file: "server.key"
```

The `auth.single_use_link_secret` value is hashed with SHA-256 before being used as the HMAC key, so its byte length is not a constraint — but it must not be empty and must not be the default placeholder value. The server will refuse to start with an insecure default.

The `server.external_url` value controls the hostname embedded in generated link URLs. Set this to your public-facing domain.

You can also supply the secret via environment variable to avoid storing it in the config file:

```bash
export CALLFS_AUTH__SINGLE_USE_LINK_SECRET="your-link-secret-here-16chars"
```

### Step 2: Upload the File

Upload requires authentication. Pass your API key as a Bearer token:

```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @contract.pdf \
  https://files.example.com/v1/files/contracts/acme-2026.pdf
```

The server records the uploading identity as the file owner in metadata. The file is now stored at `/contracts/acme-2026.pdf` on the server.

### Step 3: Generate a Single-Use Link

Link generation also requires authentication. You are generating a capability to share, not performing the share itself:

```bash
curl -X POST \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"path":"/contracts/acme-2026.pdf","expiry_seconds":3600}' \
  https://files.example.com/v1/links/generate
```

The server generates the token, stores it with the file path and expiry, and returns:

```json
{
  "url": "https://files.example.com/download/TOKEN",
  "token": "TOKEN",
  "expires": "2026-04-11T09:00:00Z"
}
```

The `url` field is the complete download URL to share with the recipient. The `token` field is the raw token if you need to construct the URL yourself. The `expires` field is the UTC timestamp after which the link will no longer work.

The `expiry_seconds` value must be between 1 and 86400. Requests outside this range are rejected with HTTP 400.

### Step 4: Share the URL with the Recipient

The recipient does not need an account or an API key. They download the file directly from the URL:

```bash
curl -O https://files.example.com/download/TOKEN
```

The server validates the token, checks that it has not expired and has not been used, atomically marks it as consumed, and streams the file. The response includes a `Content-Disposition` header with the original filename, so download managers and browsers save the file with the correct name.

### Step 5: Second Download Attempt Fails

The token is now consumed. Any subsequent request with the same URL:

```bash
curl -O https://files.example.com/download/TOKEN
# HTTP 410 Gone
```

Returns HTTP 410 Gone. This applies whether the second request comes from the same IP, the same browser, or anyone else who obtained the URL through forwarding.

---

## Automating Link Generation with Python

For workflows where link generation is part of a larger pipeline — for example, generating a link after a build completes and emailing it to QA — a short Python script handles this cleanly:

```python
import smtplib
import ssl
from email.message import EmailMessage
from datetime import datetime, timezone

import httpx


CALLFS_BASE_URL = "https://files.example.com"
API_KEY = "your-api-key-here"

SMTP_HOST = "smtp.example.com"
SMTP_PORT = 587
SMTP_USER = "notifications@example.com"
SMTP_PASSWORD = "smtp-password"


def generate_download_link(file_path: str, expiry_seconds: int) -> dict:
    response = httpx.post(
        f"{CALLFS_BASE_URL}/v1/links/generate",
        headers={
            "Authorization": f"Bearer {API_KEY}",
            "Content-Type": "application/json",
        },
        json={"path": file_path, "expiry_seconds": expiry_seconds},
        timeout=10.0,
    )
    response.raise_for_status()
    return response.json()


def send_download_email(
    recipient: str,
    subject: str,
    file_path: str,
    download_url: str,
    expires: str,
) -> None:
    expires_dt = datetime.fromisoformat(expires.replace("Z", "+00:00"))
    expires_formatted = expires_dt.strftime("%B %d, %Y at %H:%M UTC")

    body = f"""Hello,

A file is available for you to download:

  File: {file_path}
  Download link: {download_url}
  Link expires: {expires_formatted}

This link can only be used once. If you need to download the file again,
please request a new link.
"""

    message = EmailMessage()
    message["Subject"] = subject
    message["From"] = SMTP_USER
    message["To"] = recipient
    message.set_content(body)

    context = ssl.create_default_context()
    with smtplib.SMTP(SMTP_HOST, SMTP_PORT) as server:
        server.starttls(context=context)
        server.login(SMTP_USER, SMTP_PASSWORD)
        server.send_message(message)


def share_file(file_path: str, recipient_email: str, expiry_seconds: int = 3600) -> None:
    link_data = generate_download_link(file_path, expiry_seconds)

    send_download_email(
        recipient=recipient_email,
        subject=f"Your download is ready: {file_path.split('/')[-1]}",
        file_path=file_path,
        download_url=link_data["url"],
        expires=link_data["expires"],
    )

    print(f"Link sent to {recipient_email}")
    print(f"URL: {link_data['url']}")
    print(f"Expires: {link_data['expires']}")


if __name__ == "__main__":
    share_file(
        file_path="/contracts/acme-2026.pdf",
        recipient_email="legal@acme.example.com",
        expiry_seconds=3600,
    )
```

Run this script as part of a CI/CD step, a cron job, or a post-processing hook. Replace the SMTP configuration with your mail provider's details.

---

## Rate Limits

CallFS applies per-IP rate limiting to both the generation and download endpoints to prevent abuse.

**Link generation** (`POST /v1/links/generate`): 100 requests per second with a burst of 1. This is generous for interactive use but constrains scripts that attempt to generate large volumes of links rapidly.

**Link download** (`GET /download/{token}`): 10 requests per second with a burst of 5. This protects the download endpoint from enumeration attacks where an attacker sends large numbers of random token guesses hoping for a hit.

Requests that exceed these limits receive HTTP 429 Too Many Requests. If you need higher limits for a specific use case, the values are set in `server/router.go` and can be adjusted before building.

---

## Background Cleanup

Tokens do not accumulate indefinitely. A background worker runs on a configurable interval and removes:

- **Expired active links**: tokens that passed their expiry time without being used
- **Used links older than 24 hours**: tokens that were consumed and are no longer needed for audit purposes

This keeps the metadata store from growing unboundedly and avoids the need for manual housekeeping.

---

## Comparison with Alternatives

### S3 Pre-Signed URLs

AWS S3 and compatible object stores support pre-signed URLs that grant time-limited access to a specific object. The mechanism is similar in concept: a URL with embedded credentials that expires.

The key difference is single-use behavior. S3 pre-signed URLs are time-limited but not single-use. A URL with a one-hour expiry can be downloaded as many times as the recipient wishes within that hour, and forwarded to others. If you need single-use semantics, you need to build that layer on top of S3 yourself — which requires a database to track token state, an endpoint to validate and consume tokens, and logic to proxy the download.

That is essentially what CallFS implements. If you are already using S3 as your backend, you can point CallFS at the same bucket and gain the single-use guarantee without changing your storage layer.

### Custom Token Systems

Building token-based download links from scratch is not technically difficult, but it requires getting several details right: cryptographically random token generation, constant-time signature comparison, atomic consumption to prevent race conditions, cleanup of stale tokens, and proper HTTP status codes for expired and used tokens. The surface area for mistakes is larger than it looks.

The advantage of a purpose-built implementation is that these details are already handled, tested, and auditable in the source.

### WeTransfer and Similar Services

Consumer file transfer services are convenient but introduce a third party into your data chain. The service stores your file on its infrastructure, under its terms of service and data retention policies. For regulated data — health records, financial documents, legal materials — this is often a non-starter. Transfer size limits and link expiry behaviors are controlled by the service, not by you.

Self-hosted solutions give you full control over data residency, retention, and access policy, at the cost of operating the infrastructure.

---

## Choosing an Expiry Window

The right expiry depends on the use case and the sensitivity of the file:

| Scenario | Suggested expiry |
|---|---|
| Client is waiting on a call | 300-600 seconds |
| Email delivery, same-day download | 3600-7200 seconds |
| Async delivery, multi-day window | Up to 86400 seconds |
| Compliance-sensitive documents | As short as practical |

Shorter expiry windows reduce the exposure window if the link is forwarded. Single-use behavior provides an additional constraint: even within the expiry window, the link can only be used once. The two properties together mean you can use a reasonably generous expiry (say, a few hours) without worrying that forwarding the link creates persistent access.

For compliance-sensitive documents, prefer short expiry windows and ensure that link generation events are logged. The server logs include the generating user identity, the file path, the token (truncated), and the expiry time. These logs form an audit trail demonstrating that access was time-bounded.

---

## Summary

Temporary download links solve a real and common problem: sharing a specific file with a specific person for a specific window of time, without giving them credentials or leaving a permanent access channel open.

The security model rests on three properties: HMAC-SHA256 signatures that bind a token to a specific file path and cannot be forged without the server secret, time expiry that enforces a hard deadline, and single-use consumption that prevents reuse after the first download.

CallFS implements all three properties, including the atomic consumption that prevents race conditions on simultaneous download attempts. Configuration requires one secret key (`auth.single_use_link_secret`), one API call to generate a link, and one unauthenticated HTTP request for the recipient to download the file.

The full source is available at [github.com/ebogdum/callfs](https://github.com/ebogdum/callfs).
