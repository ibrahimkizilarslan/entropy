"""
Entropy — Structured log parser for .entropy/engine.log.

Log format (produced by ChaosLogger):
    2024-01-01T12:00:00+00:00 | INFO   | ENGINE STARTED | targets=svc-a interval=10s ...
    2024-01-01T12:00:10+00:00 | ACTION | STOP → service-a | result=exited
    2024-01-01T12:00:12+00:00 | SKIP   | COOLDOWN | remaining=28.0s
    2024-01-01T12:00:40+00:00 | SKIP   | MAX_DOWN | down=service-a
    2024-01-01T12:00:41+00:00 | ERROR  | STOP → service-a | Container not found
    2024-01-01T12:01:00+00:00 | INFO   | ENGINE STOPPED | cycles=6 injections=2
"""
from __future__ import annotations

import re
from collections import Counter
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from pathlib import Path
from typing import Optional

_LINE_RE = re.compile(
    r"^(?P<timestamp>\S+)\s+\|\s+(?P<level>\w+)\s+\|\s+(?P<message>.+)$"
)
_ACTION_RE = re.compile(r"(?:\[DRY-RUN\] )?(.+?)\s+\u2192\s+(\S+)")
_KV_RE = re.compile(r"(\w+)=([^\s]+)")


@dataclass
class LogLine:
    timestamp: datetime
    level: str
    message: str
    raw: str

    @classmethod
    def parse(cls, line: str) -> Optional["LogLine"]:
        m = _LINE_RE.match(line.strip())
        if not m:
            return None
        try:
            ts = datetime.fromisoformat(m.group("timestamp"))
        except ValueError:
            return None
        return cls(
            timestamp=ts,
            level=m.group("level").strip(),
            message=m.group("message").strip(),
            raw=line.rstrip(),
        )

    @property
    def is_dry_run(self) -> bool:
        return "[DRY-RUN]" in self.message


@dataclass
class InjectionRecord:
    timestamp: datetime
    action: str
    target: str
    dry_run: bool
    result: Optional[str]
    error: Optional[str]
    success: bool


@dataclass
class SessionSummary:
    started_at: Optional[datetime]
    stopped_at: Optional[datetime]
    targets: list[str]
    interval: int
    max_down: int
    cooldown: int
    dry_run: bool
    cycles: int
    injections: list[InjectionRecord] = field(default_factory=list)
    cooldown_skips: int = 0
    max_down_skips: int = 0
    is_complete: bool = False   # True if ENGINE STOPPED was seen

    @property
    def duration(self) -> Optional[timedelta]:
        if self.started_at and self.stopped_at:
            return self.stopped_at - self.started_at
        return None

    @property
    def action_counts(self) -> Counter:
        return Counter(i.action.lower() for i in self.injections)

    @property
    def target_counts(self) -> Counter:
        return Counter(i.target for i in self.injections)

    @property
    def error_count(self) -> int:
        return sum(1 for i in self.injections if not i.success)


def parse_log_file(log_file: Path) -> list[LogLine]:
    """Return all parseable lines from the log file."""
    if not log_file.exists():
        return []
    lines = []
    with open(log_file, "r", encoding="utf-8") as f:
        for raw in f:
            parsed = LogLine.parse(raw)
            if parsed:
                lines.append(parsed)
    return lines


def _parse_kv(text: str) -> dict[str, str]:
    return {m.group(1): m.group(2) for m in _KV_RE.finditer(text)}


def extract_sessions(lines: list[LogLine]) -> list[SessionSummary]:
    """Group log lines into SessionSummary objects."""
    sessions: list[SessionSummary] = []
    current: Optional[SessionSummary] = None

    for line in lines:
        if line.level == "INFO" and "ENGINE STARTED" in line.message:
            kv = _parse_kv(line.message)
            targets_raw = kv.get("targets", "")
            current = SessionSummary(
                started_at=line.timestamp,
                stopped_at=None,
                targets=targets_raw.split(",") if targets_raw else [],
                interval=int(kv.get("interval", "10").rstrip("s")),
                max_down=int(kv.get("max_down", "1")),
                cooldown=int(kv.get("cooldown", "0").rstrip("s")),
                dry_run=kv.get("dry_run", "False").lower() == "true",
                cycles=0,
            )

        elif current is not None:
            if line.level == "INFO" and "ENGINE STOPPED" in line.message:
                kv = _parse_kv(line.message)
                current.stopped_at = line.timestamp
                current.cycles = int(kv.get("cycles", "0"))
                current.is_complete = True
                sessions.append(current)
                current = None

            elif line.level == "ACTION":
                m = _ACTION_RE.search(line.message)
                if m:
                    kv = _parse_kv(line.message)
                    current.injections.append(InjectionRecord(
                        timestamp=line.timestamp,
                        action=m.group(1),
                        target=m.group(2),
                        dry_run=line.is_dry_run,
                        result=kv.get("result"),
                        error=None,
                        success=True,
                    ))

            elif line.level == "ERROR":
                m = _ACTION_RE.search(line.message)
                if m:
                    parts = line.message.split(" | ", 1)
                    current.injections.append(InjectionRecord(
                        timestamp=line.timestamp,
                        action=m.group(1),
                        target=m.group(2),
                        dry_run=line.is_dry_run,
                        result=None,
                        error=parts[1] if len(parts) > 1 else "unknown",
                        success=False,
                    ))

            elif line.level == "SKIP" and "COOLDOWN" in line.message:
                current.cooldown_skips += 1

            elif line.level == "SKIP" and "MAX_DOWN" in line.message:
                current.max_down_skips += 1

    # Running (incomplete) session
    if current is not None:
        sessions.append(current)

    return sessions
