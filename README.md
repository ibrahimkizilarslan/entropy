<div align="center">
  <h1>🔥 DevChaosKit</h1>
  <p><b>Local Chaos Engineering Toolkit for Docker Microservices</b></p>
  
  [![Python Version](https://img.shields.io/badge/python-3.10%2B-blue.svg)](https://python.org)
  [![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
  [![Status](https://img.shields.io/badge/Status-Active-success.svg)]()
</div>

<br/>

DevChaosKit is a developer-first, CLI-based chaos engineering tool designed specifically for local Docker environments. It helps you build resilient microservices by safely injecting controlled failures (stops, restarts) into your local containers to observe how your system reacts.

Instead of waiting for a production outage to find out if your retry mechanisms or circuit breakers work, DevChaosKit lets you test your fault tolerance right from your terminal during the development phase.

## ✨ Features

- **Safe by Default:** Hard allow-lists (`targets`) ensure that only the specified containers are ever touched.
- **Daemon Mode:** Runs in the background as a lightweight thread, randomly injecting failures at configured intervals.
- **Dry-Run Mode:** Simulate chaos without actually touching Docker, perfect for testing your configuration.
- **Safety Control Layer:** 
  - `max_down`: Prevent catastrophic failure by limiting how many services can be down simultaneously.
  - `cooldown`: Enforce strict waiting periods between chaos injections to allow system recovery.
- **Rich Observability:** Live terminal status, colored log viewer, and comprehensive per-session statistics reports.
- **Zero Dependencies:** Uses the official Docker Python SDK, communicating directly with the Docker daemon.

## 🚀 Installation

Prerequisites:
- Python 3.10 or higher
- Docker Daemon running (Docker Desktop or native Linux)

```bash
# 1. Clone the repository
git clone https://github.com/ibrahimkizilarslan/DevChaosKit.git
cd DevChaosKit

# 2. Install the CLI tool
pip install -e .

# 3. Verify installation
devchaos version
```

## 🎯 Quick Start

### 1. Create a configuration file
In your project directory, create a `chaos.yaml` file to define your targets and safety rules.

```yaml
interval: 10
targets:
  - my-api-gateway
  - my-auth-service
  - my-database
actions:
  - stop
  - restart
safety:
  max_down: 1          # Max 1 service down at a time
  cooldown: 15         # Wait 15s between injections
  dry_run: false       # Set to true for a safe rehearsal
```

### 2. Start the Chaos Engine
Start the engine in the background:
```bash
devchaos start --detach
```

### 3. Monitor the Chaos
Watch the live engine status and cooldown timers:
```bash
watch -n 2 devchaos status
```

Follow the structured logs in real-time to see what the engine is doing:
```bash
devchaos logs -f
```

### 4. Stop and Review
When you are done testing your system's resilience, stop the engine and view the session report:
```bash
devchaos stop
devchaos report
```

## 🛠️ CLI Command Reference

| Command | Description |
|---------|-------------|
| `devchaos start` | Starts the chaos engine. Flags: `--detach`, `--dry-run`, `--max-down`, `--cooldown` |
| `devchaos stop` | Safely terminates a running background engine. |
| `devchaos status` | Displays live engine metrics, running PID, and active cooldown bars. |
| `devchaos inject` | Manually inject a single action. Usage: `devchaos inject stop <target>` |
| `devchaos logs` | View engine logs. Flags: `-f` (follow), `-n` (lines), `--level` (ACTION/SKIP/ERROR) |
| `devchaos report` | Generate a comprehensive statistics report (hits, safety events) from the last session. |
| `devchaos docker` | Sub-commands to inspect or manually restart/stop Docker containers. |

## 🏗️ Architecture
DevChaosKit is built using Python, `Typer` for the CLI interface, and `Rich` for the beautiful terminal UI. 

Communication between the CLI and the background daemon is managed purely through an atomic JSON state file (`.devchaos/state.json`), ensuring high reliability without the need for complex IPC sockets.

## 📄 License
This project is licensed under the MIT License - see the LICENSE file for details.
