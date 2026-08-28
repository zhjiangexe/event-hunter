#!/usr/bin/env python3
"""Map canonical fixture documents with the production ingestion contract.

Fixture loaders intentionally write directly to ClickHouse so deterministic
historical examples do not enter live Kafka topics. The row shape must still
come from contracts/platform/ingestion-mapping.yaml; this module is the only
adapter between canonical fixture JSON and ClickHouse JSONEachRow documents.
"""

from __future__ import annotations

import json
import re
from pathlib import Path
from typing import Any, Mapping

import yaml


PROJECT_ROOT = Path(__file__).resolve().parents[1]
MAPPING_PATH = PROJECT_ROOT / "contracts" / "platform" / "ingestion-mapping.yaml"
DOMAIN_EVENT_PIPELINE = "domain-events-to-clickhouse-v1"
PROCESSING_ATTEMPT_PIPELINE = "processing-attempts-to-clickhouse-v1"
JSON_ENCODE_EXPRESSION = re.compile(r"^json_encode\((\$\.[A-Za-z0-9_.]+)\)$")
SQL_IDENTIFIER = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")
CONTRACT = yaml.safe_load(MAPPING_PATH.read_text(encoding="utf-8"))


def _contract() -> dict[str, Any]:
    return CONTRACT


def _pipeline(pipeline_id: str) -> dict[str, Any]:
    matches = [item for item in _contract()["pipelines"] if item["id"] == pipeline_id]
    if len(matches) != 1:
        raise ValueError(f"ingestion mapping must define exactly one pipeline {pipeline_id!r}")
    return matches[0]


def target_table(pipeline_id: str) -> str:
    table = str(_pipeline(pipeline_id)["mapping"]["target_table"])
    if not SQL_IDENTIFIER.fullmatch(table):
        raise ValueError(f"pipeline {pipeline_id!r} has unsafe target table {table!r}")
    return table


def _json_path(document: Mapping[str, Any], expression: str) -> Any:
    if not expression.startswith("$."):
        raise ValueError(f"unsupported fixture JSON path {expression!r}")
    value: Any = document
    for segment in expression[2:].split("."):
        if not isinstance(value, Mapping) or segment not in value:
            raise ValueError(f"fixture document does not contain {expression!r}")
        value = value[segment]
    return value


def _map_value(
    expression: str,
    document: Mapping[str, Any],
    metadata: Mapping[str, Any],
    headers: Mapping[str, Any],
    functions: Mapping[str, Any],
) -> Any:
    if expression.startswith("$."):
        return _json_path(document, expression)
    if expression.startswith("meta:"):
        key = expression.removeprefix("meta:")
        if key not in metadata:
            raise ValueError(f"fixture metadata does not contain {key!r}")
        return metadata[key]
    if expression.startswith("header:"):
        key = expression.removeprefix("header:")
        if key not in headers:
            raise ValueError(f"fixture headers do not contain {key!r}")
        return headers[key]
    if expression.startswith("function:"):
        key = expression.removeprefix("function:")
        if key not in functions:
            raise ValueError(f"fixture function value does not contain {key!r}")
        return functions[key]
    match = JSON_ENCODE_EXPRESSION.fullmatch(expression)
    if match:
        return json.dumps(_json_path(document, match.group(1)), separators=(",", ":"))
    raise ValueError(f"unsupported fixture mapping expression {expression!r}")


def map_pipeline_row(
    pipeline_id: str,
    document: Mapping[str, Any],
    *,
    metadata: Mapping[str, Any] | None = None,
    headers: Mapping[str, Any] | None = None,
    functions: Mapping[str, Any] | None = None,
) -> dict[str, Any]:
    columns = _pipeline(pipeline_id)["mapping"]["columns"]
    return {
        column: _map_value(expression, document, metadata or {}, headers or {}, functions or {})
        for column, expression in columns.items()
    }


def validate_mapping_contract() -> int:
    """Fail when a production mapping expression has no deterministic fixture adapter."""

    supported = 0
    for pipeline in _contract()["pipelines"]:
        for expression in pipeline["mapping"]["columns"].values():
            if (
                expression.startswith(("$.", "meta:", "header:", "function:"))
                or JSON_ENCODE_EXPRESSION.fullmatch(expression)
            ):
                supported += 1
                continue
            raise ValueError(
                f"pipeline {pipeline['id']!r} uses unsupported fixture mapping expression {expression!r}"
            )

    fixture_directory = PROJECT_ROOT / "contracts" / "fixtures"
    for fixture_name in (
        "normal-order-flow.json",
        "payment-without-shipment.json",
        "exclusion-cases.json",
        "extended-order-scenarios.json",
    ):
        fixture = json.loads((fixture_directory / fixture_name).read_text(encoding="utf-8"))
        groups = [fixture] if fixture.get("events") else fixture.get("cases", [])
        for group in groups:
            for event in group["events"]:
                map_pipeline_row(
                    DOMAIN_EVENT_PIPELINE,
                    event,
                    metadata={"kafka_topic": "fixture.events", "kafka_partition": 0, "kafka_offset": 0},
                    headers={"x-service-version": "fixture-contract-check"},
                    functions={"utc_now": event["occurredAt"]},
                )

    for fixture_name in ("processing-attempts.json", "quality-window.json"):
        fixture = json.loads((fixture_directory / fixture_name).read_text(encoding="utf-8"))
        for attempt in fixture["attempts"]:
            map_pipeline_row(PROCESSING_ATTEMPT_PIPELINE, attempt)
    return supported
