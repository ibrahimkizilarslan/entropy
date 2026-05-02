"""
Entropy CLI — Observability commands (Phase 6).

  entropy logs    [--lines N] [--follow] [--level ACTION|SKIP|ERROR|INFO]
  entropy report  [--all]
"""
from __future__ import annotations

import time
from pathlib import Path
from typing import Optional

import typer
from rich import box
from rich.console import Console
from rich.panel import Panel
from rich.table import Table
from rich.text import Text

from src.utils.log_parser import (
    LogLine,
    SessionSummary,
    extract_sessions,
    parse_log_file,
)
from src.utils.state import StateManager

console = Console()
state = StateManager()

# ---------------------------------------------------------------------------
# Shared helpers
# ---------------------------------------------------------------------------

_LEVEL_STYLE: dict[str, str] = {
    "ACTION": "bold green",
    "SKIP": "dim yellow",
    "ERROR": "bold red",
    "INFO": "cyan",
}


def _fmt_line(line: LogLine) -> str:
    ts = line.timestamp.strftime("%H:%M:%S")
    style = _LEVEL_STYLE.get(line.level, "white")
    level = f"[{style}]{line.level.ljust(6)}[/{style}]"
    return f"[dim]{ts}[/dim]  {level}  {line.message}"


def _fmt_duration(s: SessionSummary) -> str:
    dur = s.duration
    if dur is None:
        return "[yellow]still running[/yellow]"
    total = int(dur.total_seconds())
    m, sec = divmod(total, 60)
    h, m = divmod(m, 60)
    if h:
        return f"{h}h {m}m {sec}s"
    if m:
        return f"{m}m {sec}s"
    return f"{sec}s"


# ---------------------------------------------------------------------------
# ─── entropy logs ──────────────────────────────────────────────────────────
# ---------------------------------------------------------------------------


def cmd_logs(
    lines: int = typer.Option(50, "--lines", "-n", help="Number of recent lines to show.", min=1),
    follow: bool = typer.Option(False, "--follow", "-f", help="Stream new log lines in real time (like tail -f)."),
    level: Optional[str] = typer.Option(
        None,
        "--level",
        "-l",
        help="Filter by level: ACTION, SKIP, ERROR, INFO. Omit for all.",
    ),
) -> None:
    """View the chaos engine log file with colour-coded output."""
    log_file = state.log_file

    if not log_file.exists():
        console.print(
            Panel(
                "[dim]No log file found.[/dim]\n"
                "Start the engine first with [bold cyan]entropy start[/bold cyan].",
                border_style="bright_black",
                title="[bold]Logs[/bold]",
                title_align="left",
            )
        )
        return

    level_filter = level.upper() if level else None
    valid_levels = {"ACTION", "SKIP", "ERROR", "INFO"}
    if level_filter and level_filter not in valid_levels:
        console.print(f"[red]Unknown level '{level}'. Choose from: {', '.join(sorted(valid_levels))}[/red]")
        raise typer.Exit(1)

    # --- Show last N lines ---
    all_lines = parse_log_file(log_file)
    filtered = [l for l in all_lines if level_filter is None or l.level == level_filter]
    tail = filtered[-lines:]

    console.print()
    console.print(
        f"  [bold]Log:[/bold] [dim]{log_file}[/dim]"
        + (f"  [bold]Filter:[/bold] [cyan]{level_filter}[/cyan]" if level_filter else "")
    )
    console.print()

    if not tail:
        console.print("  [dim]No matching log entries.[/dim]\n")
        if not follow:
            return
    else:
        for line in tail:
            console.print("  " + _fmt_line(line))

    if not follow:
        console.print()
        return

    # --- Follow mode: poll for new content ---
    console.print()
    console.print("  [dim]Following… press Ctrl+C to stop.[/dim]\n")

    with open(log_file, "r", encoding="utf-8") as f:
        f.seek(0, 2)   # Seek to end
        try:
            while True:
                chunk = f.read()
                if chunk:
                    for raw in chunk.splitlines():
                        parsed = LogLine.parse(raw)
                        if parsed and (level_filter is None or parsed.level == level_filter):
                            console.print("  " + _fmt_line(parsed))
                time.sleep(0.5)
        except KeyboardInterrupt:
            console.print("\n  [dim]Stopped following.[/dim]\n")


# ---------------------------------------------------------------------------
# ─── entropy report ────────────────────────────────────────────────────────
# ---------------------------------------------------------------------------


def cmd_report(
    all_sessions: bool = typer.Option(False, "--all", "-a", help="Show all historical sessions."),
) -> None:
    """Generate a statistics report from the chaos engine log."""
    log_file = state.log_file

    if not log_file.exists():
        console.print(
            Panel(
                "[dim]No log file found.[/dim]\n"
                "Start the engine first with [bold cyan]entropy start[/bold cyan].",
                border_style="bright_black",
                title="[bold]Report[/bold]",
                title_align="left",
            )
        )
        return

    raw_lines = parse_log_file(log_file)
    sessions = extract_sessions(raw_lines)

    if not sessions:
        console.print("\n  [dim]No engine sessions found in the log file.[/dim]\n")
        return

    to_show = sessions if all_sessions else [sessions[-1]]

    console.print()
    for idx, s in enumerate(to_show, start=1):
        _print_session_report(s, idx if all_sessions else None, len(to_show))
    console.print()


def _print_session_report(s: SessionSummary, idx: Optional[int], total: int) -> None:
    """Render a single session report."""
    mode_tag = "[yellow]DRY-RUN[/yellow]" if s.dry_run else "[green]LIVE[/green]"
    status_tag = (
        "[green]● Complete[/green]" if s.is_complete else "[yellow]◐ Incomplete / Running[/yellow]"
    )

    title_suffix = f" ({idx}/{total})" if idx is not None else ""

    # --- Overview panel ---
    started = s.started_at.strftime("%Y-%m-%d %H:%M:%S") if s.started_at else "—"
    stopped = s.stopped_at.strftime("%H:%M:%S") if s.stopped_at else "—"

    console.print(
        Panel(
            f"[bold]Status:[/bold]   {status_tag}\n"
            f"[bold]Started:[/bold]  {started}\n"
            f"[bold]Duration:[/bold] {_fmt_duration(s)}\n"
            f"[bold]Mode:[/bold]     {mode_tag}\n"
            f"[bold]Targets:[/bold]  {', '.join(s.targets) or '—'}\n"
            f"[bold]Interval:[/bold] [cyan]{s.interval}s[/cyan]  "
            f"[bold]Cooldown:[/bold] [cyan]{s.cooldown}s[/cyan]  "
            f"[bold]Max-down:[/bold] [cyan]{s.max_down}[/cyan]\n"
            f"[bold]Cycles:[/bold]   {s.cycles}  "
            f"[bold]Injections:[/bold] {len(s.injections)}  "
            f"[bold]Errors:[/bold] {s.error_count}",
            border_style="bright_red",
            title=f"[bold bright_red]🔥 Session Report{title_suffix}[/bold bright_red]",
            title_align="left",
        )
    )

    if not s.injections:
        console.print("  [dim]No injections recorded in this session.[/dim]")
        return

    total_inj = len(s.injections)

    # --- Action breakdown ---
    action_table = Table(
        box=box.ROUNDED,
        border_style="bright_black",
        header_style="bold cyan",
        padding=(0, 2),
        title="Injections by Action",
        title_style="bold",
    )
    action_table.add_column("Action", style="white")
    action_table.add_column("Count", justify="right")
    action_table.add_column("Share", justify="right")

    for action, count in s.action_counts.most_common():
        pct = f"{count / total_inj * 100:.0f}%"
        style = "bold red" if action == "stop" else "bold cyan"
        action_table.add_row(Text(action.upper(), style=style), str(count), pct)

    # --- Target breakdown ---
    target_table = Table(
        box=box.ROUNDED,
        border_style="bright_black",
        header_style="bold cyan",
        padding=(0, 2),
        title="Most Targeted Services",
        title_style="bold",
    )
    target_table.add_column("Service", style="white")
    target_table.add_column("Hits", justify="right")
    target_table.add_column("Share", justify="right")

    for target, count in s.target_counts.most_common():
        pct = f"{count / total_inj * 100:.0f}%"
        target_table.add_row(target, str(count), pct)

    # --- Safety events ---
    safety_table = Table(
        box=box.ROUNDED,
        border_style="bright_black",
        header_style="bold cyan",
        padding=(0, 2),
        title="Safety Events",
        title_style="bold",
        show_header=False,
    )
    safety_table.add_column("Event", style="bold cyan")
    safety_table.add_column("Count", style="white")
    safety_table.add_row("Cooldown skips", str(s.cooldown_skips))
    safety_table.add_row("Max-down skips", str(s.max_down_skips))
    safety_table.add_row("Errors", str(s.error_count))

    from rich.columns import Columns
    console.print()
    console.print(Columns([action_table, target_table, safety_table], padding=(0, 2)))

    # --- Error details ---
    errors = [i for i in s.injections if not i.success]
    if errors:
        console.print()
        console.print("  [bold red]Errors:[/bold red]")
        for e in errors:
            ts = e.timestamp.strftime("%H:%M:%S")
            console.print(f"  [dim]{ts}[/dim]  [red]{e.action.upper()} → {e.target}[/red]: {e.error}")
    console.print()
