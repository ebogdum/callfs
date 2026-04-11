# Home File Server in 2026: Beyond NFS and Samba

You have a laptop, a desktop, a phone, maybe a tablet. Your photos are in three places. Your documents are in two. A file you edited on the laptop is not on the desktop. You know you should fix this, but every time you look into it you find yourself reading about Samba ACLs or NFS exports and you close the tab.

This is the story of almost every home lab. It does not have to stay that way.

A **home file server** does not need to be complicated. It does not need Windows networking, a domain controller, or a Linux kernel module. What it needs is a single machine that stays on, a drive with enough space, and software that gets out of the way. This guide covers the traditional options honestly, explains why the REST API model is a better fit for home use in 2026, and walks through a complete **Raspberry Pi file server** setup using [CallFS](https://github.com/ebogdum/callfs) -- a single Go binary that you can have running in under ten minutes.

---

## The Traditional Home File Server Stack

### NFS: Works Until It Does Not

NFS (Network File System) has been shipping with Linux since the 1980s. If every machine on your network runs Linux and you never lose power, it is solid. The problem is NFS is designed for always-on data center networks, not home environments where devices come and go.

When the NFS server goes down -- power cut, reboot, you carried it to another room -- any client that has a mount open will hang. Not error. Not reconnect. Hang. Processes freeze waiting for the network call to return. On a laptop you sometimes have to hard-reboot to recover a session. On a Raspberry Pi that restarts for a kernel update, every desktop on your network stops responding until you manually unmount and remount.

NFS also gives you nothing useful on phones or Windows machines without additional software. It speaks its own binary protocol, so there is no `curl` trick, no browser tab, no mobile app that talks to it natively.

### Samba / SMB: Cross-Platform but Fragile

Samba implements the SMB protocol that Windows uses for file sharing. It genuinely works across Linux, macOS, and Windows, which is why it remains popular. The setup is not beginner-friendly, though. You get a configuration file with dozens of sections and options, guest access that varies by SMB dialect version, and firewall rules that differ by platform.

Security history matters here too. SMB1 had serious vulnerabilities (EternalBlue, WannaCry). SMB2 and SMB3 fixed the worst of them, but home routers do not filter SMB traffic, so a misconfigured Samba share is reachable from your entire local network with no authentication layer beyond a username and password that you set once and forgot. Getting TLS on Samba requires a reverse proxy in front of it, which is a separate configuration project.

### FTP: Leave It in the Past

FTP transmits credentials in plaintext. SFTP (which is SSH-based and unrelated) is fine for technical users comfortable with SSH keys, but it was never designed for broad access from multiple device types. FTPS adds TLS but is rarely implemented correctly and blocked by many firewalls because it uses dynamic ports for data channels. FTP as a home file server solution is not worth revisiting.

---

## The REST API Approach

HTTP is the protocol that everything speaks in 2026. Your phone has an HTTP client. Your laptop has one. Your desktop has ten. Scripts, automation tools, and mobile apps all know how to make HTTP requests. A file server that speaks HTTP is a file server that every device on your network can talk to without installing anything extra.

The pattern is straightforward:

- Upload a file: `POST` or `PUT` to a URL that represents the file path
- Download a file: `GET` that same URL
- List a directory: `GET` the directory path
- Delete: `DELETE`

Authentication is a bearer token in a header -- the same model used by every API you interact with daily. No OS user mapping, no SMB dialect negotiation, no NFS portmapper.

This is not a new idea, but until recently the tools that implemented it were cloud services, not something you ran at home. CallFS changes that. It is an open-source, single static binary that implements this model against local storage, SQLite metadata, and optional S3 backends.

---

## File Server Setup on a Raspberry Pi or Mini PC

### What You Need

- A Raspberry Pi 4 or 5, or any x86 mini PC (Intel NUC, Beelink, etc.)
- A USB or SATA drive for storage
- Raspberry Pi OS Lite or any Debian/Ubuntu derivative
- About ten minutes

This **file server setup** works identically on ARM (Pi) and x86 hardware. The only difference is the binary you download.

### Step 1: Download the Binary

On a Raspberry Pi (ARM64):

```bash
curl -L -o callfs https://github.com/ebogdum/callfs/releases/download/v1.4.0/callfs-linux-arm64
```

On an x86 mini PC or any standard Linux machine:

```bash
curl -L -o callfs https://github.com/ebogdum/callfs/releases/download/v1.4.0/callfs-linux-amd64
```

### Step 2: Make It Executable

```bash
chmod +x callfs
```

That is the entire installation. No package manager, no dependencies, no `apt install` chain. The binary is about 25 MB and contains everything it needs.

### Step 3: Create the Configuration File

Create `/opt/callfs/config.yaml`:

```bash
sudo mkdir -p /opt/callfs /mnt/storage
```

```yaml
server:
  listen_addr: ":8443"
  protocol: "http"
  read_timeout: 30s
  write_timeout: 60s

auth:
  api_keys:
    - "my-home-server-key-1234"
  internal_proxy_secret: "internal-secret-1234567"
  single_use_link_secret: "link-secret-12345678"

backend:
  default_backend: "localfs"
  localfs_root_path: "/mnt/storage"

metadata_store:
  type: "sqlite"
  sqlite_path: "/opt/callfs/callfs.sqlite3"

dlm:
  type: "local"

instance_discovery:
  instance_id: "home-server"
```

Change the `api_keys` value and the secret strings before you use this. They are just strings -- make them long and random. The `localfs_root_path` should point to wherever your storage drive is mounted.

Port 8443 is used here because it does not require root. You can use 80 if you prefer, but you will need to run as root or use `setcap` to grant the binary the capability.

### Step 4: Start the Server

```bash
sudo mv callfs /usr/local/bin/callfs
callfs server --config /opt/callfs/config.yaml
```

You should see log output indicating the server is listening. From any other machine on your network, you can verify it is up:

```bash
curl http://192.168.1.100:8443/health
```

### Step 5: Upload Files from Any Device

From a laptop or desktop, upload a file:

```bash
curl -X POST \
  -H "Authorization: Bearer my-home-server-key-1234" \
  --data-binary @photo.jpg \
  http://192.168.1.100:8443/v1/files/photos/vacation/beach.jpg
```

The path in the URL (`photos/vacation/beach.jpg`) is the path on the server. Directories are created automatically.

### Step 6: Download from Anywhere on Your Network

```bash
curl \
  -H "Authorization: Bearer my-home-server-key-1234" \
  http://192.168.1.100:8443/v1/files/photos/vacation/beach.jpg \
  -o beach.jpg
```

List a directory:

```bash
curl \
  -H "Authorization: Bearer my-home-server-key-1234" \
  http://192.168.1.100:8443/v1/files/photos/vacation/
```

Any HTTP client works. `wget`, Python's `requests`, a shell script, a mobile app with HTTP support. There is nothing to configure on the client side beyond the URL and the token.

---

## Running as a System Service

You want the server to start automatically on boot and restart if it crashes. Create a systemd unit file at `/etc/systemd/system/callfs.service`:

```ini
[Unit]
Description=CallFS Home File Server
After=network.target
Wants=network.target

[Service]
Type=simple
User=callfs
Group=callfs
ExecStart=/usr/local/bin/callfs server --config /opt/callfs/config.yaml
Restart=on-failure
RestartSec=5s
StandardOutput=journal
StandardError=journal
SyslogIdentifier=callfs

# Harden the service
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=/opt/callfs /mnt/storage

[Install]
WantedBy=multi-user.target
```

Create the service user and enable the service:

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin callfs
sudo chown -R callfs:callfs /opt/callfs
sudo chown callfs:callfs /mnt/storage

sudo systemctl daemon-reload
sudo systemctl enable callfs
sudo systemctl start callfs
sudo systemctl status callfs
```

The service now starts on boot, restarts automatically on failure, and runs under a dedicated user with no login shell.

---

## Backup Script for Phones and Laptops

A simple shell script that uploads a local directory to the server:

```bash
#!/bin/bash
# backup-to-server.sh
# Usage: ./backup-to-server.sh /path/to/local/photos photos

SERVER="http://192.168.1.100:8443"
TOKEN="my-home-server-key-1234"
LOCAL_DIR="${1:?local directory required}"
REMOTE_DIR="${2:?remote directory name required}"

find "$LOCAL_DIR" -type f | while IFS= read -r file; do
    relative="${file#"$LOCAL_DIR"/}"
    remote_path="$REMOTE_DIR/$relative"
    echo "Uploading: $relative"
    curl -s -X POST \
        -H "Authorization: Bearer $TOKEN" \
        --data-binary "@$file" \
        "$SERVER/v1/files/$remote_path"
done

echo "Backup complete."
```

Run it as a cron job or manually when you want to push files. On macOS or Linux, drop it in a cron entry:

```
0 2 * * * /home/user/backup-to-server.sh /home/user/Photos phone-photos
```

For Android, apps like Termux can run shell scripts with `curl`. For iOS, Scriptable or Shortcuts can make HTTP requests directly.

---

## Resource Usage Comparison

A Raspberry Pi 4 has 4 GB of RAM and enough CPU for a home file server. Here is how the main options compare at idle with one or two active users:

| Solution | Binary / Install Size | Idle RAM | Processes | Protocol |
|---|---|---|---|---|
| Samba | Multiple packages, ~40 MB | 30-80 MB | 2-4 daemons | SMB |
| NFS (kernel) | Kernel module + userspace | 10-30 MB | Several daemons | NFS |
| CallFS | Single binary, ~25 MB | 15-40 MB | 1 | HTTP |

The real advantage of a single static binary is operational simplicity. There is no version mismatch between a client library and a server daemon. There is no init script that depends on a portmapper service. There is one process, one config file, one log stream in `journalctl -u callfs`.

On a Pi 4, CallFS leaves the majority of RAM available for the OS and any other services you run -- a Pi-hole, a Zigbee coordinator, whatever else lives on the same device.

---

## Adding S3 Backup Later

The local setup above stores everything on your attached drive. When you want off-site backup or cloud spillover, CallFS supports an S3 backend alongside local storage. You can configure a second backend pointing to an S3-compatible bucket (AWS S3, Backblaze B2, Cloudflare R2, MinIO on a second machine) and use it as a backup target or overflow tier.

This is a future step. Start local, confirm everything works the way you want, then add the cloud layer. The config change is additive -- you do not rewrite what you have, you extend it.

---

## What This Does Not Cover

A few things outside the scope of this guide:

**External access.** The setup above is local network only. For remote access from outside your home, you need a VPN (WireGuard on the Pi is a natural companion to this setup) or a reverse proxy with proper TLS certificates. Do not expose the server directly to the internet on port 8443 without TLS.

**Large media streaming.** CallFS serves files as downloads. If you want to stream video, a dedicated media server like Jellyfin or Plex handles that better. CallFS is for file storage, not media indexing.

**Multi-user access control.** The current setup has a single API key. CallFS supports multiple keys in the `api_keys` list -- you can give each device or person a different key and rotate them independently. Per-path access control is a more advanced topic.

---

## Summary

If your files are scattered across devices, a **home file server** built on a Raspberry Pi and CallFS gives you a clean central location that everything on your network can reach. The setup is:

1. Download a single binary for your architecture
2. Write a thirty-line config file
3. Create a systemd unit and enable it
4. Upload and download files with any HTTP client

No daemon ecosystem, no protocol negotiation, no SMB dialect mismatch when macOS updates. Just a process listening for HTTP requests and a drive that holds your files.

The **Raspberry Pi file server** running CallFS uses less RAM than a Samba setup, requires no client-side software, and speaks the same protocol that every scripting language and mobile platform already knows how to use. For a home lab in 2026, that is the right starting point.
