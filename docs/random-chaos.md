# Random Chaos Engine

In addition to deterministic scenarios, Entropy features a **Random Chaos Engine** designed to act like a traditional Chaos Monkey. It runs in the background as a daemon and randomly targets services in your environment based on a predefined configuration.

This is ideal for continuous background testing while developing, ensuring that your code is resilient to unexpected blips.

## Configuration (`chaos.yaml`)

The Random Chaos Engine is driven by the `chaos.yaml` file (which can be auto-generated using `entropy init`).

```yaml
# How often (seconds) to select and inject a chaos action
interval: 10

# Docker container names or docker-compose service names to target
targets:
  - service-a
  - service-b

# Actions to apply
actions:
  - stop
  - restart
  - pause

# Safety settings
safety:
  max_down: 1          # Maximum containers stopped simultaneously
  cooldown: 30         # Seconds to wait between injections
  dry_run: false       # If true, log actions without executing them
```

## How It Works

1. **Start the Engine:** Run `entropy start` for foreground mode, or `entropy start --detach` to run in the background.
2. **Action Selection:** Every `interval` seconds, the engine wakes up.
3. **Safety Check:** The engine evaluates its safety rules:
    * Is it still within the `cooldown` period from the last injection? If yes, it sleeps.
    * Would stopping another container exceed `max_down`? If yes, it skips destructive lifecycle actions.
4. **Execution:** If safety checks pass, it randomly selects a target from `targets` and an action from `actions`. It then applies the fault.
5. **Revert:** If the action has a `duration` (like network latency or resource limits), Entropy automatically schedules a background task to revert the fault after the duration expires.
6. **Monitoring:** You can view what the engine is doing by running `entropy logs`.

## Safety Mechanisms

Entropy is designed for local development, meaning it must be careful not to destroy your development environment completely.

*   **`max_down`**: This is a critical safety constraint. It tracks the state of all targeted containers. If `max_down` is set to `1`, and `service-a` is currently stopped, Entropy will *not* stop `service-b` until `service-a` recovers. This prevents total system collapse.
*   **`cooldown`**: Ensures there is a breathing period between faults. If an action is injected, the engine will not inject another fault until the `cooldown` period has passed, giving orchestrators (like Docker Swarm or Kubernetes in future versions) time to react and recover.
*   **Automatic Rollback**: For temporary faults like CPU throttling or network latency, the engine tracks the injected state and guarantees it will be rolled back after the configured duration. If the engine crashes, `entropy cleanup` can be run to manually revert these states.
