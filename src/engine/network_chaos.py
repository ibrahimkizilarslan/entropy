"""
DevChaosKit — Network chaos injection via Linux tc/netem.

Injects network faults (latency, packet loss) into a running container's
network namespace using the host-level ``tc`` (traffic control) tool and
Linux's ``nsenter`` to enter the container's network namespace.

Requirements
------------
- **Linux only** (Docker Desktop on macOS/Windows runs in a VM; unsupported).
- ``iproute2`` installed on the HOST: ``sudo apt install iproute2``
- ``sudo`` privileges for the running user (to run nsenter + tc).

How it works
------------
1. Get the container's PID via ``docker inspect``.
2. Use ``sudo nsenter -t <PID> -n`` to enter the container's network
   namespace from the host (without modifying the container image).
3. Apply ``tc qdisc ... netem`` rules to ``eth0`` inside that namespace.
4. Optionally schedule automatic cleanup after ``duration`` seconds.
"""

from __future__ import annotations

import platform
import subprocess
import threading
from typing import Optional


class NetworkChaosError(Exception):
    """Raised when a network chaos operation fails."""


class NetworkChaosManager:
    """
    Manages tc/netem network chaos rules for Docker containers.

    Thread-safe: each container has at most one active rule at a time.
    """

    def __init__(self) -> None:
        self._lock = threading.Lock()
        # container_name → True (has active rule)
        self._active: dict[str, bool] = {}
        # container_name → Timer (auto-restore)
        self._timers: dict[str, threading.Timer] = {}

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    def inject_delay(
        self,
        container_name: str,
        container_pid: int,
        latency_ms: int = 300,
        jitter_ms: int = 0,
        duration: Optional[int] = None,
    ) -> None:
        """
        Add network latency to a container.

        Parameters
        ----------
        container_name: str
            Used for tracking; not passed to tc.
        container_pid: int
            PID of the container's init process (from ``docker inspect``).
        latency_ms: int
            Base latency to inject in milliseconds.
        jitter_ms: int
            Random jitter in milliseconds (±jitter_ms variation).
        duration: int | None
            Auto-restore after this many seconds. None = permanent.
        """
        self._check_platform()
        args = ["netem", "delay", f"{latency_ms}ms"]
        if jitter_ms:
            args += [f"{jitter_ms}ms", "distribution", "normal"]
        self._apply_rule(container_name, container_pid, args, duration)

    def inject_loss(
        self,
        container_name: str,
        container_pid: int,
        percent: int = 20,
        duration: Optional[int] = None,
    ) -> None:
        """
        Inject packet loss into a container.

        Parameters
        ----------
        percent: int
            Percentage of packets to drop (0-100).
        """
        self._check_platform()
        args = ["netem", "loss", f"{percent}%"]
        self._apply_rule(container_name, container_pid, args, duration)

    def clear(self, container_name: str, container_pid: Optional[int] = None) -> None:
        """Remove all tc rules for a container (restores normal networking)."""
        with self._lock:
            self._cancel_timer(container_name)
            if container_name in self._active:
                if container_pid is not None:
                    self._run_tc(container_pid, ["qdisc", "del", "dev", "eth0", "root"])
                del self._active[container_name]

    def clear_all(self) -> None:
        """Cancel all timers (rules are left in place; caller handles cleanup)."""
        with self._lock:
            for name in list(self._timers.keys()):
                self._cancel_timer(name)
            self._active.clear()

    def has_active_rule(self, container_name: str) -> bool:
        """Return True if the container currently has a tc rule applied."""
        with self._lock:
            return container_name in self._active

    # ------------------------------------------------------------------
    # Internal helpers
    # ------------------------------------------------------------------

    @staticmethod
    def _check_platform() -> None:
        if platform.system() != "Linux":
            raise NetworkChaosError(
                "Network chaos (delay/loss) requires a Linux host.\n"
                "Docker Desktop on macOS/Windows runs in a VM and is not supported.\n"
                "Hint: use 'stop', 'restart', or 'pause' actions instead."
            )

    def _apply_rule(
        self,
        container_name: str,
        container_pid: int,
        netem_args: list[str],
        duration: Optional[int],
    ) -> None:
        """Clear existing rule, apply a new qdisc, optionally schedule cleanup."""
        with self._lock:
            # Remove any existing rule first (idempotent)
            if container_name in self._active:
                self._cancel_timer(container_name)
                self._run_tc(container_pid, ["qdisc", "del", "dev", "eth0", "root"])

            # Add the new rule
            result = self._run_tc(
                container_pid,
                ["qdisc", "add", "dev", "eth0", "root"] + netem_args,
            )
            if result.returncode != 0:
                err = result.stderr.strip() or result.stdout.strip()
                raise NetworkChaosError(
                    f"tc failed for container '{container_name}': {err}\n"
                    "Hint: ensure iproute2 is installed (sudo apt install iproute2) "
                    "and you have sudo privileges."
                )

            self._active[container_name] = True

            if duration:
                timer = threading.Timer(
                    duration,
                    self._restore,
                    args=[container_name, container_pid],
                )
                timer.daemon = True
                timer.start()
                self._timers[container_name] = timer

    def _restore(self, container_name: str, container_pid: int) -> None:
        """Timer callback: remove tc rule after duration expires."""
        with self._lock:
            self._timers.pop(container_name, None)
            if container_name in self._active:
                self._run_tc(container_pid, ["qdisc", "del", "dev", "eth0", "root"])
                del self._active[container_name]

    def _cancel_timer(self, container_name: str) -> None:
        """Cancel a pending restore timer (call with lock held)."""
        timer = self._timers.pop(container_name, None)
        if timer:
            timer.cancel()

    @staticmethod
    def _run_tc(pid: int, tc_args: list[str]) -> subprocess.CompletedProcess:
        """Run a tc command inside the container's network namespace."""
        cmd = ["sudo", "nsenter", "-t", str(pid), "-n", "--", "tc"] + tc_args
        return subprocess.run(cmd, capture_output=True, text=True)
