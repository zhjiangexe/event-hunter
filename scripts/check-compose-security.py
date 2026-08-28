#!/usr/bin/env python3
from __future__ import annotations

import json
from pathlib import Path

import yaml


PROJECT_ROOT = Path(__file__).resolve().parents[1]
COMPOSE_PATH = PROJECT_ROOT / "compose.yaml"
ALLOWED_EXPLICIT_ROOT = {"tempo"}


def main() -> int:
    compose = yaml.safe_load(COMPOSE_PATH.read_text())
    errors: list[str] = []
    warnings: list[str] = []

    for name, service in compose.get("services", {}).items():
        if service.get("privileged") is True:
            errors.append(f"{name}: privileged=true")
        if service.get("network_mode") == "host":
            errors.append(f"{name}: network_mode=host")
        if service.get("pid") == "host":
            errors.append(f"{name}: pid=host")
        if service.get("cap_add"):
            errors.append(f"{name}: cap_add={service['cap_add']}")

        for volume in service.get("volumes", []):
            source = volume.get("source", "") if isinstance(volume, dict) else str(volume).split(":", 1)[0]
            if source.endswith("/var/run/docker.sock") or source == "/var/run/docker.sock":
                errors.append(f"{name}: mounts Docker socket")

        image = service.get("image")
        if image and (":" not in image.rsplit("/", 1)[-1] or image.endswith(":latest")):
            errors.append(f"{name}: image is not pinned ({image})")

        user = str(service.get("user", ""))
        if user in {"0", "0:0", "root"}:
            finding = f"{name}: explicitly runs as root ({user})"
            if name in ALLOWED_EXPLICIT_ROOT:
                warnings.append(finding + " for local Tempo volume compatibility")
            else:
                errors.append(finding)

    result = {
        "status": "passed" if not errors else "failed",
        "errors": errors,
        "accepted_local_warnings": warnings,
    }
    print(json.dumps(result, indent=2))
    return 0 if not errors else 1


if __name__ == "__main__":
    raise SystemExit(main())
