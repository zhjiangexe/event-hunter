#!/usr/bin/env python3
"""Run the authenticated deterministic query mix from performance-profile.yaml."""

import base64
import json
import math
import os
import platform
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

import yaml


ROOT = Path(__file__).resolve().parent.parent
PROFILE_NAME = os.getenv("PERFORMANCE_PROFILE", "ci")
PROFILE_PATH = ROOT / "contracts" / "platform" / "performance-profile.yaml"
FIXTURE_REPORT_PATH = ROOT / "build" / "reports" / "performance-fixture-summary.json"
with PROFILE_PATH.open(encoding="utf-8") as stream:
    contract = yaml.safe_load(stream)
if PROFILE_NAME not in contract["profiles"]:
    raise SystemExit(f"unknown performance profile: {PROFILE_NAME}")
PROFILE = contract["profiles"][PROFILE_NAME]

if not FIXTURE_REPORT_PATH.exists():
    raise SystemExit("performance fixture report is missing; run scripts/load-performance-fixture.py first")
fixture_report = json.loads(FIXTURE_REPORT_PATH.read_text(encoding="utf-8"))
expected_dataset = {
    key: PROFILE["dataset"][key]
    for key in ("valid_events", "correlation_ids", "processing_attempts", "quality_windows")
}
if fixture_report.get("profile") != PROFILE_NAME or fixture_report.get("actual") != expected_dataset or not fixture_report.get("pass"):
    raise SystemExit(f"performance fixture does not match profile {PROFILE_NAME}: {fixture_report}")

BASE_URL = os.getenv("EVENT_HUNTER_API_URL", "http://localhost:28333").rstrip("/")
CORRELATION_ID = os.getenv("PERFORMANCE_CORRELATION_ID", fixture_report["target_correlation_id"])
FROM_VALUE = os.getenv("PERFORMANCE_FROM", fixture_report["from"])
TO_VALUE = os.getenv("PERFORMANCE_TO", fixture_report["to"])
COOKIE = ""


def api_request(method: str, path: str, body: dict | None = None, timeout: float = 10) -> tuple[int, bytes, object]:
    data = json.dumps(body, separators=(",", ":")).encode() if body is not None else None
    headers = {"Accept": "application/json"}
    if body is not None:
        headers["Content-Type"] = "application/json"
    if COOKIE:
        headers["Cookie"] = COOKIE
    request = urllib.request.Request(BASE_URL + path, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            return response.status, response.read(), response.headers
    except urllib.error.HTTPError as error:
        return error.code, error.read(), error.headers


status, login_body, login_headers = api_request("POST", "/api/v1/auth/demo-session", {"role": "INVESTIGATOR"})
if status != 200:
    raise SystemExit(f"performance login failed: HTTP {status} {login_body.decode(errors='replace')}")
set_cookie = login_headers.get("Set-Cookie", "")
COOKIE = set_cookie.split(";", 1)[0]
if not COOKIE:
    raise SystemExit("performance login returned no session cookie")


def query_string(values: dict) -> str:
    return urllib.parse.urlencode(values)


list_path = "/api/v1/investigations?" + query_string({"correlation_id": CORRELATION_ID, "page_size": 1})
status, case_body, _ = api_request("GET", list_path)
if status != 200:
    raise SystemExit(f"performance case lookup failed: HTTP {status} {case_body.decode(errors='replace')}")
cases = json.loads(case_body).get("items", [])
if cases:
    investigation_id = cases[0]["id"]
else:
    status, case_body, _ = api_request(
        "POST",
        "/api/v1/investigations",
        {"title": f"Performance profile {PROFILE_NAME}", "severity": "HIGH", "correlation_id": CORRELATION_ID},
    )
    if status != 201:
        raise SystemExit(f"performance case creation failed: HTTP {status} {case_body.decode(errors='replace')}")
    investigation_id = json.loads(case_body)["id"]

encoded_correlation = urllib.parse.quote(CORRELATION_ID, safe="")
window_query = query_string({"from": FROM_VALUE, "to": TO_VALUE})
operation_paths = {
    "getBusinessTimeline": f"/api/v1/timelines/{encoded_correlation}?{window_query}&limit=1000",
    "searchForensicsEvents": "/api/v1/events/search?"
    + query_string({"from": FROM_VALUE, "to": TO_VALUE, "correlation_id": CORRELATION_ID, "limit": 100}),
    "getInvestigationSummary": f"/api/v1/investigations/{investigation_id}/summary?{window_query}&limit=1000",
    "listInvestigations": "/api/v1/investigations?"
    + query_string({"correlation_id": CORRELATION_ID, "page_size": 50}),
}

weighted_operations: list[str] = []
for item in contract["query_mix"]:
    if item["operation_id"] not in operation_paths:
        raise SystemExit(f"performance query mix has no implemented path for {item['operation_id']}")
    weighted_operations.extend([item["operation_id"]] * item["weight"])


def request_once(operation: str) -> dict:
    started = time.perf_counter()
    response_status, _, _ = api_request("GET", operation_paths[operation])
    return {
        "operation": operation,
        "latency_ms": (time.perf_counter() - started) * 1000,
        "status": response_status,
        "ok": response_status < 400,
    }


for index in range(PROFILE["warmup_requests"]):
    request_once(weighted_operations[index % len(weighted_operations)])

measured_operations = [
    weighted_operations[index % len(weighted_operations)]
    for index in range(PROFILE["measured_requests"])
]
with ThreadPoolExecutor(max_workers=PROFILE["clients"]["concurrent_investigators"]) as pool:
    results = list(pool.map(request_once, measured_operations))


def percentile(values: list[float], percentile_value: float) -> float | None:
    if not values:
        return None
    ordered = sorted(values)
    return ordered[max(0, math.ceil(len(ordered) * percentile_value) - 1)]


grouped: dict[str, list[dict]] = defaultdict(list)
for result in results:
    grouped[result["operation"]].append(result)
operations = {}
for name, values in grouped.items():
    statuses = sorted({value["status"] for value in values})
    operations[name] = {
        "requests": len(values),
        "errors": sum(1 for value in values if not value["ok"]),
        "p95_ms": percentile([value["latency_ms"] for value in values], 0.95),
        "status_counts": {
            str(response_status): sum(1 for value in values if value["status"] == response_status)
            for response_status in statuses
        },
    }


def command_output(arguments: list[str]) -> str:
    try:
        return subprocess.run(
            arguments, cwd=ROOT, check=True, capture_output=True, text=True, timeout=15
        ).stdout.strip()
    except (OSError, subprocess.SubprocessError):
        return "unavailable"


def memory_bytes() -> int | None:
    try:
        return os.sysconf("SC_PAGE_SIZE") * os.sysconf("SC_PHYS_PAGES")
    except (ValueError, OSError, AttributeError):
        return None


def clickhouse_version() -> str:
    clickhouse_url = os.getenv("CLICKHOUSE_URL", "http://localhost:28317").rstrip("/")
    user = os.getenv("CLICKHOUSE_USER", "event_hunter")
    password = os.getenv("CLICKHOUSE_PASSWORD", "event_hunter_local_only")
    request = urllib.request.Request(clickhouse_url, data=b"SELECT version()", method="POST")
    request.add_header(
        "Authorization", "Basic " + base64.b64encode(f"{user}:{password}".encode()).decode()
    )
    try:
        with urllib.request.urlopen(request, timeout=10) as response:
            return response.read().decode().strip()
    except (OSError, urllib.error.URLError):
        return "unavailable"


errors = sum(1 for result in results if not result["ok"])
error_rate = errors / len(results) if results else 1
timeline_p95 = operations.get("getBusinessTimeline", {}).get("p95_ms")
summary_p95 = operations.get("getInvestigationSummary", {}).get("p95_ms")
thresholds = PROFILE["thresholds"]
passed = bool(results) and error_rate <= thresholds["http_error_rate_max"]
passed = passed and timeline_p95 is not None and timeline_p95 <= thresholds["timeline_p95_ms"]
passed = passed and summary_p95 is not None and summary_p95 <= thresholds["investigation_summary_p95_ms"]

report = {
    "profile": PROFILE_NAME,
    "query_mix": operations,
    "measured_requests": len(results),
    "http_errors": errors,
    "http_error_rate": error_rate,
    "timeline_p95_ms": timeline_p95,
    "investigation_summary_p95_ms": summary_p95,
    "thresholds": thresholds,
    "dataset": fixture_report["actual"],
    "environment": {
        "git_revision": command_output(["git", "rev-parse", "HEAD"]),
        "cpu_model": platform.processor() or platform.machine(),
        "cpu_count": os.cpu_count(),
        "memory_bytes": memory_bytes(),
        "docker_version": command_output(["docker", "--version"]),
        "clickhouse_version": clickhouse_version(),
        "postgres_version": command_output(
            [
                "docker", "compose", "exec", "-T", "postgres", "psql",
                "-U", os.getenv("POSTGRES_USER", "event_hunter"),
                "-d", os.getenv("POSTGRES_DB", "event_hunter"),
                "-tAc", "SHOW server_version",
            ]
        ),
    },
    "pass": passed,
}
output = ROOT / "build" / "reports" / "performance-summary.json"
output.parent.mkdir(parents=True, exist_ok=True)
output.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
print(json.dumps(report, indent=2))
if not passed:
    raise SystemExit(1)
