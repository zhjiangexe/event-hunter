#!/usr/bin/env python3
"""Load the deterministic dataset declared by performance-profile.yaml."""

import base64
import datetime as dt
import hashlib
import json
import os
import uuid
import urllib.parse
import urllib.request
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parent.parent
PROFILE_NAME = os.getenv("PERFORMANCE_PROFILE", "ci")
PROFILE_PATH = ROOT / "contracts" / "platform" / "performance-profile.yaml"
BASE_URL = os.getenv("CLICKHOUSE_URL", "http://localhost:28317").rstrip("/")
USER = os.getenv("CLICKHOUSE_USER", "event_hunter")
PASSWORD = os.getenv("CLICKHOUSE_PASSWORD", "event_hunter_local_only")
DATABASE = os.getenv("CLICKHOUSE_DB", "event_hunter")
BATCH_SIZE = int(os.getenv("PERFORMANCE_INSERT_BATCH_SIZE", "5000"))
CORRELATION_PREFIX = f"PERF-{PROFILE_NAME.upper()}-"
BASE_TIME = dt.datetime(2026, 8, 21, 0, 0, tzinfo=dt.timezone.utc)
NAMESPACE = uuid.UUID("93d476b1-52c7-4b45-b4d0-39b0c13ee7b6")


with PROFILE_PATH.open(encoding="utf-8") as stream:
    profiles = yaml.safe_load(stream)["profiles"]
if PROFILE_NAME not in profiles:
    raise SystemExit(f"unknown performance profile: {PROFILE_NAME}")
PROFILE = profiles[PROFILE_NAME]
DATASET = PROFILE["dataset"]


def clickhouse(statement: str) -> bytes:
    endpoint = f"{BASE_URL}/?database={urllib.parse.quote(DATABASE)}"
    request = urllib.request.Request(
        endpoint,
        data=statement.encode(),
        headers={
            "Content-Type": "text/plain",
            "Authorization": "Basic " + base64.b64encode(f"{USER}:{PASSWORD}".encode()).decode(),
        },
        method="POST",
    )
    with urllib.request.urlopen(request, timeout=180) as response:
        return response.read()


def insert_batch(table: str, rows: list[dict]) -> None:
    body = "\n".join(json.dumps(row, separators=(",", ":")) for row in rows)
    clickhouse(f"INSERT INTO {table} FORMAT JSONEachRow\n{body}\n")


def insert_batches(table: str, rows, expected_count: int) -> None:
    batch: list[dict] = []
    inserted = 0
    for row in rows:
        batch.append(row)
        if len(batch) >= BATCH_SIZE:
            insert_batch(table, batch)
            inserted += len(batch)
            batch.clear()
            print(f"{table}: inserted {inserted}/{expected_count}", flush=True)
    if batch:
        insert_batch(table, batch)
        inserted += len(batch)
    if inserted != expected_count:
        raise RuntimeError(f"{table}: inserted {inserted}, expected {expected_count}")
    print(f"{table}: inserted {inserted}/{expected_count}", flush=True)


def correlation_id(index: int) -> str:
    return f"{CORRELATION_PREFIX}{index:06d}"


def event_uuid(correlation_index: int, sequence: int) -> str:
    return str(uuid.uuid5(NAMESPACE, f"{PROFILE_NAME}:event:{correlation_index}:{sequence}"))


EVENT_TYPES = (
    ("OrderCreated", "order-service", "order.events"),
    ("PaymentCompleted", "payment-service", "payment.events"),
    ("ShipmentCreated", "shipping-service", "shipping.events"),
    ("PaymentRefunded", "payment-service", "payment.events"),
    ("PaymentVoided", "payment-service", "payment.events"),
)


def event_rows():
    correlation_count = DATASET["correlation_ids"]
    for index in range(DATASET["valid_events"]):
        correlation_index = index % correlation_count
        sequence = index // correlation_count + 1
        event_type, producer, topic = EVENT_TYPES[(sequence - 1) % len(EVENT_TYPES)]
        occurred_at = BASE_TIME + dt.timedelta(minutes=correlation_index % 1440, seconds=sequence)
        event_id = event_uuid(correlation_index, sequence)
        previous_id = event_uuid(correlation_index, sequence - 1) if sequence > 1 else None
        trace_id = hashlib.sha256(f"{PROFILE_NAME}:trace:{correlation_index}".encode()).hexdigest()[:32]
        current_correlation = correlation_id(correlation_index)
        yield {
            "event_id": event_id,
            "event_type": event_type,
            "event_version": 1,
            "occurred_at": occurred_at.isoformat().replace("+00:00", "Z"),
            "producer": producer,
            "correlation_id": current_correlation,
            "causation_id": previous_id,
            "trace_id": trace_id,
            "aggregate_type": "PerformanceOrder",
            "aggregate_id": current_correlation,
            "sequence": sequence,
            "kafka_topic": topic,
            "kafka_partition": correlation_index % 12,
            "kafka_offset": index + 1,
            "service_version": "performance-fixture-v1",
            "payload": json.dumps({"orderId": current_correlation, "fixture": PROFILE_NAME}, separators=(",", ":")),
            "ingested_at": (occurred_at + dt.timedelta(milliseconds=100)).isoformat().replace("+00:00", "Z"),
        }


def attempt_rows():
    correlation_count = DATASET["correlation_ids"]
    for index in range(DATASET["processing_attempts"]):
        event_index = index % DATASET["valid_events"]
        correlation_index = event_index % correlation_count
        sequence = event_index // correlation_count + 1
        event_type, _, topic = EVENT_TYPES[(sequence - 1) % len(EVENT_TYPES)]
        started_at = BASE_TIME + dt.timedelta(minutes=correlation_index % 1440, seconds=sequence, milliseconds=150)
        yield {
            "attempt_id": str(uuid.uuid5(NAMESPACE, f"{PROFILE_NAME}:attempt:{index}")),
            "event_id": event_uuid(correlation_index, sequence),
            "event_type": event_type,
            "correlation_id": correlation_id(correlation_index),
            "trace_id": hashlib.sha256(f"{PROFILE_NAME}:trace:{correlation_index}".encode()).hexdigest()[:32],
            "consumer_group_id": "performance-consumer-v1",
            "consumer_service": "performance-consumer",
            "attempt": 1,
            "processing_status": "SUCCEEDED",
            "retry_reason": None,
            "retry_topic": None,
            "kafka_topic": topic,
            "kafka_partition": correlation_index % 12,
            "kafka_offset": event_index + 1,
            "started_at": started_at.isoformat().replace("+00:00", "Z"),
            "completed_at": (started_at + dt.timedelta(milliseconds=20)).isoformat().replace("+00:00", "Z"),
            "observed_at": (started_at + dt.timedelta(milliseconds=25)).isoformat().replace("+00:00", "Z"),
        }


def quality_rows():
    for index in range(DATASET["quality_windows"]):
        window_start = BASE_TIME + dt.timedelta(minutes=index)
        yield {
            "window_start": window_start.isoformat().replace("+00:00", "Z"),
            "window_end": (window_start + dt.timedelta(minutes=1)).isoformat().replace("+00:00", "Z"),
            "calculated_at": (window_start + dt.timedelta(minutes=2)).isoformat().replace("+00:00", "Z"),
            "topic_name": "performance.events",
            "kafka_partition": index % 12,
            "consumer_group_id": "performance-consumer-v1",
            "event_count": 100,
            "duplicate_count": 0,
            "schema_violation_count": 0,
            "out_of_order_count": 0,
            "dlq_count": 0,
            "max_event_delay_ms": 100,
            "consumer_lag_messages": 0,
            "max_processing_latency_ms": 20,
            "source": f"performance-profile-{PROFILE_NAME}",
        }


def count(statement: str) -> int:
    return int(clickhouse(statement).decode().strip())


escaped_prefix = CORRELATION_PREFIX.replace("'", "''")
escaped_source = f"performance-profile-{PROFILE_NAME}".replace("'", "''")
clickhouse(f"ALTER TABLE forensics_events DELETE WHERE startsWith(correlation_id, '{escaped_prefix}') SETTINGS mutations_sync = 2")
clickhouse(f"ALTER TABLE event_processing_attempts DELETE WHERE startsWith(correlation_id, '{escaped_prefix}') SETTINGS mutations_sync = 2")
clickhouse(f"ALTER TABLE event_quality_metrics DELETE WHERE source = '{escaped_source}' SETTINGS mutations_sync = 2")

insert_batches("forensics_events", event_rows(), DATASET["valid_events"])
insert_batches("event_processing_attempts", attempt_rows(), DATASET["processing_attempts"])
insert_batches("event_quality_metrics", quality_rows(), DATASET["quality_windows"])

actual = {
    "valid_events": count(f"SELECT count() FROM forensics_events WHERE startsWith(correlation_id, '{escaped_prefix}')"),
    "correlation_ids": count(f"SELECT uniqExact(correlation_id) FROM forensics_events WHERE startsWith(correlation_id, '{escaped_prefix}')"),
    "processing_attempts": count(f"SELECT count() FROM event_processing_attempts WHERE startsWith(correlation_id, '{escaped_prefix}')"),
    "quality_windows": count(f"SELECT count() FROM event_quality_metrics WHERE source = '{escaped_source}'"),
}
expected = {key: DATASET[key] for key in actual}
if actual != expected:
    raise RuntimeError(f"performance fixture count mismatch: actual={actual}, expected={expected}")

report = {
    "profile": PROFILE_NAME,
    "random_seed": DATASET["random_seed"],
    "expected": expected,
    "actual": actual,
    "target_correlation_id": correlation_id(0),
    "from": BASE_TIME.isoformat().replace("+00:00", "Z"),
    "to": (BASE_TIME + dt.timedelta(days=1)).isoformat().replace("+00:00", "Z"),
    "pass": True,
}
output = ROOT / "build" / "reports" / "performance-fixture-summary.json"
output.parent.mkdir(parents=True, exist_ok=True)
output.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
print(json.dumps(report, indent=2))
