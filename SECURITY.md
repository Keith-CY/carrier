# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Carrier, please report it responsibly. **Do not open a public issue.**

### Private Reporting

- **GitHub Security Advisories:** Use [GitHub's private vulnerability reporting](https://github.com/Keith-CY/carrier/security/advisories/new) to submit a report directly.
- **Email:** Contact the maintainer at the email listed in their GitHub profile.

### What to Include

- Description of the vulnerability
- Steps to reproduce
- Potential impact assessment
- Suggested fix (if any)

## Response Timeline

- **Initial acknowledgment:** within 3 business days of report submission.
- **Triage and severity assessment:** within 7 business days.
- **Status updates:** at least every 14 days until resolution.
- **Fix and disclosure:** coordinated with the reporter before public disclosure.

## Supported Versions

| Version | Supported |
|---------|-----------|
| `main` branch (latest) | ✅ Active development and security fixes |
| Release tags | ✅ Latest release receives critical security patches |
| Older releases | ❌ No backport guarantees |

This project is in Phase 1 development. Security fix policy will be refined as the project matures.

## Security Practices

- All command execution inputs are sanitized (see `docs/security-audit-command-execution.md`).
- Download tokens use crypto-secure generation.
- Gateway session management enforces pairing checks before command routing.
