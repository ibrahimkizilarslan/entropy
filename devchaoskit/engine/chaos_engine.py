"""
DevChaosKit — Chaos Engine Core.

The ChaosEngine drives interval-based random fault injection into Docker
containers. It runs in a dedicated background thread so the CLI remains
responsive while chaos is in progress.

Lifecycle
---------
1. ``engine.start()``  — spawns the worker thread, begins injection loop
2. ``engine.stop()``   — signals the thread to stop, waits for clean exit
3. ``engine.status()`` — returns a point-in-time EngineStatus snapshot

Thread safety
-------------
All shared mutable state (``_running``, ``_last_event``, ``_down_set``) is
protected by a single ``threading.Lock``. The worker thread holds the lock
only during state updates, never during sleeps or Docker calls, so the main
thread never blocks for long.
"""

from __future__ import annotations

import random
import threading
import time
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Optional

from devchaoskit.config.schema import ChaosConfig
from devchaoskit.engine.actions import dispatch
from devchaoskit.engine.docker_client import ContainerInfo, DockerClient
from devchaoskit.engine.exceptions import ChaosKitError


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


# ---------------------------------------------------------------------------
# Engine
# ---------------------------------------------------------------------------


class ChaosEngine:
    """
    Interval-based chaos injection engine.

    Parameters
    ----------
    config:
        Validated ChaosConfig instance.
    on_event:
        Optional callback invoked after every injection attempt (in the
        worker thread). Receives the InjectionEvent. Use this to stream
        live output to the CLI.
    """

    def __init__(
        self,
        config: ChaosConfig,
        on_event: Optional[callable] = None,
    ) -> None:
        self._config = config
        self._on_event = on_event

        # Thread control
        self._stop_event = threading.Event()
        self._thread: Optional[threading.Thread] = None
        self._lock = threading.Lock()

        # Shared mutable state (always accessed under _lock)
        self._running: bool = False
        self._cycle_count: int = 0
        self._down_set: set[str] = set()          # containers currently down
        self._last_event: Optional[InjectionEvent] = None
        self._history: list[InjectionEvent] = []

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
            Maximum seconds to wait for the worker thread to finish its
            current sleep/operation before forcibly returning.
        """
        self._stop_event.set()
        if self._thread is not None:
            self._thread.join(timeout=timeout)
        with self._lock:
            self._running = False

    def status(self) -> EngineStatus:
        """Return a thread-safe snapshot of engine state."""
        with self._lock:
            return EngineStatus(
                running=self._running,
                config=self._config,
                cycle_count=self._cycle_count,
                down_containers=set(self._down_set),
                last_event=self._last_event,
                history=list(self._history),
            )

    # ------------------------------------------------------------------
    # Worker loop
    # ------------------------------------------------------------------

    def _run_loop(self) -> None:
        """Main loop executed in the worker thread."""
        with DockerClient(allowed_targets=set(self._config.targets)) as docker:
            while not self._stop_event.is_set():
                self._run_cycle(docker)

                # Interruptible sleep: break into 1-second slices so
                # stop() wakes up quickly even during a long interval.
                interval = self._config.interval
                for _ in range(interval):
                    if self._stop_event.is_set():
                        break
                    time.sleep(1)

        with self._lock:
            self._running = False

    def _run_cycle(self, docker: DockerClient) -> None:
        """Execute one chaos injection cycle."""
        with self._lock:
            self._cycle_count += 1
            down_count = len(self._down_set)

        # Safety: do not exceed max_down
        if down_count >= self._config.safety.max_down:
            return

        # Pick a random target that is NOT already down
        with self._lock:
            already_down = set(self._down_set)

        available = [t for t in self._config.targets if t not in already_down]
        if not available:
            return

        target = random.choice(available)
        action = random.choice(self._config.actions)

        event = self._execute(docker, action, target)

        with self._lock:
            # Track containers we stopped (so we know what's "down")
            if event.success and action == "stop":
                self._down_set.add(target)
            elif action == "restart":
                self._down_set.discard(target)

            self._last_event = event
            self._history.append(event)

        if self._on_event:
            try:
                self._on_event(event)
            except Exception:
                pass  # Never let a callback crash the engine

    def _execute(
        self, docker: DockerClient, action: str, target: str
    ) -> InjectionEvent:
        """Run a single action and return its event record."""
        now = datetime.now(tz=timezone.utc)
        dry_run = self._config.safety.dry_run

        if dry_run:
            return InjectionEvent(
                timestamp=now,
                action=action,
                target=target,
                dry_run=True,
                result_status="(dry-run)",
            )

        try:
            info: ContainerInfo = dispatch(action, docker, target)
            return InjectionEvent(
                timestamp=now,
                action=action,
                target=target,
                dry_run=False,
                result_status=info.status,
            )
        except ChaosKitError as exc:
            return InjectionEvent(
                timestamp=now,
                action=action,
                target=target,
                dry_run=False,
                result_status=None,
                error=str(exc),
            )
