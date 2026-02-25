# Installation Guide

This guide provides instructions for installing and setting up CallFS. Whether you prefer using Docker for a quick start, building from source for development, or deploying pre-built binaries for production, you'll find the necessary steps here.

## Prerequisites

- **Go**: Version 1.21 or later (for building from source).
- **Docker & Docker Compose**: For the quickest and easiest setup of CallFS and its dependencies.
- **PostgreSQL**: Version 12 or later for the metadata store.
- **Redis**: Version 6 or later for the distributed lock manager.
- **TLS Certificates**: A valid TLS certificate and private key are required for HTTPS.

## 1. Quick Start with Docker Compose (Recommended)

Using Docker Compose is the fastest way to get CallFS and its dependencies running locally.

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/ebogdum/callfs.git
    cd callfs
    ```

2.  **Configure CallFS:**
    Copy the example configuration file. You must edit `config.yaml` to add your own secure API keys and secrets.
    ```bash
    cp config.yaml.example config.yaml
    nano config.yaml
    ```

3.  **Start dependency services:**
    This command starts PostgreSQL and Redis in detached mode.
    ```bash
    docker-compose up -d postgres redis
    ```

4.  **Verify dependencies and run CallFS:**
    Confirm dependencies are healthy, then start the CallFS binary.
    ```bash
    docker-compose ps
    ./callfs server --config ./config.yaml
    ```
    In another terminal, check the health endpoint:
    ```bash
    curl http://localhost:8443/health
    ```

## 2. Installation from Source

This method is ideal for developers who want to work on the CallFS source code.

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/ebogdum/callfs.git
    cd callfs
    ```

2.  **Install dependencies:**
    Ensure you have PostgreSQL and Redis installed and running. You can use `docker-compose` to launch them:
    ```bash
    docker-compose up -d postgres redis
    ```

3.  **Build the binary:**
    ```bash
    go build -o callfs ./cmd/main.go
    ```

4.  **Configure and run:**
    Copy and edit the configuration file, then start the server.
    ```bash
    cp config.yaml.example config.yaml
    nano config.yaml
    ./callfs server --config ./config.yaml
    ```

## 3. Using Pre-Built Binaries

For production deployments, you can use pre-built binaries from the [GitHub Releases](https://github.com/ebogdum/callfs/releases) page.

1.  **Download the appropriate binary** for your operating system and architecture.
2.  **Make it executable:** `chmod +x callfs-linux-amd64`
3.  **Create a `config.yaml` file** and place it in the same directory.
4.  **Run the server:** `./callfs-linux-amd64 server --config ./config.yaml`

## Database & Lock Manager Setup

If you are not using the provided `docker-compose` setup, you will need to configure your own PostgreSQL and Redis instances.

### PostgreSQL

CallFS requires a dedicated database and user. Connect to your PostgreSQL instance and run the following SQL commands:

```sql
CREATE DATABASE callfs;
CREATE USER callfs_user WITH ENCRYPTED PASSWORD 'a-very-secure-password';
GRANT ALL PRIVILEGES ON DATABASE callfs TO callfs_user;
```

Update your `config.yaml` with the correct database DSN:
```yaml
metadata_store:
    type: "postgres"
  dsn: "postgres://callfs_user:a-very-secure-password@localhost:5432/callfs?sslmode=disable"
```

### Redis

Ensure your Redis instance is running and update `config.yaml` with the connection details:
```yaml
dlm:
    type: "redis"
  redis_addr: "localhost:6379"
  redis_password: "your-redis-password" # Leave empty if no password
```

## System Service (systemd)

For production environments, it's best to run CallFS as a `systemd` service.

1.  **Create a system user for CallFS:**
    ```bash
    sudo useradd --system --shell /bin/false --home /var/lib/callfs callfs
    sudo mkdir -p /var/lib/callfs
    sudo chown callfs:callfs /var/lib/callfs
    ```

2.  **Place the CallFS binary, config, and certs in a dedicated directory:**
    ```bash
    sudo mkdir -p /opt/callfs
    sudo cp ./callfs /opt/callfs/
    sudo cp ./config.yaml /opt/callfs/
    sudo cp ./certs/server.* /opt/callfs/
    sudo chown -R callfs:callfs /opt/callfs
    ```

3.  **Create the systemd service file:**
    ```bash
    sudo nano /etc/systemd/system/callfs.service
    ```
    Paste the following content:
    ```ini
    [Unit]
    Description=CallFS REST API Filesystem
    After=network.target postgresql.service redis.service
    Wants=postgresql.service redis.service

    [Service]
    Type=simple
    User=callfs
    Group=callfs
    WorkingDirectory=/opt/callfs
    ExecStart=/opt/callfs/callfs server --config /opt/callfs/config.yaml
    Restart=on-failure
    RestartSec=5

    # Security Hardening
    NoNewPrivileges=true
    PrivateTmp=true
    ProtectSystem=strict
    ProtectHome=true
    ReadWritePaths=/var/lib/callfs
    CapabilityBoundingSet=~CAP_SYS_ADMIN CAP_NET_ADMIN

    [Install]
    WantedBy=multi-user.target
    ```

4.  **Enable and start the service:**
    ```bash
    sudo systemctl daemon-reload
    sudo systemctl enable callfs.service
    sudo systemctl start callfs.service
    ```

5.  **Check the status:**
    ```bash
    sudo systemctl status callfs.service
    sudo journalctl -u callfs -f
    ```

## Next Steps

Now that CallFS is installed, proceed to the [Configuration Guide](02-configuration.md) to customize your setup.
