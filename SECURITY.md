# Security Policy

## Supported Versions

This project is currently in early-stage development.
Security fixes are prioritized for the latest release on the `main` branch.

## Reporting a Vulnerability

If you discover a security issue, please do not open a public issue first.

- Send a private report to the maintainer through GitHub security advisories:
  [Report a vulnerability](https://github.com/ibrahimkizilarslan/entropy-cli/security/advisories/new)
- Include:
  - A clear description of the issue
  - Reproduction steps or proof of concept
  - Impact assessment
  - Any proposed remediation

You can expect an initial response within 72 hours.

## Operational Safety Notice

Entropy can intentionally disrupt running services and network behavior.
Use it only in controlled environments (local/dev/staging).

The Docker client includes a production environment guard:
- If `ENTROPY_ENVIRONMENT=production` and `ENTROPY_ALLOW_PRODUCTION` is not `true`,
  Entropy refuses to run.
