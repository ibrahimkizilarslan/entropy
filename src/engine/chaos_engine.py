"""
Entropy — Chaos Engine Core.

The ChaosEngine drives interval-based random fault injection into Docker
containers. It runs in a dedicated background thread so the CLI remains
responsive while chaos is in progress.

Safety mechanisms (Phase 5)
---------------------------
- **max_down**   : hard cap on concurrent stopped containers (checked every cycle)
- **cooldown**   : minimum seconds between any two injections (checked every cycle)
- **dry_run**    : log all actions without touching Docker (config or CLI flag)

Lifecycle
---------
1. ``engine.start()``  — spawns the worker thread, begins injection loop
2. ``engine.stop()``   — signals the thread to stop, waits for clean exit
3. ``engine.status()`` — returns a point-in-time EngineStatus snapshot

Thread safety
-------------
All shared mutable state (``_running``, ``_last_event``, ``_down_set``,
``_last_injection_time``) is protected by a single ``threading.Lock``.
The worker thread holds the lock only during state updates, never during
sleeps or Docker calls, so the main thread never blocks for long.
"""

from __future__ import annotations

import random
import threading
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Optional

from src.config.schema import ChaosConfig, ActionSpec
from src.engine.actions import dispatch, cleanup_all
from src.engine.docker_client import ContainerInfo, DockerClient
from src.engine.exceptions import EntropyError


# ---------------------------------------------------------------------------
# Event record
# ---------------------------------------------------------------------------


@dataclass
class InjectionEvent:
    """Immutable record of a single chaos injection."""

    timestamp: datetime
    action: str
    target: str
    dry_run: bool
    result_status: Optional[str]       # Container status after action, or None
    error: Optional[str] = None        # Set if the action raised an exception

    @property
    def success(self) -> bool:
        return self.error is None

    def __str__(self) -> str:
        tag = "[DRY-RUN] " if self.dry_run else ""
        ts = self.timestamp.strftime("%H:%M:%S")
        if self.success:
            return f"{ts} {tag}{self.action.upper()} → {self.target} [{self.result_status}]"
        return f"{ts} {tag}FAILED {self.action.upper()} → {self.target}: {self.error}"


# ---------------------------------------------------------------------------
# Engine status snapshot
# ---------------------------------------------------------------------------


@dataclass
class EngineStatus:
    """Point-in-time status snapshot returned by ChaosEngine.status()."""

    running: bool
    config: ChaosConfig
    cycle_count: int
    down_containers: set[str]
    last_event: Optional[InjectionEvent]
    history: list[InjectionEvent]
    last_injection_time: Optional[datetime] = None
    cooldown_remaining: float = 0.0   # Seconds until next injection allowed (0 = ready)


# ---------------------------------------------------------------------------
# Engine
# ---------------------------------------------------------------------------


class ChaosEngine:
    """
    Interval-based chaos injection engine with full safety controls.

    Parameters
    ----------
    config:
        Validated ChaosConfig instance. Safety settings (max_down, cooldown,
        dry_run) are read from ``config.safety`` each cycle so live overrides
        are respected without restarting the engine.
    on_event:
        Optional callback invoked after every injection attempt (runs in the
        worker thread). Receives the InjectionEvent. Use this to stream
        live output to the CLI or write state files.
    logger:
        Optional ChaosLogger instance. When provided, all actions, skips,
        and lifecycle events are written to the structured log file.
    """

    def __init__(
        self,
        config: ChaosConfig,
        on_event: Optional[callable] = None,
        logger=None,   # Optional[ChaosLogger] — avoid circular import at runtime
    ) -> None:
        self._config = config
        self._on_event = on_event
        self._logger = logger

        # Thread control
        self._stop_event = threading.Event()
        self._thread: Optional[threading.Thread] = None
        self._lock = threading.Lock()

        # Shared mutable state (always accessed under _lock)
        self._running: bool = False
        self._cycle_count: int = 0
        self._down_set: set[str] = set()
        self._last_event: Optional[InjectionEvent] = None
        self._history: list[InjectionEvent] = []
        self._last_injection_time: Optional[datetime] = None   # Phase 5: cooldown tracking

    # ------------------------------------------------------------------
    # Public control API
    # ------------------------------------------------------------------

    def start(self) -> None:
        """Start the chaos engine in a background thread."""
        with self._lock:
            if self._running:
                raise RuntimeError("ChaosEngine is already running.")
            self._running = True
            self._stop_event.clear()

        self._thread = threading.Thread(
            target=self._run_loop,
            name="chaos-engine",
            daemon=True,
        )
        self._thread.start()

    def stop(self, timeout: float = 15.0) -> None:
        """
        Signal the engine to stop and wait for the thread to exit.

        Parameters
        ----------
        timeout:
            Maximum seconds to wait for the worker thread to finish before
            forcibly returning.
        """
        self._stop_event.set()
        if self._thread is not None:
            self._thread.join(timeout=timeout)
        with self._lock:
            self._running = False

    def status(self) -> EngineStatus:
        """Return a thread-safe snapshot of engine state."""
        with self._lock:
            last_inj = self._last_injection_time
            cooldown = self._config.safety.cooldown
            if cooldown > 0 and last_inj is not None:
                elapsed = (datetime.now(tz=timezone.utc) - last_inj).total_seconds()
                cooldown_remaining = max(0.0, float(cooldown) - elapsed)
            else:
                cooldown_remaining = 0.0

            return EngineStatus(
                running=self._running,
                config=self._config,
                cycle_count=self._cycle_count,
                down_containers=set(self._down_set),
                last_event=self._last_event,
                history=list(self._history),
                last_injection_time=last_inj,
                cooldown_remaining=cooldown_remaining,
            )

    # ------------------------------------------------------------------
    # Worker loop
    # ------------------------------------------------------------------

    def _run_loop(self) -> None:
        """Main loop executed in the worker thread."""
        if self._logger:
            self._logger.log_start(self._config)

        with DockerClient(allowed_targets=set(self._config.targets)) as docker:
            while not self._stop_event.is_set():
                self._run_cycle(docker)

                # Interruptible sleep — break into 1-second slices so
                # stop() wakes up quickly even during a long interval.
                for _ in range(self._config.interval):
                    if self._stop_event.is_set():
                        break
                    time.sleep(1)

        with self._lock:
            self._running = False
            cycle_count = self._cycle_count
            injection_count = len(self._history)

        try:
            cleanup_all()
        except Exception:
            pass

        if self._logger:
            self._logger.log_stop(cycle_count, injection_count)

    def _run_cycle(self, docker: DockerClient) -> None:
        """Execute one chaos injection cycle with all safety checks."""
        with self._lock:
            self._cycle_count += 1
            down_count = len(self._down_set)
            last_inj = self._last_injection_time

        # ── Safety check 1: Cooldown ─────────────────────────────────
        cooldown = self._config.safety.cooldown
        if cooldown > 0 and last_inj is not None:
            elapsed = (datetime.now(tz=timezone.utc) - last_inj).total_seconds()
            remaining = cooldown - elapsed
            if remaining > 0:
                if self._logger:
                    self._logger.log_cooldown_skip(remaining)
                return   # Still in cooldown — skip this cycle

        # ── Safety check 2: Max-down ─────────────────────────────────
        if down_count >= self._config.safety.max_down:
            if self._logger:
                with self._lock:
                    self._logger.log_max_down_skip(set(self._down_set))
            return   # Too many containers already down

        # ── Target selection ─────────────────────────────────────────
        with self._lock:
            already_down = set(self._down_set)

        available = [t for t in self._config.targets if t not in already_down]
        if not available:
            return

        target = random.choice(available)
        action_spec: ActionSpec = random.choice(self._config.actions)

        # ── Execute ──────────────────────────────────────────────────
        event = self._execute(docker, action_spec, target)

        with self._lock:
            # Track cooldown timestamp (after any injection, success or failure)
            self._last_injection_time = event.timestamp

            # Update down-set based on action result
            if event.success and action_spec.name in ("stop", "pause"):
                self._down_set.add(target)
            elif action_spec.name in ("restart", "unpause"):
                self._down_set.discard(target)

            self._last_event = event
            self._history.append(event)

        # Log to file
        if self._logger:
            self._logger.log_injection(event)

        # Notify CLI callback
        if self._on_event:
            try:
                self._on_event(event)
            except Exception:
                pass   # Never let a callback crash the engine

    def _execute(
        self, docker: DockerClient, action_spec: ActionSpec, target: str
    ) -> InjectionEvent:
        """Run a single action and return its event record."""
        now = datetime.now(tz=timezone.utc)
        dry_run = self._config.safety.dry_run

        action_name = str(action_spec)

        if dry_run:
            return InjectionEvent(
                timestamp=now,
                action=action_name,
                target=target,
                dry_run=True,
                result_status="(dry-run)",
            )

        try:
            info: ContainerInfo = dispatch(action_spec, docker, target)
            return InjectionEvent(
                timestamp=now,
                action=action_name,
                target=target,
                dry_run=False,
                result_status=info.status,
            )
        except EntropyError as exc:
            return InjectionEvent(
                timestamp=now,
                action=action_name,
                target=target,
                dry_run=False,
                result_status=None,
                error=str(exc),
            )
