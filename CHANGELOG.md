# Changelog

All notable changes to this project are documented in this file.

The format is inspired by [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Security
- **[BREAKING]** `entropy inject` now fails closed when its allow-list config
  (`chaos.yaml` by default) cannot be loaded. Previously, a missing or invalid
  config file silently disabled the target allow-list, allowing `inject` to
  target *any* container/pod on the host. It now refuses to run unless
  `--skip-validation` is explicitly passed, which now also prints a visible
  warning. See [pkg/cli/chaos.go](pkg/cli/chaos.go).
- **[BREAKING]** Exec probe command validation switched from a blocklist to an
  allowlist. Previously, `validateExecCommand` blocked a fixed list of known-bad
  executables (shells, interpreters, network tools), which could be bypassed
  via commands like `env sh -c '...'`, `busybox sh`, `awk 'BEGIN{system(...)}'`,
  or `find -exec` — none of which were on the blocklist. Only a small set of
  read-only diagnostic commands (`cat`, `ls`, `stat`, `test`, `true`, `false`,
  `echo`, `pgrep`, `ps`, `head`, `tail`, `wc`, `grep`) is now permitted by
  default, with arguments additionally scanned for shell metacharacters.
  Scenarios relying on other commands can opt in via the new
  `ENTROPY_EXEC_ALLOWLIST` environment variable. See
  [pkg/engine/probes.go](pkg/engine/probes.go).
- Hardened SSRF protection for `http`/`tcp` scenario probes. The previous
  implementation resolved the probe's hostname once for validation, then let
  the HTTP client / TCP dialer resolve it again independently to connect —
  leaving a DNS-rebinding / TOCTOU gap where a hostname could pass validation
  but connect to a blocked IP. Validation now also runs at actual dial time
  (via `net.Dialer.Control`), checked against the exact IP the connection is
  being made to. Cloud metadata IPs (AWS/GCP/Azure `169.254.169.254`, Alibaba
  `100.100.100.200`, AWS IMDSv2 IPv6) are always blocked. Private/loopback/
  link-local ranges remain allowed by default (chaos probes legitimately
  target local infrastructure) but can now be blocked via the new
  `ENTROPY_ALLOW_PRIVATE_NETWORKS=false` environment variable. See
  [pkg/engine/probes.go](pkg/engine/probes.go).

### Added
- End-to-end smoke pipeline script at `scripts/e2e-smoke.sh`.
- CI smoke workflow job to validate command-level behavior against the demo stack.
- `SECURITY.md` with vulnerability reporting guidance and runtime safety notes.
- `docs/release-checklist.md` with release and launch verification steps.

### Changed
- Refreshed `README.md` badges and public repository links.
- Updated `chaos.example.yaml` to reflect the latest supported action patterns.
- Updated `chaos-scenario.example.yaml` to a runnable, fully English scenario example.


