# Entropy

Entropy is a developer-first chaos engineering CLI for Docker-based microservices.

It is written in Go and shipped as a single binary so developers can quickly run resilience checks in local environments.

## What You Can Do

- Auto-discover services from `docker-compose.yml` and generate `chaos.yaml`
- Run random chaos in a daemon with safety constraints
- Run deterministic scenarios with `probe`, `inject`, and `wait` steps
- Inject lifecycle, network, and resource faults into target containers
- Generate topology and resilience analysis outputs with `topology` and `doctor`

## Installation

Use one of the following methods:

```bash
# Install latest binary with Go
go install github.com/ibrahimkizilarslan/entropy/cmd/entropy@latest

# Or clone and build
git clone https://github.com/ibrahimkizilarslan/entropy.git
cd entropy
go build -o entropy ./cmd/entropy
```

## Quick Start

```bash
# 1) Generate a chaos config from compose services
./entropy init

# 2) Run deterministic scenario
./entropy scenario run chaos-scenario.example.yaml

# 3) Or run background random chaos
./entropy start --detach
./entropy status
./entropy stop
```

## Next Docs

- [Scenario DSL Reference](scenarios.md)
- [Random Chaos Engine](random-chaos.md)
- [CLI Reference](cli-reference.md)
- [Release Checklist](release-checklist.md)

