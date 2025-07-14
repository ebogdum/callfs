# Changelog

All notable changes to CallFS will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2025-07-13

### Added
- Initial public release of CallFS
- REST API filesystem with GET, POST, PUT, HEAD, DELETE operations
- Multiple storage backends: Local filesystem and S3
- Multi-server architecture with transparent internal routing
- Secure single-use links with HMAC signatures
- Distributed locking with Redis backend
- In-memory metadata caching with TTL and LRU eviction
- Prometheus metrics integration
- Rate limiting for API endpoints
- API key authentication
- Unix socket authorization for local access
- Comprehensive logging with structured JSON output
- Security headers middleware (CSP, HSTS, etc.)
- Request ID tracking across requests
- OpenAPI/Swagger documentation
- Docker and Docker Compose support
- Comprehensive test coverage
- CI/CD workflows for GitHub Actions
- Security scanning and code quality checks

### Security
- Added security headers middleware
- Implemented proper error handling to prevent information leakage
- Added rate limiting to prevent abuse
- Secure file path handling to prevent directory traversal attacks

### Documentation
- Installation guide
- Configuration reference
- API documentation with OpenAPI spec
- Developer guide
- Troubleshooting guide
- Contributing guidelines
- Code of conduct
- Security policy
