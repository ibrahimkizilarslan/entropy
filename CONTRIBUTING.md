# Contributing to Entropy CLI

First off, thank you for considering contributing to Entropy CLI! It's people like you that make Entropy such a great tool for Chaos Engineering.

## How to Contribute

### 1. Reporting Bugs
- Use the **Bug Report** issue template.
- Ensure you have the latest version of Entropy installed.
- Include your OS, Docker version, and steps to reproduce.

### 2. Suggesting Enhancements
- Use the **Feature Request** issue template.
- Clearly describe the problem you are trying to solve or the value the new feature adds.

### 3. Submitting Pull Requests
1. Fork the repository and create your branch from `main`.
2. Ensure your code passes `go vet ./...` and `go test ./...`.
3. Update the documentation (`README.md` or the `docs/` folder) if you are adding a new feature.
4. Issue that pull request!

## Local Development Setup

1. **Prerequisites:** 
   - Go 1.21+
   - Docker daemon running locally (Docker Desktop or native Linux).

2. **Building the CLI:**
   ```bash
   go build -o entropy ./cmd/entropy
   ```

3. **Running the Distributed Demo:**
   Entropy includes a built-in polyglot microservice demo for testing:
   ```bash
   cd examples/demo-distributed
   docker-compose up -d --build
   ```
   You can now test your local Entropy build against these containers.

## Architecture Guidelines

- **Keep the core agnostic:** While we currently heavily rely on the Docker Engine API, the core scenario runner should not be tightly coupled to Docker. Ensure interfaces are used where applicable.
- **Fail gracefully:** Entropy is a resilience tool. It must never crash unexpectedly. Always handle errors and ensure `RevertAll` functions clean up any injected chaos.

We look forward to your contributions!
