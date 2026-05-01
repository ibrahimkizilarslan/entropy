"""
DevChaosKit — Configuration loader.

Reads a chaos.yaml (or chaos.json) file, maps it to a ChaosConfig dataclass,
runs validation, and raises friendly errors for common mistakes.
"""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

import yaml

from src.config.schema import (
    ActionSpec,
    ChaosConfig,
    SafetyConfig,
    ScenarioConfig,
    ScenarioStep,
    ProbeSpec,
)


# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------


class ConfigError(Exception):
    """Raised when the chaos config file is missing, malformed, or invalid."""


def load_config(path: str | Path = "chaos.yaml") -> ChaosConfig:
    """
    Load and validate a chaos config file.

    Supports YAML (.yaml / .yml) and JSON (.json) formats.
    """
    config_path = Path(path)

    if not config_path.exists():
        raise ConfigError(
            f"Config file not found: '{config_path}'\n"
            "  → Copy chaos.example.yaml to chaos.yaml and edit it."
        )

    raw = _read_file(config_path)
    return _build_config(raw, config_path)


def load_scenario(path: str | Path) -> ScenarioConfig:
    """Load and validate a scenario config file."""
    config_path = Path(path)
    if not config_path.exists():
        raise ConfigError(f"Scenario file not found: '{config_path}'")

    raw = _read_file(config_path)

    name = raw.get("name", "Untitled Scenario")
    description = raw.get("description", "")
    hypothesis = raw.get("hypothesis", "")

    raw_steps = raw.get("steps", [])
    if not isinstance(raw_steps, list):
        raise ConfigError("'steps' must be a YAML list.")

    steps = []
    for idx, raw_step in enumerate(raw_steps):
        if not isinstance(raw_step, dict):
            raise ConfigError(f"Step {idx} must be a dictionary.")

        if "wait" in raw_step:
            val = str(raw_step["wait"]).strip().lower()
            if val.endswith("s"):
                val = val[:-1]
            try:
                duration = int(val)
            except ValueError:
                raise ConfigError(f"Invalid wait duration: {raw_step['wait']}")
            steps.append(ScenarioStep(type="wait", duration_s=duration))

        elif "inject" in raw_step:
            inj = raw_step["inject"]
            if not isinstance(inj, dict):
                raise ConfigError(f"'inject' must be a dict in step {idx}")
            action = _parse_action(inj.get("action") or inj)
            target = inj.get("target")
            if not target:
                raise ConfigError(f"'target' is required in inject step {idx}")
            steps.append(ScenarioStep(type="inject", action=action, target=target))

        elif "probe" in raw_step:
            prb = raw_step["probe"]
            if not isinstance(prb, dict):
                raise ConfigError(f"'probe' must be a dict in step {idx}")
            probe_spec = ProbeSpec(
                type=prb.get("type", "http"),
                url=prb.get("url", ""),
                expect_status=prb.get("expect_status"),
                expect_not_status=prb.get("expect_not_status"),
                timeout=int(prb.get("timeout", 5)),
            )
            steps.append(ScenarioStep(type="probe", probe=probe_spec))
        else:
            raise ConfigError(f"Unknown step type in step {idx}: {list(raw_step.keys())}")

    return ScenarioConfig(
        name=name,
        description=description,
        hypothesis=hypothesis,
        steps=steps,
    )


# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------


def _read_file(path: Path) -> dict[str, Any]:
    """Parse YAML or JSON from disk."""
    try:
        text = path.read_text(encoding="utf-8")
    except OSError as exc:
        raise ConfigError(f"Cannot read config file '{path}': {exc}") from exc

    suffix = path.suffix.lower()
    try:
        if suffix in {".yaml", ".yml"}:
            data = yaml.safe_load(text)
        elif suffix == ".json":
            data = json.loads(text)
        else:
            raise ConfigError(
                f"Unsupported config format '{suffix}'. Use .yaml or .json."
            )
    except (yaml.YAMLError, json.JSONDecodeError) as exc:
        raise ConfigError(f"Failed to parse '{path}': {exc}") from exc

    if not isinstance(data, dict):
        raise ConfigError(
            f"Config file '{path}' must be a YAML/JSON mapping, got {type(data).__name__}."
        )
    return data


def _parse_action(raw: Any) -> ActionSpec:
    """
    Parse one entry from the actions list into an ActionSpec.

    Accepts two formats:
      - A plain string:  "stop"  →  ActionSpec(name="stop")
      - A dict:          {"name": "delay", "latency_ms": 300, ...}
    """
    if isinstance(raw, str):
        return ActionSpec(name=raw.strip().lower())

    if isinstance(raw, dict):
        name = str(raw.get("name", "")).strip().lower()
        if not name:
            raise ConfigError(
                f"Action dict must have a 'name' field. Got: {raw!r}"
            )
        return ActionSpec(
            name=name,
            latency_ms=int(raw.get("latency_ms", 300)),
            jitter_ms=int(raw.get("jitter_ms", 0)),
            loss_percent=int(raw.get("percent", raw.get("loss_percent", 20))),
            cpus=float(raw.get("cpus", 0.25)),
            memory_mb=int(raw.get("memory_mb", 128)),
            duration=int(raw["duration"]) if "duration" in raw else None,
        )

    raise ConfigError(
        f"Each action must be a string or a dict, got {type(raw).__name__}: {raw!r}"
    )


def _build_config(raw: dict[str, Any], path: Path) -> ChaosConfig:
    """Map a raw dict to ChaosConfig and run validation."""
    # --- Required fields ---
    if "interval" not in raw:
        raise ConfigError("Missing required field 'interval' in config.")
    if "targets" not in raw:
        raise ConfigError("Missing required field 'targets' in config.")

    try:
        interval = int(raw["interval"])
    except (TypeError, ValueError):
        raise ConfigError(
            f"'interval' must be an integer (seconds), got {raw['interval']!r}."
        )

    targets: list[str] = raw.get("targets") or []
    if not isinstance(targets, list):
        raise ConfigError("'targets' must be a YAML list of container names.")

    raw_actions = raw.get("actions") or ["stop"]
    if not isinstance(raw_actions, list):
        raise ConfigError("'actions' must be a YAML list.")

    try:
        actions = [_parse_action(a) for a in raw_actions]
    except ConfigError:
        raise
    except Exception as exc:
        raise ConfigError(f"Failed to parse actions in '{path}': {exc}") from exc

    # --- Safety sub-config ---
    safety_raw: dict = raw.get("safety") or {}
    safety = SafetyConfig(
        max_down=int(safety_raw.get("max_down", 1)),
        cooldown=int(safety_raw.get("cooldown", 30)),
        dry_run=bool(safety_raw.get("dry_run", False)),
    )

    # --- Assemble and validate ---
    try:
        config = ChaosConfig(
            interval=interval,
            targets=[str(t) for t in targets],
            actions=actions,
            safety=safety,
        )
        config.validate()
    except ValueError as exc:
        raise ConfigError(f"Invalid config in '{path}': {exc}") from exc

    return config
