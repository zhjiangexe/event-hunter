#!/usr/bin/env python3
"""Load quality-window.json through the same ClickHouse table contracts used by the worker."""
import json
import os
import urllib.request
import base64

from fixture_mapping import (
    DOMAIN_EVENT_PIPELINE,
    PROCESSING_ATTEMPT_PIPELINE,
    map_pipeline_row,
    target_table,
)


ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
FIXTURE = os.path.join(ROOT, "contracts", "fixtures", "quality-window.json")
BASE_URL = os.getenv("CLICKHOUSE_URL", "http://localhost:28317").rstrip("/")
USER = os.getenv("CLICKHOUSE_USER", "event_hunter")
PASSWORD = os.getenv("CLICKHOUSE_PASSWORD", "event_hunter_local_only")
DATABASE = os.getenv("CLICKHOUSE_DB", "event_hunter")
DOMAIN_EVENT_TABLE = target_table(DOMAIN_EVENT_PIPELINE)
PROCESSING_ATTEMPT_TABLE = target_table(PROCESSING_ATTEMPT_PIPELINE)


def insert(table, rows):
    if not rows:
        return
    body = "\n".join(json.dumps(row, separators=(",", ":")) for row in rows) + "\n"
    request = urllib.request.Request(
        f"{BASE_URL}/?database={DATABASE}&query=INSERT%20INTO%20{table}%20FORMAT%20JSONEachRow",
        data=body.encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    request.add_header("Authorization", "Basic " + base64.b64encode(f"{USER}:{PASSWORD}".encode()).decode())
    with urllib.request.urlopen(request) as response:
        if response.status >= 300:
            raise RuntimeError(response.read().decode())


def execute(statement):
    request = urllib.request.Request(
        f"{BASE_URL}/?database={DATABASE}",
        data=statement.encode(),
        headers={
            "Content-Type": "text/plain",
            "Authorization": "Basic " + base64.b64encode(f"{USER}:{PASSWORD}".encode()).decode(),
        },
        method="POST",
    )
    with urllib.request.urlopen(request) as response:
        if response.status >= 300:
            raise RuntimeError(response.read().decode())


with open(FIXTURE, encoding="utf-8") as stream:
    fixture = json.load(stream)

# The contract reserves synthetic partition 99 for this deterministic quality
# fixture. It still exercises the production topic name without colliding with
# demo events that legitimately share the fixed event-time window.
if fixture["expectedMetric"]["kafkaPartition"] != 99:
    raise RuntimeError("quality fixture must use reserved synthetic partition 99")

# Keep repeated local E2E runs deterministic instead of accumulating aggregate versions.
for table in ("event_quality_metrics", "redpanda_consumer_group_metrics", "event_ingestion_failures"):
    execute(f"TRUNCATE TABLE {table}")
execute(f"ALTER TABLE {DOMAIN_EVENT_TABLE} DELETE WHERE correlation_id = 'ORDER-Q-1' SETTINGS mutations_sync = 2")
execute(
    f"ALTER TABLE {PROCESSING_ATTEMPT_TABLE} DELETE WHERE correlation_id = 'ORDER-Q-1' SETTINGS mutations_sync = 2"
)

metadata = {item["eventIndex"]: item for item in fixture["kafkaMetadata"]}
events = []
for index, event in enumerate(fixture["events"]):
    source = metadata[index]
    events.append(
        map_pipeline_row(
            DOMAIN_EVENT_PIPELINE,
            event,
            metadata={
                "kafka_topic": source["topic"],
                "kafka_partition": source["partition"],
                "kafka_offset": source["offset"],
            },
            headers={"x-service-version": "fixture-loader"},
            functions={"utc_now": source["ingestedAt"]},
        )
    )
insert(DOMAIN_EVENT_TABLE, events)
insert(
    PROCESSING_ATTEMPT_TABLE,
    [map_pipeline_row(PROCESSING_ATTEMPT_PIPELINE, attempt) for attempt in fixture["attempts"]],
)
insert("redpanda_consumer_group_metrics", [
    {"sampled_at": sample["sampledAt"], "topic_name": sample["topic"], "kafka_partition": sample["partition"],
     "consumer_group_id": sample["consumerGroupId"], "lag_messages": sample["lagMessages"]}
    for sample in fixture["consumerLagSamples"]
])
insert("event_ingestion_failures", [
    {"source_topic": failure["sourceTopic"], "source_partition": failure["sourcePartition"],
     "source_offset": failure["sourceOffset"], "event_id": None, "event_type": None, "correlation_id": None,
     "error_type": failure["errorType"], "error_code": failure["errorCode"], "error_summary": "fixture quality violation",
     "payload_sha256": failure["payloadSha256"], "failed_at": failure["failedAt"]}
    for failure in fixture["ingestionFailures"]
])
print(f"loaded quality fixture events={len(events)} attempts={len(fixture['attempts'])}")
