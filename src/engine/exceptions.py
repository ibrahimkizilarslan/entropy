"""
Entropy — Custom exception hierarchy.

All exceptions raised by the engine layer inherit from EntropyError so
callers can catch them at any granularity they need.
"""

from __future__ import annotations


class EntropyError(Exception):
    """Base exception for all Entropy errors."""


class DockerConnectionError(EntropyError):
    """Raised when the Docker daemon is unreachable."""


class ContainerNotFoundError(EntropyError):
    """Raised when the requested container name does not exist."""

    def __init__(self, name: str) -> None:
        self.name = name
        super().__init__(f"Container not found: '{name}'")


class ContainerOperationError(EntropyError):
    """Raised when a container lifecycle operation fails."""

    def __init__(self, name: str, operation: str, reason: str) -> None:
        self.name = name
        self.operation = operation
        self.reason = reason
        super().__init__(
            f"Failed to {operation} container '{name}': {reason}"
        )
