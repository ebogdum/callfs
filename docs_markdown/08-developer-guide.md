# Developer Guide

This guide is for developers who want to contribute to CallFS or understand its internal architecture. It covers setting up a development environment, the project structure, coding standards, and testing strategies.

## Development Environment Setup

### Prerequisites
- **Go**: Version 1.25+
- **Docker & Docker Compose**: For running dependencies and integration tests.
- **`golangci-lint`**: Recommended for code linting.

### Quick Start

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/ebogdum/callfs.git
    cd callfs
    ```

2.  **Build the application:**
    ```bash
    go build -o callfs ./cmd/main.go
    ```

3.  **Run the server in development mode:**
    Copy the example config, edit it to add API keys and secrets, then start.
    ```bash
    cp config.yaml.example config.dev.yaml
    nano config.dev.yaml
    ./callfs server --config config.dev.yaml
    ```

## Project Structure

The CallFS codebase is organized into several packages, each with a distinct responsibility:

- **`cmd/`**: The main application entry point and CLI command definitions.
- **`server/`**: The HTTP server, including the router, middleware, and API handlers.
- **`core/`**: The core business logic and orchestration layer (the "Engine"). It connects the API layer with the backends and metadata store.
- **`backends/`**: Contains the storage backend implementations (`localfs`, `s3`, `internalproxy`). Each backend implements the `Storage` interface.
- **`metadata/`**: Metadata store implementations for PostgreSQL, SQLite, Redis, and Raft. Each sub-package implements the `metadata.Store` interface.
- **`auth/`**: Handles authentication (API keys) and authorization (owner-based access control).
- **`locks/`**: Implements the distributed lock manager using Redis.
- **`links/`**: Manages the creation and validation of single-use download links.
- **`config/`**: Handles loading and validating the application configuration.
- **`metrics/`**: Defines and registers the Prometheus metrics.
- **`internal/`**: Shared utility packages used across the application.

## Architectural Principles

- **Separation of Concerns**: Each package has a clear and single responsibility. The API layer knows nothing about storage backends; the core engine orchestrates but doesn't implement the details.
- **Dependency Inversion**: Components depend on interfaces, not concrete implementations. For example, the `core.Engine` depends on the `backends.Storage` interface, allowing different storage backends to be plugged in.
- **Context Propagation**: The `context.Context` is passed through all long-running operations and external calls for cancellation and timeouts.
- **Structured Logging**: All logging is done using a structured logger (`zap`) to provide rich, queryable logs.

## Coding Standards

- **Formatting**: All code must be formatted with `gofmt`.
- **Linting**: Use `golangci-lint` to check for style issues, bugs, and performance problems.
- **Error Handling**:
    - Errors should never be ignored.
    - Use `fmt.Errorf` with the `%w` verb to wrap errors with context.
    - Return errors from functions instead of causing panics.
- **Testing**:
    - All new features must be accompanied by unit tests.
    - Aim for high test coverage. Use `go test -cover ./...` to check.
    - Table-driven tests are preferred for testing multiple cases of the same function.

## Testing

CallFS has a comprehensive testing strategy.

### Unit Tests
- Located alongside the code they test (e.g., `auth/unix_authorizer_test.go`).
- Focus on testing a single package or component in isolation.
- Use stub implementations for external dependencies (e.g., stub metadata store when testing the authorizer).
- Run with `go test ./...`.

### Integration Tests
- Located in the `tests/integration/` directory.
- Docker-based: builds the CallFS image, spins up a 3-node Raft cluster with PostgreSQL, Redis, and MinIO.
- 35 test suites covering CRUD, auth, permissions, cross-server operations, Raft consensus, S3 backend, erasure coding, WebSocket transfers, rate limiting, binary integrity, concurrent operations, config validation, and more.
- Run with `cd tests/integration && bash run-tests.sh`.
- The test suites are shell scripts in `tests/integration/tests/` that make real HTTP requests with `curl` and verify responses.
- Shared test helpers in `tests/integration/lib.sh` provide assertion functions and HTTP wrappers.
