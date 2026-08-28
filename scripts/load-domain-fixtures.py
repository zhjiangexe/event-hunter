#!/usr/bin/env python3
"""Load canonical domain fixtures into ClickHouse using the ingestion table shape."""
import base64
import json
import os
import urllib.request

from fixture_mapping import (
    DOMAIN_EVENT_PIPELINE,
    PROCESSING_ATTEMPT_PIPELINE,
    map_pipeline_row,
    target_table,
)

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BASE_URL = os.getenv("CLICKHOUSE_URL", "http://localhost:28317").rstrip("/")
USER = os.getenv("CLICKHOUSE_USER", "event_hunter")
PASSWORD = os.getenv("CLICKHOUSE_PASSWORD", "event_hunter_local_only")
DATABASE = os.getenv("CLICKHOUSE_DB", "event_hunter")
DOMAIN_EVENT_TABLE = target_table(DOMAIN_EVENT_PIPELINE)
PROCESSING_ATTEMPT_TABLE = target_table(PROCESSING_ATTEMPT_PIPELINE)
FIXTURES = (
    "normal-order-flow.json",
    "payment-without-shipment.json",
    "exclusion-cases.json",
    "extended-order-scenarios.json",
)


def query(statement: str) -> bytes:
    request = urllib.request.Request(
        f"{BASE_URL}/?database={DATABASE}",
        data=statement.encode(),
        headers={
            "Content-Type": "text/plain",
            "Authorization": "Basic " + base64.b64encode(f"{USER}:{PASSWORD}".encode()).decode(),
        },
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=10) as response:
        return response.read()


def insert(table: str, rows: list[dict]) -> None:
    if not rows:
        return
    body = "\n".join(json.dumps(row, separators=(",", ":")) for row in rows) + "\n"
    query(f"INSERT INTO {table} FORMAT JSONEachRow\n{body}")


def event_rows() -> list[dict]:
    events: list[dict] = []
    for fixture_name in FIXTURES:
        with open(os.path.join(ROOT, "contracts", "fixtures", fixture_name), encoding="utf-8") as stream:
            fixture = json.load(stream)
        cases = fixture.get("cases", [])
        groups = [fixture] if fixture.get("events") else cases
        for group in groups:
            for event in group["events"]:
                kafka_metadata = {
                    "kafka_topic": event["producer"].removesuffix("-service") + ".events",
                    "kafka_partition": 0,
                    "kafka_offset": 1000 + len(events),
                }
                events.append(
                    map_pipeline_row(
                        DOMAIN_EVENT_PIPELINE,
                        event,
                        metadata=kafka_metadata,
                        headers={"x-service-version": "fixture-loader"},
                        functions={"utc_now": event["occurredAt"]},
                    )
                )
    return events


def attempt_rows() -> list[dict]:
    with open(os.path.join(ROOT, "contracts", "fixtures", "processing-attempts.json"), encoding="utf-8") as stream:
        attempts = json.load(stream)["attempts"]
    return [map_pipeline_row(PROCESSING_ATTEMPT_PIPELINE, item) for item in attempts]


events = event_rows()
attempts = attempt_rows()
fixture_correlations = sorted({event["correlation_id"] for event in events})
correlations_sql = ",".join("'" + value.replace("'", "''") + "'" for value in fixture_correlations)
query(f"ALTER TABLE {DOMAIN_EVENT_TABLE} DELETE WHERE correlation_id IN ({correlations_sql})")
query(f"ALTER TABLE {PROCESSING_ATTEMPT_TABLE} DELETE WHERE correlation_id IN ({correlations_sql})")
insert(DOMAIN_EVENT_TABLE, events)
insert(PROCESSING_ATTEMPT_TABLE, attempts)
print("loaded domain fixtures", len(events), "events and", len(attempts), "processing attempts")
