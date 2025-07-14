# Developer Guide

This guide covers development setup, contribution guidelines, testing strategies, and advanced development topics for CallFS.

## Development Environment Setup

### Prerequisites

#### Required Tools
```bash
# Go development environment
go version  # Requires Go 1.21+

# Database tools
psql --version    # PostgreSQL client
redis-cli --version  # Redis client

# Development tools
git --version
make --version
docker --version
docker-compose --version

# Optional but recommended
golangci-lint --version  # Code linting
mockgen --version       # Mock generation
wire --version         # Dependency injection
```

#### Environment Variables
```bash
# Development environment
export CALLFS_ENV=development
export CALLFS_LOG_LEVEL=debug
export CALLFS_LOG_FORMAT=text

# Database setup
export CALLFS_METADATA_STORE_DSN="postgres://callfs:password@localhost:5432/callfs_dev?sslmode=disable"
export CALLFS_DLM_REDIS_ADDR="localhost:6379"

# Development secrets (use random values for dev)
export CALLFS_AUTH_INTERNAL_PROXY_SECRET="dev-proxy-secret"
export CALLFS_AUTH_SINGLE_USE_LINK_SECRET="dev-link-secret"
export CALLFS_AUTH_API_KEYS="dev-api-key-1,dev-api-key-2"

# Storage backend for development
export CALLFS_BACKEND_LOCALFS_ROOT_PATH="./data/dev"

# Development server configuration
export CALLFS_SERVER_LISTEN_ADDR=":8443"
export CALLFS_SERVER_EXTERNAL_URL="https://localhost:8443"
```

### Quick Start

#### 1. Clone and Setup
```bash
# Clone repository
git clone https://github.com/yourusername/callfs.git
cd callfs

# Install dependencies
go mod download

# Install development tools
make install-dev-tools

# Generate development certificates
make dev-certs

# Setup development database
make dev-db-setup
```

#### 2. Start Development Services
```bash
# Start PostgreSQL and Redis
docker-compose -f docker-compose.dev.yml up -d

# Run database migrations
make migrate-up

# Start CallFS in development mode
make run-dev
```

#### 3. Verify Installation
```bash
# Test health endpoint
curl -k https://localhost:8443/health

# Test API with dev key
curl -k -H "Authorization: Bearer dev-api-key-1" \
  https://localhost:8443/api/v1/files/
```

### Development Database Setup

#### Docker Compose for Development
```yaml
# docker-compose.dev.yml
version: '3.8'

services:
  postgres-dev:
    image: postgres:14
    container_name: callfs-postgres-dev
    environment:
      POSTGRES_DB: callfs_dev
      POSTGRES_USER: callfs
      POSTGRES_PASSWORD: password
      POSTGRES_INITDB_ARGS: "--encoding=UTF8 --data-checksums"
    ports:
      - "5432:5432"
    volumes:
      - postgres_dev_data:/var/lib/postgresql/data
      - ./metadata/schema:/docker-entrypoint-initdb.d
    command: >
      postgres
      -c log_statement=all
      -c log_duration=on
      -c log_line_prefix='%t [%p]: [%l-1] user=%u,db=%d,app=%a,client=%h '

  redis-dev:
    image: redis:7-alpine
    container_name: callfs-redis-dev
    ports:
      - "6379:6379"
    volumes:
      - redis_dev_data:/data
    command: redis-server --appendonly yes --appendfsync everysec

  postgres-test:
    image: postgres:14
    container_name: callfs-postgres-test
    environment:
      POSTGRES_DB: callfs_test
      POSTGRES_USER: callfs_test
      POSTGRES_PASSWORD: test_password
    ports:
      - "5433:5432"
    volumes:
      - ./metadata/schema:/docker-entrypoint-initdb.d
    tmpfs:
      - /var/lib/postgresql/data

volumes:
  postgres_dev_data:
  redis_dev_data:
```

#### Migration Scripts
```bash
#!/bin/bash
# scripts/migrate.sh

set -e

DB_URL="${CALLFS_METADATA_STORE_DSN:-postgres://callfs:password@localhost:5432/callfs_dev?sslmode=disable}"

case "$1" in
  up)
    echo "Running migrations up..."
    migrate -database "$DB_URL" -path metadata/schema up
    ;;
  down)
    echo "Running migrations down..."
    migrate -database "$DB_URL" -path metadata/schema down
    ;;
  reset)
    echo "Resetting database..."
    migrate -database "$DB_URL" -path metadata/schema drop -f
    migrate -database "$DB_URL" -path metadata/schema up
    ;;
  version)
    migrate -database "$DB_URL" -path metadata/schema version
    ;;
  *)
    echo "Usage: $0 {up|down|reset|version}"
    exit 1
    ;;
esac
```

### Makefile

```makefile
# Makefile
.PHONY: help build test lint clean dev-setup run-dev

# Default target
help:
	@echo "Available targets:"
	@echo "  build        - Build the application"
	@echo "  test         - Run all tests"
	@echo "  test-unit    - Run unit tests only"
	@echo "  test-integration - Run integration tests"
	@echo "  lint         - Run linter"
	@echo "  clean        - Clean build artifacts"
	@echo "  dev-setup    - Setup development environment"
	@echo "  run-dev      - Run in development mode"
	@echo "  migrate-up   - Run database migrations"
	@echo "  migrate-down - Rollback database migrations"

# Build configuration
BINARY_NAME := callfs
BUILD_DIR := bin
VERSION := $(shell git describe --tags --always --dirty)
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildDate=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)"

# Go configuration
GO := go
GOFLAGS := -v
GOTAGS := 
TEST_TIMEOUT := 30m

# Development dependencies
DEV_TOOLS := \
	github.com/golangci/golangci-lint/cmd/golangci-lint@latest \
	github.com/golang/mock/mockgen@latest \
	github.com/google/wire/cmd/wire@latest \
	github.com/swaggo/swag/cmd/swag@latest \
	github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Build targets
build: clean
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd

build-all: clean
	$(GO) build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/callfs ./cmd

# Test targets
test: test-unit test-integration

test-unit:
	$(GO) test $(GOFLAGS) -timeout $(TEST_TIMEOUT) -race -cover ./...

test-integration:
	$(GO) test $(GOFLAGS) -timeout $(TEST_TIMEOUT) -tags=integration ./tests/integration/...

test-benchmark:
	$(GO) test $(GOFLAGS) -bench=. -benchmem ./...

test-coverage:
	$(GO) test $(GOFLAGS) -timeout $(TEST_TIMEOUT) -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

# Code quality
lint:
	golangci-lint run --timeout 5m

lint-fix:
	golangci-lint run --fix --timeout 5m

# Code generation
generate:
	$(GO) generate ./...

wire:
	wire ./...

swagger:
	swag init -g cmd/main.go -o docs/

# Development setup
install-dev-tools:
	@for tool in $(DEV_TOOLS); do \
		echo "Installing $$tool..."; \
		$(GO) install $$tool; \
	done

dev-certs:
	mkdir -p certs
	openssl req -x509 -newkey rsa:4096 -keyout certs/server.key -out certs/server.crt \
		-days 365 -nodes -subj "/C=US/ST=CA/L=SF/O=CallFS/OU=Dev/CN=localhost"

dev-db-setup:
	docker-compose -f docker-compose.dev.yml up -d postgres-dev redis-dev
	sleep 5
	$(MAKE) migrate-up

dev-db-teardown:
	docker-compose -f docker-compose.dev.yml down -v

# Database migrations
migrate-up:
	./scripts/migrate.sh up

migrate-down:
	./scripts/migrate.sh down

migrate-reset:
	./scripts/migrate.sh reset

# Development server
run-dev: dev-certs
	$(GO) run $(GOFLAGS) ./cmd server --config config.dev.yaml

run-dev-watch:
	air -c .air.toml

# Debugging
debug:
	dlv debug ./cmd -- server --config config.dev.yaml

# Container targets
docker-build:
	docker build -t callfs:$(VERSION) .
	docker tag callfs:$(VERSION) callfs:latest

docker-run:
	docker run --rm -p 8443:8443 -v $(PWD)/config.yaml:/app/config.yaml callfs:latest

# Clean targets
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	rm -f callfs

clean-all: clean
	docker-compose -f docker-compose.dev.yml down -v
	docker system prune -f

# Security
security-scan:
	gosec ./...

dependency-check:
	$(GO) list -m -u all
	$(GO) mod tidy
	$(GO) mod verify

# Performance
profile-cpu:
	$(GO) run ./cmd server --cpuprofile=cpu.prof --config config.dev.yaml

profile-mem:
	$(GO) run ./cmd server --memprofile=mem.prof --config config.dev.yaml

# Release
release-dry-run:
	goreleaser release --snapshot --rm-dist

release:
	goreleaser release --rm-dist
```

### Development Configuration

#### Development Config File
```yaml
# config.dev.yaml
log:
  level: debug
  format: text
  file: ""  # Log to stdout

server:
  listen_addr: ":8443"
  external_url: "https://localhost:8443"
  cert_file: "certs/server.crt"
  key_file: "certs/server.key"
  shutdown_timeout: 30s
  
auth:
  api_keys:
    - "dev-api-key-1"
    - "dev-api-key-2"
  internal_proxy_secret: "dev-proxy-secret"
  single_use_link_secret: "dev-link-secret"
  unix_ownership_enforcement: false  # Disabled for development

metadata_store:
  dsn: "postgres://callfs:password@localhost:5432/callfs_dev?sslmode=disable"
  max_open_conns: 10
  max_idle_conns: 5
  conn_max_lifetime: 5m

dlm:
  redis_addr: "localhost:6379"
  redis_password: ""
  lock_timeout: 30s
  retry_delay: 100ms

backend:
  localfs_root_path: "./data/dev"
  
cache:
  ttl: 5m
  cleanup_interval: 10m

rate_limit:
  requests_per_second: 1000  # High limit for development
  burst: 2000

metrics:
  enabled: true
  listen_addr: ":9090"

# Development-specific settings
development:
  auto_reload: true
  debug_endpoints: true
  verbose_errors: true
```

### Testing

#### Test Structure
```
tests/
├── integration/           # Integration tests
│   ├── api_test.go
│   ├── auth_test.go
│   ├── backend_test.go
│   └── helpers/
├── testdata/             # Test fixtures
│   ├── files/
│   ├── configs/
│   └── certs/
└── mocks/               # Generated mocks
    ├── auth/
    ├── backends/
    └── metadata/
```

#### Test Utilities
```go
// tests/helpers/testutil.go
package helpers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"callfs/config"
	"callfs/core"
	"callfs/metadata/postgres"
)

// TestConfig creates a test configuration
func TestConfig(t *testing.T) *config.AppConfig {
	cfg := config.DefaultConfig()
	cfg.Log.Level = "debug"
	cfg.Log.Format = "text"
	cfg.Auth.APIKeys = []string{"test-api-key"}
	cfg.Auth.InternalProxySecret = "test-proxy-secret"
	cfg.Auth.SingleUseLinkSecret = "test-link-secret"
	cfg.Backend.LocalFSRootPath = t.TempDir()
	return cfg
}

// TestServer creates a test server instance
func TestServer(t *testing.T, cfg *config.AppConfig) *httptest.Server {
	engine, err := core.NewEngine(cfg)
	require.NoError(t, err)

	router := engine.Router()
	return httptest.NewTLSServer(router)
}

// PostgresContainer starts a test PostgreSQL container
func PostgresContainer(t *testing.T) *sql.DB {
	ctx := context.Background()
	
	req := testcontainers.ContainerRequest{
		Image:        "postgres:14",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       "callfs_test",
			"POSTGRES_USER":     "callfs_test",
			"POSTGRES_PASSWORD": "test_password",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	// Cleanup
	t.Cleanup(func() {
		container.Terminate(ctx)
	})

	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	dsn := fmt.Sprintf("postgres://callfs_test:test_password@%s:%s/callfs_test?sslmode=disable", 
		host, port.Port())

	db, err := sql.Open("postgres", dsn)
	require.NoError(t, err)

	// Run migrations
	store, err := postgres.NewStore(dsn)
	require.NoError(t, err)
	require.NoError(t, store.Migrate())

	return db
}

// RedisContainer starts a test Redis container
func RedisContainer(t *testing.T) string {
	ctx := context.Background()
	
	req := testcontainers.ContainerRequest{
		Image:        "redis:7-alpine",
		ExposedPorts: []string{"6379/tcp"},
		WaitingFor:   wait.ForListeningPort("6379/tcp").WithStartupTimeout(30 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		container.Terminate(ctx)
	})

	host, err := container.Host(ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(ctx, "6379")
	require.NoError(t, err)

	return fmt.Sprintf("%s:%s", host, port.Port())
}

// APIClient provides a test HTTP client
type APIClient struct {
	BaseURL string
	APIKey  string
	Client  *http.Client
}

func NewAPIClient(baseURL, apiKey string) *APIClient {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	
	return &APIClient{
		BaseURL: baseURL,
		APIKey:  apiKey,
		Client:  &http.Client{Transport: tr},
	}
}

func (c *APIClient) Request(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	return c.Client.Do(req)
}
```

#### Integration Test Example
```go
// tests/integration/api_test.go
//go:build integration
// +build integration

package integration

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"callfs/tests/helpers"
)

func TestFileOperations(t *testing.T) {
	// Setup test environment
	cfg := helpers.TestConfig(t)
	cfg.MetadataStore.DSN = "postgres://callfs_test:test_password@localhost:5433/callfs_test?sslmode=disable"
	cfg.DLM.RedisAddr = "localhost:6379"

	server := helpers.TestServer(t, cfg)
	defer server.Close()

	client := helpers.NewAPIClient(server.URL, "test-api-key")

	t.Run("upload and download file", func(t *testing.T) {
		// Upload file
		content := "Hello, CallFS!"
		resp, err := client.Request("PUT", "/api/v1/files/test.txt", strings.NewReader(content))
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		// Download file
		resp, err = client.Request("GET", "/api/v1/files/test.txt", nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Equal(t, content, string(body))
	})

	t.Run("file metadata", func(t *testing.T) {
		// Head request for metadata
		resp, err := client.Request("HEAD", "/api/v1/files/test.txt", nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "13", resp.Header.Get("Content-Length"))
		assert.Equal(t, "text/plain; charset=utf-8", resp.Header.Get("Content-Type"))
	})

	t.Run("delete file", func(t *testing.T) {
		// Delete file
		resp, err := client.Request("DELETE", "/api/v1/files/test.txt", nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)

		// Verify file is gone
		resp, err = client.Request("GET", "/api/v1/files/test.txt", nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestDirectoryOperations(t *testing.T) {
	cfg := helpers.TestConfig(t)
	server := helpers.TestServer(t, cfg)
	defer server.Close()

	client := helpers.NewAPIClient(server.URL, "test-api-key")

	// Create test files
	files := map[string]string{
		"dir1/file1.txt": "content1",
		"dir1/file2.txt": "content2",
		"dir2/file3.txt": "content3",
	}

	for path, content := range files {
		resp, err := client.Request("PUT", "/api/v1/files/"+path, strings.NewReader(content))
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	t.Run("list root directory", func(t *testing.T) {
		resp, err := client.Request("GET", "/api/v1/files/", nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		
		// Should contain both directories
		content := string(body)
		assert.Contains(t, content, "dir1/")
		assert.Contains(t, content, "dir2/")
	})

	t.Run("list subdirectory", func(t *testing.T) {
		resp, err := client.Request("GET", "/api/v1/files/dir1/", nil)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		
		content := string(body)
		assert.Contains(t, content, "file1.txt")
		assert.Contains(t, content, "file2.txt")
		assert.NotContains(t, content, "file3.txt")
	})
}
```

### Code Generation

#### Mock Generation
```go
//go:generate mockgen -source=interfaces.go -destination=mocks/mock_interfaces.go

// auth/interfaces.go
package auth

import "context"

type Authenticator interface {
	Authenticate(ctx context.Context, token string) (*Principal, error)
}

type Authorizer interface {
	Authorize(ctx context.Context, principal *Principal, resource string, action string) error
}
```

#### Wire Dependency Injection
```go
// wire.go
//go:build wireinject
// +build wireinject

package main

import (
	"github.com/google/wire"
	
	"callfs/auth"
	"callfs/backends"
	"callfs/config"
	"callfs/core"
	"callfs/metadata"
)

func InitializeEngine(cfg *config.AppConfig) (*core.Engine, error) {
	wire.Build(
		// Configuration
		wire.FieldsOf(new(*config.AppConfig), "MetadataStore", "Auth", "Backend", "DLM"),
		
		// Metadata store
		metadata.NewStore,
		
		// Authentication
		auth.NewAPIKeyAuthenticator,
		auth.NewUnixAuthorizer,
		
		// Backends
		backends.NewStorageBackend,
		
		// Core engine
		core.NewEngine,
	)
	return &core.Engine{}, nil
}
```

### Debugging

#### Debug Configuration
```go
// main.go debug support
var (
	cpuprofile = flag.String("cpuprofile", "", "write cpu profile to file")
	memprofile = flag.String("memprofile", "", "write memory profile to file")
)

func main() {
	flag.Parse()
	
	// CPU profiling
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	
	// Memory profiling
	if *memprofile != "" {
		defer func() {
			f, err := os.Create(*memprofile)
			if err != nil {
				log.Fatal(err)
			}
			defer f.Close()
			
			runtime.GC()
			pprof.WriteHeapProfile(f)
		}()
	}
	
	// Regular application startup
	// ...
}
```

#### VS Code Debug Configuration
```json
// .vscode/launch.json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Debug CallFS Server",
            "type": "go",
            "request": "launch",
            "mode": "debug",
            "program": "./cmd",
            "args": [
                "server",
                "--config", "config.dev.yaml"
            ],
            "env": {
                "CALLFS_LOG_LEVEL": "debug"
            },
            "cwd": "${workspaceFolder}",
            "showLog": true
        },
        {
            "name": "Debug Tests",
            "type": "go",
            "request": "launch",
            "mode": "test",
            "program": "./tests/integration",
            "env": {
                "CALLFS_TEST_ENV": "true"
            },
            "args": [
                "-test.v"
            ]
        }
    ]
}
```

## Contributing Guidelines

### Code Style

#### Go Code Conventions
```go
// Package naming: lowercase, single word
package callfs

// Interface naming: noun or adjective + -er
type FileStorer interface {
    Store(ctx context.Context, path string, data io.Reader) error
}

// Struct naming: PascalCase
type APIKeyAuthenticator struct {
    keys map[string]bool
}

// Method naming: PascalCase verbs
func (a *APIKeyAuthenticator) Authenticate(ctx context.Context, token string) (*Principal, error) {
    // Implementation
}

// Constant naming: PascalCase or SCREAMING_SNAKE_CASE for exported
const (
    DefaultTimeout = 30 * time.Second
    MAX_FILE_SIZE  = 10 << 30 // 10GB
)

// Error handling: always check and wrap errors
func processFile(path string) error {
    file, err := os.Open(path)
    if err != nil {
        return fmt.Errorf("failed to open file %s: %w", path, err)
    }
    defer file.Close()
    
    // Process file...
    if err := doSomething(file); err != nil {
        return fmt.Errorf("failed to process file: %w", err)
    }
    
    return nil
}

// Context handling: context as first parameter
func (s *Service) ProcessRequest(ctx context.Context, req *Request) (*Response, error) {
    // Check context cancellation
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    
    // Process request...
}
```

#### Linting Configuration
```yaml
# .golangci.yml
run:
  timeout: 5m
  modules-download-mode: readonly

linters-settings:
  govet:
    check-shadowing: true
  gocyclo:
    min-complexity: 15
  maligned:
    suggest-new: true
  dupl:
    threshold: 100
  goconst:
    min-len: 2
    min-occurrences: 2
  misspell:
    locale: US
  lll:
    line-length: 140
  goimports:
    local-prefixes: callfs
  gocritic:
    enabled-tags:
      - diagnostic
      - experimental
      - opinionated
      - performance
      - style

linters:
  disable-all: true
  enable:
    - bodyclose
    - deadcode
    - depguard
    - dogsled
    - dupl
    - errcheck
    - exhaustive
    - exportloopref
    - funlen
    - gochecknoinits
    - goconst
    - gocritic
    - gocyclo
    - gofmt
    - goimports
    - gomnd
    - goprintffuncname
    - gosec
    - gosimple
    - govet
    - ineffassign
    - lll
    - misspell
    - nakedret
    - noctx
    - nolintlint
    - rowserrcheck
    - staticcheck
    - structcheck
    - stylecheck
    - typecheck
    - unconvert
    - unparam
    - unused
    - varcheck
    - whitespace

issues:
  exclude-rules:
    - path: _test\.go
      linters:
        - gomnd
        - funlen
```

### Git Workflow

#### Branch Naming
```bash
# Feature branches
feature/add-s3-backend
feature/implement-caching

# Bug fixes
bugfix/fix-auth-middleware
bugfix/handle-large-uploads

# Hotfixes
hotfix/security-patch-cve-2023-12345

# Documentation
docs/update-api-documentation

# Chores
chore/update-dependencies
chore/improve-ci-pipeline
```

#### Commit Messages
```
type(scope): description

[optional body]

[optional footer(s)]
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, etc.)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks
- `perf`: Performance improvements
- `security`: Security fixes

**Examples:**
```bash
feat(auth): add OIDC authentication support

Implement OpenID Connect authentication provider to support
enterprise SSO integration.

Closes #123

fix(backend): handle S3 connection timeouts

Add retry logic and proper error handling for S3 connection
timeouts to improve reliability.

Fixes #456

docs(api): update file upload examples

Add comprehensive examples for file upload API including
multipart uploads and error handling.
```

### Pull Request Process

#### PR Template
```markdown
## Description
Brief description of changes and motivation.

## Type of Change
- [ ] Bug fix (non-breaking change which fixes an issue)
- [ ] New feature (non-breaking change which adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to not work as expected)
- [ ] Documentation update

## Testing
- [ ] Unit tests pass
- [ ] Integration tests pass
- [ ] New tests added for new functionality
- [ ] Manual testing performed

## Checklist
- [ ] Code follows the project's style guidelines
- [ ] Self-review of code completed
- [ ] Code is commented, particularly in hard-to-understand areas
- [ ] Documentation updated if needed
- [ ] No new warnings or errors introduced

## Related Issues
Closes #[issue_number]

## Screenshots (if applicable)
<!-- Add screenshots for UI changes -->

## Additional Notes
<!-- Any additional information that reviewers should know -->
```

#### Review Guidelines

**For Authors:**
1. Ensure all tests pass
2. Keep PRs focused and small
3. Write clear commit messages
4. Update documentation
5. Add tests for new functionality

**For Reviewers:**
1. Check code quality and adherence to guidelines
2. Verify test coverage
3. Test functionality manually if needed
4. Provide constructive feedback
5. Approve only when confident in changes

### Release Process

#### Semantic Versioning
- **MAJOR**: Breaking changes
- **MINOR**: New features (backward compatible)
- **PATCH**: Bug fixes (backward compatible)

#### Release Checklist
1. **Pre-release:**
   - [ ] All tests pass
   - [ ] Documentation updated
   - [ ] CHANGELOG.md updated
   - [ ] Version bumped in relevant files
   - [ ] Security scan passed
   - [ ] Performance benchmarks acceptable

2. **Release:**
   - [ ] Tag created (`git tag v1.2.3`)
   - [ ] GitHub release created
   - [ ] Docker images built and pushed
   - [ ] Documentation deployed
   - [ ] Announcement prepared

3. **Post-release:**
   - [ ] Monitor for issues
   - [ ] Update development branch
   - [ ] Clean up feature branches
   - [ ] Update project roadmap

## Advanced Development Topics

### Performance Optimization

#### Profiling
```go
// Enable pprof endpoint in development
import _ "net/http/pprof"

func main() {
    if os.Getenv("CALLFS_ENV") == "development" {
        go func() {
            log.Println(http.ListenAndServe("localhost:6060", nil))
        }()
    }
    
    // Rest of application...
}
```

#### Memory Management
```go
// Use sync.Pool for frequently allocated objects
var bufferPool = sync.Pool{
    New: func() interface{} {
        return make([]byte, 32*1024) // 32KB buffer
    },
}

func processData(data []byte) error {
    buf := bufferPool.Get().([]byte)
    defer bufferPool.Put(buf)
    
    // Use buffer...
    return nil
}

// Pre-allocate slices when size is known
func processItems(count int) []Item {
    items := make([]Item, 0, count) // Pre-allocate capacity
    
    for i := 0; i < count; i++ {
        items = append(items, Item{ID: i})
    }
    
    return items
}
```

#### I/O Optimization
```go
// Use streaming for large files
func streamFile(w io.Writer, r io.Reader) error {
    // Use a buffer pool for the copy operation
    buf := make([]byte, 64*1024) // 64KB buffer
    _, err := io.CopyBuffer(w, r, buf)
    return err
}

// Batch database operations
func batchInsert(tx *sql.Tx, items []Item) error {
    stmt, err := tx.Prepare("INSERT INTO items (id, name) VALUES ($1, $2)")
    if err != nil {
        return err
    }
    defer stmt.Close()
    
    for _, item := range items {
        if _, err := stmt.Exec(item.ID, item.Name); err != nil {
            return err
        }
    }
    
    return nil
}
```

### Security Considerations

#### Input Validation
```go
// Validate file paths to prevent directory traversal
func validatePath(path string) error {
    // Clean the path
    cleaned := filepath.Clean(path)
    
    // Check for directory traversal attempts
    if strings.Contains(cleaned, "..") {
        return errors.New("invalid path: contains directory traversal")
    }
    
    // Check for absolute paths
    if filepath.IsAbs(cleaned) {
        return errors.New("invalid path: absolute paths not allowed")
    }
    
    return nil
}

// Rate limiting implementation
type RateLimiter struct {
    limiters map[string]*rate.Limiter
    mu       sync.RWMutex
    rate     rate.Limit
    burst    int
}

func (rl *RateLimiter) Allow(key string) bool {
    rl.mu.RLock()
    limiter, exists := rl.limiters[key]
    rl.mu.RUnlock()
    
    if !exists {
        rl.mu.Lock()
        limiter = rate.NewLimiter(rl.rate, rl.burst)
        rl.limiters[key] = limiter
        rl.mu.Unlock()
    }
    
    return limiter.Allow()
}
```

#### Secure Defaults
```go
// TLS configuration with secure defaults
func tlsConfig() *tls.Config {
    return &tls.Config{
        MinVersion: tls.VersionTLS12,
        CipherSuites: []uint16{
            tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
            tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
            tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
        },
        PreferServerCipherSuites: true,
        CurvePreferences: []tls.CurveID{
            tls.CurveP256,
            tls.X25519,
        },
    }
}
```

### Monitoring and Observability

#### Structured Logging
```go
// Use structured logging with context
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    logger := h.logger.With(
        "request_id", middleware.GetRequestID(ctx),
        "method", r.Method,
        "path", r.URL.Path,
        "remote_addr", r.RemoteAddr,
    )
    
    logger.Info("request started")
    
    start := time.Now()
    defer func() {
        logger.With("duration", time.Since(start)).Info("request completed")
    }()
    
    // Handle request...
}
```

#### Custom Metrics
```go
// Define custom metrics
var (
    fileOperations = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "callfs_file_operations_total",
            Help: "Total number of file operations",
        },
        []string{"operation", "backend", "status"},
    )
    
    fileSize = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "callfs_file_size_bytes",
            Help: "Size of files processed",
            Buckets: prometheus.ExponentialBuckets(1024, 2, 10), // 1KB to 1MB
        },
        []string{"operation"},
    )
)

func init() {
    prometheus.MustRegister(fileOperations, fileSize)
}

// Use metrics in handlers
func (h *Handler) uploadFile(w http.ResponseWriter, r *http.Request) {
    start := time.Now()
    
    defer func() {
        fileOperations.With(prometheus.Labels{
            "operation": "upload",
            "backend":   h.backend.Name(),
            "status":    "success", // or "error"
        }).Inc()
    }()
    
    // Handle upload...
    size := getUploadSize(r)
    fileSize.With(prometheus.Labels{"operation": "upload"}).Observe(float64(size))
}
```

This comprehensive developer guide provides everything needed to contribute to the CallFS project, from environment setup to advanced development topics. The focus on Go best practices, testing strategies, and production readiness ensures high-quality contributions that align with the project's standards.
