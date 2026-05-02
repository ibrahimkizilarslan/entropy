"""
Entropy — Configuration schema.

Defines the ChaosConfig dataclass that represents a validated chaos.yaml file.
All fields have sane defaults so minimal configs work out of the box.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Literal, Optional

# ---------------------------------------------------------------------------
# Types
# ---------------------------------------------------------------------------

ActionName = Literal["stop", "restart", "pause", "delay", "loss", "limit_cpu", "limit_memory"]

VALID_ACTIONS: set[str] = {
    "stop",
    "restart",
    "pause",
    "delay",
    "loss",
    "limit_cpu",
    "limit_memory",
}

# Network-level actions that require tc/nsenter on Linux
NETWORK_ACTIONS: set[str] = {"delay", "loss"}

# Resource-level actions that use docker update
RESOURCE_ACTIONS: set[str] = {"limit_cpu", "limit_memory"}


# ---------------------------------------------------------------------------
# ActionSpec — represents one entry in the actions list
# ---------------------------------------------------------------------------


@dataclass
class ActionSpec:
    """
    Represents a single chaos action with its parameters.

    Simple actions (stop, restart, pause) need only ``name``.
    Network actions (delay, loss) accept additional optional params.
    Resource actions (limit_cpu, limit_memory) accept resource params.

    YAML examples::

        # Simple string (backward-compatible)
        actions:
          - stop
          - restart
          - pause

        # Parameterised dict
        actions:
          - name: delay
            latency_ms: 300
            jitter_ms: 50
            duration: 30        # seconds; auto-restore after N seconds

          - name: loss
            percent: 20
            duration: 20

          - name: limit_cpu
            cpus: 0.25          # max 0.25 CPU cores
            duration: 30

          - name: limit_memory
            memory_mb: 128      # cap container at 128 MB
            duration: 30
    """

    name: str
    """Action identifier."""

    # --- Network chaos params ---
    latency_ms: int = 300
    """Latency to add in milliseconds (delay action)."""

    jitter_ms: int = 0
    """Random jitter in milliseconds (delay action)."""

    loss_percent: int = 20
    """Percentage of packets to drop (loss action)."""

    # --- Resource chaos params ---
    cpus: float = 0.25
    """CPU quota (limit_cpu action). E.g. 0.25 = 25% of one core."""

    memory_mb: int = 128
    """Memory cap in MB (limit_memory action)."""

    # --- Shared params ---
    duration: Optional[int] = None
    """
    Auto-restore duration in seconds.
    After this many seconds the chaos is reverted automatically.
    None = permanent (until engine stops or clear is called).
    """

    @property
    def is_network(self) -> bool:
        return self.name in NETWORK_ACTIONS

    @property
    def is_resource(self) -> bool:
        return self.name in RESOURCE_ACTIONS

    def __str__(self) -> str:
        extras = []
        if self.name == "delay":
            extras.append(f"{self.latency_ms}ms")
            if self.jitter_ms:
                extras.append(f"±{self.jitter_ms}ms")
        elif self.name == "loss":
            extras.append(f"{self.loss_percent}%")
        elif self.name == "limit_cpu":
            extras.append(f"{self.cpus} CPUs")
        elif self.name == "limit_memory":
            extras.append(f"{self.memory_mb}MB")
        if self.duration:
            extras.append(f"for {self.duration}s")
        suffix = f"({', '.join(extras)})" if extras else ""
        return f"{self.name} {suffix}".strip()


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
          - pause
          - name: delay
            latency_ms: 300
            jitter_ms: 50
            duration: 30
        safety:
          max_down: 1
          cooldown: 30
          dry_run: false
    """

    interval: int
    """Seconds between chaos injection cycles."""

    targets: list[str]
    """Docker container names (or docker-compose service names) to target."""

    actions: list[ActionSpec] = field(default_factory=lambda: [ActionSpec(name="stop")])
    """Actions the engine may perform. Chosen randomly each cycle."""

    safety: SafetyConfig = field(default_factory=SafetyConfig)
    """Safety constraint sub-config."""

    def validate(self) -> None:
        """Raise ValueError for any invalid field combination."""
        if self.interval < 1:
            raise ValueError("interval must be >= 1 second")
        if not self.targets:
            raise ValueError("targets must contain at least one container name")
        if not self.actions:
            raise ValueError("actions must contain at least one action")
        bad = [a.name for a in self.actions if a.name not in VALID_ACTIONS]
        if bad:
            raise ValueError(
                f"Unknown action(s): {bad!r}. Valid: {sorted(VALID_ACTIONS)}"
            )
        self.safety.validate()

# ---------------------------------------------------------------------------
# Scenario Engine schemas (Phase 8)
# ---------------------------------------------------------------------------


@dataclass
class ProbeSpec:
    """Represents a health check or system test."""

    type: str
    """Type of probe, e.g. 'http'"""

    url: str
    """Target URL to test"""

    expect_status: Optional[int] = None
    """HTTP status code that MUST be returned (e.g. 200)"""

    expect_not_status: Optional[int] = None
    """HTTP status code that MUST NOT be returned (e.g. 503)"""

    timeout: int = 5
    """Request timeout in seconds"""


@dataclass
class ScenarioStep:
    """Represents a single step in a deterministic chaos scenario."""

    type: Literal["wait", "inject", "probe"]
    
    # --- wait step ---
    duration_s: Optional[int] = None
    """Seconds to wait"""
    
    # --- inject step ---
    action: Optional[ActionSpec] = None
    """Chaos action to inject"""
    
    target: Optional[str] = None
    """Target container name"""
    
    # --- probe step ---
    probe: Optional[ProbeSpec] = None
    """Probe specification"""


@dataclass
class ScenarioConfig:
    """A full deterministic chaos scenario."""

    name: str
    description: str
    hypothesis: str
    steps: list[ScenarioStep] = field(default_factory=list)
