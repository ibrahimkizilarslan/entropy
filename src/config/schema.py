"""
DevChaosKit — Configuration schema.

Defines the ChaosConfig dataclass that represents a validated chaos.yaml file.
All fields have sane defaults so minimal configs work out of the box.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Literal

# ---------------------------------------------------------------------------
# Types
# ---------------------------------------------------------------------------

ActionType = Literal["stop", "restart"]


# ---------------------------------------------------------------------------
# Safety sub-config
# ---------------------------------------------------------------------------


@dataclass
class SafetyConfig:
    """Safety constraints for the chaos engine."""

    max_down: int = 1
    """Maximum number of containers that may be stopped simultaneously."""

    cooldown: int = 30
    """Minimum seconds between consecutive chaos injections."""

    dry_run: bool = False
    """When True, log every action without actually executing it."""

    def validate(self) -> None:
        if self.max_down < 1:
            raise ValueError("safety.max_down must be >= 1")
        if self.cooldown < 0:
            raise ValueError("safety.cooldown must be >= 0")


# ---------------------------------------------------------------------------
# Root config
# ---------------------------------------------------------------------------


@dataclass
class ChaosConfig:
    """
    Validated representation of a chaos.yaml file.

    Example
    -------
    ::

        interval: 10
        targets:
          - service-a
          - service-b
        actions:
          - stop
          - restart
        safety:
          max_down: 1
          cooldown: 30
          dry_run: false
    """

    interval: int
    """Seconds between chaos injection cycles."""

    targets: list[str]
    """Docker container names (or docker-compose service names) to target."""

    actions: list[ActionType] = field(default_factory=lambda: ["stop"])
    """Actions the engine may perform. Chosen randomly each cycle."""

    safety: SafetyConfig = field(default_factory=SafetyConfig)
    """Safety constraint sub-config."""

    def validate(self) -> None:
        """Raise ValueError for any invalid field combination."""
        if self.interval < 1:
            raise ValueError("interval must be >= 1 second")
        if not self.targets:
            raise ValueError("targets must contain at least one container name")
        valid_actions: set[str] = {"stop", "restart"}
        bad = [a for a in self.actions if a not in valid_actions]
        if bad:
            raise ValueError(
                f"Unknown action(s): {bad!r}. Valid: {sorted(valid_actions)}"
            )
        if not self.actions:
            raise ValueError("actions must contain at least one action")
        self.safety.validate()
