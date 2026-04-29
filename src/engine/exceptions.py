"""
DevChaosKit — Custom exception hierarchy.

All exceptions raised by the engine layer inherit from ChaosKitError so
callers can catch them at any granularity they need.
"""

from __future__ import annotations


class ChaosKitError(Exception):
    """Base exception for all DevChaosKit errors."""


class DockerConnectionError(ChaosKitError):
    """Raised when the Docker daemon is unreachable."""


class ContainerNotFoundError(ChaosKitError):
    """Raised when the requested container name does not exist."""

    def __init__(self, name: str) -> None:
        self.name = name
        super().__init__(f"Container not found: '{name}'")


class ContainerOperationError(ChaosKitError):
    """Raised when a container lifecycle operation fails."""

    def __init__(self, name: str, operation: str, reason: str) -> None:
        self.name = name
        self.operation = operation
        self.reason = reason
        super().__init__(
            f"Failed to {operation} container '{name}': {reason}"
        )
