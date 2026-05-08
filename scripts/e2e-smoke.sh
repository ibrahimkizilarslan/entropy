#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$ROOT_DIR/entropy"
DEMO_DIR="$ROOT_DIR/examples/demo-distributed"
SMOKE_SCENARIO="/tmp/entropy-smoke-scenario.yaml"

echo "==> [1/8] Building entropy binary"
go build -o "$BIN" "$ROOT_DIR/cmd/entropy"

echo "==> [2/8] Verifying CLI surface"
"$BIN" --help >/dev/null
"$BIN" doctor --help >/dev/null
"$BIN" topology --help >/dev/null
"$BIN" scenario --help >/dev/null
"$BIN" inject --help >/dev/null

echo "==> [3/8] Verifying Docker connectivity via entropy"
"$BIN" docker list >/dev/null

echo "==> [4/8] Preparing chaos config from demo compose"
(
  cd "$DEMO_DIR"
  "$BIN" init --force >/dev/null
)

echo "==> [5/8] Running topology and doctor analysis"
(
  cd "$DEMO_DIR"
  "$BIN" topology >/dev/null
  "$BIN" doctor >/dev/null
)

echo "==> [6/8] Verifying manual injection path"
(
  cd "$DEMO_DIR"
  "$BIN" inject restart auth-service -c chaos.yaml >/dev/null
)

echo "==> [7/8] Verifying daemon lifecycle (start/status/stop)"
(
  cd "$DEMO_DIR"
  "$BIN" start --detach -c chaos.yaml --dry-run --cooldown 1 >/dev/null
  sleep 2
  "$BIN" status >/dev/null
  "$BIN" stop >/dev/null
  "$BIN" status >/dev/null
)

echo "==> [8/8] Running scenario engine with a known-good smoke scenario"
cat > "$SMOKE_SCENARIO" <<'EOF'
name: "Smoke Scenario"
description: "Basic e2e sanity check against the demo gateway"
hypothesis: "Gateway remains reachable"
steps:
  - probe:
      type: http
      url: "http://localhost:8085/api/catalog"
      expect_status: 200
EOF
(
  cd "$DEMO_DIR"
  "$BIN" scenario run "$SMOKE_SCENARIO" >/dev/null
)

echo "✅ E2E smoke pipeline passed."
