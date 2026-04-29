"""
DevChaosKit CLI — `devchaos docker` subcommand group.

Exposes the Docker abstraction layer interactively so developers can
inspect and manually drive container lifecycle operations before the
chaos engine takes over in Phase 3.

Commands
--------
  devchaos docker list              List running containers
  devchaos docker list --all        Include stopped containers
  devchaos docker stop <name>       Gracefully stop a container
  devchaos docker restart <name>    Restart a container
"""

from __future__ import annotations

import typer
from rich.console import Console
from rich.table import Table
from rich.panel import Panel
from rich.text import Text
from rich import box

from src.engine.docker_client import DockerClient
from src.engine.exceptions import (
    ChaosKitError,
    DockerConnectionError,
    ContainerNotFoundError,
)

# ---------------------------------------------------------------------------
# Sub-application
# ---------------------------------------------------------------------------

docker_app = typer.Typer(
    name="docker",
    help="Inspect and control Docker containers.",
    rich_markup_mode="rich",
)

console = Console()


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

_STATUS_STYLE: dict[str, str] = {
    "running": "bold green",
    "paused": "bold yellow",
    "restarting": "bold cyan",
    "exited": "bold red",
    "dead": "red",
    "created": "dim",
}


def _status_badge(status: str) -> Text:
    style = _STATUS_STYLE.get(status.lower(), "white")
    return Text(f"● {status}", style=style)


def _handle_error(exc: ChaosKitError) -> None:
    """Print a user-friendly error panel and exit with code 1."""
    console.print(
        Panel(
            f"[bold red]Error:[/bold red] {exc}",
            border_style="red",
            title="[bold red]DevChaosKit[/bold red]",
            title_align="left",
        )
    )
    raise typer.Exit(code=1)


def _get_client(targets: set[str] | None = None) -> DockerClient:
    """Create a DockerClient or exit with a friendly error."""
    try:
        return DockerClient(allowed_targets=targets)
    except DockerConnectionError as exc:
        _handle_error(exc)
        raise  # unreachable — keeps type-checker happy


# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------

@docker_app.command("list")
def list_containers(
    all: bool = typer.Option(
        False,
        "--all",
        "-a",
        help="Include stopped / exited containers.",
    ),
) -> None:
    """List Docker containers visible on this host."""
    with _get_client() as dc:
        containers = dc.list_containers(all=all)

    if not containers:
        console.print(
            Panel(
                "[yellow]No containers found.[/yellow]\n"
                "Start your docker-compose stack first.",
                border_style="yellow",
                title="[bold]docker list[/bold]",
                title_align="left",
            )
        )
        return

    table = Table(
        title="[bold]Running Containers[/bold]",
        box=box.ROUNDED,
        border_style="bright_black",
        show_lines=False,
        header_style="bold cyan",
    )
    table.add_column("ID", style="dim", no_wrap=True)
    table.add_column("Name", style="bold white", no_wrap=True)
    table.add_column("Image", style="blue")
    table.add_column("Status", no_wrap=True)
    table.add_column("Ports", style="dim")

    for c in containers:
        port_str = (
            ", ".join(f"{v}→{k}" for k, v in c.ports.items()) or "—"
        )
        table.add_row(c.id, c.name, c.image, _status_badge(c.status), port_str)

    console.print()
    console.print(table)
    console.print(
        f"\n  [dim]{len(containers)} container(s) found.[/dim]\n"
    )


@docker_app.command("stop")
def stop_container(
    name: str = typer.Argument(..., help="Container or docker-compose service name."),
    timeout: int = typer.Option(
        10,
        "--timeout",
        "-t",
        help="Seconds to wait for graceful shutdown before SIGKILL.",
    ),
) -> None:
    """Gracefully stop a container by name (SIGTERM → SIGKILL)."""
    console.print(f"\n  [yellow]⏹  Stopping[/yellow] [bold]{name}[/bold] …")
    try:
        with _get_client() as dc:
            info = dc.stop_container(name, timeout=timeout)
    except (ContainerNotFoundError, ChaosKitError) as exc:
        _handle_error(exc)
        return

    console.print(
        Panel(
            f"[bold]Container:[/bold]  {info.name}\n"
            f"[bold]Status:[/bold]     {_status_badge(info.status)}\n"
            f"[bold]Image:[/bold]      {info.image}",
            border_style="yellow",
            title="[bold yellow]⏹  Stopped[/bold yellow]",
            title_align="left",
        )
    )
    console.print()


@docker_app.command("restart")
def restart_container(
    name: str = typer.Argument(..., help="Container or docker-compose service name."),
    timeout: int = typer.Option(
        10,
        "--timeout",
        "-t",
        help="Seconds to wait for old process to stop before forcing.",
    ),
) -> None:
    """Restart a container by name."""
    console.print(f"\n  [cyan]🔄  Restarting[/cyan] [bold]{name}[/bold] …")
    try:
        with _get_client() as dc:
            info = dc.restart_container(name, timeout=timeout)
    except (ContainerNotFoundError, ChaosKitError) as exc:
        _handle_error(exc)
        return

    console.print(
        Panel(
            f"[bold]Container:[/bold]  {info.name}\n"
            f"[bold]Status:[/bold]     {_status_badge(info.status)}\n"
            f"[bold]Image:[/bold]      {info.image}",
            border_style="cyan",
            title="[bold cyan]🔄  Restarted[/bold cyan]",
            title_align="left",
        )
    )
    console.print()
