#!/usr/bin/env bash
set -euo pipefail

EVENT_HUNTER_PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${EVENT_HUNTER_PROJECT_DIR}"

EVENT_HUNTER_GRAFANA_URL="${GRAFANA_URL:-http://localhost:${GRAFANA_PORT:-28332}}"
EVENT_HUNTER_GRAFANA_USER="${GRAFANA_ADMIN_USER:-admin}"
EVENT_HUNTER_GRAFANA_PASSWORD="${GRAFANA_ADMIN_PASSWORD:-admin_local_only}"

grafana_get() {
  curl --fail --silent --show-error \
    --user "${EVENT_HUNTER_GRAFANA_USER}:${EVENT_HUNTER_GRAFANA_PASSWORD}" \
    "${EVENT_HUNTER_GRAFANA_URL}$1"
}

for EVENT_HUNTER_ATTEMPT in $(seq 1 30); do
  if grafana_get /api/health >/dev/null 2>&1; then
    break
  fi
  if [[ "${EVENT_HUNTER_ATTEMPT}" == "30" ]]; then
    echo "Grafana did not become healthy at ${EVENT_HUNTER_GRAFANA_URL}." >&2
    exit 1
  fi
  sleep 2
done

for EVENT_HUNTER_DATASOURCE_UID in clickhouse prometheus loki tempo; do
  grafana_get "/api/datasources/uid/${EVENT_HUNTER_DATASOURCE_UID}" >/dev/null
done

EVENT_HUNTER_CLICKHOUSE_HEALTH="$(grafana_get /api/datasources/uid/clickhouse/health)"
python3 -c 'import json,sys; value=json.load(sys.stdin); assert value.get("status") == "OK", value' \
  <<<"${EVENT_HUNTER_CLICKHOUSE_HEALTH}"

grafana_get /api/dashboards/uid/event-quality >/dev/null

for EVENT_HUNTER_ALERT_UID in \
  event-quality-duplicate-warning \
  event-quality-duplicate-critical \
  event-quality-schema \
  event-quality-dlq \
  event-quality-delay \
  event-quality-consumer-lag \
  event-hunter-dlq-investigation \
  event-hunter-poc-admission-quarantine; do
  grafana_get "/api/v1/provisioning/alert-rules/${EVENT_HUNTER_ALERT_UID}" >/dev/null
done

EVENT_HUNTER_CONTACT_POINTS="$(grafana_get /api/v1/provisioning/contact-points)"
python3 -c 'import json,os,sys
items=json.load(sys.stdin)
point=next(item for item in items if item.get("uid") == "event-hunter-investigation-webhook")
assert point["name"] == "event-hunter-investigation"
assert point["type"] == "webhook"
settings=point["settings"]
assert settings["url"] == "http://event-hunter-api:8080/api/v1/integrations/grafana/alerts"
hmac=settings["hmacConfig"]
assert hmac["header"] == "X-Grafana-Alerting-Signature"
assert hmac["timestampHeader"] == "X-Grafana-Alerting-Timestamp"
secret=hmac.get("secret", "")
expected=os.environ.get("GRAFANA_WEBHOOK_SECRET", "grafana_webhook_local_only")
assert secret and secret != expected, "Grafana API must return a redacted HMAC secret"' <<<"${EVENT_HUNTER_CONTACT_POINTS}"

EVENT_HUNTER_POLICY="$(grafana_get /api/v1/provisioning/policies)"
python3 -c 'import json,sys
root=json.load(sys.stdin)
routes=root.get("routes", [])
route=next(route for route in routes if route.get("receiver") == "event-hunter-investigation")
matchers=route.get("object_matchers", [])
assert ["event_hunter", "=", "investigate"] in matchers
assert "correlation_id" in route.get("group_by", [])' <<<"${EVENT_HUNTER_POLICY}"

for EVENT_HUNTER_ATTEMPT in $(seq 1 30); do
  EVENT_HUNTER_RULES="$(grafana_get '/api/prometheus/grafana/api/v1/rules?type=alert')"
  if python3 -c 'import json,sys
expected={"event-quality-duplicate-warning","event-quality-duplicate-critical","event-quality-schema","event-quality-dlq","event-quality-delay","event-quality-consumer-lag","event-hunter-dlq-investigation","event-hunter-poc-admission-quarantine"}
payload=json.load(sys.stdin)
rules=[rule for group in payload["data"]["groups"] for rule in group["rules"] if rule.get("uid") in expected]
assert {rule["uid"] for rule in rules} == expected
assert all(rule.get("health") == "ok" and rule.get("provenance") == "file" for rule in rules)' \
    <<<"${EVENT_HUNTER_RULES}" 2>/dev/null; then
    break
  fi
  if [[ "${EVENT_HUNTER_ATTEMPT}" == "30" ]]; then
    echo "Grafana alert rules did not reach healthy file-provisioned evaluation state." >&2
    exit 1
  fi
  sleep 2
done

echo "Grafana provisioning verified: 4 datasources, 1 dashboard, 8 healthy file-provisioned alert rules, signed Event Hunter contact point and notification policy."
