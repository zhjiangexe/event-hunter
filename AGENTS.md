# Event Hunter Agent Contract

## Source precedence

When artifacts conflict, use this order:

1. `project-scope.yaml` and `requirements/traceability.yaml`
2. `openapi.yaml`, `contracts/asyncapi.yaml`, and contracts under `contracts/events`, `contracts/platform`, `contracts/quality`, `contracts/integrations`, `contracts/telemetry`, and `contracts/demo-services`
3. `backend/migrations/**` and `e2e/**/*.feature`
4. Markdown documents and `ui-prototype.html`

Do not silently reconcile a conflict. Update the lower-precedence artifact in the same change.

## MVP scope guard

Implement only `REQ-EH-001` through `REQ-EH-009` unless the user explicitly expands scope.

- Temporal is optional and disabled by default. Core startup and all MVP acceptance scenarios must pass without it.
- Pattern evaluation is deterministic Go code. Do not add LLM, runtime rule editing, arbitrary SQL, or a Python service.
- Evidence Bundle is a JSON manifest containing references and RFC 8785 JCS SHA-256. Do not add ZIP, PDF, raw logs, raw traces, or unmasked PII.
- Authentication is a signed HttpOnly demo-role cookie with no user table. Do not build passwords, signup, password reset, or OIDC in MVP.
- Event Catalog, Topic Registry, custom observability UI, generic alert management, replay, and production redrive are deferred.
- `REQ-EH-007` is only signed Grafana business-alert intake; do not rebuild Grafana notification routing or on-call.
- `REQ-EH-008` owns a deterministic one-minute quality worker and provisioned Grafana assets; do not add an Event Hunter Quality Console.
- `REQ-EH-009` owns the three demo services, Outbox, Debezium, Redpanda, and Redpanda Connect vertical slice.

## Architecture rules

- Build a modular monolith under `backend/`; use constructor injection and explicit composition roots.
- Domain packages must not import chi, Huma, pgx, ClickHouse, Temporal, or transport DTOs.
- HTTP handlers validate and map data, then call application use cases; they do not execute SQL.
- PostgreSQL mutable aggregates use `lock_version`; append-only tables use idempotency or unique keys, not optimistic locking.
- ClickHouse queries must use allowlisted parameters, bounded time windows, result limits, timeouts, and a read-only account.
- Never infer Consumer retries from duplicate events. Show retry data only from processing-attempt telemetry.
- Kafka transport identity is topic + partition + offset. Sink redelivery of the same identity is not a business duplicate.
- Validate Grafana HMAC against timestamp + raw body before JSON business processing; never log or persist the shared secret, signature, or raw webhook body.
- A resolved Grafana alert records evidence but never auto-closes an Investigation Case.
- Store UTC timestamps and do not log tokens or unmasked event payloads.
- Allocate every local host port from `contracts/platform/port-registry.yaml`; keep dependency container ports unchanged unless the upstream image requires a change.

## Contract and test rules

- Keep Huma operation IDs identical to `openapi.yaml`.
- Generate frontend API types from OpenAPI; do not hand-maintain duplicate TypeScript DTOs.
- Use fixtures in `contracts/fixtures`; do not add test-only branches in application code.
- A task is complete only when its mapped Karate scenarios and proportionate Go/React tests pass.
- Do not delete or weaken an acceptance assertion to make an implementation pass.

## Implementation order

Follow `requirements/implementation-plan.yaml`. Do not start a task while an item in `depends_on` is incomplete.
