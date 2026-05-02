"""
Entropy — Background worker entry point.

This module is launched by `entropy start --detach` via:

    python -m src._worker --config <path> [--dry-run] [--max-down N] [--cooldown N]

It runs the ChaosEngine in the foreground of the detached subprocess,
writing state to .entropy/state.json and structured logs to
.entropy/engine.log after every injection cycle.

This is an internal module — not part of the public CLI surface.
"""

from __future__ import annotations

import argparse
import os
import signal
import sys
from pathlib import Path

from src.config.loader import ConfigError, load_config
from src.engine.chaos_engine import ChaosEngine, InjectionEvent
from src.engine.docker_client import DockerConnectionError
from src.utils.logger import ChaosLogger
from src.utils.state import StateManager


def main() -> None:
    parser = argparse.ArgumentParser(prog="src._worker", add_help=False)
    parser.add_argument("--config", default="chaos.yaml")
    # Safety overrides passed from `entropy start --detach`
    parser.add_argument("--dry-run", dest="dry_run", action="store_true", default=None)
    parser.add_argument("--no-dry-run", dest="dry_run", action="store_false")
    parser.add_argument("--max-down", dest="max_down", type=int, default=None)
    parser.add_argument("--cooldown", dest="cooldown", type=int, default=None)
    args, _ = parser.parse_known_args()

    config_path = Path(args.config)
    state = StateManager()
    my_pid = os.getpid()

    # --- Load and patch config ---
    try:
        cfg = load_config(config_path)
    except ConfigError as exc:
        sys.stderr.write(f"[worker] Config error: {exc}\n")
        sys.exit(1)

    # Apply safety overrides forwarded from the CLI
    if args.dry_run is not None:
        cfg.safety.dry_run = args.dry_run
    if args.max_down is not None:
        cfg.safety.max_down = args.max_down
    if args.cooldown is not None:
        cfg.safety.cooldown = args.cooldown

    # --- Set up structured logger ---
    state.ensure_dir()
    chaos_logger = ChaosLogger(log_file=state.log_file)

    # --- Engine event callback ---
    def on_event(event: InjectionEvent) -> None:
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

    # Write initial state immediately (so `status` works before first cycle)
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

    # --- Signal handlers ---
    def _shutdown(sig: int, frame: object) -> None:
        engine.stop(timeout=10)
        chaos_logger.close()
        state.clear()
        sys.exit(0)

    signal.signal(signal.SIGTERM, _shutdown)
    signal.signal(signal.SIGINT, _shutdown)

    # --- Start engine ---
    try:
        engine.start()
    except DockerConnectionError as exc:
        sys.stderr.write(f"[worker] Docker error: {exc}\n")
        chaos_logger.log_error(str(exc))
        chaos_logger.close()
        sys.exit(1)

    # Block the main thread while the engine thread runs
    try:
        engine._thread.join()
    except (AttributeError, RuntimeError):
        pass


if __name__ == "__main__":
    main()
