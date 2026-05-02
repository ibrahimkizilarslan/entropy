# Entropy

Entropy is a developer-first chaos engineering platform designed to inject controlled faults into local microservice environments. 

By prioritizing the developer workflow, Entropy enables teams to validate system resilience, identify single points of failure, and confidently test hypothesis-driven scenarios before code reaches production.

## Core Capabilities

- **Zero-Config Discovery:** Automatically parses `docker-compose.yml` to identify and target running services.
- **Hypothesis-Driven Scenarios:** Define deterministic chaos experiments using a declarative YAML DSL. Execute actions, wait for state propagation, and probe APIs to verify system recovery.
- **Resource Constraints:** Dynamically enforce CPU and Memory limits on active containers using Docker SDK integration.
- **Network Degradation:** Inject precise network latency, packet loss, and jitter using Linux `tc` and `netem` within container namespaces.
- **Stateful Safety Mechanisms:** Enforces global cooldowns and maximum simultaneous failure limits to prevent unrecoverable system states.

## Architecture & Vision

Entropy is engineered with a modular, runtime-agnostic abstraction layer. While the current implementation specifically targets local Docker environments (to solve the critical gap in developer-side testing), the core engine is designed to be extensible. 

Future iterations will introduce `KubernetesClient` and Cloud Provider adapters, allowing the exact same scenario configurations to seamlessly transition from a developer's laptop to staging clusters and cloud infrastructure.

## Installation

```bash
# Clone the repository
git clone https://github.com/ibrahimkizilarslan/Entropy.git
cd Entropy

# Install dependencies
go mod download

# Build the binary
go build -o entropy ./cmd/entropy

# (Optional) Move to your path
sudo mv entropy /usr/local/bin/
```

## Quick Start

### 1. Auto-Discovery
Navigate to any directory containing a `docker-compose.yml` file and initialize the workspace:

```bash
entropy init
```
This generates a `chaos.yaml` configuration populated with your discovered services.

### 2. Random Fault Injection (Chaos Monkey Mode)
Start the background engine to randomly inject faults based on your safety constraints:

```bash
entropy start
```

Monitor the active session:
```bash
entropy status
entropy logs
```

### 3. Scenario-Based Testing
Execute deterministic, hypothesis-driven tests:

```bash
entropy scenario run chaos-scenario.example.yaml
```

## Documentation

For a deep dive into configuring and running Entropy, check out the documentation:

- [Scenario DSL Reference](docs/scenarios.md) - Learn how to write deterministic chaos scenarios.
- [Random Chaos Engine](docs/random-chaos.md) - Details on background daemon configuration and safety mechanisms.
- [CLI Reference](docs/cli-reference.md) - Full list of available CLI commands.

## Development

Entropy is written in Go for maximum performance and easy distribution as a single binary. The CLI is built with `Cobra` and `Pterm` for a premium terminal experience.

### Build from source
```bash
go build -o entropy ./cmd/entropy
```

## License
MIT License

