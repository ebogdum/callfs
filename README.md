# CallFS: A High-Performance, Distributed REST API Filesystem

CallFS is an ultra-lightweight, high-performance REST API filesystem that provides precise Linux filesystem semantics over various backends, including local storage, Amazon S3, and a distributed peer-to-peer network. It is designed for speed, reliability, and horizontal scalability.

[![Go Report Card](https://goreportcard.com/badge/github.com/ebogdum/callfs)](https://goreportcard.com/report/github.com/ebogdum/callfs)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Key Features

- **Multi-Backend Storage**: Seamlessly use Local Filesystem, Amazon S3, or other S3-compatible services (like MinIO) as storage backends.
- **Distributed Architecture**: Scale horizontally by adding more CallFS instances. The system automatically routes operations to the correct node.
- **Cross-Server Operations**: Operations like move, copy, and delete work across the cluster with automatic conflict detection and resolution.
- **High-Performance API**: A clean REST API for all filesystem operations, built for low latency and high throughput.
- **Secure, Ephemeral Links**: Generate secure, time-limited, single-use download links for any file.
- **Distributed Locking**: Supports Redis or local lock managers depending on deployment mode.
- **Rich Metadata Store**: Supports PostgreSQL, SQLite, Redis, and Raft-backed metadata coordination.
- **Comprehensive Security**: Supports HTTP/HTTPS modes, API key authentication, and Unix-style permission authorization.
- **First-Class Observability**: Structured logging (JSON/console) and extensive Prometheus metrics for deep operational insight.
- **Zero-Copy I/O**: Efficiently streams large files without buffering them in memory, ensuring a low memory footprint.

## Architecture

CallFS consists of several core components that work together to provide a unified, distributed filesystem API:

- **API Server**: The public-facing HTTP server that handles all incoming REST API requests.
- **Core Engine**: The central orchestrator that processes requests, manages metadata, and selects the appropriate storage backend.
- **Storage Backends**: Pluggable modules for different storage systems:
    - **LocalFS**: Manages files on the local disk.
    - **S3**: Interfaces with Amazon S3 or compatible object stores.
    - **Internal Proxy**: Routes requests to other CallFS instances in the cluster.
- **Metadata Store**: A PostgreSQL database that stores all filesystem metadata, such as file names, sizes, timestamps, permissions, and backend locations.
- **Distributed Lock Manager (DLM)**: Uses Redis to provide cluster-wide locks, preventing race conditions during concurrent file modifications.
- **Link Manager**: Generates and validates secure single-use download tokens.

This modular design allows CallFS to be both powerful and flexible, suitable for a wide range of applications from simple file serving to complex, distributed storage architectures.

## API Endpoints

All API endpoints are prefixed with `/v1` and require `Bearer <api-key>` authentication, except where noted.

### File & Directory Operations

- `GET /files/{path}`: Downloads a file or lists a directory's contents.
- `HEAD /files/{path}`: Retrieves file metadata. This operation is "enhanced" and can route requests across the cluster to find the correct node.
- `POST /files/{path}`: Creates a new file or directory. It features cross-server conflict detection to prevent overwriting.
- `PUT /files/{path}`: Uploads or updates a file's content. Supports streaming data and cross-server proxying.
- `DELETE /files/{path}`: Deletes a file or directory, with cross-server routing capabilities.

### Enhanced Directory Listings

- `GET /directories/{path}`: Provides a detailed JSON listing of a directory's contents.
- `GET /directories/{path}?recursive=true`: Performs a recursive listing of all subdirectories.
- `GET /directories/{path}?recursive=true&max_depth=N`: Limits the depth of the recursive listing.

### Single-Use Download Links

- `POST /links/generate`: Creates a secure, single-use download link for a file. The link's expiry time is configurable.
- `GET /download/{token}`: Downloads a file using a single-use token. **(No authentication required)**.

### System Health & Metrics

- `GET /health`: Returns the operational status of the server. **(No authentication required)**.
- `GET /metrics`: Exposes detailed performance metrics in Prometheus format. **(No authentication required)**.

## Getting Started

### Prerequisites
- **Go**: Version 1.21 or later.
- **PostgreSQL**: A running instance for the metadata store.
- **Redis**: A running instance for the distributed lock manager.
- **Docker & Docker Compose**: Recommended for easily running PostgreSQL and Redis.
- A valid `config.yaml` file (see `config.yaml.example`)

### Running the Server

1.  **Start dependent services:**
    ```bash
    docker-compose up -d postgres redis
    ```

2.  **Build the binary:**
    ```bash
    go build -o callfs ./cmd/main.go
    ```

3.  **Run the server:**
    ```bash
    ./callfs server --config /path/to/your/config.yaml
    ```

### Command-Line Interface (CLI)

CallFS includes a simple CLI for managing the server:

- **`callfs server`**: Starts the main API server.
  - `--config, -c`: Specifies the path to the configuration file.
- **`callfs config validate`**: Validates the configuration file and displays the loaded settings.
- **`callfs --help`**: Shows all available commands and flags.

## Full Documentation

For detailed information on configuration, API usage, security, and more, please refer to the `docs_markdown/` directory:

- [01 - Installation](docs_markdown/01-installation.md)
- [02 - Configuration](docs_markdown/02-configuration.md)
- [03 - API Reference](docs_markdown/03-api-reference.md)
- [04 - Authentication & Security](docs_markdown/04-authentication-security.md)
- [05 - Backend Configuration](docs_markdown/05-backend-configuration.md)
- [06 - Monitoring & Metrics](docs_markdown/06-monitoring-metrics.md)
- [07 - Clustering & Distribution](docs_markdown/07-clustering-distribution.md)
- [08 - Developer Guide](docs_markdown/08-developer-guide.md)
- [09 - Troubleshooting](docs_markdown/09-troubleshooting.md)
- [10 - Enhanced Cross-Server Operations](docs_markdown/10-enhanced-cross-server.md)

## Contributing

Contributions are welcome! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to participate in this project.

## License

CallFS is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
