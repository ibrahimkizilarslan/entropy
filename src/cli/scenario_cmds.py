"""
DevChaosKit CLI — Scenario Engine commands.

devchaos scenario run <file>
"""

from __future__ import annotations

from pathlib import Path

import typer
from rich.console import Console
from rich.panel import Panel

from src.config.loader import ConfigError, load_scenario
from src.engine.scenario_runner import ScenarioRunner

scenario_app = typer.Typer(
    help="Run deterministic chaos scenarios.",
    no_args_is_help=True,
)
console = Console()


@scenario_app.command("run")
def cmd_scenario_run(
    scenario_path: Path = typer.Argument(
        ...,
        help="Path to the scenario YAML file.",
        exists=True,
        dir_okay=False,
    )
) -> None:
    """Run a deterministic chaos scenario and verify hypotheses."""
    try:
        config = load_scenario(scenario_path)
    except ConfigError as exc:
        console.print(Panel(str(exc), border_style="red", title="[bold red]Config Error[/bold red]"))
        raise typer.Exit(1)

    # We define a simple print wrapper to pass to the runner
    def _log(msg: str) -> None:
        console.print(f"  {msg}")

    console.print()
    runner = ScenarioRunner(config=config, log_callback=_log)
    result = runner.run()

    # Print summary
    console.print()
    if result.success:
        summary_title = "[bold green]✅ Hypothesis Confirmed[/bold green]"
        border = "green"
    else:
        summary_title = "[bold red]❌ Hypothesis Failed[/bold red]"
        border = "red"
        
    summary_text = (
        f"[bold]Probes passed:[/bold] {result.probes_passed} / {result.probes_total}\n"
        f"[bold]Steps run:[/bold]     {result.executed_steps} / {result.total_steps}"
    )
    if result.error:
        summary_text += f"\n[bold red]Error:[/bold red] {result.error}"

    console.print(
        Panel(
            summary_text,
            border_style=border,
            title=summary_title,
            title_align="left",
            padding=(1, 2)
        )
    )
    console.print()

    if not result.success:
        raise typer.Exit(1)
