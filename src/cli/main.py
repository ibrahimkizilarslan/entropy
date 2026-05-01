"""
DevChaosKit CLI — Main entry point.

Defines the Typer application and top-level commands.
All subcommand groups will be registered here as the project grows.
"""

from __future__ import annotations

import typer
from rich.console import Console
from rich.panel import Panel
from rich.table import Table
from rich.text import Text
from rich import box

from src import __version__
from src.cli.docker_cmds import docker_app
from src.cli.chaos_cmds import chaos_app
from src.cli.commands import (
    cmd_start,
    cmd_stop,
    cmd_status,
    cmd_inject,
    cmd_init,
)
from src.cli.observability_cmds import cmd_logs, cmd_report

# ---------------------------------------------------------------------------
# Application bootstrap
# ---------------------------------------------------------------------------

app = typer.Typer(
    name="devchaos",
    help="🔥 DevChaosKit — Local chaos engineering for Docker microservices.",
    add_completion=True,
    rich_markup_mode="rich",
    no_args_is_help=True,
)

# Register sub-applications (groups)
app.add_typer(docker_app, name="docker")
app.add_typer(chaos_app, name="chaos")

# Register top-level commands
app.command("init",   help="Auto-discover docker-compose services and create config.")(cmd_init)
app.command("start",  help="Start the chaos engine.")(cmd_start)
app.command("stop",   help="Stop a running background chaos engine.")(cmd_stop)
app.command("status", help="Show the current chaos engine status.")(cmd_status)
app.command("inject", help="Manually inject a single chaos action.")(cmd_inject)
app.command("logs",   help="View the chaos engine log file.")(cmd_logs)
app.command("report", help="Show a statistics report from the log.")(cmd_report)

console = Console()


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _banner() -> Panel:
    """Return the DevChaosKit ASCII banner as a Rich Panel."""
    art = Text(justify="center")
    art.append(
        "  ____            ____ _                    _  _____ _   \n",
        style="bold red",
    )
    art.append(
        " |  _ \\  _____   / ___| |__   __ _  ___  | |/ /_ _| |_ \n",
        style="bold red",
    )
    art.append(
        " | | | |/ _ \\ \\ / / __| '_ \\ / _` |/ _ \\ | ' / | || __|\n",
        style="bold yellow",
    )
    art.append(
        " | |_| |  __/\\ V /\\__ \\ | | | (_| | (_) || . \\ | || |_ \n",
        style="bold yellow",
    )
    art.append(
        " |____/ \\___| \\_/ |___/_| |_|\\__,_|\\___/ |_|\\_\\___|\\__|\n",
        style="bold green",
    )
    art.append(
        "\n  Local Chaos Engineering Toolkit for Docker Microservices\n",
        style="dim white",
    )
    return Panel(
        art,
        border_style="bold bright_red",
        padding=(1, 4),
    )


def _version_table() -> Table:
    """Return a Rich Table with version and environment metadata."""
    import platform
    import sys

    table = Table(
        box=box.ROUNDED,
        border_style="bright_black",
        show_header=False,
        padding=(0, 2),
    )
    table.add_column("Field", style="bold cyan", no_wrap=True)
    table.add_column("Value", style="white")

    table.add_row("Version", f"[bold green]{__version__}[/bold green]")
    table.add_row("Python", sys.version.split()[0])
    table.add_row("Platform", platform.system())
    table.add_row("Status", "[bold yellow]Phase 6 — Observability[/bold yellow]")
    table.add_row("Scope", "Local Docker · Developer Tooling")

    return table


# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------

@app.command("version")
def version_cmd() -> None:
    """Show DevChaosKit version and environment information."""
    console.print()
    console.print(_banner())
    console.print()
    console.print(_version_table())
    console.print()
    console.print(
        "  Run [bold cyan]devchaos --help[/bold cyan] to see available commands.\n",
        style="dim",
    )


@app.callback(invoke_without_command=True)
def _root_callback(
    ctx: typer.Context,
    version: bool = typer.Option(
        False,
        "--version",
        "-v",
        help="Print version and exit.",
        is_eager=True,
    ),
) -> None:
    """🔥 DevChaosKit — Inject controlled failures into local Docker microservices."""
    if version:
        version_cmd()
        raise typer.Exit()


# ---------------------------------------------------------------------------
# Entry point (direct script execution)
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    app()
