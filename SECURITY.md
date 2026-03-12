# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability, please report it privately:

1. **Do not** open a public issue
2. Email: security@luigiagosti.com (or open a private security advisory on GitHub)
3. Include details about the vulnerability and steps to reproduce

We will respond within 48 hours and work with you to understand and fix the issue.

## Scope

SeaPortal is a content extraction library. Security considerations include:

- **URL handling**: Malformed URLs should not cause crashes
- **HTML parsing**: Malicious HTML should not cause memory issues
- **HTTP requests**: Should follow redirects safely, respect timeouts

## Supported Versions

Only the latest release is supported with security updates.
