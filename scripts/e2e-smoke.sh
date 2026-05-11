#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$ROOT_DIR/entropy"
DEMO_DIR="$ROOT_DIR/examples/demo-distributed"
SMOKE_SCENARIO="/tmp/entropy-smoke-scenario.yaml"
WITH_DEMO_COMPOSE=false

if [[ "${1:-}" == "--with-demo-compose" ]]; then
  WITH_DEMO_COMPOSE=true
fi

compose_cmd() {
  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    docker compose "$@"
    return
  fi
  if command -v docker-compose >/dev/null 2>&1; then
    docker-compose "$@"
    return
  fi
  echo "ERROR: neither 'docker compose' nor 'docker-compose' is available." >&2
  exit 1
}

wait_for_demo() {
  local tries=30
  local url="http://localhost:8085/api/catalog"
  for _ in $(seq 1 "$tries"); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 2
  done
  echo "ERROR: demo endpoint did not become ready: $url" >&2
  exit 1
}

cleanup() {
  if [[ "$WITH_DEMO_COMPOSE" == "true" ]]; then
    echo "==> [cleanup] Stopping demo environment"
    (cd "$DEMO_DIR" && compose_cmd down -v)
  fi
}

trap cleanup EXIT

if [[ "$WITH_DEMO_COMPOSE" == "true" ]]; then
  echo "==> [0/8] Starting demo environment"
  (cd "$DEMO_DIR" && compose_cmd up -d --build)
  wait_for_demo
fi

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



