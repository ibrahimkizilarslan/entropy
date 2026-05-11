# Scenario DSL Reference

Entropy's Scenario Engine allows you to define deterministic, hypothesis-driven chaos experiments. Instead of randomly injecting faults, you can write step-by-step scenarios to test exactly how your system reacts to specific failures.

## Scenario File Structure

A scenario is defined in a YAML file (e.g., `chaos-scenario.yaml`) and consists of a name, description, hypothesis, and a list of sequential steps.

```yaml
name: "Scenario Name"
description: "What this scenario tests"
hypothesis: "What should happen if the system is resilient"
steps:
  - # Step 1
  - # Step 2
```

## Step Types

Entropy supports three types of steps: `probe`, `inject`, and `wait`.

### 1. Probe Step
Used to check the state of the system before, during, or after an injection. Entropy supports `http`, `tcp`, and `exec` probes.

```yaml
- probe:
    type: http
    url: "http://localhost:8080/health"
    expect_status: 200        # Optional: Expected HTTP status code
    expect_not_status: 500    # Optional: Status code that should NOT be returned
    timeout: 5                # Optional: Timeout in seconds
```

### 2. Inject Step
Injects a specific fault into a target container. This can be a Docker lifecycle action, a network fault, or a resource constraint.

```yaml
- inject:
    target: "my-service"      # The name of the Docker container or service
    action:
      name: "stop"            # The fault to inject (see Action Types below)

# Shorthand is supported when no action parameters are needed:
- inject:
    target: "my-service"
    action: stop
```

**Action Types:**

*   **Lifecycle Actions:**
    *   `stop`: Stops the target container.
    *   `restart`: Restarts the target container.
    *   `pause`: Pauses the target container processes.

*   **Network Actions (Requires `tc` and `netem` in the container):**
    *   `delay`: Injects network latency.
        ```yaml
                action:
                    name: "delay"
                    latency_ms: 300
                    jitter_ms: 50
                    duration: 30 # Seconds
        ```
    *   `loss`: Drops a percentage of network packets.
        ```yaml
                action:
                    name: "loss"
                    percent: 20
                    duration: 30
        ```

*   **Resource Constraints:**
    *   `limit_cpu`: Restricts CPU usage.
        ```yaml
                action:
                    name: "limit_cpu"
                    cpus: 0.5  # Limit to 0.5 cores
                    duration: 60
        ```
    *   `limit_memory`: Restricts Memory usage.
        ```yaml
                action:
                    name: "limit_memory"
                    memory_mb: 256 # Limit to 256 MB
                    duration: 60
        ```

### 3. Wait Step
Pauses the scenario execution for a specified duration to allow state to propagate (e.g., waiting for an orchestrator to spin up a replacement container, or for a database connection to timeout).

```yaml
- wait: 5s   # Supports formats like '5s', '1m', '10' (defaults to seconds)
```

## Running a Scenario

To execute a scenario, use the `entropy scenario run` command:

```bash
entropy scenario run path/to/chaos-scenario.yaml
```

Entropy will execute each step sequentially. If a `probe` step fails, the scenario will abort, indicating that the hypothesis failed. If all steps complete successfully, the hypothesis is validated.
