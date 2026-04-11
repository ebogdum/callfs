# Secure File Sharing for Business: API Keys, Permissions, and Expiring Links

Sharing files inside a company sounds simple until something goes wrong. A contractor retains access to a folder after their engagement ends. A shared drive link gets forwarded outside the organization. A password-protected zip archive sits in someone's sent mail for years. These are not edge cases. They are the routine failures that lead to data breaches, compliance violations, and the kind of headlines no business wants.

This guide walks through a three-layer approach to secure file sharing: authenticating who can upload and access files, authorizing what each identity is permitted to do, and generating expiring single-use links so that external recipients never need credentials at all.

The examples use [CallFS](https://github.com/ebogdum/callfs), an open-source file server built around these principles. The patterns themselves apply broadly.

---

## Why Secure File Sharing Matters for Business

The average cost of a data breach reached $4.88 million in 2024, according to IBM's annual report. A meaningful proportion of those incidents trace back not to sophisticated attacks but to misconfigured access controls — files readable by anyone with the link, shared drives with no expiry, credentials that were never rotated after an employee left.

Regulated industries carry additional exposure. HIPAA requires covered entities to implement technical safeguards controlling access to protected health information. SOC 2 Type II audits scrutinize access logs, least-privilege enforcement, and how sensitive data moves between systems. GDPR's data minimization principle pushes organizations to share only what is necessary, with whom it is necessary, for as long as it is necessary.

The practical implication: "share files securely" is not a feature request. It is a compliance requirement for a large and growing set of businesses.

---

## Layer One: Authentication with API Keys

The first question any file server must answer is: who is making this request?

Username/password authentication has well-understood weaknesses in automated and API contexts. Passwords get committed to repositories, hardcoded in scripts, and reused across services. API keys are not immune to misuse, but they are easier to scope, rotate, and revoke without disrupting users.

### Issuing Keys per Team Member

In CallFS, API keys are defined in `config.yaml`. Each key maps to a named owner identity. That owner label follows the file through its lifecycle — it is recorded at upload time and checked at delete time.

```yaml
api_keys:
  - key: "ak_finance_prod_abc123"
    owner: "alice"
  - key: "ak_ops_prod_def456"
    owner: "bob"
  - key: "ak_contractor_temp_ghi789"
    owner: "charlie"

server:
  port: 8443
  tls_cert: "/etc/callfs/cert.pem"
  tls_key: "/etc/callfs/key.pem"
```

This structure gives you a few practical benefits. When you need to revoke a contractor's access, you remove their key from the config and reload the server — no database migration, no user account cleanup. When you need to audit who uploaded a sensitive file, the owner is recorded in metadata. When you rotate keys on a schedule, the owner identity persists across key changes.

Each key should be treated like a password: generated with sufficient entropy, stored in a secrets manager or environment variable, never committed to version control.

### Uploading a File

With a key in hand, uploading a file is a single HTTP request:

```bash
curl -X POST \
  -H "Authorization: Bearer ak_finance_prod_abc123" \
  -H "Content-Type: application/octet-stream" \
  --data-binary @report.pdf \
  http://localhost:8443/v1/files/finance/q4-report.pdf
```

The server authenticates the key, resolves the owner (`alice`), records that owner in the file's metadata, and stores the file at `/finance/q4-report.pdf`. Any request that does not carry a valid key receives a 401.

---

## Layer Two: Authorization with Owner-Based Permissions

Authentication answers "who are you." Authorization answers "what are you allowed to do."

Many file-sharing systems conflate these two questions or handle them with coarse controls — a single shared password, or a group folder where any member can delete any file. Owner-based permissions offer a finer model without requiring a complex role hierarchy.

### How Owner-Based Access Works

In CallFS, the access model is:

- **Upload**: any authenticated user can upload a file to any path they have access to
- **Read and download**: any authenticated user can read any file
- **Delete**: only the file's owner can delete it

This asymmetry is intentional. It prevents the scenario where a team member, intentionally or accidentally, deletes a file they did not create. The finance team can all read the quarterly report, but only the person who uploaded it can remove it.

### Inspecting File Metadata

To see who owns a file and what its current state is, send a HEAD request:

```bash
curl -I \
  -H "Authorization: Bearer ak_finance_prod_abc123" \
  http://localhost:8443/v1/files/finance/q4-report.pdf
```

The response headers surface the relevant metadata without transferring the file body:

```
HTTP/1.1 200 OK
X-CallFS-Owner: alice
X-CallFS-Mode: 0644
X-CallFS-Size: 2457600
X-CallFS-MTime: 2026-04-11T08:30:00Z
Content-Type: application/octet-stream
```

`X-CallFS-Owner` tells you which identity uploaded the file. `X-CallFS-Mode` reflects the Unix permission bits stored in metadata. `X-CallFS-Size` and `X-CallFS-MTime` let you verify integrity and freshness without downloading the full content.

This is useful in automated pipelines: a script can check ownership and modification time before deciding whether to process a file, without pulling gigabytes across the network.

### Least Privilege in Practice

The owner model maps naturally to least-privilege principles. Give each team member or service account its own API key. When a process needs to read files but should never delete them, that constraint is enforced structurally — the process can authenticate, but unless it owns the file, a delete request will be rejected.

For service-to-service communication, create a dedicated key per service rather than sharing a key across multiple services. If one service is compromised, you revoke its key without affecting others. The owner label in metadata also makes it straightforward to audit which service uploaded which files.

---

## Layer Three: Expiring Single-Use Download Links

Authentication and authorization solve the internal access problem. But businesses regularly need to share files with people who should not have persistent credentials: clients receiving a deliverable, auditors reviewing documentation, partners downloading a dataset for a one-time analysis.

Giving an external party an API key is almost always the wrong answer. They get more access than they need, the key persists indefinitely unless manually revoked, and you have no way to know whether they actually downloaded the file.

Expiring single-use links solve all three problems.

### Generating a Secure Download Link

To generate a link, send a POST request with the file path and an expiry window:

```bash
curl -X POST \
  -H "Authorization: Bearer ak_finance_prod_abc123" \
  -H "Content-Type: application/json" \
  -d '{"path":"/finance/q4-report.pdf","expiry_seconds":3600}' \
  http://localhost:8443/v1/links/generate
```

The server generates a cryptographically random token, associates it with the file path and expiry time, and returns a pre-signed URL:

```json
{
  "url": "https://your-server/download/eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires": "2026-04-11T09:00:00Z"
}
```

### How the Recipient Downloads the File

The recipient does not need an API key. They do not need an account. They need only the URL:

```bash
curl -O https://your-server/download/eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
```

The server validates the token, checks that it has not expired and has not already been used, streams the file, and then invalidates the token. A second request with the same URL returns a 410 Gone.

### Why Single-Use Matters

Time-limited links alone are an improvement over permanent shared links, but they leave a window of exposure. If a link with a 24-hour expiry is forwarded to an unintended recipient, that recipient has up to 24 hours to download the file.

Single-use closes that window. Once the intended recipient downloads the file, the link is dead. Forward it all you like — it will not work again.

This also gives you an audit signal. If a token is consumed before you expect it to be, someone downloaded the file. That is actionable information.

### Choosing an Appropriate Expiry Window

The right expiry depends on context:

- **Immediate handoff** (client is waiting on a call): 300–600 seconds. The link is valid long enough to download the file but expires before it can be misused.
- **Async delivery** (sending by email, recipient downloads later): 3600–86400 seconds. Balance convenience with exposure window.
- **Compliance-sensitive documents**: keep the window short and log all link generation events. Some compliance frameworks require demonstrating that access to sensitive documents was time-bounded.

There is no universally correct value. The goal is to match the expiry to the expected use — not to set a generous default and forget about it.

---

## Putting It Together: A Secure File Sharing Workflow

A typical workflow for sharing a sensitive document externally looks like this:

1. Team member authenticates with their personal API key and uploads the file.
2. The file is stored with the uploader's owner label in metadata.
3. When ready to share externally, the team member requests a single-use link with an appropriate expiry.
4. The link is sent to the external recipient via whatever channel is appropriate (email, Slack, a ticketing system).
5. The recipient downloads the file using the link. The link is immediately invalidated.
6. If the recipient needs the file again, a new link is generated — creating a new audit event.

At no point does the external recipient have credentials. At no point does the link remain valid longer than necessary. The internal team member's key is the only persistent credential, and it is scoped to that individual.

This model supports secure file sharing for business contexts where you need to share files with external parties without granting them ongoing access to your infrastructure.

---

## Operational Considerations

### Key Rotation

Rotate API keys on a schedule — quarterly is a common baseline. Because owner identities are stored in file metadata rather than derived from the key itself, rotating a key does not change the ownership of existing files. Generate the new key, update the config, reload the server, update the secret in your secrets manager. The transition is zero-downtime.

### Revocation

When a team member leaves or a contractor's engagement ends, remove their key from the config immediately. Files they uploaded retain their owner label in metadata, which is useful for auditing. They simply cannot authenticate to make new requests or generate new links.

### TLS

All of this is meaningless without transport encryption. Run the server behind TLS. CallFS supports configuring a cert and key directly. For internet-facing deployments, terminate TLS at a load balancer or reverse proxy with a certificate from a trusted CA.

### Logging

Log all authentication events, file operations, and link generation and consumption events. These logs are your audit trail. For regulated industries, retention requirements vary — HIPAA commonly requires six years, some financial regulations require seven. Make sure your logging infrastructure is configured before you start using the system for sensitive data.

---

## Summary

Secure file sharing for business is not a single feature — it is a stack of controls that work together. API keys establish identity and make access auditable and revocable. Owner-based permissions enforce least privilege without requiring a complex role system. Expiring single-use links solve the external sharing problem without handing out persistent credentials.

Each layer addresses a different failure mode. Authentication stops unauthenticated access. Authorization prevents privilege escalation within the system. Expiring links limit the damage from forwarded URLs and constrain the window of external exposure.

CallFS implements all three layers and is available at [github.com/ebogdum/callfs](https://github.com/ebogdum/callfs). The patterns described here — keyed identities, owner metadata, pre-signed expiring tokens — are well-established and applicable regardless of which file server you use.

The right time to implement these controls is before you have a reason to wish you had.
