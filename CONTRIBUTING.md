# Contributing to CallFS

Thank you for your interest in contributing to CallFS! This document provides guidelines and instructions for contributing to the project.

## Code of Conduct

By participating in this project, you are expected to uphold our [Code of Conduct](CODE_OF_CONDUCT.md).

## Branching Model

We follow a branching model designed for collaboration and maintainability:

1. **`main`**: The production-ready branch. All releases are tagged from this branch.

2. **`dev`**: Development branch where features are integrated before being merged to `main`.

3. **Feature branches**: When working on a new feature or bugfix, create a branch from `dev` with the format:
   - For features: `feature/short-description`
   - For bugfixes: `fix/short-description`
   - For documentation: `docs/short-description`

4. **Pull requests**: When your changes are ready, create a pull request to the `dev` branch for review.

## Development Workflow

1. **Fork the repository** and clone your fork locally.

2. **Set up the development environment**:
   ```bash
   # Install dependencies
   go mod download
   
   # Copy example config
   cp config.yaml.example config.yaml
   
   # Generate self-signed certificates for development
   go run cmd/main.go gencert
   ```

3. **Create a new branch** for your changes:
   ```bash
   git checkout -b feature/your-feature-name
   ```

4. **Make your changes** and ensure they pass all tests and lint checks.

5. **Commit your changes** with clear, descriptive commit messages.

6. **Push your branch** to your forked repository.

7. **Create a pull request** to the `dev` branch of the main repository.

## Testing and Linting

Before submitting a pull request, ensure all tests pass and the code follows our style guidelines:

```bash
# Run unit tests
go test ./...

# Run tests with race detection
go test -race ./...

# Run linting
golangci-lint run
```

## Pull Request Requirements

All pull requests must:

1. Include appropriate tests for new functionality
2. Pass all existing tests
3. Pass linting checks
4. Include documentation updates if necessary
5. Be rebased on the latest `dev` branch
6. Adhere to the existing code style

## Removed Directory Policy

We follow the "Safety-Net rule" for file removal:

When moving or deleting files, first copy them to `_removed/...` directory structure to ensure we don't accidentally lose important data. This directory is git-ignored but serves as a local backup.

For example:
```bash
# When removing a file
cp path/to/file.go _removed/path/to/file.go
git rm path/to/file.go
```

## Documentation

- Code should be well-commented
- Public APIs must have comprehensive documentation
- Each exported package should have a package-level doc comment
- Significant features should be documented in the `/docs` directory

## Code Review Process

1. At least one maintainer must review and approve every pull request
2. Automated checks must pass before merging
3. Feedback should be addressed before approval
4. Maintainers may request changes before merging

## Getting Help

If you need help or have questions:

- Open an issue with the label "question"
- Contact the maintainers via email

Thank you for contributing to CallFS!
