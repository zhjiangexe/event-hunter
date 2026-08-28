import { afterEach, describe, expect, it, vi } from "vitest";
import { api, type Investigation } from "./api";

const fetchMock = vi.fn();
vi.stubGlobal("fetch", fetchMock);

afterEach(() => fetchMock.mockReset());

function respond(body: unknown, status = 200) {
  fetchMock.mockResolvedValueOnce(
    new Response(body === undefined ? null : JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

function requestAt(index = 0): Request {
  return fetchMock.mock.calls[index][0] as Request;
}

function pathAndQuery(request: Request): string {
  const url = new URL(request.url);
  return `${url.pathname}${url.search}`;
}

describe("Event Hunter API client", () => {
  it("creates a demo session with same-origin credentials", async () => {
    respond({
      subject: "demo-investigator",
      role: "INVESTIGATOR",
      permissions: ["investigation:write"],
    });

    await api.createSession("INVESTIGATOR");

    const request = requestAt();
    expect(pathAndQuery(request)).toBe("/api/v1/auth/demo-session");
    expect(request.method).toBe("POST");
    expect(request.credentials).toBe("include");
    expect(await request.clone().text()).toBe('{"role":"INVESTIGATOR"}');
  });

  it("builds a bounded timeline query from the correlation id", async () => {
    respond({
      correlation_id: "ORDER-2001",
      event_count: 2,
      events: [],
      truncated: false,
    });

    await api.timeline("ORDER-2001/unsafe");

    const url = new URL(requestAt().url);
    expect(url.pathname).toBe("/api/v1/timelines/ORDER-2001%2Funsafe");
    expect(url.searchParams.get("from")).toBe("2026-08-20T11:00:00Z");
    expect(url.searchParams.get("to")).toBe("2026-08-20T11:06:00Z");
  });

  it("builds one bounded Business Journey request", async () => {
    respond({
      correlation_id: "ORDER-4002",
      profile_id: "order-fulfillment",
      profile_version: 1,
      profile_title: "Order Fulfillment",
      from: "2026-08-20T16:00:00Z",
      to: "2026-08-20T16:21:00Z",
      status: "COMPLETED",
      event_count: 6,
      milestones: [],
      anomalies: [],
      unmapped_event_count: 0,
    });

    await api.businessJourney(
      "ORDER-4002/unsafe",
      "2026-08-20T16:00:00Z",
      "2026-08-20T16:21:00Z",
    );

    const url = new URL(requestAt().url);
    expect(url.pathname).toBe("/api/v1/business-journeys/ORDER-4002%2Funsafe");
    expect(url.searchParams.get("from")).toBe("2026-08-20T16:00:00Z");
    expect(url.searchParams.get("to")).toBe("2026-08-20T16:21:00Z");
  });

  it("loads the immutable Journey Profile Registry", async () => {
    respond({ items: [] });

    await api.journeyProfiles();

    expect(pathAndQuery(requestAt())).toBe("/api/v1/journey-profiles");
    expect(requestAt().credentials).toBe("include");
  });

  it("loads the read-only Pattern Registry", async () => {
    respond([]);

    await api.patterns();

    expect(pathAndQuery(requestAt())).toBe("/api/v1/patterns");
    expect(requestAt().credentials).toBe("include");
  });

  it("loads backend-owned Pattern effectiveness metrics", async () => {
    respond({
      generated_at: "2026-08-24T12:00:00Z",
      window: {
        from: "2026-07-25T12:00:00Z",
        to: "2026-08-24T12:00:00Z",
      },
      items: [],
    });

    await api.patternEffectiveness();

    expect(pathAndQuery(requestAt())).toBe("/api/v1/patterns/effectiveness");
    expect(requestAt().credentials).toBe("include");
  });

  it("loads the server-side investigation overview without client aggregation", async () => {
    respond({
      generated_at: "2026-08-22T12:00:00Z",
      window: {
        from: "2026-08-21T12:00:00Z",
        to: "2026-08-22T12:00:00Z",
      },
      partial: false,
      warnings: [],
      control_plane: null,
      events: null,
      sources: [],
    });

    await api.overview();

    expect(pathAndQuery(requestAt())).toBe("/api/v1/investigations/overview");
  });

  it("queries safe ingestion issues with generated filters and cursor", async () => {
    respond({ items: [], page_size: 20, next_cursor: null });

    await api.ingestionIssues(
      {
        from: "2026-08-24T12:00:00Z",
        to: "2026-08-27T12:00:00Z",
        kind: "TECHNICAL_DLQ",
        source_topic: "order.events",
      },
      "opaque/cursor",
      20,
    );

    const url = new URL(requestAt().url);
    expect(url.pathname).toBe("/api/v1/ingestion-issues");
    expect(url.searchParams.get("kind")).toBe("TECHNICAL_DLQ");
    expect(url.searchParams.get("source_topic")).toBe("order.events");
    expect(url.searchParams.get("cursor")).toBe("opaque/cursor");
    expect(url.searchParams.get("page_size")).toBe("20");
  });

  it("sends Smart Search input only to the deterministic identification endpoint", async () => {
    respond({
      input: "ORDER-2001",
      normalized_input: "ORDER-2001",
      status: "AMBIGUOUS",
      candidates: [],
      message: "SELECT_IDENTIFIER_TYPE",
    });

    await api.identifySearchInput("ORDER-2001");

    const request = requestAt();
    expect(pathAndQuery(request)).toBe("/api/v1/search/identify");
    expect(request.method).toBe("POST");
    expect(await request.clone().text()).toBe('{"input":"ORDER-2001"}');
  });

  it("uses generated Event Check request types and Snapshot idempotency header", async () => {
    const evaluationRequest = {
      identifier: { type: "CORRELATION_ID" as const, value: "ORDER-2001" },
      from: "2026-08-20T11:00:00Z",
      to: "2026-08-20T11:10:00Z",
    };
    respond({ resolution_status: "EVALUATED" });
    await api.evaluateEventCheck(evaluationRequest);

    const evaluationHTTP = requestAt(0);
    expect(pathAndQuery(evaluationHTTP)).toBe(
      "/api/v1/event-checks/evaluations",
    );
    expect(JSON.parse(await evaluationHTTP.clone().text())).toEqual(
      evaluationRequest,
    );

    const hash = "a".repeat(64);
    respond({ id: "snapshot-1" }, 201);
    await api.createCheckSnapshot(
      {
        evaluation_request: evaluationRequest,
        expected_event_set_hash: hash,
        expected_evaluation_hash: hash,
      },
      "save-event-check-1",
    );

    const snapshotHTTP = requestAt(1);
    expect(pathAndQuery(snapshotHTTP)).toBe("/api/v1/check-snapshots");
    expect(snapshotHTTP.headers.get("Idempotency-Key")).toBe(
      "save-event-check-1",
    );

    respond({ items: [], page_size: 20, next_cursor: null });
    await api.checkSnapshots(
      { identifier: "ORDER-2001", check_status: "DEVIATED" },
      "next-page",
      20,
    );
    const listHTTP = requestAt(2);
    const listURL = new URL(listHTTP.url);
    expect(listURL.pathname).toBe("/api/v1/check-snapshots");
    expect(listURL.searchParams.get("identifier")).toBe("ORDER-2001");
    expect(listURL.searchParams.get("check_status")).toBe("DEVIATED");
    expect(listURL.searchParams.get("cursor")).toBe("next-page");
  });

  it("uses optimistic locking for Finding feedback and Case handoff", async () => {
    respond({ finding_id: "finding-1", lock_version: 1 });
    await api.classifyCheckFinding("finding-1", 0, "NEEDS_REVIEW");

    const feedbackHTTP = requestAt(0);
    expect(pathAndQuery(feedbackHTTP)).toBe(
      "/api/v1/check-findings/finding-1/feedback",
    );
    expect(feedbackHTTP.headers.get("If-Match")).toBe('"v0"');

    respond({ investigation_id: "case-1", snapshot_id: "snapshot-1" }, 201);
    await api.attachInvestigationCheckSnapshot("case-1", "snapshot-1", 4);

    const attachHTTP = requestAt(1);
    expect(pathAndQuery(attachHTTP)).toBe(
      "/api/v1/investigations/case-1/check-snapshots",
    );
    expect(attachHTTP.headers.get("If-Match")).toBe('"v4"');
    expect(await attachHTTP.clone().json()).toEqual({
      snapshot_id: "snapshot-1",
    });
  });

  it("creates a bounded relative Saved Search without storing payload options", async () => {
    respond({
      id: "a11883b0-67f1-4fc2-832f-87d11173041f",
      owner_subject: "demo-viewer",
      name: "最近付款失敗",
      target: "TIMELINE",
      query: {
        time_mode: "RELATIVE",
        relative_window_seconds: 86_400,
        from: "2026-08-20T11:00:00Z",
        to: "2026-08-20T11:06:00Z",
        event_type: "PaymentFailed",
        include_processing_attempts: true,
      },
      open_url: "/timeline?event_type=PaymentFailed",
      created_at: "2026-08-22T11:00:00Z",
      updated_at: "2026-08-22T11:00:00Z",
    });

    await api.createSavedSearch("最近付款失敗", "TIMELINE", {
      time_mode: "RELATIVE",
      relative_window_seconds: 86_400,
      from: "2026-08-20T11:00:00Z",
      to: "2026-08-20T11:06:00Z",
      event_type: "PaymentFailed",
      include_processing_attempts: true,
    });

    const request = requestAt();
    expect(pathAndQuery(request)).toBe("/api/v1/saved-searches");
    expect(request.method).toBe("POST");
    const body = JSON.parse(await request.clone().text());
    expect(body.query.event_type).toBe("PaymentFailed");
    expect(body.query.time_mode).toBe("RELATIVE");
    expect(body.query.relative_window_seconds).toBe(86_400);
    expect(body.query).not.toHaveProperty("include_payload");
  });

  it("deletes one Saved Search through its encoded path parameter", async () => {
    respond(undefined, 204);

    await api.deleteSavedSearch("a11883b0-67f1-4fc2-832f-87d11173041f");

    const request = requestAt();
    expect(pathAndQuery(request)).toBe(
      "/api/v1/saved-searches/a11883b0-67f1-4fc2-832f-87d11173041f",
    );
    expect(request.method).toBe("DELETE");
  });

  it("builds a multi-condition event search query", async () => {
    respond({ events: [], count: 0, truncated: false });

    await api.searchEvents({
      correlation_id: "ORDER-2001",
      event_type: "PaymentCompleted",
      producer: "payment-service",
      causation_id: "evt-order-001",
      kafka_topic: "payment.events",
      kafka_partition: "0",
      kafka_offset: "42",
      pattern_id: "payment-completed-without-shipment",
      alert_id: "grafana-fingerprint-1",
      severity: "HIGH",
      include_payload: true,
      include_processing_attempts: true,
      from: "2026-08-20T11:00:00.000Z",
      to: "2026-08-20T11:06:00.000Z",
    });

    const requestPath = pathAndQuery(requestAt());
    expect(requestPath).toContain("/api/v1/events/search?");
    expect(requestPath).toContain("correlation_id=ORDER-2001");
    expect(requestPath).toContain("event_type=PaymentCompleted");
    expect(requestPath).toContain("producer=payment-service");
    expect(requestPath).toContain("causation_id=evt-order-001");
    expect(requestPath).toContain("kafka_topic=payment.events");
    expect(requestPath).toContain("kafka_partition=0");
    expect(requestPath).toContain("kafka_offset=42");
    expect(requestPath).toContain(
      "pattern_id=payment-completed-without-shipment",
    );
    expect(requestPath).toContain("alert_id=grafana-fingerprint-1");
    expect(requestPath).toContain("severity=HIGH");
    expect(requestPath).toContain("include_payload=true");
    expect(requestPath).toContain("include_processing_attempts=true");
  });

  it("uses the deployed investigation cursor contract", async () => {
    respond({ items: [], next_cursor: null });

    await api.investigations("cursor/next");

    const url = new URL(requestAt().url);
    expect(url.pathname).toBe("/api/v1/investigations");
    expect(url.searchParams.get("page_size")).toBe("10");
    expect(url.searchParams.get("cursor")).toBe("cursor/next");
    expect(url.searchParams.has("page_cursor")).toBe(false);
  });

  it("creates an investigation with the Timeline incident window", async () => {
    respond({ id: "case-1" }, 201);

    await api.createInvestigation({
      title: "payment completed without shipment",
      severity: "HIGH",
      correlation_id: "ORDER-2001",
      incident_from: "2026-08-20T11:00:00Z",
      incident_to: "2026-08-20T11:06:00Z",
    });

    const request = requestAt();
    expect(pathAndQuery(request)).toBe("/api/v1/investigations");
    expect(request.method).toBe("POST");
    expect(JSON.parse(await request.clone().text())).toEqual({
      title: "payment completed without shipment",
      severity: "HIGH",
      correlation_id: "ORDER-2001",
      incident_from: "2026-08-20T11:00:00Z",
      incident_to: "2026-08-20T11:06:00Z",
      start_workflow: false,
    });
  });

  it("leaves default Pattern selection to the backend and only sends explicit advanced choices", async () => {
    respond({
      investigation_id: "case-1",
      execution_mode: "SYNC",
      analyzed_at: "2026-08-26T12:00:00Z",
      analysis_status: "EVALUATED",
      executed_pattern_ids: ["payment-completed-without-shipment"],
      effective_window: null,
      findings: [],
    });

    await api.analyze("case-1");
    expect(JSON.parse(await requestAt().clone().text())).toEqual({
      execution_mode: "SYNC",
    });

    respond({
      investigation_id: "case-1",
      execution_mode: "SYNC",
      analyzed_at: "2026-08-26T12:00:00Z",
      analysis_status: "EVALUATED",
      executed_pattern_ids: ["payment-completed-without-shipment"],
      effective_window: null,
      findings: [],
    });
    await api.analyze("case-1", ["payment-completed-without-shipment"]);
    expect(JSON.parse(await requestAt(1).clone().text())).toEqual({
      pattern_ids: ["payment-completed-without-shipment"],
      execution_mode: "SYNC",
    });
  });

  it("passes compound filters and backend sort into the investigation list query", async () => {
    respond({ items: [], next_cursor: null });

    await api.investigations(undefined, {
      status: "INVESTIGATING",
      priority: "P0",
      assignee: "shipping-oncall",
      tag: "urgent",
      correlation_id: "ORDER-2001",
      sort_by: "updated_at",
      sort_order: "asc",
    });

    const url = new URL(requestAt().url);
    expect(url.searchParams.get("status")).toBe("INVESTIGATING");
    expect(url.searchParams.get("priority")).toBe("P0");
    expect(url.searchParams.get("assignee")).toBe("shipping-oncall");
    expect(url.searchParams.get("tag")).toBe("urgent");
    expect(url.searchParams.get("correlation_id")).toBe("ORDER-2001");
    expect(url.searchParams.get("sort_by")).toBe("updated_at");
    expect(url.searchParams.get("sort_order")).toBe("asc");
  });

  it("does not inject demo dates into case reads and preserves an explicit window", async () => {
    respond({ investigation_id: "case-1" });
    respond({ investigation_id: "case-1", items: [] });

    await api.summary("case-1");
    await api.evidence("case-1", {
      from: "2026-08-23T10:00:00Z",
      to: "2026-08-23T11:00:00Z",
    });

    const summaryURL = new URL(requestAt(0).url);
    expect(summaryURL.pathname).toBe("/api/v1/investigations/case-1/summary");
    expect(summaryURL.searchParams.has("from")).toBe(false);
    expect(summaryURL.searchParams.has("to")).toBe(false);

    const evidenceURL = new URL(requestAt(1).url);
    expect(evidenceURL.pathname).toBe(
      "/api/v1/investigations/case-1/evidence-bundle",
    );
    expect(evidenceURL.searchParams.get("from")).toBe("2026-08-23T10:00:00Z");
    expect(evidenceURL.searchParams.get("to")).toBe("2026-08-23T11:00:00Z");
  });

  it("sends optimistic-lock headers when updating a case", async () => {
    respond({ id: "case-1", lock_version: 3 });
    const item: Investigation = {
      id: "case-1",
      case_no: "MANUAL-1",
      title: "Case",
      severity: "HIGH",
      status: "OPEN",
      allowed_transitions: ["INVESTIGATING", "CLOSED"],
      correlation_id: "ORDER-1",
      incident_from: "2026-08-20T10:00:00Z",
      incident_to: "2026-08-20T11:00:00Z",
      incident_window_source: "TIMELINE_SEARCH",
      priority: "P1",
      tags: [],
      related_correlation_ids: [],
      last_updated_by: "demo:investigator",
      sla_status: "ON_TRACK",
      sla_due_at: "2026-08-20T15:00:00Z",
      collaboration_notes: [],
      pattern_findings: [],
      evidence: [],
      lock_version: 2,
      created_at: "2026-08-20T11:00:00Z",
      updated_at: "2026-08-20T11:00:00Z",
    };

    await api.patchInvestigation(item, { status: "INVESTIGATING" });

    const request = requestAt();
    expect(pathAndQuery(request)).toBe("/api/v1/investigations/case-1");
    expect(request.method).toBe("PATCH");
    expect(request.headers.get("Content-Type")).toBe("application/json");
    expect(request.headers.get("If-Match")).toBe('"v2"');
    expect(await request.clone().text()).toBe('{"status":"INVESTIGATING"}');
  });

  it("appends a case note through the dedicated optimistic-lock endpoint", async () => {
    respond({ investigation: { id: "case-1" }, note: { id: "note-1" } }, 201);
    const item: Investigation = {
      id: "case-1",
      case_no: "MANUAL-1",
      title: "Case",
      severity: "HIGH",
      status: "INVESTIGATING",
      allowed_transitions: ["WAITING_APPROVAL", "RESOLVED", "CLOSED"],
      correlation_id: "ORDER-1",
      incident_from: "2026-08-20T10:00:00Z",
      incident_to: "2026-08-20T11:00:00Z",
      incident_window_source: "TIMELINE_SEARCH",
      priority: "P1",
      tags: [],
      related_correlation_ids: [],
      last_updated_by: "demo:investigator",
      sla_status: "ON_TRACK",
      sla_due_at: "2026-08-20T15:00:00Z",
      collaboration_notes: [],
      pattern_findings: [],
      evidence: [],
      lock_version: 4,
      created_at: "2026-08-20T11:00:00Z",
      updated_at: "2026-08-20T11:00:00Z",
    };

    await api.addInvestigationNote(item, "consumer lag confirmed");

    const request = requestAt();
    expect(pathAndQuery(request)).toBe("/api/v1/investigations/case-1/notes");
    expect(request.method).toBe("POST");
    expect(request.headers.get("If-Match")).toBe('"v4"');
    expect(await request.clone().text()).toBe(
      '{"body":"consumer lag confirmed"}',
    );
  });

  it("attaches a bounded Timeline event through the optimistic-lock endpoint", async () => {
    respond({
      investigation: { id: "case-1", lock_version: 5 },
      evidence: {
        id: "evidence-1",
        evidence_type: "EVENT",
        reference: "evt-order-1",
        source: "CLICKHOUSE",
        open_action: "GRAFANA_EVENT",
        collected_at: "2026-08-20T11:02:00Z",
        checksum: "a".repeat(64),
      },
      attached: true,
    });
    const item: Investigation = {
      id: "case-1",
      case_no: "MANUAL-1",
      title: "Case",
      severity: "HIGH",
      status: "INVESTIGATING",
      allowed_transitions: ["WAITING_APPROVAL", "RESOLVED", "CLOSED"],
      correlation_id: "ORDER-1",
      incident_from: "2026-08-20T10:00:00Z",
      incident_to: "2026-08-20T11:00:00Z",
      incident_window_source: "TIMELINE_SEARCH",
      priority: "P1",
      tags: [],
      related_correlation_ids: [],
      last_updated_by: "demo:investigator",
      sla_status: "ON_TRACK",
      sla_due_at: "2026-08-20T15:00:00Z",
      collaboration_notes: [],
      pattern_findings: [],
      evidence: [],
      lock_version: 4,
      created_at: "2026-08-20T11:00:00Z",
      updated_at: "2026-08-20T11:00:00Z",
    };

    await api.attachInvestigationEvent(
      item,
      "evt-order-1",
      "2026-08-20T11:00:00Z",
      "2026-08-20T11:06:00Z",
    );

    const request = requestAt();
    expect(pathAndQuery(request)).toBe(
      "/api/v1/investigations/case-1/evidence/events",
    );
    expect(request.method).toBe("POST");
    expect(request.headers.get("If-Match")).toBe('"v4"');
    expect(await request.clone().json()).toEqual({
      event_id: "evt-order-1",
      from: "2026-08-20T11:00:00Z",
      to: "2026-08-20T11:06:00Z",
    });
  });

  it("classifies a persisted Pattern finding with its own optimistic lock", async () => {
    respond({
      finding_id: "11111111-1111-4111-8111-111111111111",
      status: "CONFIRMED",
      actor_id: "demo-investigator",
      actor_role: "INVESTIGATOR",
      updated_at: "2026-08-24T12:00:00Z",
      lock_version: 2,
    });

    await api.classifyPatternFinding(
      "case-1",
      "11111111-1111-4111-8111-111111111111",
      1,
      "CONFIRMED",
    );

    const request = requestAt();
    expect(pathAndQuery(request)).toBe(
      "/api/v1/investigations/case-1/findings/11111111-1111-4111-8111-111111111111/feedback",
    );
    expect(request.method).toBe("PATCH");
    expect(request.headers.get("If-Match")).toBe('"v1"');
    expect(await request.clone().json()).toEqual({ status: "CONFIRMED" });
  });

  it("surfaces API error codes to the UI", async () => {
    respond({ code: "OPTIMISTIC_LOCK_CONFLICT" }, 409);

    await expect(api.investigations()).rejects.toThrow(
      "OPTIMISTIC_LOCK_CONFLICT",
    );
  });
});
