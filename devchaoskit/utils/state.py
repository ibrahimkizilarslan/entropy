"""
DevChaosKit — Engine state persistence.

Manages a .devchaos/state.json file that acts as the IPC bridge between
a running daemon (start --detach) and the stop / status commands.

The state file is written atomically (write-to-temp → rename) so readers
never see a partial write.
"""

from __future__ import annotations

import json
import os
import signal
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Optional

from devchaoskit.engine.chaos_engine import InjectionEvent


# ---------------------------------------------------------------------------
# Paths
# ---------------------------------------------------------------------------

STATE_DIR_NAME = ".devchaos"
STATE_FILE_NAME = "state.json"
LOG_FILE_NAME = "engine.log"


# ---------------------------------------------------------------------------
# State manager
# ---------------------------------------------------------------------------


class StateManager:
    """
    Reads and writes engine state to a local .devchaos/state.json file.

    Parameters
    ----------
    cwd:
        Root directory for the .devchaos/ folder. Defaults to the process
        working directory so each project keeps its own state.
    """

    def __init__(self, cwd: Optional[Path] = None) -> None:
        root = cwd or Path.cwd()
        self._dir = root / STATE_DIR_NAME
        self._state_file = self._dir / STATE_FILE_NAME
        self._log_file = self._dir / LOG_FILE_NAME

    # ------------------------------------------------------------------
    # Paths (public, for CLI consumers)
    # ------------------------------------------------------------------

    @property
    def state_file(self) -> Path:
        return self._state_file

    @property
    def log_file(self) -> Path:
        return self._log_file

    @property
    def state_dir(self) -> Path:
        return self._dir

    # ------------------------------------------------------------------
    # Write
    # ------------------------------------------------------------------

    def ensure_dir(self) -> None:
        self._dir.mkdir(parents=True, exist_ok=True)

    def write(self, data: dict[str, Any]) -> None:
        """Atomically write *data* to the state file."""
        self.ensure_dir()
        tmp = self._state_file.with_suffix(".tmp")
        tmp.write_text(json.dumps(data, default=str, indent=2), encoding="utf-8")
        tmp.replace(self._state_file)

    def write_from_engine(
        self,
        pid: int,
        config_path: str,
        cycle_count: int,
        down_containers: set[str],
        history: list[InjectionEvent],
        dry_run: bool,
        cooldown_remaining: float = 0.0,
        cooldown_total: int = 0,
    ) -> None:
        """Convenience wrapper that serialises engine state into the state file."""
        last = history[-1] if history else None
        self.write(
            {
                "pid": pid,
                "started_at": datetime.now(tz=timezone.utc).isoformat(),
                "config_path": str(config_path),
                "dry_run": dry_run,
                "cycle_count": cycle_count,
                "down_containers": sorted(down_containers),
                "cooldown_remaining": round(cooldown_remaining, 1),
                "cooldown_total": cooldown_total,
                "last_event": _serialise_event(last),
                "history": [_serialise_event(e) for e in history[-50:]],
            }
        )

    # ------------------------------------------------------------------
    # Read
    # ------------------------------------------------------------------

    def read(self) -> Optional[dict[str, Any]]:
        """Return the parsed state dict, or None if no state file exists."""
        if not self._state_file.exists():
            return None
        try:
            return json.loads(self._state_file.read_text(encoding="utf-8"))
        except (json.JSONDecodeError, OSError):
            return None

    # ------------------------------------------------------------------
    # Process management
    # ------------------------------------------------------------------

    @staticmethod
    def is_alive(pid: int) -> bool:
        """Return True if a process with *pid* is running."""
        try:
            os.kill(pid, 0)  # Signal 0 = existence check only
            return True
        except ProcessLookupError:
            return False
        except PermissionError:
            return True  # Exists but owned by another user

    def running_pid(self) -> Optional[int]:
        """
        Return the PID from the state file if that process is still alive,
        otherwise return None (stale state).
        """
        state = self.read()
        if state is None:
            return None
        pid = state.get("pid")
        if pid and self.is_alive(int(pid)):
            return int(pid)
        return None

    # ------------------------------------------------------------------
    # Cleanup
    # ------------------------------------------------------------------

    def clear(self) -> None:
        """Remove the state file (called after a clean stop)."""
        self._state_file.unlink(missing_ok=True)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _serialise_event(event: Optional[InjectionEvent]) -> Optional[dict]:
    if event is None:
        return None
    return {
        "timestamp": event.timestamp.isoformat(),
        "action": event.action,
        "target": event.target,
        "dry_run": event.dry_run,
        "result_status": event.result_status,
        "error": event.error,
    }
