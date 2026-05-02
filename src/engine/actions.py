"""
Entropy — Chaos action definitions and dispatcher.

Defines the set of actions the chaos engine can perform and provides a
single dispatch function that maps action names to DockerClient calls.
Adding a new action in the future requires only:
  1. Adding its name to ActionType in config/schema.py
  2. Adding a handler function here
  3. Registering it in ACTION_HANDLERS
"""

from __future__ import annotations

import threading
from typing import Callable

from src.config.schema import ActionSpec
from src.engine.docker_client import ContainerInfo, DockerClient
from src.engine.network_chaos import NetworkChaosManager


# ---------------------------------------------------------------------------
# Global Managers for Stateful/Delayed Actions
# ---------------------------------------------------------------------------

NETWORK_MANAGER = NetworkChaosManager()


class ResourceChaosManager:
    """Manages auto-restore timers for resource limits (CPU/Memory)."""

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._timers: dict[str, threading.Timer] = {}

    def schedule_restore(
        self, client: DockerClient, target: str, duration: int
    ) -> None:
        """Schedule a timer to remove resource limits."""
        with self._lock:
            # Cancel existing timer if any
            timer = self._timers.pop(target, None)
            if timer:
                timer.cancel()

            def _restore() -> None:
                with self._lock:
                    self._timers.pop(target, None)
                try:
                    # 0 values remove the limits in docker update
                    client.update_container_resources(
                        target, cpu_quota=0, mem_limit=0
                    )
                except Exception:
                    pass  # Ignore errors during auto-restore (container might be dead)

            timer = threading.Timer(duration, _restore)
            timer.daemon = True
            timer.start()
            self._timers[target] = timer

    def clear_all(self) -> None:
        """Cancel all pending resource timers."""
        with self._lock:
            for timer in self._timers.values():
                timer.cancel()
            self._timers.clear()


RESOURCE_MANAGER = ResourceChaosManager()


# ---------------------------------------------------------------------------
# Handler type
# ---------------------------------------------------------------------------

ActionHandler = Callable[[DockerClient, str, ActionSpec], ContainerInfo]


# ---------------------------------------------------------------------------
# Individual action implementations
# ---------------------------------------------------------------------------


def _action_stop(
    client: DockerClient, target: str, spec: ActionSpec
) -> ContainerInfo:
    return client.stop_container(target)


def _action_restart(
    client: DockerClient, target: str, spec: ActionSpec
) -> ContainerInfo:
    return client.restart_container(target)


def _action_pause(
    client: DockerClient, target: str, spec: ActionSpec
) -> ContainerInfo:
    # We could implement duration-based auto-unpause, but for now we just pause.
    # The user can use "entropy docker unpause" or restart it.
    return client.pause_container(target)


def _action_delay(
    client: DockerClient, target: str, spec: ActionSpec
) -> ContainerInfo:
    pid = client.get_container_pid(target)
    NETWORK_MANAGER.inject_delay(
        container_name=target,
        container_pid=pid,
        latency_ms=spec.latency_ms,
        jitter_ms=spec.jitter_ms,
        duration=spec.duration,
    )
    # Return current state
    return client.list_containers(all=True)[0] # Hacky way to get info without inspecting again if we had a get info method. Actually we can use _get_container.
    # Better: return ContainerInfo.from_sdk(client._get_container(target))
    # Let's avoid accessing private method if possible, but we don't have get_info public.
    # Actually wait, I will access private since I am in engine module.


def _action_loss(
    client: DockerClient, target: str, spec: ActionSpec
) -> ContainerInfo:
    pid = client.get_container_pid(target)
    NETWORK_MANAGER.inject_loss(
        container_name=target,
        container_pid=pid,
        percent=spec.loss_percent,
        duration=spec.duration,
    )
    return ContainerInfo.from_sdk(client._get_container(target))


# redefine delay to use the right return pattern
def _action_delay_fix(
    client: DockerClient, target: str, spec: ActionSpec
) -> ContainerInfo:
    pid = client.get_container_pid(target)
    NETWORK_MANAGER.inject_delay(
        container_name=target,
        container_pid=pid,
        latency_ms=spec.latency_ms,
        jitter_ms=spec.jitter_ms,
        duration=spec.duration,
    )
    return ContainerInfo.from_sdk(client._get_container(target))


def _action_limit_cpu(
    client: DockerClient, target: str, spec: ActionSpec
) -> ContainerInfo:
    # cpu_period is usually 100000 (100ms).
    # cpu_quota = cpus * 100000
    period = 100000
    quota = int(spec.cpus * period)
    info = client.update_container_resources(
        target, cpu_quota=quota, cpu_period=period
    )
    if spec.duration:
        RESOURCE_MANAGER.schedule_restore(client, target, spec.duration)
    return info


def _action_limit_memory(
    client: DockerClient, target: str, spec: ActionSpec
) -> ContainerInfo:
    mem_bytes = spec.memory_mb * 1024 * 1024
    info = client.update_container_resources(target, mem_limit=mem_bytes)
    if spec.duration:
        RESOURCE_MANAGER.schedule_restore(client, target, spec.duration)
    return info


# ---------------------------------------------------------------------------
# Registry
# ---------------------------------------------------------------------------

ACTION_HANDLERS: dict[str, ActionHandler] = {
    "stop": _action_stop,
    "restart": _action_restart,
    "pause": _action_pause,
    "delay": _action_delay_fix,
    "loss": _action_loss,
    "limit_cpu": _action_limit_cpu,
    "limit_memory": _action_limit_memory,
}


def dispatch(action: ActionSpec, client: DockerClient, target: str) -> ContainerInfo:
    """
    Execute a named chaos action against a container.

    Parameters
    ----------
    action:
        ActionSpec representing the chaos to inject.
    client:
        Connected DockerClient instance.
    target:
        Container name to act on.

    Returns
    -------
    ContainerInfo
        Post-action snapshot of the container.
    """
    handler = ACTION_HANDLERS.get(action.name)
    if handler is None:
        raise ValueError(
            f"Unknown action '{action.name}'. "
            f"Registered actions: {sorted(ACTION_HANDLERS)}"
        )
    return handler(client, target, action)


def cleanup_all() -> None:
    """Cancel all active timers and network chaos rules."""
    NETWORK_MANAGER.clear_all()
    RESOURCE_MANAGER.clear_all()
