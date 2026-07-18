# Changelog

All notable changes to this project are documented in this file.

The format is inspired by [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [3.0.0] - 2026-07-18

This release closes out a full engineering-quality pass across security,
concurrency correctness, CI, and test coverage. **It contains breaking
changes** — see the Security section below before upgrading.

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
- Strengthened the production-safety guard for the Docker and Kubernetes
  runtimes. Previously it only checked `ENTROPY_ENVIRONMENT=production`, an
  Entropy-specific env var that real production environments have no reason
  to set — making the guard effectively dead in practice. It now also
  refuses to start when the active Docker context (`~/.docker/config.json`)
  or Kubernetes kubeconfig context name looks like production (case-
  insensitive match on `prod`/`production`, e.g. `prod-us-east`,
  `acme-production`). Both checks are bypassed by
  `ENTROPY_ALLOW_PRODUCTION=true`. See
  [pkg/engine/safety.go](pkg/engine/safety.go).

### Added
- `entropy --version` and `entropy version` now print the build version
  (previously the `Version` variable was set via ldflags but never wired
  into the root command, so `--version` was not recognized). See
  [pkg/cli/root.go](pkg/cli/root.go).
- CI now runs `golangci-lint` (config: `.golangci.yml`) and `govulncheck` on
  every push/PR, alongside the existing `go vet`/`go test` job. See
  `make lint` / `make vulncheck` for local equivalents.
- `.github/dependabot.yml` for automated weekly `gomod` and
  `github-actions` dependency update PRs.
- End-to-end smoke pipeline script at `scripts/e2e-smoke.sh`.
- CI smoke workflow job to validate command-level behavior against the demo stack.
- `SECURITY.md` with vulnerability reporting guidance and runtime safety notes.
- `docs/release-checklist.md` with release and launch verification steps.

### Fixed
- The chaos engine's main loop no longer blocks on a slow injection cycle
  when handling stop/Ctrl+C. Previously, `runCycle` (which can call a
  Docker/Kubernetes API, e.g. stopping a container or injecting network
  chaos) ran synchronously inside the loop's `select`, so a slow API call
  delayed the loop from observing the stop signal until the cycle finished.
  `runCycle` now runs in a background goroutine via `startCycleAsync`, with
  a single-flight guard (overlapping cycles could otherwise race on the
  cooldown/max_down safety checks) and a `sync.WaitGroup` that `runLoop`
  waits on before running cleanup, so cleanup never races with a cycle
  that's still injecting or reverting chaos. The new goroutine has its own
  panic recovery — `runLoop`'s existing recover only protects its own
  goroutine, so without this, a panic during an async cycle would have
  crashed the whole process instead of being logged. See
  [pkg/engine/chaos_engine.go](pkg/engine/chaos_engine.go).
- Network and resource chaos injection/revert no longer serializes across
  unrelated targets. `NetworkChaosManager` and `ResourceChaosManager`
  previously held a single mutex across their `tc` exec / registry I/O calls,
  so a slow operation on one container (e.g. a stalled Docker exec or
  Kubernetes SPDY stream) blocked injection/revert for every other container.
  Both managers now use per-target locking: operations on different targets
  run fully in parallel, while operations on the same target remain
  serialized for correctness. See
  [pkg/engine/network_chaos.go](pkg/engine/network_chaos.go) and
  [pkg/engine/actions.go](pkg/engine/actions.go).
- Fixed a data-loss race in `FaultRegistry.persist()`
  ([pkg/registry/store.go](pkg/registry/store.go)), found while making the
  fix above: concurrent `Write`/`MarkReverted`/`GarbageCollect` calls wrote to
  the same fixed temp file path with no serialization between them, so one
  call's rename could win with a disk snapshot missing another call's
  already-committed in-memory record — silently dropping fault records. This
  was latent before (nothing called the registry concurrently), but the
  lock-contention fix above makes concurrent registry writes a normal,
  expected code path. `persist()` is now serialized via a dedicated mutex.
- The fault registry's `persist()` now fsyncs the containing directory
  after the atomic rename, hardening the rename's durability against a
  crash immediately afterward (a rename is a metadata operation on the
  directory; POSIX only guarantees it survives a crash once that
  directory's own fsync has completed). Best-effort: harmless on platforms
  that don't support syncing a directory handle. See
  [pkg/registry/store.go](pkg/registry/store.go).

### Changed
- Upgraded the Go toolchain requirement from `1.26.0` to `1.26.5` and
  `golang.org/x/net` from `v0.49.0` to `v0.57.0`, resolving 25
  `govulncheck`-reported vulnerabilities (Go standard library CVEs fixed in
  1.26.1–1.26.5, plus `golang.org/x/net` CVEs). `github.com/docker/docker@v24`
  still carries 6 known findings that require a v24→v25 major-version
  upgrade; that upgrade is tracked separately (breaking API risk, needs a
  real Docker daemon to validate) and the `vulncheck` CI job is configured
  with `continue-on-error` until it lands — see the job's inline comment in
  `.github/workflows/ci.yml`.
- `worker.RunDaemon` now takes a single `DaemonOptions` struct (with a
  `SafetyOverrides` sub-struct for the CLI-flag overrides) instead of six
  positional parameters, three of which were pointers used purely as an
  ad hoc "was this flag passed?" signal. Same behavior, clearer call sites.
  See [pkg/worker/daemon.go](pkg/worker/daemon.go).
- `ScenarioStep` YAML parsing now dispatches on which top-level key
  (`wait`/`inject`/`probe`) is present, instead of guessing the step type
  from which fields happen to be non-empty. A step with a recognized key
  but a missing required field (e.g. `inject:` without `target:`) now fails
  with a specific, actionable error instead of the previous generic
  "unknown scenario step format" — the old field-sniffing approach never
  matched the step's actual shape, so it silently fell through every case
  and lost the fact that the author had written `inject:` at all. Multiple
  step-type keys in a single step are also now rejected explicitly. See
  [pkg/config/schema.go](pkg/config/schema.go).
- Corrected README references to the fault registry as a "WAL" (write-ahead
  log) — the registry actually persists a full state snapshot on every
  write, not an append-only log. Now described as a "Persistent Fault
  Registry (Atomic State File)". No behavior change.
- The `README.md` "Go Report Card" badge was a hand-authored static image
  (`img.shields.io/badge/go%20report-A%2B-...`), not the real
  goreportcard.com badge — it always showed "A+" regardless of the actual
  score. Replaced with the real, dynamically-generated goreportcard.com
  badge.
- `README.md`'s documented `--help` output was missing the `registry` and
  `version` subcommands; synced with actual CLI output.
- Removed an unreachable `"unpause"` check in `ChaosEngine.runCycle`
  ([pkg/engine/chaos_engine.go](pkg/engine/chaos_engine.go)): `"unpause"`
  is not a valid standalone action (see `config.ValidActions` /
  `actionHandlers`), so `actionSpec.Name` can never equal it in the
  random-chaos model. No behavior change.
- Refreshed `README.md` badges and public repository links.
- Updated `chaos.example.yaml` to reflect the latest supported action patterns.
- Updated `chaos-scenario.example.yaml` to a runnable, fully English scenario example.
- Backfilled `CHANGELOG.md` with entries for the previously-undocumented
  `v0.1.0-beta.1` through `v2.0.0` releases (the file only had an
  `[Unreleased]` section despite six published tags), derived from the
  git history of each release range.

### Testing
- Raised test coverage for `pkg/worker` (8.6% → 67.6%) and `pkg/cli`
  (13.0% → 26.1%), which previously had almost no coverage on the daemon
  lifecycle and CLI command wiring.
  - `worker.RunDaemon` was split into `applySafetyOverrides` and
    `runDaemonLoop`, with the state directory and stop signal channel now
    injectable. This allows driving the daemon's actual start → write-
    state → stop-signal → clear-state lifecycle in a test without a live
    Docker daemon or real OS signal delivery. See
    [pkg/worker/daemon.go](pkg/worker/daemon.go).
  - Added tests that call `doctorCmd.Run`, `topologyCmd.Run`,
    `validateCmd.Run`, and `initCmd.Run` directly (rather than only their
    underlying `engine`/`config` helpers), exercising the real command
    wiring for the commands that don't require a live Docker/Kubernetes
    connection.

## [2.0.0] - 2026-06-09

### Added
- Persistent Fault Registry (`pkg/registry`): a durable, atomic on-disk
  record of every active chaos injection, enabling crash-safe recovery.
- Auto-recovery on boot: orphaned faults from a previous crash are
  discovered and reverted before a new engine session starts.
- Background expiry watcher: sweeps faults whose scheduled auto-revert
  timer was missed (e.g. due to a hang), as a second layer of safety.
- `entropy registry list` / `entropy registry clear` commands and a
  `recoverAll` flag for manual registry inspection and recovery.

## [2.0.0-beta.1] - 2026-06-09

### Security
- Critical security hardening (6 fixes) across network chaos, probes, and
  scenario lifecycle handling.

### Changed
- Runtime lookup and polling performance optimizations.
- General code quality improvements.

## [1.2.0] - 2026-06-06

### Added
- Network chaos duration handling for the Kubernetes runtime.
- Network segmentation in the bundled distributed demo.
- Error logging for previously-swallowed errors.

### Changed
- Migrated from `math/rand` to `math/rand/v2`.
- Extracted the HTML report template to a separate file via `go:embed`.

### Fixed
- Restored the `cleanup` command with a working implementation.

## [1.1.0] - 2026-05-26

### Added
- Configuration validation.

### Changed
- Enhanced engine observability.
- Hardened core engine resilience and consolidated the Kubernetes client
  construction into a single source of truth.

## [1.0.0] - 2026-05-16

### Added
- Kubernetes runtime support: agentless chaos injection via ephemeral
  containers (no DaemonSets, no node-level agents).
- `ContainerRuntime` interface abstracting Docker and Kubernetes behind a
  single API, with `context.Context` propagated through every call.
- `entropy doctor` analysis for Kubernetes topologies.
- `MockRuntime`-based behavioral test suite for the engine.
- CI pipeline (GitHub Actions) and goreleaser-based release process.

### Security
- Hardened network chaos, probes, and scenario lifecycle handling.

## [0.1.0-beta.1] - 2026-05-11

Initial public release, ported from an earlier Python prototype
(rebranded from "DevChaosKit" to "Entropy").

### Added
- Core chaos engine: Docker abstraction layer, engine core, CLI.
- Network and resource chaos actions (delay, loss, CPU/memory limits).
- Scenario Engine with HTTP probes and step execution.
- `entropy init` for zero-config discovery of `docker-compose` services.
- Bundled polyglot distributed microservices demo.
- MkDocs Material documentation site.

