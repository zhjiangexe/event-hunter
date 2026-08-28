#!/usr/bin/env python3
"""Load synthetic, replayable E2E traces and logs through the OTLP gateway.

These records are deliberately isolated from live SDK streams with a synthetic
service namespace and fixture service instance. They are demo evidence, not
proof that the instrumented domain services emitted telemetry.
"""

from __future__ import annotations

import datetime as dt
import hashlib
import json
import os
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
OTEL_URL = os.getenv("OTEL_HTTP_URL", "http://localhost:28330").rstrip("/")
LOKI_URL = os.getenv("LOKI_URL", "http://localhost:28327").rstrip("/")
TEMPO_URL = os.getenv("TEMPO_URL", "http://localhost:28328").rstrip("/")
FLUSH_WAIT_SECONDS = int(os.getenv("OBSERVABILITY_FIXTURE_FLUSH_WAIT_SECONDS", "45"))
FIXTURES = (
    "normal-order-flow.json",
    "payment-without-shipment.json",
    "exclusion-cases.json",
    "extended-order-scenarios.json",
)


def fixture_events() -> list[dict]:
    result: list[dict] = []
    for name in FIXTURES:
        payload = json.loads((ROOT / "contracts" / "fixtures" / name).read_text(encoding="utf-8"))
        groups = [payload] if payload.get("events") else payload.get("cases", [])
        for group in groups:
            result.extend(group["events"])
    return result


def nanos(value: str) -> int:
    return int(dt.datetime.fromisoformat(value.replace("Z", "+00:00")).timestamp() * 1_000_000_000)


def otlp_id(hex_value: str) -> str:
    # OTLP/HTTP JSON represents trace_id and span_id as fixed-width hex IDs.
    return hex_value


def span_id(event_id: str) -> str:
    return hashlib.sha256(event_id.encode()).hexdigest()[:16]


def attributes(event: dict) -> list[dict]:
    return [
        {"key": "event.id", "value": {"stringValue": event["eventId"]}},
        {"key": "event.type", "value": {"stringValue": event["eventType"]}},
        {"key": "correlation.id", "value": {"stringValue": event["correlationId"]}},
        {"key": "fixture.source", "value": {"stringValue": "event-hunter-domain-v1"}},
    ]


def resource(service_name: str, correlation_id: str) -> dict:
    return {
        "attributes": [
            {"key": "service.name", "value": {"stringValue": service_name}},
            {"key": "service.namespace", "value": {"stringValue": "event-hunter.synthetic"}},
            {
                "key": "service.instance.id",
                "value": {"stringValue": f"fixture-loader-v1-{correlation_id}"},
            },
            {"key": "deployment.environment.name", "value": {"stringValue": "local-demo"}},
            {"key": "telemetry.source", "value": {"stringValue": "synthetic-fixture"}},
        ]
    }


def trace_payload(events: list[dict]) -> dict:
    resources: list[dict] = []
    for event in events:
        started = nanos(event["occurredAt"])
        resources.append(
            {
                "resource": resource(event["producer"], event["correlationId"]),
                "scopeSpans": [
                    {
                        "scope": {"name": "event-hunter.fixture-loader", "version": "1"},
                        "spans": [
                            {
                                "traceId": otlp_id(event["traceId"]),
                                "spanId": otlp_id(span_id(event["eventId"])),
                                "name": f'Handle {event["eventType"]}',
                                "kind": 2,
                                "startTimeUnixNano": str(started),
                                "endTimeUnixNano": str(started + 50_000_000),
                                "attributes": attributes(event),
                                "status": {"code": 1},
                            }
                        ],
                    }
                ],
            }
        )
    return {"resourceSpans": resources}


def log_payload(events: list[dict]) -> dict:
    resources: list[dict] = []
    for event in events:
        timestamp = nanos(event["occurredAt"])
        message = (
            f'processed event_type={event["eventType"]} event_id={event["eventId"]} '
            f'correlation_id={event["correlationId"]}'
        )
        resources.append(
            {
                "resource": resource(event["producer"], event["correlationId"]),
                "scopeLogs": [
                    {
                        "scope": {"name": "event-hunter.fixture-loader", "version": "1"},
                        "logRecords": [
                            {
                                "timeUnixNano": str(timestamp),
                                "observedTimeUnixNano": str(timestamp + 50_000_000),
                                "severityNumber": 9,
                                "severityText": "INFO",
                                "body": {"stringValue": message},
                                "attributes": attributes(event),
                                "traceId": otlp_id(event["traceId"]),
                                "spanId": otlp_id(span_id(event["eventId"])),
                            }
                        ],
                    }
                ],
            }
        )
    return {"resourceLogs": resources}


def post_json(url: str, payload: dict) -> None:
    request = urllib.request.Request(
        url,
        data=json.dumps(payload, separators=(",", ":")).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=15) as response:
            if response.status >= 300:
                raise RuntimeError(f"POST {url} returned {response.status}")
    except urllib.error.HTTPError as error:
        detail = error.read().decode(errors="replace")
        raise RuntimeError(f"POST {url} returned {error.code}: {detail}") from error


def get_status(url: str) -> int:
    try:
        with urllib.request.urlopen(url, timeout=5) as response:
            return response.status
    except urllib.error.HTTPError as error:
        return error.code


def loki_event_ids(events: list[dict]) -> set[str]:
    timestamps = [nanos(event["occurredAt"]) for event in events]
    query = urllib.parse.urlencode(
        {
            "query": (
                '{service_namespace="event-hunter.synthetic",'
                'service_instance_id=~"fixture-loader-v1-.+"}'
            ),
            "start": str(min(timestamps) - 60_000_000_000),
            "end": str(max(timestamps) + 60_000_000_000),
            "limit": str(max(100, len(events) * 2)),
        }
    )
    with urllib.request.urlopen(f"{LOKI_URL}/loki/api/v1/query_range?{query}", timeout=10) as response:
        payload = json.load(response)
    return {
        stream["stream"]["event_id"]
        for stream in payload.get("data", {}).get("result", [])
        if stream.get("stream", {}).get("event_id")
    }


def loki_has_event(event: dict) -> bool:
    timestamp = nanos(event["occurredAt"])
    query = urllib.parse.urlencode(
        {
            "query": (
                f'{{service_name="{event["producer"]}",service_namespace="event-hunter.synthetic"}} '
                f'| correlation_id="{event["correlationId"]}" | event_id="{event["eventId"]}"'
            ),
            "start": str(timestamp - 60_000_000_000),
            "end": str(timestamp + 60_000_000_000),
            "limit": "10",
        }
    )
    with urllib.request.urlopen(f"{LOKI_URL}/loki/api/v1/query_range?{query}", timeout=5) as response:
        payload = json.load(response)
    return bool(payload.get("data", {}).get("result"))


events = fixture_events()
post_json(f"{OTEL_URL}/v1/traces", trace_payload(events))
post_json(f"{OTEL_URL}/v1/logs", log_payload(events))

expected_event_ids = {event["eventId"] for event in events}
events_by_trace: dict[str, list[dict]] = {}
for event in events:
    events_by_trace.setdefault(event["traceId"], []).append(event)

# Historical fixture timestamps are queried from Loki's store, not its live
# ingester. Wait for the Collector batch and Loki's periodic idle-flush scan
# before querying so a just-accepted historical batch is not reported absent.
time.sleep(FLUSH_WAIT_SECONDS)
observed_event_ids = loki_event_ids(events)

for attempt in range(3):
    missing_log_events = [event for event in events if event["eventId"] not in observed_event_ids]
    missing_trace_events = [
        trace_events
        for trace_id, trace_events in events_by_trace.items()
        if get_status(f"{TEMPO_URL}/api/traces/{trace_id}") != 200
    ]
    if not missing_log_events and not missing_trace_events:
        print(f"loaded observability fixtures traces={len(events)} logs={len(events)} verified=all")
        break
    if attempt == 2:
        missing_logs = [event["eventId"] for event in missing_log_events]
        missing_traces = [trace_events[0]["traceId"] for trace_events in missing_trace_events]
        raise RuntimeError(
            f"observability fixtures incomplete: missing_logs={missing_logs} missing_traces={missing_traces}"
        )

    # Retry only data that is still absent. Existing event IDs and trace IDs
    # are never replayed, preventing fixture retries from creating duplicates.
    if missing_log_events:
        post_json(f"{OTEL_URL}/v1/logs", log_payload(missing_log_events))
    for trace_events in missing_trace_events:
        post_json(f"{OTEL_URL}/v1/traces", trace_payload(trace_events))
    time.sleep(FLUSH_WAIT_SECONDS)
    observed_event_ids.update(
        event["eventId"] for event in missing_log_events if loki_has_event(event)
    )
else:
    raise RuntimeError("observability fixture verification loop ended unexpectedly")
