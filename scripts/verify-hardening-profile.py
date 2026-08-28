#!/usr/bin/env python3
"""Fail closed when the single-host hardening profile is unsafe or incomplete."""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import stat
import subprocess
import sys
from urllib.parse import urlparse


PROJECT_ROOT = Path(__file__).resolve().parents[1]
SECRET_KEYS = (
    "DEMO_SESSION_SECRET",
    "GRAFANA_WEBHOOK_SECRET",
    "POSTGRES_PASSWORD",
    "DEMO_ORDER_POSTGRES_PASSWORD",
    "DEMO_PAYMENT_POSTGRES_PASSWORD",
    "DEMO_SHIPPING_POSTGRES_PASSWORD",
    "CLICKHOUSE_PASSWORD",
    "GRAFANA_ADMIN_PASSWORD",
    "GRAFANA_CLICKHOUSE_PASSWORD",
)
CERT_KEYS = (
    "EVENT_HUNTER_TLS_CERT_PATH",
    "EVENT_HUNTER_TLS_KEY_PATH",
    "GRAFANA_TLS_CERT_PATH",
    "GRAFANA_TLS_KEY_PATH",
)
EDGE_SERVICES = {"event-hunter-edge", "grafana-edge"}


def parse_env(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for line_number, raw in enumerate(path.read_text().splitlines(), 1):
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        if "=" not in line:
            raise ValueError(f"{path}:{line_number}: expected KEY=VALUE")
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip()
    return values


def validate_env(values: dict[str, str]) -> list[str]:
    errors: list[str] = []
    secret_values: list[str] = []
    for key in SECRET_KEYS:
        value = values.get(key, "")
        if len(value) < 24:
            errors.append(f"{key} must contain at least 24 characters")
        if "CHANGE_ME" in value or "local_only" in value or value == "admin":
            errors.append(f"{key} still uses a placeholder or local default")
        if value:
            secret_values.append(value)
    if len(secret_values) != len(set(secret_values)):
        errors.append("all hardening secrets must use distinct values")

    for key in ("EVENT_HUNTER_PUBLIC_URL", "GRAFANA_PUBLIC_URL", "VITE_GRAFANA_URL"):
        parsed = urlparse(values.get(key, ""))
        if parsed.scheme != "https" or not parsed.hostname or parsed.username or parsed.password:
            errors.append(f"{key} must be a credential-free https URL")

    for key in CERT_KEYS:
        raw = values.get(key, "")
        path = Path(raw)
        if not path.is_absolute() or not path.is_file():
            errors.append(f"{key} must point to an existing absolute file")
            continue
        if key.endswith("KEY_PATH"):
            mode = stat.S_IMODE(path.stat().st_mode)
            if mode & (stat.S_IRWXG | stat.S_IRWXO):
                errors.append(f"{key} must not be accessible by group or others")
    return errors


def compose_config(env_file: Path) -> dict[str, object]:
    command = [
        "docker",
        "compose",
        "--env-file",
        str(env_file),
        "-f",
        str(PROJECT_ROOT / "compose.yaml"),
        "-f",
        str(PROJECT_ROOT / "compose.hardening.yaml"),
        "config",
        "--format",
        "json",
    ]
    result = subprocess.run(command, cwd=PROJECT_ROOT, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(result.stderr.strip() or "docker compose config failed")
    return json.loads(result.stdout)


def validate_compose(config: dict[str, object]) -> list[str]:
    errors: list[str] = []
    services = config.get("services", {})
    if not isinstance(services, dict):
        return ["compose services are missing"]
    for name, raw in services.items():
        if not isinstance(raw, dict):
            continue
        ports = raw.get("ports", [])
        if name not in EDGE_SERVICES and ports:
            errors.append(f"{name} unexpectedly publishes host ports")
        if name in EDGE_SERVICES and len(ports) != 1:
            errors.append(f"{name} must publish exactly one TLS port")
    missing_edges = EDGE_SERVICES.difference(services)
    if missing_edges:
        errors.append(f"missing TLS edge services: {sorted(missing_edges)}")
    return errors


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--env-file", required=True, type=Path)
    args = parser.parse_args()
    env_file = args.env_file.resolve()
    if not env_file.is_file():
        print(f"ERROR: env file not found: {env_file}", file=sys.stderr)
        return 1
    try:
        values = parse_env(env_file)
        errors = validate_env(values)
        if not errors:
            errors.extend(validate_compose(compose_config(env_file)))
    except (OSError, ValueError, RuntimeError, json.JSONDecodeError) as error:
        errors = [str(error)]
    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1
    print(
        "OK hardening_profile=tls-edge-only secrets=distinct non_edge_ports=0 "
        "deployment_class=single-host-internal-pilot"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
