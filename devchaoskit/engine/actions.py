"""
DevChaosKit — Chaos action definitions and dispatcher.

Defines the set of actions the chaos engine can perform and provides a
single dispatch function that maps action names to DockerClient calls.
Adding a new action in the future requires only:
  1. Adding its name to ActionType in config/schema.py
  2. Adding a handler function here
  3. Registering it in ACTION_HANDLERS
"""

from __future__ import annotations

from typing import Callable

from devchaoskit.engine.docker_client import ContainerInfo, DockerClient


# ---------------------------------------------------------------------------
# Handler type
# ---------------------------------------------------------------------------

ActionHandler = Callable[[DockerClient, str], ContainerInfo]


# ---------------------------------------------------------------------------
# Individual action implementations
# ---------------------------------------------------------------------------


def _action_stop(client: DockerClient, target: str) -> ContainerInfo:
    """Stop a container gracefully (SIGTERM → SIGKILL after timeout)."""
    return client.stop_container(target)


def _action_restart(client: DockerClient, target: str) -> ContainerInfo:
    """Restart a container (stop + start)."""
    return client.restart_container(target)


# ---------------------------------------------------------------------------
# Registry
# ---------------------------------------------------------------------------

ACTION_HANDLERS: dict[str, ActionHandler] = {
    "stop": _action_stop,
    "restart": _action_restart,
}


def dispatch(action: str, client: DockerClient, target: str) -> ContainerInfo:
    """
    Execute a named chaos action against a container.

    Parameters
    ----------
    action:
        Action name (must be a key in ACTION_HANDLERS).
    client:
        Connected DockerClient instance.
    target:
        Container name to act on.

    Returns
    -------
    ContainerInfo
        Post-action snapshot of the container.

    Raises
    ------
    ValueError
        If *action* is not a registered handler.
    """
    handler = ACTION_HANDLERS.get(action)
    if handler is None:
        raise ValueError(
            f"Unknown action '{action}'. "
            f"Registered actions: {sorted(ACTION_HANDLERS)}"
        )
    return handler(client, target)
