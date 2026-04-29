"""
DevChaosKit — Background worker entry point.

This module is launched by `devchaos start --detach` via:

    python -m devchaoskit._worker --config <path>

It runs the ChaosEngine in the foreground of the detached subprocess,
writing state to .devchaos/state.json after every injection so that
`devchaos status` and `devchaos stop` can observe it.

This is an internal module — not part of the public CLI surface.
"""

from __future__ import annotations

import os
import signal
import sys
from pathlib import Path

import argparse

from devchaoskit.config.loader import ConfigError, load_config
from devchaoskit.engine.chaos_engine import ChaosEngine, InjectionEvent
from devchaoskit.engine.docker_client import DockerConnectionError
from devchaoskit.utils.state import StateManager


def main() -> None:
    parser = argparse.ArgumentParser(prog="devchaoskit._worker", add_help=False)
    parser.add_argument("--config", default="chaos.yaml")
    args, _ = parser.parse_known_args()

    config_path = Path(args.config)
    state = StateManager()
    my_pid = os.getpid()

    try:
        cfg = load_config(config_path)
    except ConfigError as exc:
        sys.stderr.write(f"[worker] Config error: {exc}\n")
        sys.exit(1)

    events: list[InjectionEvent] = []

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
        )

    # Write initial state immediately (so `status` works before first cycle)
    state.ensure_dir()
    state.write_from_engine(
        pid=my_pid,
        config_path=str(config_path.resolve()),
        cycle_count=0,
        down_containers=set(),
        history=[],
        dry_run=cfg.safety.dry_run,
    )

    engine = ChaosEngine(config=cfg, on_event=on_event)

    def _shutdown(sig: int, frame: object) -> None:
        engine.stop(timeout=10)
        state.clear()
        sys.exit(0)

    signal.signal(signal.SIGTERM, _shutdown)
    signal.signal(signal.SIGINT, _shutdown)

    try:
        engine.start()
    except DockerConnectionError as exc:
        sys.stderr.write(f"[worker] Docker error: {exc}\n")
        sys.exit(1)

    # Block the main thread while the engine thread runs
    try:
        engine._thread.join()
    except (AttributeError, RuntimeError):
        pass


if __name__ == "__main__":
    main()
