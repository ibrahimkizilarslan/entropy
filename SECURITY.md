# Security Policy

## Supported Versions

This project is currently in early-stage development.
Security fixes are prioritized for the latest release on the `main` branch.

## Reporting a Vulnerability

If you discover a security issue, please do not open a public issue first.

- Send a private report to the maintainer through GitHub security advisories:
  [Report a vulnerability](https://github.com/ibrahimkizilarslan/entropy/security/advisories/new)
- Include:
  - A clear description of the issue
  - Reproduction steps or proof of concept
  - Impact assessment
  - Any proposed remediation

You can expect an initial response within 72 hours.

## Operational Safety Notice

Entropy can intentionally disrupt running services and network behavior.
Use it only in controlled environments (local/dev/staging).

Both the Docker and Kubernetes runtimes include a production-safety guard
that refuses to start unless `ENTROPY_ALLOW_PRODUCTION=true` is set, when
either of the following is true:
- `ENTROPY_ENVIRONMENT=production` is set explicitly, or
- the active runtime context name looks like a production environment — a
  case-insensitive match on `prod` or `production` against the active Docker
  context (from `~/.docker/config.json`) or Kubernetes kubeconfig context
  (`current-context`). This catches common naming conventions like
  `prod-us-east` or `acme-production` without requiring operators to remember
  to set `ENTROPY_ENVIRONMENT` themselves.

This is a heuristic safety net, not an access-control mechanism: it can be
bypassed intentionally (`ENTROPY_ALLOW_PRODUCTION=true`) and cannot see into
contexts it has no way to name (e.g. a remote `DOCKER_HOST` target, or
in-cluster Kubernetes config, where there is no "context" name to inspect).
Scope what Entropy can reach — via `--runtime`, kubeconfig/Docker context
selection, and the `targets` allow-list in `chaos.yaml` — rather than relying
on this guard alone.
