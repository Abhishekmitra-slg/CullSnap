# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.1.x   | Yes                |
| < 1.1   | No                 |

Only the latest release line (v1.1.x) receives security updates. Users on older versions should upgrade to the latest release.

## Reporting a Vulnerability

If you discover a security vulnerability in CullSnap, please report it responsibly via email:

**Email:** Abhishekmitra-slg@users.noreply.github.com

**Do not** open a public GitHub issue for security vulnerabilities.

### What to Include

- A clear description of the vulnerability
- Steps to reproduce the issue
- An assessment of the potential impact (e.g., data loss, unauthorized file access, code execution)
- Any relevant environment details (OS, version, configuration)

### Response Timeline

- **Acknowledgment:** Within 48 hours of your report
- **Resolution:** Within 7 days for critical issues; non-critical issues will be prioritized accordingly

## Scope

CullSnap is a local desktop application. Most web-based attack vectors (XSS, CSRF, session hijacking, etc.) do not apply to its threat model. The following areas are in scope for security reports:

- **File handling:** Path traversal, unsafe file operations, or unintended access to files outside the selected directory
- **Local network exposure:** Any unintended network listeners or data leakage over the local network
- **Dependency vulnerabilities:** Issues in third-party libraries that affect CullSnap's functionality
- **Code execution:** Any vector that could lead to arbitrary code execution through crafted input (e.g., malicious image files or EXIF data)

Thank you for helping keep CullSnap secure.
