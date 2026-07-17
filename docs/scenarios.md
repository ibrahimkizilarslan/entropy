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

**Security: `http`/`tcp` probes are protected against SSRF.** Since probe
targets come from scenario YAML, Entropy validates every connection at the
network level (not just by hostname) before it is made — this closes DNS
rebinding attacks where a hostname resolves to a safe IP at validation time
but a different, blocked IP at connection time. Cloud metadata endpoints
(`169.254.169.254`, `metadata.google.internal`, etc.) are **always** blocked,
since anything that can reach them can exfiltrate cloud credentials. Private,
loopback, and link-local IPs (e.g. `192.168.x.x`, `127.0.0.1`, containers on a
Docker network) are allowed by default, since chaos probes legitimately target
local infrastructure. Set `ENTROPY_ALLOW_PRIVATE_NETWORKS=false` to also block
those ranges (useful when running scenario files you don't fully trust).

Entropy also supports `tcp` probes (`host_port: "localhost:6379"`) and `exec` probes that run a diagnostic command inside the target container/pod:

```yaml
- probe:
    type: exec
    target: "my-service"
    command: "cat /proc/uptime"
```

**Security: exec probe commands are allowlisted.** Because `command` comes from
scenario YAML files (which may be untrusted or shared), only a small set of
read-only diagnostic executables is permitted by default: `cat`, `ls`, `stat`,
`test`, `true`, `false`, `echo`, `pgrep`, `ps`, `head`, `tail`, `wc`, `grep`.
Arguments containing shell metacharacters (`; | & \` $ ( ) < >`) are rejected
even for allowlisted commands. Shells, interpreters (`sh`, `python`, `perl`,
...), and network tools (`curl`, `nc`, `ssh`, ...) are always blocked — no
command can escalate to running arbitrary code or reaching the network through
an exec probe.

To permit additional read-only commands your scenarios rely on, set
`ENTROPY_EXEC_ALLOWLIST` (comma-separated) before running Entropy:

```bash
ENTROPY_EXEC_ALLOWLIST=whoami,id entropy scenario run my-scenario.yaml
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
