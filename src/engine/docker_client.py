"""
DevChaosKit — Docker abstraction layer.

Provides a safe, narrow interface over the docker-py SDK for container
lifecycle management. All methods target containers by *name* and refuse to
operate on containers that are not in the caller-supplied allow-list, which
prevents accidental wide-blast operations.

Design constraints
------------------
- No destructive global commands (e.g., docker system prune, kill all)
- Every public method validates the target name before touching Docker
- docker-compose named containers are supported transparently because
  docker-py works with container names regardless of how they were created
- Raises custom ChaosKitError subclasses so the CLI layer can present
  friendly messages without leaking SDK internals
"""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import Optional

import docker
import docker.errors
from docker.models.containers import Container

from src.engine.exceptions import (
    ContainerNotFoundError,
    ContainerOperationError,
    DockerConnectionError,
)


# ---------------------------------------------------------------------------
# Data model
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class ContainerInfo:
    """Lightweight, serialisable snapshot of a running container."""

    id: str                        # Short container ID (12 chars)
    name: str                      # Container name (without leading '/')
    image: str                     # Image name : tag
    status: str                    # running | paused | exited | …
    ports: dict[str, str] = field(default_factory=dict)  # host:container map

    @classmethod
    def from_sdk(cls, container: Container) -> "ContainerInfo":
        """Build a ContainerInfo from a docker-py Container object."""
        # Ports: {container_port/proto: [{HostIp, HostPort}]} or None
        raw_ports: dict = container.ports or {}
        port_map: dict[str, str] = {}
        for c_port, bindings in raw_ports.items():
            if bindings:
                host_port = bindings[0].get("HostPort", "?")
                port_map[c_port] = host_port

        return cls(
            id=container.short_id,
            name=container.name.lstrip("/"),
            image=container.image.tags[0] if container.image.tags else container.image.short_id,
            status=container.status,
            ports=port_map,
        )


# ---------------------------------------------------------------------------
# Client
# ---------------------------------------------------------------------------


class DockerClient:
    """
    Safe wrapper around docker-py for chaos engineering operations.

    Parameters
    ----------
    allowed_targets:
        Optional set of container names the client is permitted to act on.
        When provided, *stop* and *restart* operations will be refused for
        any container not in this set, even if the container exists.
        Pass ``None`` (the default) to allow any container.
    """

    def __init__(self, allowed_targets: Optional[set[str]] = None) -> None:
        self._allowed: Optional[set[str]] = (
            set(allowed_targets) if allowed_targets is not None else None
        )
        try:
            self._client = self._connect()
            # Ping immediately so we fail fast with a clear error
            self._client.ping()
        except docker.errors.DockerException as exc:
            raise DockerConnectionError(
                "Cannot connect to the Docker daemon. "
                "Is Docker running? (docker info)"
            ) from exc

    @staticmethod
    def _connect() -> docker.DockerClient:
        """
        Connect to Docker, preferring the Docker Desktop socket on Linux
        when it exists and the default /var/run/docker.sock is inaccessible.
        """
        desktop_sock = Path.home() / ".docker" / "desktop" / "docker.sock"
        if desktop_sock.exists() and not os.access("/var/run/docker.sock", os.R_OK):
            return docker.DockerClient(base_url=f"unix://{desktop_sock}")
        if desktop_sock.exists():
            # Try desktop socket first (active context on Docker Desktop installs)
            try:
                client = docker.DockerClient(base_url=f"unix://{desktop_sock}")
                client.ping()
                return client
            except docker.errors.DockerException:
                pass  # Fall through to default
        return docker.from_env()

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    def _get_container(self, name: str) -> Container:
        """Resolve a container by exact name or docker-compose service name."""
        try:
            return self._client.containers.get(name)
        except docker.errors.NotFound:
            # Fallback: try to find it by docker-compose service name
            containers = self._client.containers.list(
                filters={"label": f"com.docker.compose.service={name}"},
                all=True
            )
            if containers:
                return containers[0]
            raise ContainerNotFoundError(name)
        except docker.errors.APIError as exc:
            raise ContainerOperationError(name, "inspect", str(exc)) from exc

    def _assert_allowed(self, name: str) -> None:
        """Guard: refuse operations on containers outside the allow-list."""
        if self._allowed is not None and name not in self._allowed:
            raise ContainerOperationError(
                name,
                "target",
                f"'{name}' is not in the configured chaos targets. "
                "Add it to your chaos.yaml to enable injection.",
            )

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    def list_containers(self, all: bool = False) -> list[ContainerInfo]:
        """
        Return a snapshot list of containers visible to Docker.

        Parameters
        ----------
        all:
            If True, include stopped/exited containers in addition to
            running ones. Defaults to False (running only).
        """
        try:
            containers = self._client.containers.list(all=all)
        except docker.errors.APIError as exc:
            raise ContainerOperationError("*", "list", str(exc)) from exc
        return [ContainerInfo.from_sdk(c) for c in containers]

    def stop_container(self, name: str, timeout: int = 10) -> ContainerInfo:
        """
        Gracefully stop a container by name.

        Sends SIGTERM and waits up to *timeout* seconds before forcing SIGKILL.

        Parameters
        ----------
        name:
            Container name (or docker-compose service name).
        timeout:
            Seconds to wait for graceful shutdown before sending SIGKILL.

        Returns
        -------
        ContainerInfo
            A snapshot of the container *after* the stop operation.

        Raises
        ------
        ContainerNotFoundError
            If no container with that name exists.
        ContainerOperationError
            If the stop call fails for any Docker-level reason.
        """
        self._assert_allowed(name)
        container = self._get_container(name)

        try:
            container.stop(timeout=timeout)
            container.reload()  # Refresh state after the operation
        except docker.errors.APIError as exc:
            raise ContainerOperationError(name, "stop", str(exc)) from exc

        return ContainerInfo.from_sdk(container)

    def restart_container(self, name: str, timeout: int = 10) -> ContainerInfo:
        """
        Restart a container by name.

        Equivalent to ``docker restart <name>``. Safe for both running and
        stopped containers.

        Parameters
        ----------
        name:
            Container name (or docker-compose service name).
        timeout:
            Seconds to wait for the old process to stop before forcing.

        Returns
        -------
        ContainerInfo
            A snapshot of the container *after* the restart operation.

        Raises
        ------
        ContainerNotFoundError
            If no container with that name exists.
        ContainerOperationError
            If the restart call fails for any Docker-level reason.
        """
        self._assert_allowed(name)
        container = self._get_container(name)

        try:
            container.restart(timeout=timeout)
            container.reload()
        except docker.errors.APIError as exc:
            raise ContainerOperationError(name, "restart", str(exc)) from exc

        return ContainerInfo.from_sdk(container)

    def pause_container(self, name: str) -> ContainerInfo:
        """
        Pause all processes within a container.
        """
        self._assert_allowed(name)
        container = self._get_container(name)
        try:
            container.pause()
            container.reload()
        except docker.errors.APIError as exc:
            raise ContainerOperationError(name, "pause", str(exc)) from exc
        return ContainerInfo.from_sdk(container)

    def unpause_container(self, name: str) -> ContainerInfo:
        """
        Unpause all processes within a container.
        """
        self._assert_allowed(name)
        container = self._get_container(name)
        try:
            container.unpause()
            container.reload()
        except docker.errors.APIError as exc:
            raise ContainerOperationError(name, "unpause", str(exc)) from exc
        return ContainerInfo.from_sdk(container)

    def get_container_pid(self, name: str) -> int:
        """
        Get the init PID of a running container. Used for network namespace injection.
        """
        self._assert_allowed(name)
        container = self._get_container(name)
        if container.status != "running":
            raise ContainerOperationError(
                name, "pid", f"Container is {container.status}, not running."
            )
        try:
            pid = container.attrs["State"]["Pid"]
            if not pid:
                raise ValueError("PID is 0 or missing")
            return int(pid)
        except (KeyError, ValueError) as exc:
            raise ContainerOperationError(name, "pid", f"Failed to get PID: {exc}") from exc

    def update_container_resources(
        self, name: str, cpu_quota: int = 0, cpu_period: int = 0, mem_limit: int = 0
    ) -> ContainerInfo:
        """
        Dynamically update container resource limits.
        """
        self._assert_allowed(name)
        container = self._get_container(name)
        try:
            container.update(
                cpu_quota=cpu_quota, cpu_period=cpu_period, mem_limit=mem_limit
            )
            container.reload()
        except docker.errors.APIError as exc:
            raise ContainerOperationError(name, "update", str(exc)) from exc
        return ContainerInfo.from_sdk(container)

    def close(self) -> None:
        """Release the underlying Docker SDK connection."""
        self._client.close()

    # Context manager support so callers can use `with DockerClient() as dc:`
    def __enter__(self) -> "DockerClient":
        return self

    def __exit__(self, *_: object) -> None:
        self.close()
