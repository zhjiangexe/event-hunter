import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  CheckModelsRegistry,
  EventCheckWorkspace,
  SavedCheckResults,
} from "./event-check-workspace";
import { SavedCheckResultsPage } from "./main";
import {
  api,
  type CheckModelRegistryEntry,
  type CheckSnapshot,
  type CheckSnapshotSummary,
  type EventCheckEvaluation,
  type InvestigationPage,
  type Principal,
} from "./api";

const principal: Principal = {
  subject: "tester",
  role: "INVESTIGATOR",
  permissions: [],
};

const evaluation: EventCheckEvaluation = {
  resolution_status: "EVALUATED",
  normalized_request: {
    identifier: { type: "CORRELATION_ID", value: "ORDER-1001" },
    from: "2026-08-28T00:00:00Z",
    to: "2026-08-28T01:00:00Z",
  },
  source_health: {
    status: "HEALTHY",
    checked_at: "2026-08-28T01:00:01Z",
    coverage_from: "2026-08-28T00:00:00Z",
    coverage_to: "2026-08-28T01:00:00Z",
    watermark: "2026-08-28T01:00:00Z",
    truncated: false,
    components: [
      {
        component: "CANONICAL_EVENTS",
        status: "HEALTHY",
        detail_code: "QUERY_COMPLETE",
      },
      {
        component: "INGESTION_WATERMARK",
        status: "HEALTHY",
        detail_code: "CURRENT",
      },
      {
        component: "RELATION_INDEX",
        status: "HEALTHY",
        detail_code: "READY",
      },
    ],
  },
  scope: {
    mode: "STANDARD_SCOPE",
    seeds: ["event-1"],
    events: [
      {
        event_id: "event-1",
        event_type: "OrderCreated",
        event_version: 1,
        occurred_at: "2026-08-28T00:01:00Z",
        producer: "order-service",
        aggregate_type: "Order",
        aggregate_id: "ORDER-1001",
        sequence: 1,
        correlation_id: "ORDER-1001",
        trace_id: "a".repeat(32),
        payload_sha256: "b".repeat(64),
        ordinal: 0,
      },
    ],
    excluded_events: [],
    relationships: [
      {
        ordinal: 0,
        from_event_id: null,
        to_event_id: "event-1",
        relation_type: "SEED",
        source_field: null,
        source_model_id: null,
        source_rule_id: null,
      },
    ],
    limits: {
      max_duration_seconds: 604800,
      max_events: 10000,
      max_correlations: 20,
      max_relationship_depth: 3,
    },
  },
  identifier_candidates: [],
  model_candidates: [],
  model: {
    id: "order-fulfillment",
    version: 2,
    kind: "FLOW",
    source_path: "contracts/check-models/order-fulfillment.yaml",
    checksum: "c".repeat(64),
  },
  result: {
    check_status: "IN_PROGRESS",
    business_outcome: null,
    expectations: [
      {
        id: "payment-requires-shipment",
        state: "WAITING",
        trigger_event_ids: ["event-1"],
        satisfying_event_ids: [],
        reminder_at: "2026-08-28T00:10:00Z",
        deadline_at: "2026-08-28T00:20:00Z",
      },
    ],
    flows: [
      {
        model: {
          id: "order-fulfillment",
          version: 2,
          kind: "FLOW",
          source_path: "contracts/check-models/order-fulfillment.yaml",
          checksum: "c".repeat(64),
        },
        role: "ROOT",
        status: "IN_PROGRESS",
        candidate_path_ids: ["happy-path"],
        matched_path_id: null,
        outcome: null,
      },
    ],
    global_checks: [],
    findings: [
      {
        id: null,
        rule_kind: "FLOW_EXPECTATION",
        rule_id: "payment-requires-shipment",
        rule_version: 1,
        rule_checksum: "d".repeat(64),
        severity: "HIGH",
        code: "MISSING_SHIPMENT_AFTER_PAYMENT",
        expectation_state: "WAITING",
        evidence_references: [{ type: "EVENT", value: "event-1" }],
        recommended_query_template_id: "events.by-correlation.v1",
      },
    ],
    unmapped_event_ids: [],
    evaluator_contract_version: 1,
    evaluator_build_version: "event-check-v1",
  },
  event_set_hash: "e".repeat(64),
  evaluation_hash: "f".repeat(64),
  warnings: [],
};

const snapshot: CheckSnapshot = {
  id: "11111111-1111-4111-8111-111111111111",
  provenance: "LIVE_EVALUATION",
  created_by: "tester",
  created_by_role: "INVESTIGATOR",
  created_at: "2026-08-28T01:00:02Z",
  evaluation_request: evaluation.normalized_request,
  as_of: evaluation.normalized_request.to,
  source_health: evaluation.source_health,
  model: evaluation.model!,
  result: {
    ...evaluation.result!,
    findings: [
      {
        ...evaluation.result!.findings[0],
        id: "22222222-2222-4222-8222-222222222222",
      },
    ],
  },
  event_references: evaluation.scope.events.map((event) => ({
    event_id: event.event_id,
    event_type: event.event_type,
    occurred_at: event.occurred_at,
    producer: event.producer,
    aggregate_type: event.aggregate_type,
    aggregate_id: event.aggregate_id,
    correlation_id: event.correlation_id,
    trace_id: event.trace_id,
    payload_sha256: event.payload_sha256,
    ordinal: event.ordinal,
    disposition: "INCLUDED",
    adjustment_reason: null,
    source_available: true,
  })),
  relationships: evaluation.scope.relationships,
  finding_feedback: [
    {
      finding_id: "22222222-2222-4222-8222-222222222222",
      status: "UNREVIEWED",
      actor_id: "",
      actor_role: "",
      updated_at: null,
      lock_version: 0,
    },
  ],
  event_set_hash: evaluation.event_set_hash!,
  evaluation_hash: evaluation.evaluation_hash!,
  result_schema_version: 1,
  retention_profile: null,
};

const flowModel = {
  source_path: "contracts/check-models/order-fulfillment.yaml",
  checksum: "c".repeat(64),
  model: {
    contract_version: 1,
    model_id: "order-fulfillment",
    version: 2,
    kind: "FLOW",
    status: "ACTIVE",
    title: "Order Fulfillment",
    description: "Order to shipment paths.",
    domain: "logistics",
    source: { authoring: "YAML_GIT", mutable_at_runtime: false },
    applies_to: {
      aggregate_types: ["Order"],
      trigger_event_types: ["OrderCreated"],
      event_versions: [{ event_type: "OrderCreated", versions: [1] }],
    },
    scope: {
      max_duration_seconds: 604800,
      max_events: 10000,
      max_correlations: 20,
      max_relationship_depth: 3,
      relations: ["SAME_CORRELATION"],
      business_keys: [],
      parent_child_relations: [],
    },
    nodes: [
      {
        id: "order-created",
        label: "Order created",
        event: { event_types: ["OrderCreated"] },
        min_occurs: 1,
        max_occurs: 1,
      },
    ],
    paths: [
      {
        id: "happy-path",
        label: "Happy path",
        nodes: ["order-created"],
        terminal: true,
        outcome: {
          code: "FULFILLED",
          label: "Fulfilled",
          category: "SUCCESS",
        },
      },
    ],
    expectations: [
      {
        id: "PAYMENT_REQUIRES_SHIPMENT",
        label: "Payment requires shipment",
        trigger: { event_types: ["PaymentCompleted"] },
        expected: { event_types: ["ShipmentCreated"] },
        temporal_relation: "AFTER_OR_AT",
        reminder_after_seconds: 120,
        deadline_seconds: 300,
        exclusions: { any_event_types: [] },
        severity: "HIGH",
        finding_code: "MISSING_SHIPMENT_AFTER_PAYMENT",
        recommended_query_template_id: "shipping.events.by_order.v1",
      },
    ],
    child_models: [],
    unmapped_event_policy: {
      default: "INFORMATIONAL",
      escalate_event_types: [],
    },
    fixtures: {
      scenario_file:
        "contracts/event-check/fixtures/check-model-scenarios.json",
      case_ids: ["happy-path", "in-progress"],
    },
  },
} as unknown as CheckModelRegistryEntry;

const globalModel = {
  ...flowModel,
  source_path: "contracts/check-models/event-integrity.yaml",
  model: {
    ...flowModel.model,
    model_id: "event-integrity",
    version: 1,
    kind: "GLOBAL_CHECK",
    title: "Event Integrity",
    rules: [],
    paths: undefined,
    nodes: undefined,
    expectations: undefined,
  },
} as unknown as CheckModelRegistryEntry;

const cases: InvestigationPage = {
  items: [
    {
      id: "33333333-3333-4333-8333-333333333333",
      case_no: "EH-2026-0001",
      title: "Investigate order",
      severity: "HIGH",
      status: "OPEN",
      allowed_transitions: ["INVESTIGATING"],
      correlation_id: "ORDER-1001",
      incident_from: "2026-08-28T00:00:00Z",
      incident_to: "2026-08-28T01:00:00Z",
      incident_window_source: "TIMELINE_SEARCH",
      assignee: "",
      priority: "P1",
      tags: [],
      related_correlation_ids: [],
      last_updated_by: "tester",
      sla_status: "ON_TRACK",
      sla_due_at: "2026-08-28T05:00:00Z",
      collaboration_notes: [],
      pattern_findings: [],
      evidence: [],
      lock_version: 0,
      created_at: "2026-08-28T01:00:00Z",
      updated_at: "2026-08-28T01:00:00Z",
    },
  ],
  next_cursor: null,
};

const savedSummary: CheckSnapshotSummary = {
  id: snapshot.id,
  created_by: snapshot.created_by,
  created_by_role: snapshot.created_by_role,
  created_at: snapshot.created_at,
  evaluation_request: snapshot.evaluation_request,
  as_of: snapshot.as_of,
  source_health_status: snapshot.source_health.status,
  model: {
    id: snapshot.model.id,
    version: snapshot.model.version,
    kind: snapshot.model.kind,
  },
  check_status: snapshot.result.check_status,
  event_count: snapshot.event_references.length,
  finding_count: snapshot.result.findings.length,
  linked_case_count: 0,
};

function LocationOutput() {
  const location = useLocation();
  return (
    <output data-testid="location">
      {location.pathname}
      {location.search}
    </output>
  );
}

function renderWorkspace(entry: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[entry]}>
        <EventCheckWorkspace principal={principal} />
        <LocationOutput />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function renderModels(entry = "/check-models") {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[entry]}>
        <CheckModelsRegistry />
        <LocationOutput />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function renderSavedResults() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/event-check/saved-results"]}>
        <SavedCheckResults principal={principal} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function renderSavedResultsPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/event-check/saved-results"]}>
        <SavedCheckResultsPage principal={principal} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  vi.spyOn(api, "checkModels").mockResolvedValue([flowModel, globalModel]);
  vi.spyOn(api, "evaluateEventCheck").mockResolvedValue(evaluation);
  vi.spyOn(api, "checkSnapshot").mockResolvedValue(snapshot);
  vi.spyOn(api, "createCheckSnapshot").mockResolvedValue(snapshot);
  vi.spyOn(api, "checkSnapshots").mockResolvedValue({
    items: [savedSummary],
    page_size: 20,
    next_cursor: null,
  });
  vi.spyOn(api, "investigations").mockResolvedValue(cases);
  vi.spyOn(api, "createInvestigation").mockResolvedValue(cases.items[0]);
  vi.spyOn(api, "attachInvestigationCheckSnapshot").mockResolvedValue({
    investigation_id: cases.items[0].id,
    snapshot_id: snapshot.id,
    linked_by: "tester",
    linked_by_role: "INVESTIGATOR",
    linked_at: "2026-08-28T01:01:00Z",
  });
  vi.spyOn(api, "classifyCheckFinding").mockResolvedValue({
    finding_id: snapshot.finding_feedback[0].finding_id,
    status: "CONFIRMED",
    actor_id: "tester",
    actor_role: "INVESTIGATOR",
    updated_at: "2026-08-28T01:02:00Z",
    lock_version: 1,
  });
  vi.spyOn(api, "savedSearches").mockResolvedValue({ items: [] });
  vi.spyOn(api, "createSavedSearch").mockResolvedValue({
    id: "44444444-4444-4444-8444-444444444444",
    owner_subject: "tester",
    name: "Order check",
    target: "EVENT_CHECK",
    query: {
      from: evaluation.normalized_request.from,
      to: evaluation.normalized_request.to,
      time_mode: "ABSOLUTE",
      include_processing_attempts: false,
      identifier_type: "CORRELATION_ID",
      identifier_value: "ORDER-1001",
      workspace_tab: "summary",
    },
    open_url: "/event-check?identifier=ORDER-1001",
    created_at: "2026-08-28T01:00:00Z",
    updated_at: "2026-08-28T01:00:00Z",
  });
  vi.spyOn(api, "deleteSavedSearch").mockResolvedValue(undefined);
});

afterEach(() => vi.restoreAllMocks());

describe("Event Check workspace", () => {
  it("writes the bounded identifier and second-precision window into the URL", async () => {
    renderWorkspace("/event-check");
    fireEvent.change(screen.getByTestId("event-check-identifier"), {
      target: { value: "ORDER-1001" },
    });
    fireEvent.click(screen.getByTestId("event-check-submit"));
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toContain(
        "identifier=ORDER-1001",
      ),
    );
    expect(screen.getByTestId("location").textContent).toContain("from=");
    expect(screen.getByTestId("location").textContent).toContain("to=");
  });

  it("renders server-owned status and opens Summary by default", async () => {
    renderWorkspace(
      "/event-check?identifier_type=CORRELATION_ID&identifier=ORDER-1001&from=2026-08-28T00%3A00%3A00Z&to=2026-08-28T01%3A00%3A00Z",
    );
    expect((await screen.findAllByText("IN_PROGRESS")).length).toBeGreaterThan(
      0,
    );
    expect(screen.getByText("order-fulfillment v2")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Summary" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByText("資料可信度")).toBeInTheDocument();
  });

  it("offers trusted Grafana, Loki and Tempo links for each Event Check event", async () => {
    renderWorkspace(
      "/event-check?identifier_type=CORRELATION_ID&identifier=ORDER-1001&from=2026-08-28T00%3A00%3A00Z&to=2026-08-28T01%3A00%3A00Z&tab=timeline",
    );
    await screen.findByText("OrderCreated");
    const grafana = screen.getByRole("link", { name: "Grafana Explore ↗" });
    const logs = screen.getByRole("link", { name: "Loki logs ↗" });
    const trace = screen.getByRole("link", { name: "Tempo trace ↗" });
    const dashboard = screen.getByRole("link", {
      name: "Quality Dashboard ↗",
    });
    expect(grafana).toHaveAttribute(
      "href",
      expect.stringContaining("http://localhost:28332/explore?"),
    );
    expect(logs).toHaveAttribute("href", expect.stringContaining("loki"));
    expect(trace).toHaveAttribute("href", expect.stringContaining("tempo"));
    expect(dashboard).toHaveAttribute(
      "href",
      expect.stringContaining("/d/event-quality?"),
    );
    expect(grafana).toHaveAttribute("target", "_blank");
  });

  it("keeps legacy null collections from blanking the workspace", async () => {
    vi.mocked(api.evaluateEventCheck).mockResolvedValue({
      ...evaluation,
      source_health: { ...evaluation.source_health, components: null },
      result: {
        ...evaluation.result!,
        unmapped_event_ids: null,
        flows: evaluation.result!.flows.map((flow) => ({
          ...flow,
          candidate_path_ids: null,
        })),
        expectations: evaluation.result!.expectations.map((expectation) => ({
          ...expectation,
          trigger_event_ids: null,
          satisfying_event_ids: null,
        })),
        findings: evaluation.result!.findings.map((finding) => ({
          ...finding,
          evidence_references: null,
        })),
      },
      warnings: null,
    } as unknown as EventCheckEvaluation);
    renderWorkspace(
      "/event-check?identifier_type=CORRELATION_ID&identifier=ORDER-1001&from=2026-08-28T00%3A00%3A00Z&to=2026-08-28T01%3A00%3A00Z",
    );
    expect((await screen.findAllByText("IN_PROGRESS")).length).toBeGreaterThan(
      0,
    );
    fireEvent.click(screen.getByRole("tab", { name: "Flow" }));
    expect(await screen.findByText(/候選：無/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: /Findings/ }));
    expect(
      await screen.findByText("MISSING_SHIPMENT_AFTER_PAYMENT"),
    ).toBeInTheDocument();
  });

  it("saves a snapshot with the evaluation hashes and keeps it in URL state", async () => {
    renderWorkspace(
      "/event-check?identifier_type=CORRELATION_ID&identifier=ORDER-1001&from=2026-08-28T00%3A00%3A00Z&to=2026-08-28T01%3A00%3A00Z",
    );
    fireEvent.click(await screen.findByRole("button", { name: "保存結果" }));
    await waitFor(() => expect(api.createCheckSnapshot).toHaveBeenCalled());
    expect(vi.mocked(api.createCheckSnapshot).mock.calls[0][0]).toMatchObject({
      expected_event_set_hash: evaluation.event_set_hash,
      expected_evaluation_hash: evaluation.evaluation_hash,
    });
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toContain(
        `snapshot_id=${snapshot.id}`,
      ),
    );
  });

  it("leads with Timeline when no event data is available", async () => {
    vi.mocked(api.evaluateEventCheck).mockResolvedValue({
      ...evaluation,
      resolution_status: "NO_DATA",
      scope: { ...evaluation.scope, seeds: [], events: [], relationships: [] },
      model: null,
      result: null,
      event_set_hash: null,
      evaluation_hash: null,
    });
    renderWorkspace(
      "/event-check?identifier_type=CORRELATION_ID&identifier=ORDER-NONE&from=2026-08-28T00%3A00%3A00Z&to=2026-08-28T01%3A00%3A00Z",
    );
    expect(await screen.findByText("NO_DATA")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Timeline" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByText("此範圍沒有事件。")).toBeInTheDocument();
  });

  it("records a custom exclusion reason in URL state and the next request", async () => {
    renderWorkspace(
      "/event-check?identifier_type=CORRELATION_ID&identifier=ORDER-1001&from=2026-08-28T00%3A00%3A00Z&to=2026-08-28T01%3A00%3A00Z&tab=timeline",
    );
    await screen.findByText("OrderCreated");
    fireEvent.click(screen.getByText("自訂事件範圍"));
    fireEvent.change(screen.getByLabelText("Event ID"), {
      target: { value: "event-1" },
    });
    fireEvent.change(screen.getByLabelText("人工原因"), {
      target: { value: "排除已知重送事件" },
    });
    fireEvent.click(screen.getByRole("button", { name: "套用並重新檢查" }));
    await waitFor(() =>
      expect(screen.getByTestId("location").textContent).toContain(
        "exclude_event_id=event-1",
      ),
    );
    await waitFor(() =>
      expect(api.evaluateEventCheck).toHaveBeenLastCalledWith(
        expect.objectContaining({
          scope_adjustments: {
            include: [],
            exclude: [{ event_id: "event-1", reason: "排除已知重送事件" }],
          },
        }),
      ),
    );
  });

  it("uses persisted feedback lock version when classifying a Finding", async () => {
    renderWorkspace(
      `/event-check?identifier_type=CORRELATION_ID&identifier=ORDER-1001&from=2026-08-28T00%3A00%3A00Z&to=2026-08-28T01%3A00%3A00Z&snapshot_id=${snapshot.id}&tab=findings`,
    );
    fireEvent.click(await screen.findByRole("button", { name: "確認異常" }));
    await waitFor(() =>
      expect(api.classifyCheckFinding).toHaveBeenCalledWith(
        snapshot.finding_feedback[0].finding_id,
        0,
        "CONFIRMED",
      ),
    );
  });

  it("restores focus when the join Case dialog closes with Escape", async () => {
    renderWorkspace(
      `/event-check?identifier_type=CORRELATION_ID&identifier=ORDER-1001&from=2026-08-28T00%3A00%3A00Z&to=2026-08-28T01%3A00%3A00Z&snapshot_id=${snapshot.id}`,
    );
    const trigger = await screen.findByRole("button", {
      name: "加入案件",
    });
    trigger.focus();
    fireEvent.click(trigger);
    const dialog = await screen.findByRole("dialog", {
      name: "加入案件",
    });
    fireEvent.keyDown(dialog, { key: "Escape" });
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    expect(trigger).toHaveFocus();
  });

  it("attaches a saved Snapshot with the selected Case lock version", async () => {
    renderWorkspace(
      `/event-check?identifier_type=CORRELATION_ID&identifier=ORDER-1001&from=2026-08-28T00%3A00%3A00Z&to=2026-08-28T01%3A00%3A00Z&snapshot_id=${snapshot.id}`,
    );
    fireEvent.click(await screen.findByRole("button", { name: "加入案件" }));
    fireEvent.click(await screen.findByRole("button", { name: "加入此案件" }));
    await waitFor(() =>
      expect(api.attachInvestigationCheckSnapshot).toHaveBeenCalledWith(
        cases.items[0].id,
        snapshot.id,
        0,
      ),
    );
    expect(await screen.findByText("Snapshot 已連結案件")).toBeInTheDocument();
  });

  it("creates a Case and attaches the saved Snapshot", async () => {
    renderWorkspace(
      `/event-check?identifier_type=CORRELATION_ID&identifier=ORDER-1001&from=2026-08-28T00%3A00%3A00Z&to=2026-08-28T01%3A00%3A00Z&snapshot_id=${snapshot.id}`,
    );
    fireEvent.click(await screen.findByTestId("event-check-create-case"));
    fireEvent.click(
      await screen.findByTestId("event-check-create-case-confirm"),
    );
    await waitFor(() =>
      expect(api.createInvestigation).toHaveBeenCalledWith(
        expect.objectContaining({
          correlation_id: "ORDER-1001",
          incident_from: "2026-08-28T00:00:00Z",
          incident_to: "2026-08-28T01:00:00Z",
        }),
      ),
    );
    await waitFor(() =>
      expect(api.attachInvestigationCheckSnapshot).toHaveBeenCalledWith(
        cases.items[0].id,
        snapshot.id,
        cases.items[0].lock_version,
      ),
    );
    expect(
      await screen.findByText("案件已建立並加入 Snapshot"),
    ).toBeInTheDocument();
  });

  it("stores the current bounded request as an Event Check shortcut", async () => {
    renderWorkspace(
      "/event-check?identifier_type=CORRELATION_ID&identifier=ORDER-1001&from=2026-08-28T00%3A00%3A00Z&to=2026-08-28T01%3A00%3A00Z&tab=flow",
    );
    fireEvent.click(screen.getByRole("button", { name: "查詢捷徑" }));
    fireEvent.change(await screen.findByTestId("event-check-shortcut-name"), {
      target: { value: "Order check" },
    });
    fireEvent.click(screen.getByTestId("event-check-shortcut-save"));
    await waitFor(() =>
      expect(api.createSavedSearch).toHaveBeenCalledWith(
        "Order check",
        "EVENT_CHECK",
        expect.objectContaining({
          identifier_type: "CORRELATION_ID",
          identifier_value: "ORDER-1001",
          workspace_tab: "flow",
        }),
      ),
    );
  });
});

describe("Saved Results", () => {
  it("highlights only the exact Saved Results navigation item", async () => {
    renderSavedResultsPage();
    await screen.findByText("ORDER-1001");
    expect(screen.getByTestId("nav-saved-results")).toHaveClass("nav-active");
    expect(screen.getByTestId("nav-event-check")).not.toHaveClass("nav-active");
  });

  it("lists immutable snapshots and offers view create and join actions", async () => {
    renderSavedResults();
    expect(await screen.findByText("ORDER-1001")).toBeInTheDocument();
    expect(screen.getByText("DEVIATED")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "查看結果" })).toHaveAttribute(
      "href",
      expect.stringContaining(`snapshot_id=${snapshot.id}`),
    );
    expect(
      screen.getByRole("button", { name: "建立案件" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "加入案件" }),
    ).toBeInTheDocument();
  });

  it("sends identifier and status filters to the listing API", async () => {
    renderSavedResults();
    await screen.findByText("ORDER-1001");
    fireEvent.change(screen.getByTestId("saved-results-identifier"), {
      target: { value: "ORDER-1001" },
    });
    fireEvent.change(screen.getByTestId("saved-results-status"), {
      target: { value: "DEVIATED" },
    });
    fireEvent.click(screen.getByTestId("saved-results-search"));
    await waitFor(() =>
      expect(api.checkSnapshots).toHaveBeenLastCalledWith(
        { identifier: "ORDER-1001", check_status: "DEVIATED" },
        undefined,
        20,
      ),
    );
  });
});

describe("Check Models registry", () => {
  it("provides Flow, version and fixture scenario views from one registry read", async () => {
    renderModels();
    const row = await screen.findByRole("button", {
      name: /Order Fulfillment/,
    });
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    fireEvent.click(row);
    expect(
      await screen.findByRole("dialog", { name: "Order Fulfillment" }),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("tab", { name: "Test Scenarios" }));
    expect(await screen.findByText("happy-path")).toBeInTheDocument();
    fireEvent.click(
      screen.getByRole("button", { name: "關閉 Check Model 詳細資料" }),
    );
    await waitFor(() =>
      expect(screen.queryByRole("dialog")).not.toBeInTheDocument(),
    );
    fireEvent.click(screen.getByRole("tab", { name: "Global Checks" }));
    expect(await screen.findByText("Event Integrity")).toBeInTheDocument();
    expect(api.checkModels).toHaveBeenCalledTimes(1);
  });

  it("opens a migrated Pattern at its one canonical expectation", async () => {
    renderModels(
      "/check-models?kind=FLOW&model_id=order-fulfillment&version=2&panel=overview&focus=PAYMENT_REQUIRES_SHIPMENT&legacy_pattern_id=payment-completed-without-shipment",
    );
    const expectation = await screen.findByText("Payment requires shipment");
    expect(expectation.closest("article")).toHaveClass("focused");
    expect(screen.getByText(/唯一正式判定來源/)).toBeInTheDocument();
  });
});
