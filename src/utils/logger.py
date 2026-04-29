"""
DevChaosKit — Structured logger for chaos engine events.

Writes timestamped, structured log lines to .devchaos/engine.log with
automatic rotation (1 MB cap, 3 backups kept).

Log format
----------
::

    2024-01-01T12:00:00+00:00 | INFO   | ENGINE STARTED | targets=svc-a,svc-b interval=10s cooldown=30s
    2024-01-01T12:00:10+00:00 | ACTION | STOP → service-a | result=exited
    2024-01-01T12:00:12+00:00 | SKIP   | COOLDOWN | remaining=28.0s
    2024-01-01T12:00:40+00:00 | SKIP   | MAX_DOWN | down=service-a
    2024-01-01T12:00:41+00:00 | ERROR  | STOP → service-a | Container not found: 'service-a'
    2024-01-01T12:01:00+00:00 | INFO   | ENGINE STOPPED | cycles=6 injections=2

Usage
-----
::

    logger = ChaosLogger(log_file=Path(".devchaos/engine.log"))
    logger.log_start(config)
    logger.log_injection(event)
    logger.close()
"""

from __future__ import annotations

import logging
import logging.handlers
from datetime import datetime, timezone
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from src.config.schema import ChaosConfig
    from src.engine.chaos_engine import InjectionEvent

# ---------------------------------------------------------------------------
# Custom log levels (sit between DEBUG and INFO to avoid noise in default
# Python logging output while still being filterable in the file)
# ---------------------------------------------------------------------------

_LEVEL_ACTION = 25   # Between INFO(20) and WARNING(30)
_LEVEL_SKIP = 15     # Between DEBUG(10) and INFO(20)

logging.addLevelName(_LEVEL_ACTION, "ACTION")
logging.addLevelName(_LEVEL_SKIP, "SKIP")

# ---------------------------------------------------------------------------
# Formatter
# ---------------------------------------------------------------------------


class _IsoFormatter(logging.Formatter):
    """Write ISO-8601 UTC timestamps with pipe-separated fields."""

    def format(self, record: logging.LogRecord) -> str:
        ts = datetime.fromtimestamp(record.created, tz=timezone.utc).isoformat(
            timespec="seconds"
        )
        level = record.levelname.ljust(6)
        return f"{ts} | {level} | {record.getMessage()}"


# ---------------------------------------------------------------------------
# ChaosLogger
# ---------------------------------------------------------------------------


class ChaosLogger:
    """
    Structured file logger for the chaos engine.

    Each ChaosLogger instance owns its own logging.Logger (keyed by its
    ``id``) so multiple engine instances in tests don't share handlers.

    Parameters
    ----------
    log_file:
        Destination log file path. Parent directories are created
        automatically. Defaults to ``.devchaos/engine.log``.
    """

    def __init__(self, log_file: Path) -> None:
        self._log_file = log_file
        self._logger = logging.getLogger(f"src.engine.{id(self)}")
        self._logger.setLevel(logging.DEBUG)
        self._logger.propagate = False   # Don't bubble up to root logger

        log_file.parent.mkdir(parents=True, exist_ok=True)

        handler = logging.handlers.RotatingFileHandler(
            log_file,
            maxBytes=1_000_000,   # 1 MB per file
            backupCount=3,
            encoding="utf-8",
        )
        handler.setFormatter(_IsoFormatter())
        self._logger.addHandler(handler)

    # ------------------------------------------------------------------
    # Lifecycle events
    # ------------------------------------------------------------------

    def log_start(self, config: "ChaosConfig") -> None:
        """Log engine startup with full config summary."""
        self._logger.info(
            f"ENGINE STARTED | targets={','.join(config.targets)} "
            f"interval={config.interval}s "
            f"max_down={config.safety.max_down} "
            f"cooldown={config.safety.cooldown}s "
            f"dry_run={config.safety.dry_run}"
        )

    def log_stop(self, cycle_count: int, injection_count: int) -> None:
        """Log clean engine shutdown with run summary."""
        self._logger.info(
            f"ENGINE STOPPED | cycles={cycle_count} injections={injection_count}"
        )

    # ------------------------------------------------------------------
    # Cycle events
    # ------------------------------------------------------------------

    def log_injection(self, event: "InjectionEvent") -> None:
        """Log a single chaos injection (success or failure)."""
        dry = "[DRY-RUN] " if event.dry_run else ""
        if event.success:
            self._logger.log(
                _LEVEL_ACTION,
                f"{dry}{event.action.upper()} → {event.target} "
                f"| result={event.result_status}",
            )
        else:
            self._logger.error(
                f"{dry}{event.action.upper()} → {event.target} "
                f"| {event.error}"
            )

    def log_cooldown_skip(self, remaining: float) -> None:
        """Log a skipped cycle due to active cooldown."""
        self._logger.log(
            _LEVEL_SKIP,
            f"COOLDOWN | remaining={remaining:.1f}s",
        )

    def log_max_down_skip(self, down_containers: set[str]) -> None:
        """Log a skipped cycle because max_down limit is reached."""
        self._logger.log(
            _LEVEL_SKIP,
            f"MAX_DOWN | down={','.join(sorted(down_containers))}",
        )

    def log_error(self, message: str) -> None:
        """Log an unexpected engine-level error."""
        self._logger.error(f"ENGINE ERROR | {message}")

    # ------------------------------------------------------------------
    # Cleanup
    # ------------------------------------------------------------------

    def close(self) -> None:
        """Flush and close all file handlers."""
        for handler in self._logger.handlers[:]:
            handler.flush()
            handler.close()
            self._logger.removeHandler(handler)
