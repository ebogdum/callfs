# Security Policy

## Reporting Security Vulnerabilities

We take the security of CallFS seriously. If you discover a security vulnerability, please report it to us as described below.

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, please send an email to [admin.nxpoint@gmail.com](mailto:admin.nxpoint@gmail.com) with the following information:

- Description of the vulnerability
- Steps to reproduce or proof-of-concept
- Affected versions
- Potential impact

We will acknowledge your email within 48 hours and will send a more detailed response within 72 hours indicating the next steps in handling your report.

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |

## Security Measures

CallFS implements several security measures:

- API key authentication for all endpoints
- Unix socket-based authorization for local access
- Rate limiting to prevent abuse
- Input validation and sanitization
- Secure file path handling to prevent directory traversal
- TLS encryption for network communication

## Security Best Practices

When deploying CallFS:

1. Use strong, randomly generated API keys
2. Enable TLS/HTTPS in production
3. Regularly update to the latest version
4. Monitor access logs for suspicious activity
5. Restrict network access to necessary endpoints only
6. Use appropriate file system permissions
