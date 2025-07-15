# Developer Guide

This guide is for developers who want to contribute to CallFS or understand its internal architecture. It covers setting up a development environment, the project structure, coding standards, and testing strategies.

## Development Environment Setup

### Prerequisites
- **Go**: Version 1.21+
- **Docker & Docker Compose**: For running dependencies like PostgreSQL and Redis.
- **`make`**: For using the provided Make commands.
- **`golangci-lint`**: Recommended for code linting.

### Quick Start

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/ebogdum/callfs.git
    cd callfs
    ```

2.  **Start development services:**
    This command uses Docker Compose to start PostgreSQL and Redis containers.
    ```bash
    make dev-db-setup
    ```

3.  **Build the application:**
    ```bash
    make build
    ```

4.  **Run the server in development mode:**
    Copy the example config and then run the server. You will need to edit the config to add API keys and secrets.
    ```bash
    cp config.yaml.example config.dev.yaml
    nano config.dev.yaml
    make run-dev
    ```
    The server will start on `https://localhost:8443`.

## Project Structure

The CallFS codebase is organized into several packages, each with a distinct responsibility:

- **`cmd/`**: The main application entry point and CLI command definitions.
- **`server/`**: The HTTP server, including the router, middleware, and API handlers.
- **`core/`**: The core business logic and orchestration layer (the "Engine"). It connects the API layer with the backends and metadata store.
- **`backends/`**: Contains the storage backend implementations (`localfs`, `s3`, `internalproxy`). Each backend implements the `Storage` interface.
- **`metadata/`**: Manages the PostgreSQL metadata store, including the database schema, queries, and data access layer.
- **`auth/`**: Handles authentication (API keys) and authorization (Unix permissions).
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
- **Linting**: Use `golangci-lint` to check for style issues, bugs, and performance problems. Run `make lint` before committing.
- **Error Handling**:
    - Errors should never be ignored.
    - Use `fmt.Errorf` with the `%w` verb to wrap errors with context.
    - Return errors from functions instead of causing panics.
- **Testing**:
    - All new features must be accompanied by unit tests.
    - Aim for high test coverage. Use `make test-coverage` to check.
    - Table-driven tests are preferred for testing multiple cases of the same function.

## Testing

CallFS has a comprehensive testing strategy.

### Unit Tests
- Located alongside the code they test (e.g., `engine_test.go`).
- Focus on testing a single package or component in isolation.
- Use mocks for external dependencies (e.g., mocking the `backends.Storage` interface when testing the `core.Engine`).
- Run with `make test-unit`.

### Integration Tests
- Located in the `tests/integration` directory.
- Test the interaction between multiple components (e.g., API handlers, core engine, and a real database).
- Require running services (PostgreSQL, Redis). The test setup can use `testcontainers-go` to manage these.
- Run with `make test-integration`.

### End-to-End (E2E) Tests
- The shell scripts in the `scripts/` directory (e.g., `test.sh`, `04-test-cross-server.sh`) serve as E2E tests.
- They start the full application and test its functionality by making real HTTP requests with `curl`.
- These are crucial for verifying the complete system behavior, including cross-server operations.
