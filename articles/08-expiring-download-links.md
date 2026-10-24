# How to Generate Expiring Single-Use Download Links with CallFS

Sharing a file securely is harder than it looks. Handing out an API key so a recipient can download one document means that key can be reused indefinitely, shared further, or leaked. CallFS solves this with built-in temporary download links: signed, time-limited URLs that work exactly once. The recipient downloads the file without any credentials, and the link is used up the moment the download starts. A second attempt returns HTTP 410 Gone.

This tutorial covers CallFS 2.0.0 end to end: configuring the feature, creating a link with the API or the web console, downloading through the link, managing and revoking links, and the limits that apply.

---

## How It Works

1. You (or your application) ask CallFS for a link to one file and choose how long it stays valid.
2. CallFS returns a URL that points at `server.external_url`. The link is signed with your `auth.single_use_link_secret`, so a changed or invented token is rejected, and it is bound to that one path.
3. You send the URL to the recipient by email, chat or ticket.
4. The recipient opens the URL. A browser shows a confirmation page ("This link can be used only once") with a **Download file** button. Pressing it starts the download and uses up the link.
5. Any later attempt, or any attempt after the expiry time, is refused.

The confirmation page protects the link from email scanners, chat previews and antivirus tools. They fetch the page but never press the button, so the link is not consumed by accident. The download itself is a `POST` to the same URL.

| Situation | Answer |
|-----------|--------|
| First download before expiry | `200` with the file |
| Link already used | `410` `LINK_INVALID` (`link is invalid or has been used`) |
| Link past its expiry time | `410` `LINK_EXPIRED` (`link has expired`) |
| Revoked link | `410` `LINK_INVALID` |
| Token CallFS does not know | `404` `LINK_NOT_FOUND` |
| Too many requests from one address | `429` `RATE_LIMIT_EXCEEDED` |

---

## Prerequisites

- A running CallFS 2.0.0 server with `auth.single_use_link_secret` and `server.external_url` set (see Configuration below).
- An access key whose user can read the file you want to share.
- `curl` for the shell examples. The Python section needs the `requests` library.

---

## Configuration

```yaml
server:
  listen_addr: ":8443"
  protocol: "https"
  external_url: "https://files.example.com"
  cert_file: "/etc/callfs/server.crt"
  key_file: "/etc/callfs/server.key"

auth:
  api_key_users:
    alice: "REPLACE_WITH_RANDOM_API_KEY_1"
  internal_proxy_secret: "REPLACE_WITH_RANDOM_CLUSTER_SECRET"
  single_use_link_secret: "REPLACE_WITH_RANDOM_LINK_SECRET"
```

### `auth.single_use_link_secret`

The key that signs download links. Generate it with `openssl rand -hex 32`.

- At least 16 characters (use 32 or more).
- Must not be the default `change-me-link-secret` or contain `your-`, `change-me` or `a-strong-`. Startup fails with `auth.single_use_link_secret must be set and not use default value`.
- Must differ from `auth.internal_proxy_secret` and from every API key.
- Changing it needs a restart and stops every active link.
- In a cluster it must be identical on every node, so a link opens on any node.

### `server.external_url`

The base address CallFS puts in the `url` field of a new link. Set it to the address your recipients can reach, with the scheme, for example `https://files.example.com`. If you leave the default `localhost:8443`, links point at localhost and nobody else can open them.

You can also set both from the environment: `CALLFS_AUTH__SINGLE_USE_LINK_SECRET` and `CALLFS_SERVER__EXTERNAL_URL`.

---

## Step by Step

### Step 1: Upload a file

```bash
curl -X PUT \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @agreement.pdf \
  https://files.example.com/v1/files/contracts/agreement.pdf
```

`PUT` creates the file (`201 Created`) or replaces it (`200 OK`). Missing folders are created for you.

### Step 2: Create the link

Send the file path and the lifetime in seconds. The caller must be signed in and have read permission on the file:

```bash
curl -X POST \
  -H "Authorization: Bearer your-api-key" \
  -H "Content-Type: application/json" \
  -d '{"path":"/contracts/agreement.pdf","expiry_seconds":3600}' \
  https://files.example.com/v1/links/generate
```

CallFS answers `201`:

```json
{
  "id": 42,
  "url": "https://files.example.com/download/TOKEN",
  "token": "TOKEN",
  "expires": "2026-04-11T09:00:00Z"
}
```

- `url` is the complete link to send.
- `id` is the number you use to look up or revoke the link.
- `token` is the same value that is inside the URL.
- `expires` is the UTC time after which the link no longer works, even if it was never used.

Errors you can get: `400` when the body is not valid JSON, `path` is missing or invalid, the path is a folder (`links work for files only`) or `expiry_seconds` is outside the range; `401` without a valid key; `403` when you cannot read the file; `429` when you already have the maximum number of active links.

### Step 3: Share the link

Send the `url`. The recipient needs no account and no key. Anyone who has the URL can use it once, so send it over a channel you trust.

### Step 4: The recipient downloads the file

In a browser, the recipient opens the URL and presses **Download file**. From a script, the download is a `POST` and does not need an `Authorization` header:

```bash
curl -X POST -OJ https://files.example.com/download/TOKEN
```

`-OJ` saves the file under the name CallFS sends in the `Content-Disposition` header (`agreement.pdf`).

### Step 5: A second attempt fails

```bash
curl -i -X POST https://files.example.com/download/TOKEN
# HTTP/2 410
# {"code":"LINK_INVALID","message":"link is invalid or has been used"}
```

A plain `GET` of the URL after use also answers `410`, so a recipient who reopens an old link sees an error instead of the confirmation page.

---

## Expiry Limits

| Minimum | Maximum |
|---------|---------|
| 1 second | 86400 seconds (24 hours) |

A value outside the range returns `400` (`expiry must be between 1 and 86400 seconds`). There is no default: always send `expiry_seconds`.

Pick the shortest lifetime that works. A contract sent by email can use 3600 seconds. A link posted in a build notification can use 300.

---

## Managing Links

### In the web console

Open **Sharing** in the console (see [Web console](../docs_markdown/08-web-console.md)). The console is part of the Docker image and of source builds; the release binaries contain the API only.

- **New download link** asks for the file path and the lifetime, with a **Check file** button that tells you if the path is a folder.
- The tabs **Active**, **Used**, **Expired**, **Revoked** and **All** filter the list. Columns show the file, status, who created the link, when it was created, when it expires, when it was used and from which address.
- Numbers at the top show active links, links used today and failed opens in the last 24 hours, split by reason (expired, not found, invalid).
- Select a link to see its history: created, each open attempt (success, expired, invalid), used or revoked.
- **Revoke link** (or a bulk revoke) cancels an active link at once. Only active links can be revoked.
- **Clean up old links now** runs the cleanup described below on demand.

### With the API

| Request | What it does |
|---------|--------------|
| `GET /v1/links` | Lists your links with `counts` of active, used, expired and revoked. Query parameters: `status` (`active`, `used`, `expired`, `revoked`), `path`, `created_by`, `limit` (1 to 1000, default 200) and `cursor` (use `next_cursor` from the previous page). |
| `GET /v1/links/{id}` | One link: `id`, `token_prefix`, `file_path`, `status`, `created_by`, `created_at`, `expires_at`, `used_at`, `used_by_ip`, and the list of open attempts. |
| `DELETE /v1/links/{id}` | Revokes an active link and returns `{"id":42,"status":"revoked"}`. A link that is not active returns `409` (`link is not active`). |
| `GET /v1/links/stats?window=1h\|24h\|7d` | Links generated, links consumed by result (`success`, `not_found`, `expired`, `invalid`), active links, links used in the last 24 hours and the last cleanup report. Default window is `24h`. An admin gets cluster-wide counts; any other user gets counts for their own links only. The cleanup report is `last_cleanup` with `at`, `expired_removed`, `used_removed` and `error`. |

A user sees only their own links. An admin (a user in `auth.admin_users`) sees and revokes every user's links. Other users' links return `404`.

```bash
curl -s -H "Authorization: Bearer your-api-key" \
  "https://files.example.com/v1/links?status=active&limit=50"

curl -s -X DELETE -H "Authorization: Bearer your-api-key" \
  https://files.example.com/v1/links/42
```

---

## Limits and Rate Limiting

Creating and opening links each have a per-address rate limit, and each user has a cap on active links. All values can be changed in `config.yaml` or in the console under **Settings**.

| Setting | Default | What it limits |
|---------|---------|----------------|
| `server.rate_limit.link_generate_rps` | `100` | Link creations per second, per address (`POST /v1/links/generate`). |
| `server.rate_limit.link_generate_burst` | `100` | Short bursts above that rate. |
| `server.rate_limit.download_rps` | `10` | Link opens per second, per address (`GET` and `POST /download/{token}`). |
| `server.rate_limit.download_burst` | `5` | Short bursts above that rate. |
| `server.rate_limit.max_active_links_per_user` | `1000` | Active links per user. Another request returns `429`. `0` means the default of 1000. Needs a restart. |

The rate settings apply without a restart. When a limit is hit, CallFS returns `429` with a `Retry-After` header and:

```json
{"code":"RATE_LIMIT_EXCEEDED","message":"Rate limit exceeded"}
```

The tight limit on opening links makes guessing tokens impractical.

---

## Python Example: Create a Link and Email It

This script creates a link and sends it with Python's standard library. Install the `requests` package first (`pip install requests`).

```python
import smtplib
import requests
from email.message import EmailMessage

CALLFS_API = "https://files.example.com"
API_KEY = "your-api-key"

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
        f"Open the link and press Download file. It works once."
    )

    with smtplib.SMTP(SMTP_HOST, SMTP_PORT) as server:
        server.starttls()
        server.login(SMTP_USER, SMTP_PASSWORD)
        server.send_message(msg)


def main() -> None:
    file_path = "/contracts/agreement.pdf"
    recipient_email = "client@example.com"
    expiry_seconds = 3600

    link_data = generate_download_link(file_path, expiry_seconds)
    send_link_email(recipient_email, "agreement.pdf", link_data)
    print(f"Link {link_data['id']} sent to {recipient_email}")
    print(f"Expires: {link_data['expires']}")


if __name__ == "__main__":
    main()
```

The script keeps the link `id`. If the email goes to the wrong person, revoke it with `DELETE /v1/links/{id}`. Because the confirmation page stops scanners from using the link, a mail filter that previews the URL does not break delivery.

---

## Background Cleanup

A background worker named `link_cleanup` runs every 5 minutes and removes old link records:

- **Expired links** that were never used are removed 24 hours after they expire.
- **Used links** are kept for 24 hours after the download, then removed. Until then you can see who opened the link and from which address.

The worker starts with the server. You can run it at once from the console (**Sharing, Clean up old links now**) or with `POST /v1/admin/workers/link_cleanup/run` (admin). `/health/ready` includes a `link_cleanup` check that reports the time of the last run. In a cluster the job runs on one node at a time; pressing **Run now** on another node is forwarded to it.

---

## In a Cluster

Link records live in the shared file index, so you can create a link on one node and open it on another. Two requirements:

- Every node uses the same `auth.single_use_link_secret`.
- Set each node's `server.external_url` to an address recipients can reach, or the same load balancer address on every node.

If the file's bytes are on a different node, the node that receives the download fetches them from the owner and streams them to the recipient. If that owner node is down, the download fails and the link is not used up. See [Setting up a CallFS cluster](06-callfs-clustering-nats.md).

---

## Monitoring

| Metric | Meaning |
|--------|---------|
| `callfs_single_use_link_generations_total` | Links created. |
| `callfs_single_use_link_consumptions_total{status}` | Open attempts by result: `success`, `expired`, `invalid`, `not_found` or `error`. A rising `invalid` count can mean a misconfigured client or token guessing. |
| `callfs_single_use_links_active` | Links that can still be opened. |
| `callfs_rate_limit_rejections_total{endpoint}` | Requests rejected by a rate limit (`download`, `link_generate`, `session`). |

See [Monitoring](../docs_markdown/06-monitoring-metrics.md) for scraping and alert rules.

---

## Use Cases

Single-use expiring links suit any workflow where you deliver a file to a specific person without granting ongoing access.

**Contracts and legal documents.** Create a link for a signed PDF and send it to the counterparty. It expires after a few hours, and the link history shows when it was opened and from which address.

**Build artifacts and release packages.** Your CI pipeline uploads an artifact and posts a short-lived link in a deployment ticket. Engineers download once; the link cannot be reused.

**Invoices and financial documents.** Accounting software creates a link for each invoice and puts it in the outgoing email.

**Onboarding documents.** HR systems create per-person links that expire after 24 hours, so a forwarded email does not grant access.

**Sensitive data exports.** Put on-demand exports behind links with short lifetimes so stale data does not stay reachable.

In every case the access is scoped to one download, one moment in time and one signed token. No API key leaves your own systems.
