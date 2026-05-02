"""
Entropy CLI — `entropy chaos` subcommand group (Phase 3 preview).

Provides a minimal `chaos run` command that drives the ChaosEngine with a
config file.  The full entropy start / stop / status / inject interface
is implemented in Phase 4.

Commands
--------
  entropy chaos run --config chaos.yaml
  entropy chaos run --config chaos.yaml --dry-run
"""

from __future__ import annotations

import signal
import sys
from pathlib import Path
from typing import Optional

import typer
from rich.console import Console
from rich.panel import Panel
from rich.live import Live
from rich.table import Table
from rich.text import Text
from rich import box

from src.config.loader import ConfigError, load_config
from src.engine.chaos_engine import ChaosEngine, EngineStatus, InjectionEvent
from src.engine.docker_client import DockerConnectionError


# ---------------------------------------------------------------------------
# Sub-application
# ---------------------------------------------------------------------------

chaos_app = typer.Typer(
    name="chaos",
    help="Run the chaos engine against your Docker containers.",
    rich_markup_mode="rich",
)

console = Console()

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

_ACTION_STYLE: dict[str, str] = {
    "stop": "bold red",
    "restart": "bold cyan",
}

_STATUS_STYLE: dict[str, str] = {
    "running": "green",
    "exited": "red",
    "restarting": "cyan",
    "(dry-run)": "dim yellow",
}


def _event_row(event: InjectionEvent) -> tuple:
    """Format an InjectionEvent as a Rich table row tuple."""
    ts = event.timestamp.strftime("%H:%M:%S")
    action_text = Text(event.action.upper(), style=_ACTION_STYLE.get(event.action, "white"))
    target_text = Text(event.target, style="bold white")

    if event.dry_run:
        status_text = Text("DRY-RUN", style="dim yellow")
        result_text = Text("—", style="dim")
    elif event.success:
        status_text = Text("✓ OK", style="bold green")
        result_text = Text(
            event.result_status or "—",
            style=_STATUS_STYLE.get(event.result_status or "", "white"),
        )
    else:
        status_text = Text("✗ ERROR", style="bold red")
        result_text = Text(event.error or "—", style="red")

    return ts, action_text, target_text, status_text, result_text


def _build_history_table(history: list[InjectionEvent]) -> Table:
    table = Table(
        box=box.SIMPLE,
        border_style="bright_black",
        show_header=True,
        header_style="bold cyan",
        padding=(0, 1),
        expand=True,
    )
    table.add_column("Time", style="dim", no_wrap=True, width=10)
    table.add_column("Action", no_wrap=True, width=10)
    table.add_column("Target", no_wrap=True)
    table.add_column("Result", no_wrap=True, width=10)
    table.add_column("Status", no_wrap=True)

    for event in reversed(history[-15:]):   # Show last 15 events, newest first
        ts, action, target, result, status = _event_row(event)
        table.add_row(ts, action, target, result, status)

    return table


# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------


@chaos_app.command("run")
def chaos_run(
    config_path: Path = typer.Option(
        Path("chaos.yaml"),
        "--config",
        "-c",
        help="Path to chaos config file (YAML or JSON).",
        exists=False,   # We give a nicer error ourselves
    ),
    dry_run: Optional[bool] = typer.Option(
        None,
        "--dry-run",
        help="Override config: log actions without executing them.",
    ),
) -> None:
    """
    Start the chaos engine with the given config file.

    Press [bold]Ctrl+C[/bold] to stop gracefully.
    """
    # --- Load config ---
    try:
        cfg = load_config(config_path)
    except ConfigError as exc:
        console.print(
            Panel(str(exc), border_style="red", title="[bold red]Config Error[/bold red]", title_align="left")
        )
        raise typer.Exit(1)

    # CLI flag overrides config dry_run
    if dry_run is not None:
        cfg.safety.dry_run = dry_run

    # --- Print startup banner ---
    mode_tag = "[bold yellow]DRY-RUN[/bold yellow]" if cfg.safety.dry_run else "[bold green]LIVE[/bold green]"
    console.print()
    console.print(
        Panel(
            f"[bold]Targets:[/bold]  {', '.join(cfg.targets)}\n"
            f"[bold]Actions:[/bold]  {', '.join(cfg.actions)}\n"
            f"[bold]Interval:[/bold] every [cyan]{cfg.interval}s[/cyan]\n"
            f"[bold]Mode:[/bold]     {mode_tag}\n"
            f"[bold]Max down:[/bold] {cfg.safety.max_down} container(s)\n\n"
            "Press [bold]Ctrl+C[/bold] to stop.",
            border_style="bright_red",
            title="[bold bright_red]🔥 Chaos Engine Starting[/bold bright_red]",
            title_align="left",
        )
    )
    console.print()

    # --- Shared event list for Live display ---
    events: list[InjectionEvent] = []

    def on_event(event: InjectionEvent) -> None:
        events.append(event)

    # --- Start engine ---
    engine = ChaosEngine(config=cfg, on_event=on_event)

    # Handle Ctrl+C and SIGTERM gracefully
    def _shutdown(sig: int, frame: object) -> None:
        console.print("\n\n  [yellow]Stopping chaos engine…[/yellow]")
        engine.stop(timeout=10)
        status = engine.status()
        console.print(
            Panel(
                f"[bold]Cycles completed:[/bold] {status.cycle_count}\n"
                f"[bold]Total injections:[/bold] {len(status.history)}",
                border_style="yellow",
                title="[bold yellow]⏹  Engine Stopped[/bold yellow]",
                title_align="left",
            )
        )
        sys.exit(0)

    signal.signal(signal.SIGINT, _shutdown)
    signal.signal(signal.SIGTERM, _shutdown)

    try:
        engine.start()
    except DockerConnectionError as exc:
        console.print(
            Panel(str(exc), border_style="red", title="[bold red]Docker Error[/bold red]", title_align="left")
        )
        raise typer.Exit(1)

    # --- Live display loop ---
    with Live(console=console, refresh_per_second=2, screen=False) as live:
        while True:
            status: EngineStatus = engine.status()
            table = _build_history_table(status.history)
            live.update(table)
