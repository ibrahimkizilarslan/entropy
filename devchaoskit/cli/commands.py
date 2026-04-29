"""
DevChaosKit CLI — Top-level commands (Phase 4).

Implements the primary developer-facing interface:

  devchaos start [--config chaos.yaml] [--detach]
  devchaos stop
  devchaos status
  devchaos inject <action> <target> [--config chaos.yaml]

Architecture
------------
`start` without --detach runs the engine foreground (press Ctrl+C to quit).
`start --detach` spawns the engine as a background daemon via subprocess,
writing its PID + state to .devchaos/state.json.

`stop` and `status` communicate with the running engine through that state
file — no sockets, no HTTP, just an atomic JSON file.
"""

from __future__ import annotations

import os
import signal
import subprocess
import sys
from datetime import datetime, timezone
from pathlib import Path
from typing import Optional

import typer
from rich.console import Console
from rich.live import Live
from rich.panel import Panel
from rich.table import Table
from rich.text import Text
from rich import box

from devchaoskit.config.loader import ConfigError, load_config
from devchaoskit.engine.actions import dispatch
from devchaoskit.engine.chaos_engine import ChaosEngine, InjectionEvent
from devchaoskit.engine.docker_client import DockerClient, DockerConnectionError
from devchaoskit.engine.exceptions import ChaosKitError
from devchaoskit.utils.logger import ChaosLogger
from devchaoskit.utils.state import StateManager

# ---------------------------------------------------------------------------
# Module-level singletons
# ---------------------------------------------------------------------------

console = Console()
state = StateManager()

# ---------------------------------------------------------------------------
# Shared style helpers
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


def _action_text(action: str) -> Text:
    return Text(action.upper(), style=_ACTION_STYLE.get(action, "white"))


def _status_text(s: Optional[str]) -> Text:
    if s is None:
        return Text("—", style="dim")
    return Text(s, style=_STATUS_STYLE.get(s, "white"))


def _err_panel(msg: str, title: str = "Error") -> None:
    console.print(
        Panel(
            msg,
            border_style="red",
            title=f"[bold red]{title}[/bold red]",
            title_align="left",
        )
    )


def _ok_panel(msg: str, title: str = "OK") -> None:
    console.print(
        Panel(
            msg,
            border_style="green",
            title=f"[bold green]{title}[/bold green]",
            title_align="left",
        )
    )


def _history_table(history: list) -> Table:
    """Build a Rich Table from a list of event dicts or InjectionEvent objects."""
    table = Table(
        box=box.SIMPLE,
        border_style="bright_black",
        header_style="bold cyan",
        padding=(0, 1),
        expand=True,
    )
    table.add_column("Time", style="dim", no_wrap=True, width=10)
    table.add_column("Action", no_wrap=True, width=10)
    table.add_column("Target", no_wrap=True)
    table.add_column("Result", no_wrap=True, width=10)
    table.add_column("Post-Status", no_wrap=True)

    for e in reversed(history[-15:]):
        if isinstance(e, dict):
            ts_raw = e.get("timestamp", "")
            ts = ts_raw[11:19] if len(ts_raw) >= 19 else ts_raw
            action = e.get("action", "?")
            target = e.get("target", "?")
            error = e.get("error")
            dry = e.get("dry_run", False)
            result_status = e.get("result_status")
        else:
            ts = e.timestamp.strftime("%H:%M:%S")
            action = e.action
            target = e.target
            error = e.error
            dry = e.dry_run
            result_status = e.result_status

        if dry:
            result = Text("DRY-RUN", style="dim yellow")
            post = Text("—", style="dim")
        elif error:
            result = Text("✗ ERROR", style="bold red")
            post = Text(error[:40], style="red")
        else:
            result = Text("✓ OK", style="bold green")
            post = _status_text(result_status)

        table.add_row(ts, _action_text(action), Text(target), result, post)

    return table


# ---------------------------------------------------------------------------
# ─── devchaos start ─────────────────────────────────────────────────────────
# ---------------------------------------------------------------------------


def cmd_start(
    config_path: Path = typer.Option(
        Path("chaos.yaml"),
        "--config",
        "-c",
        help="Path to chaos config file.",
    ),
    detach: bool = typer.Option(
        False,
        "--detach",
        "-d",
        help="Run engine in background (daemon mode).",
    ),
    dry_run: Optional[bool] = typer.Option(
        None,
        "--dry-run/--no-dry-run",
        help="Override config: log actions without executing them.",
    ),
    max_down: Optional[int] = typer.Option(
        None,
        "--max-down",
        min=1,
        help="Override config: max containers stopped simultaneously.",
    ),
    cooldown: Optional[int] = typer.Option(
        None,
        "--cooldown",
        min=0,
        help="Override config: min seconds between injections.",
    ),
) -> None:
    """Start the chaos engine. Use --detach / -d to run in the background."""
    # --- Validate config first ---
    try:
        cfg = load_config(config_path)
    except ConfigError as exc:
        _err_panel(str(exc), "Config Error")
        raise typer.Exit(1)

    # --- Apply CLI safety overrides (Phase 5) ---
    if dry_run is not None:
        cfg.safety.dry_run = dry_run
    if max_down is not None:
        cfg.safety.max_down = max_down
    if cooldown is not None:
        cfg.safety.cooldown = cooldown

    # --- Check if already running ---
    pid = state.running_pid()
    if pid is not None:
        _err_panel(
            f"Chaos engine is already running (PID {pid}).\n"
            "Run [bold]devchaos stop[/bold] first.",
            "Already Running",
        )
        raise typer.Exit(1)

    if detach:
        _start_detached(config_path, cfg)
    else:
        _start_foreground(cfg, config_path)


def _start_detached(config_path: Path, cfg) -> None:
    """Spawn the engine as a detached background subprocess."""
    state.ensure_dir()
    log_path = state.log_file

    # Pass any safety overrides that were applied to cfg
    extra_args: list[str] = [
        "--dry-run" if cfg.safety.dry_run else "--no-dry-run",
        "--max-down", str(cfg.safety.max_down),
        "--cooldown", str(cfg.safety.cooldown),
    ]

    proc = subprocess.Popen(
        [
            sys.executable,
            "-m",
            "devchaoskit._worker",
            "--config",
            str(config_path.resolve()),
            *extra_args,
        ],
        stdout=open(log_path, "w"),
        stderr=subprocess.STDOUT,
        start_new_session=True,
        close_fds=True,
    )

    console.print(
        Panel(
            f"[bold]PID:[/bold]     {proc.pid}\n"
            f"[bold]Config:[/bold]  {config_path}\n"
            f"[bold]Mode:[/bold]    {'[yellow]DRY-RUN[/yellow]' if cfg.safety.dry_run else '[green]LIVE[/green]'}\n"
            f"[bold]Cooldown:[/bold] {cfg.safety.cooldown}s  [bold]Max-down:[/bold] {cfg.safety.max_down}\n"
            f"[bold]Logs:[/bold]    {log_path}\n\n"
            "Run [bold cyan]devchaos status[/bold cyan] to monitor.\n"
            "Run [bold cyan]devchaos stop[/bold cyan]   to terminate.",
            border_style="bright_red",
            title="[bold bright_red]🔥 Chaos Engine Started (background)[/bold bright_red]",
            title_align="left",
        )
    )


def _start_foreground(cfg, config_path: Path) -> None:
    """Run the engine in the foreground with a Live event display."""
    dry_tag = "[bold yellow]DRY-RUN[/bold yellow]" if cfg.safety.dry_run else "[bold green]LIVE[/bold green]"
    console.print()
    console.print(
        Panel(
            f"[bold]Config:[/bold]   {config_path}\n"
            f"[bold]Targets:[/bold]  {', '.join(cfg.targets)}\n"
            f"[bold]Actions:[/bold]  {', '.join(cfg.actions)}\n"
            f"[bold]Interval:[/bold] every [cyan]{cfg.interval}s[/cyan]\n"
            f"[bold]Cooldown:[/bold] [cyan]{cfg.safety.cooldown}s[/cyan] between injections\n"
            f"[bold]Max down:[/bold] {cfg.safety.max_down} container(s) simultaneously\n"
            f"[bold]Mode:[/bold]     {dry_tag}\n\n"
            "Press [bold]Ctrl+C[/bold] to stop.",
            border_style="bright_red",
            title="[bold bright_red]🔥 Chaos Engine Starting[/bold bright_red]",
            title_align="left",
        )
    )
    console.print()

    my_pid = os.getpid()
    events: list[InjectionEvent] = []
    state.ensure_dir()

    # Create structured file logger (Phase 5)
    chaos_logger = ChaosLogger(log_file=state.log_file)

    def on_event(event: InjectionEvent) -> None:
        events.append(event)
        engine_status = engine.status()
        state.write_from_engine(
            pid=my_pid,
            config_path=str(config_path.resolve()),
            cycle_count=engine_status.cycle_count,
            down_containers=engine_status.down_containers,
            history=engine_status.history,
            dry_run=cfg.safety.dry_run,
            cooldown_remaining=engine_status.cooldown_remaining,
            cooldown_total=cfg.safety.cooldown,
        )

    # Write initial state so `status` works immediately
    state.write_from_engine(
        pid=my_pid,
        config_path=str(config_path.resolve()),
        cycle_count=0,
        down_containers=set(),
        history=[],
        dry_run=cfg.safety.dry_run,
        cooldown_remaining=0.0,
        cooldown_total=cfg.safety.cooldown,
    )

    engine = ChaosEngine(config=cfg, on_event=on_event, logger=chaos_logger)

    def _shutdown(sig: int, frame: object) -> None:
        console.print("\n\n  [yellow]Stopping chaos engine…[/yellow]")
        engine.stop(timeout=10)
        chaos_logger.close()
        st = engine.status()
        state.clear()
        console.print(
            Panel(
                f"[bold]Cycles completed:[/bold]  {st.cycle_count}\n"
                f"[bold]Total injections:[/bold]  {len(st.history)}\n"
                f"[bold]Log file:[/bold]          {state.log_file}",
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
        _err_panel(str(exc), "Docker Error")
        raise typer.Exit(1)

    with Live(console=console, refresh_per_second=2, screen=False) as live:
        while True:
            live.update(_history_table(engine.status().history))


# ---------------------------------------------------------------------------
# ─── devchaos stop ──────────────────────────────────────────────────────────
# ---------------------------------------------------------------------------


def cmd_stop() -> None:
    """Stop a running background chaos engine."""
    pid = state.running_pid()

    if pid is None:
        raw = state.read()
        if raw:
            _err_panel(
                "Chaos engine process is no longer alive (stale state file cleaned up).",
                "Not Running",
            )
            state.clear()
        else:
            _err_panel(
                "No chaos engine is running.\n"
                "Start one with [bold cyan]devchaos start[/bold cyan].",
                "Not Running",
            )
        raise typer.Exit(1)

    console.print(f"\n  [yellow]Sending SIGTERM to PID {pid}…[/yellow]")
    try:
        os.kill(pid, signal.SIGTERM)
    except ProcessLookupError:
        _err_panel(f"Process {pid} does not exist.", "Stop Failed")
        state.clear()
        raise typer.Exit(1)

    # Wait up to 10s for process to exit
    for _ in range(20):
        import time
        time.sleep(0.5)
        if not state.is_alive(pid):
            break

    state.clear()
    _ok_panel(
        f"Chaos engine (PID {pid}) has been stopped.",
        "⏹  Engine Stopped",
    )
    console.print()


# ---------------------------------------------------------------------------
# ─── devchaos status ────────────────────────────────────────────────────────
# ---------------------------------------------------------------------------


def cmd_status() -> None:
    """Show the status of the chaos engine."""
    raw = state.read()

    if raw is None:
        console.print(
            Panel(
                "[dim]No chaos engine is currently running.[/dim]\n"
                "Start one with [bold cyan]devchaos start[/bold cyan].",
                border_style="bright_black",
                title="[bold]Status[/bold]",
                title_align="left",
            )
        )
        return

    pid: int = raw.get("pid", 0)
    alive = state.is_alive(pid)
    engine_state = "[bold green]● Running[/bold green]" if alive else "[bold red]○ Stopped (stale)[/bold red]"

    # Meta table
    meta = Table(box=box.ROUNDED, border_style="bright_black", show_header=False, padding=(0, 2))
    meta.add_column("Field", style="bold cyan", no_wrap=True)
    meta.add_column("Value", style="white")
    meta.add_row("Engine", engine_state)
    meta.add_row("PID", str(pid))
    meta.add_row("Config", raw.get("config_path", "—"))
    meta.add_row("Mode", "[yellow]DRY-RUN[/yellow]" if raw.get("dry_run") else "[green]LIVE[/green]")
    meta.add_row("Cycles", str(raw.get("cycle_count", 0)))
    meta.add_row("Down now", ", ".join(raw.get("down_containers") or []) or "—")

    # Cooldown display (Phase 5)
    cooldown_remaining = raw.get("cooldown_remaining", 0)
    cooldown_total = raw.get("cooldown_total", 0)
    if cooldown_total > 0:
        if cooldown_remaining > 0:
            filled = int((1 - cooldown_remaining / cooldown_total) * 20)
            bar = "█" * filled + "░" * (20 - filled)
            meta.add_row(
                "Cooldown",
                f"[yellow]{bar}[/yellow] {cooldown_remaining:.0f}s remaining",
            )
        else:
            meta.add_row("Cooldown", "[green]Ready[/green]")

    last = raw.get("last_event")
    if last:
        ts_raw = last.get("timestamp", "")
        ts = ts_raw[11:19] if len(ts_raw) >= 19 else ts_raw
        meta.add_row(
            "Last injection",
            f"{last.get('action', '?').upper()} → {last.get('target', '?')} at {ts}",
        )

    console.print()
    console.print(meta)

    history = raw.get("history") or []
    if history:
        console.print()
        console.print("  [bold]Recent injections:[/bold]")
        console.print(_history_table(history))
    console.print()


# ---------------------------------------------------------------------------
# ─── devchaos inject ────────────────────────────────────────────────────────
# ---------------------------------------------------------------------------


def cmd_inject(
    action: str = typer.Argument(..., help="Action to inject: stop | restart"),
    target: str = typer.Argument(..., help="Container or docker-compose service name."),
    config_path: Path = typer.Option(
        Path("chaos.yaml"),
        "--config",
        "-c",
        help="Path to chaos config (for allow-list validation).",
        show_default=True,
    ),
    skip_validation: bool = typer.Option(
        False,
        "--skip-validation",
        help="Bypass allow-list check (use with caution).",
    ),
) -> None:
    """
    Manually inject a single chaos action into a target container.

    Examples
    --------
    \\b
      devchaos inject stop service-a
      devchaos inject restart service-b --config ./my-chaos.yaml
    """
    # Validate action name
    from devchaoskit.engine.actions import ACTION_HANDLERS
    if action not in ACTION_HANDLERS:
        _err_panel(
            f"Unknown action '{action}'.\n"
            f"Valid actions: {', '.join(sorted(ACTION_HANDLERS))}",
            "Invalid Action",
        )
        raise typer.Exit(1)

    # Optional config-based allow-list
    allowed: Optional[set[str]] = None
    if not skip_validation and config_path.exists():
        try:
            cfg = load_config(config_path)
            allowed = set(cfg.targets)
            if target not in allowed:
                _err_panel(
                    f"'{target}' is not in the targets list of [cyan]{config_path}[/cyan].\n"
                    "Use [bold]--skip-validation[/bold] to bypass, or add it to your config.",
                    "Target Not Allowed",
                )
                raise typer.Exit(1)
        except ConfigError:
            # Config file exists but is invalid — allow without list
            allowed = None

    console.print(f"\n  [bold]{_action_text(action)}[/bold]  →  [bold white]{target}[/bold white] …\n")

    try:
        with DockerClient(allowed_targets=allowed) as dc:
            info = dispatch(action, dc, target)
    except ChaosKitError as exc:
        _err_panel(str(exc), "Inject Failed")
        raise typer.Exit(1)

    console.print(
        Panel(
            f"[bold]Action:[/bold]    {action.upper()}\n"
            f"[bold]Target:[/bold]    {info.name}\n"
            f"[bold]New status:[/bold] {_status_text(info.status)}\n"
            f"[bold]Image:[/bold]     {info.image}",
            border_style="green",
            title="[bold green]✓ Injected[/bold green]",
            title_align="left",
        )
    )
    console.print()
