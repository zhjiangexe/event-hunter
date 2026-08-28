import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, useLocation, useNavigate } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  BusinessJourneyPage,
  DashboardPage,
  FeatureGuidePage,
  IngestionIssuesPage,
  InvestigationsPage,
  JourneyProfilesPage,
  PatternsPage,
  ScenarioLabPage,
  TimelinePage,
} from "./main";
import {
  api,
  type BusinessJourney,
  type EvidenceManifest,
  type Investigation,
  type InvestigationSummary,
  type InvestigationOverview,
  type IngestionIssue,
  type JourneyProfile,
  type Principal,
  type SavedSearch,
} from "./api";
import {
  scenarioApi,
  type ScenarioDefinition,
  type ScenarioRun,
} from "./scenario-api";

const principal: Principal = {
  subject: "tester",
  role: "INVESTIGATOR",
  permissions: [],
};

const investigation = (
  overrides: Partial<Investigation> = {},
): Investigation => ({
  id: "case-1",
  case_no: "EH-2026-0001",
  title: "付款後未出貨",
  severity: "HIGH",
  status: "OPEN",
  allowed_transitions: ["INVESTIGATING", "CLOSED"],
  correlation_id: "ORDER-2001",
  incident_from: "2026-08-20T11:00:00Z",
  incident_to: "2026-08-20T11:06:00Z",
  incident_window_source: "TIMELINE_SEARCH",
  assignee: "ops@example.com",
  priority: "P1",
  tags: ["payments", "shipping"],
  related_correlation_ids: ["PAYMENT-2001"],
  last_updated_by: "tester",
  sla_status: "ON_TRACK",
  sla_due_at: "2026-08-20T15:00:00Z",
  collaboration_notes: [],
  pattern_findings: [],
  evidence: [],
  lock_version: 1,
  created_at: "2026-08-20T11:00:00Z",
  updated_at: "2026-08-20T11:01:00Z",
  ...overrides,
});

const evidenceManifest = (): EvidenceManifest => ({
  schema_version: 1,
  investigation_id: "case-1",
  generated_at: "2026-08-20T11:01:00Z",
  query_window: {
    from: "2026-08-20T11:00:00Z",
    to: "2026-08-20T11:06:00Z",
  },
  partial: false,
  warnings: [],
  source_status: {
    postgres: "OK",
    clickhouse: "NOT_REQUESTED",
    technical_observability: "NOT_REQUESTED",
  },
  checksum_algorithm: "SHA-256",
  manifest_sha256: "a".repeat(64),
  items: [
    {
      id: "evidence-1",
      evidence_type: "PATTERN_FINDING",
      reference: "payment-completed-without-shipment:v1:ORDER-2001",
      source: "PATTERN_ENGINE",
      open_action: "PATTERN_LIBRARY",
      collected_at: "2026-08-20T11:01:00Z",
      checksum: "b".repeat(64),
    },
    {
      id: "evidence-alert",
      evidence_type: "GRAFANA_ALERT",
      reference: "receipt-1",
      source: "GRAFANA",
      open_action: "GRAFANA_ALERT",
      source_locator: "/alerting/grafana/event-quality-delay/view",
      source_org_id: 1,
      collected_at: "2026-08-20T11:06:00Z",
      checksum: "c".repeat(64),
    },
  ],
});

const investigationSummary = (
  overrides: Partial<InvestigationSummary> = {},
): InvestigationSummary => ({
  investigation_id: "case-1",
  generated_at: "2026-08-20T11:01:00Z",
  query_window: {
    from: "2026-08-20T11:00:00Z",
    to: "2026-08-20T11:06:00Z",
  },
  event_retention_boundary: "2026-05-22T11:01:00Z",
  case: investigation(),
  timeline: {
    correlation_id: "ORDER-2001",
    from: "2026-08-20T11:00:00Z",
    to: "2026-08-20T11:06:00Z",
    event_count: 0,
    truncated: false,
    events: [],
  },
  pattern_findings: [],
  evidence_references: [],
  audit_entries: [],
  partial: false,
  warnings: [],
  source_status: {
    postgres: "OK",
    clickhouse: "OK",
    technical_observability: "NOT_REQUESTED",
  },
  source_last_success_at: {
    postgres: "2026-08-20T11:01:00Z",
    clickhouse: "2026-08-20T11:01:00Z",
    technical_observability: null,
  },
  ...overrides,
});

function CurrentTestLocation() {
  const location = useLocation();
  return (
    <>
      <output data-testid="test-location">{location.pathname}</output>
      <output data-testid="test-search">{location.search}</output>
    </>
  );
}

function TestHistoryControls() {
  const navigate = useNavigate();
  return (
    <>
      <button data-testid="test-history-back" onClick={() => navigate(-1)}>
        Back
      </button>
      <button data-testid="test-history-forward" onClick={() => navigate(1)}>
        Forward
      </button>
    </>
  );
}

function renderPage(
  role: Principal["role"] = principal.role,
  entry = "/investigations",
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[entry]}>
        <InvestigationsPage principal={{ ...principal, role }} />
        <CurrentTestLocation />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function renderDashboardPage(role: Principal["role"] = principal.role) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/dashboard"]}>
        <DashboardPage principal={{ ...principal, role }} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function renderFeatureGuidePage(entry = "/guide") {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[entry]}>
        <FeatureGuidePage principal={principal} />
        <CurrentTestLocation />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function renderBusinessJourneyPage(
  entry = "/journey?correlation_id=ORDER-2001&from=2026-08-20T11%3A00%3A00Z&to=2026-08-20T11%3A06%3A00Z",
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[entry]}>
        <BusinessJourneyPage principal={principal} />
        <CurrentTestLocation />
        <TestHistoryControls />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function renderJourneyProfilesPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/journey-profiles"]}>
        <JourneyProfilesPage principal={principal} />
        <CurrentTestLocation />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function renderIngestionIssuesPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={["/ingestion-issues"]}>
        <IngestionIssuesPage principal={principal} />
        <CurrentTestLocation />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function renderTimelinePage(
  role: Principal["role"] = principal.role,
  entry = "/timeline?from=2026-08-20T11%3A00%3A00Z&to=2026-08-20T11%3A06%3A00Z",
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[entry]}>
        <TimelinePage principal={{ ...principal, role }} />
        <CurrentTestLocation />
        <TestHistoryControls />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function renderPatternsPage(
  role: Principal["role"] = principal.role,
  entry = "/patterns",
) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[entry]}>
        <PatternsPage principal={{ ...principal, role }} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function renderScenarioLabPage(role: Principal["role"] = principal.role) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <ScenarioLabPage principal={{ ...principal, role }} />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

afterEach(() => vi.restoreAllMocks());

describe("IngestionIssuesPage", () => {
  it("uses a bounded default query and opens a safe detail drawer", async () => {
    const issue: IngestionIssue = {
      id: "technical-1",
      kind: "TECHNICAL_DLQ",
      occurred_at: "2026-08-27T02:03:04Z",
      pipeline: "kafka-connect/clickhouse-sink",
      error_code: "CONNECTOR_TASK_FAILURE",
      event_id: null,
      event_type: null,
      correlation_id: null,
      source_topic: "order.events",
      source_partition: 1,
      source_offset: 42,
      dlq_topic: "event-hunter.poc-clickhouse-sink.dlq",
      dlq_partition: 0,
      dlq_offset: 5,
      payload_sha256: "a".repeat(64),
      admission_profile: null,
      connector_name: "event-hunter-poc-raw-landing",
      connector_task: 0,
      failure_stage: "VALUE_CONVERTER",
      exception_class: "org.example.BadValue",
    };
    const search = vi.spyOn(api, "ingestionIssues").mockResolvedValue({
      items: [issue],
      page_size: 20,
      next_cursor: null,
    });

    renderIngestionIssuesPage();

    expect(
      await screen.findByText("CONNECTOR_TASK_FAILURE"),
    ).toBeInTheDocument();
    const [filters] = search.mock.calls[0];
    expect(Date.parse(filters.to!) - Date.parse(filters.from!)).toBe(
      72 * 60 * 60_000,
    );
    fireEvent.click(screen.getByTestId("ingestion-issue-row-0"));
    const detail = screen.getByTestId("ingestion-issue-detail");
    expect(detail).toHaveTextContent("Payload SHA-256");
    expect(detail).toHaveTextContent("不保存 raw payload");
    expect(detail).not.toHaveTextContent("exception message content");
  });

  it("keeps cursor pagination client-side and applies explicit filters", async () => {
    const search = vi
      .spyOn(api, "ingestionIssues")
      .mockResolvedValueOnce({
        items: [],
        page_size: 20,
        next_cursor: "next-page",
      })
      .mockResolvedValue({ items: [], page_size: 20, next_cursor: null });
    renderIngestionIssuesPage();
    await waitFor(() => expect(search).toHaveBeenCalledTimes(1));

    const nextButton = screen.getByRole("button", { name: "下一頁" });
    await waitFor(() => expect(nextButton).toBeEnabled());
    fireEvent.click(nextButton);
    await waitFor(() =>
      expect(search).toHaveBeenCalledWith(expect.any(Object), "next-page", 20),
    );

    fireEvent.change(screen.getByTestId("ingestion-issue-kind"), {
      target: { value: "ADMISSION_QUARANTINE" },
    });
    fireEvent.click(screen.getByTestId("ingestion-issue-search"));
    await waitFor(() =>
      expect(search).toHaveBeenCalledWith(
        expect.objectContaining({ kind: "ADMISSION_QUARANTINE" }),
        undefined,
        20,
      ),
    );
  });
});

describe("FeatureGuidePage", () => {
  it("switches feature guidance through a shareable selector and opens the selected page", async () => {
    renderFeatureGuidePage();

    expect(screen.getByTestId("feature-guide-select")).toHaveValue(
      "getting-started",
    );
    expect(screen.getByTestId("feature-guide-title")).toHaveTextContent(
      "Getting Started & Integration",
    );
    expect(screen.getByText("8 篇導覽")).toBeInTheDocument();
    expect(screen.getByTestId("integration-guide")).toHaveTextContent(
      "Minimum",
    );
    expect(screen.getByTestId("integration-guide")).toHaveTextContent(
      "Normalization Adapter",
    );
    expect(screen.getByTestId("integration-guide")).toHaveTextContent(
      "eventId",
    );
    expect(screen.getByTestId("integration-guide")).toHaveTextContent(
      "不是 ClickHouse sink",
    );
    expect(screen.getByTestId("integration-quick-start")).toHaveTextContent(
      "先回答三個問題",
    );
    expect(screen.getByTestId("integration-change-cases")).toHaveTextContent(
      "既有 canonical topic ＋新 event type",
    );
    expect(screen.getByTestId("integration-glossary")).toHaveTextContent(
      "事件信箱／頻道",
    );
    expect(screen.getByTestId("integration-no-data")).toHaveTextContent(
      "照這個順序查",
    );
    expect(screen.getByTestId("integration-data-plane")).toHaveTextContent(
      "Event Check 不直接從 Kafka",
    );
    expect(screen.getByTestId("integration-admission-gates")).toHaveTextContent(
      "五關都通過",
    );
    expect(screen.getByTestId("integration-runbook")).toHaveTextContent(
      "從接入決策到正式交接",
    );
    expect(
      screen.getByTestId("integration-runbook-step-scope"),
    ).toHaveAttribute("open");
    expect(screen.getByTestId("integration-failure-modes")).toHaveTextContent(
      "格式正確但語意錯誤",
    );
    expect(screen.getByTestId("integration-commands")).toHaveTextContent(
      "bash scripts/verify-event-pipeline-readiness.sh",
    );
    expect(screen.getByText("目前仍缺少")).toBeInTheDocument();

    fireEvent.change(screen.getByTestId("feature-guide-select"), {
      target: { value: "check-models" },
    });

    expect(await screen.findByTestId("feature-guide-title")).toHaveTextContent(
      "Check Models",
    );
    expect(screen.getByText(/唯一正式判定來源/)).toBeInTheDocument();

    fireEvent.change(screen.getByTestId("feature-guide-select"), {
      target: { value: "investigations" },
    });

    expect(await screen.findByTestId("feature-guide-title")).toHaveTextContent(
      "Investigation Cases",
    );
    expect(screen.getByTestId("feature-guide-question")).toHaveTextContent(
      "這個問題由誰處理",
    );

    fireEvent.click(screen.getByTestId("feature-guide-open"));
    expect(screen.getByTestId("test-location")).toHaveTextContent(
      "/investigations",
    );
  });

  it("opens a feature-specific introduction directly from the URL", () => {
    renderFeatureGuidePage("/guide?feature=event-check");

    expect(screen.getByTestId("feature-guide-select")).toHaveValue(
      "event-check",
    );
    expect(screen.getByTestId("feature-guide-title")).toHaveTextContent(
      "Event Check",
    );
    expect(
      screen.getByText(/Snapshot 固定 Model checksum/),
    ).toBeInTheDocument();
  });
});

describe("DashboardPage", () => {
  const overview: InvestigationOverview = {
    generated_at: "2026-08-22T12:00:00Z",
    window: {
      from: "2026-08-19T12:00:00Z",
      to: "2026-08-22T12:00:00Z",
    },
    partial: false,
    warnings: [],
    control_plane: {
      cases: { open: 4, investigating: 2, closed: 7 },
      severity: { low: 1, medium: 1, high: 3, critical: 1 },
      activity: {
        cases_created: 3,
        cases_closed: 1,
        grafana_alerts: 2,
        scenario_passed: 8,
        scenario_failed: 1,
        scenario_timed_out: 0,
      },
      top_patterns: [{ key: "payment-completed-without-shipment", count: 5 }],
    },
    events: {
      event_count: 18,
      latest_event_at: "2026-08-22T11:59:00Z",
      latest_processing_attempt_at: "2026-08-22T11:59:01Z",
      top_producers: [{ key: "order-service", count: 9 }],
      top_event_types: [{ key: "OrderCreated", count: 6 }],
    },
    sources: [
      {
        name: "postgresql",
        state: "fresh",
        last_success_at: "2026-08-22T12:00:00Z",
        lag_ms: null,
        reason: null,
      },
      {
        name: "clickhouse",
        state: "fresh",
        last_success_at: "2026-08-22T12:00:00Z",
        lag_ms: 60_000,
        reason: null,
      },
    ],
  };

  it("renders backend aggregate counts and filter-preserving links", async () => {
    vi.spyOn(api, "overview").mockResolvedValue(overview);

    renderDashboardPage();

    expect(await screen.findByTestId("overview-open-cases")).toHaveTextContent(
      "4",
    );
    expect(screen.getByTestId("overview-event-count")).toHaveTextContent(
      "Events / 72h",
    );
    expect(screen.getByText("近 72 小時")).toBeInTheDocument();
    expect(screen.getByTestId("overview-open-cases")).toHaveAttribute(
      "href",
      "/investigations?status=OPEN",
    );
    expect(screen.getByText("order-service").closest("a")).toHaveAttribute(
      "href",
      expect.stringContaining("producer=order-service"),
    );
    expect(
      screen.getByText("payment-completed-without-shipment").closest("a"),
    ).toHaveAttribute("href", expect.stringContaining("/check-models?"));
    expect(screen.getByText("CRITICAL").closest("a")).toHaveAttribute(
      "href",
      "/investigations?severity=CRITICAL",
    );
  });

  it("shows unavailable instead of zero for a failed aggregate source", async () => {
    vi.spyOn(api, "overview").mockResolvedValue({
      ...overview,
      partial: true,
      warnings: ["POSTGRES_OVERVIEW_UNAVAILABLE"],
      control_plane: null,
      sources: [
        {
          name: "postgresql",
          state: "unavailable",
          last_success_at: null,
          lag_ms: null,
          reason: "POSTGRES_QUERY_FAILED",
        },
      ],
    });

    renderDashboardPage();

    expect(
      await screen.findByTestId("overview-partial-warning"),
    ).toBeInTheDocument();
    expect(screen.getByTestId("overview-open-cases")).toHaveTextContent(
      "不可用",
    );
    expect(screen.getByTestId("source-postgresql")).toHaveTextContent(
      "unavailable",
    );
  });

  it("returns identifier candidates before running any event query", async () => {
    vi.spyOn(api, "overview").mockResolvedValue(overview);
    const identify = vi.spyOn(api, "identifySearchInput").mockResolvedValue({
      input: "ORDER-2001",
      normalized_input: "ORDER-2001",
      status: "AMBIGUOUS",
      candidates: [
        {
          identifier_type: "CORRELATION_ID",
          query_parameter: "correlation_id",
          certainty: "CANDIDATE",
          reason: "OPAQUE_IDENTIFIER_FORMAT",
        },
        {
          identifier_type: "AGGREGATE_ID",
          query_parameter: "aggregate_id",
          certainty: "CANDIDATE",
          reason: "OPAQUE_IDENTIFIER_FORMAT",
        },
      ],
      message: "SELECT_IDENTIFIER_TYPE",
    });
    const search = vi.spyOn(api, "searchEvents");

    renderDashboardPage();
    fireEvent.change(screen.getByTestId("smart-search-input"), {
      target: { value: "ORDER-2001" },
    });
    fireEvent.click(screen.getByTestId("smart-search-identify"));

    expect(
      await screen.findByTestId("smart-search-candidate-correlation_id"),
    ).toHaveTextContent("Correlation ID");
    expect(identify).toHaveBeenCalledWith("ORDER-2001");
    expect(search).not.toHaveBeenCalled();
  });
});

describe("BusinessJourneyPage", () => {
  const journey: BusinessJourney = {
    correlation_id: "ORDER-2001",
    profile_id: "order-fulfillment",
    profile_version: 1,
    profile_title: "Order Fulfillment",
    from: "2026-08-20T11:00:00Z",
    to: "2026-08-20T11:06:00Z",
    status: "IN_PROGRESS",
    event_count: 2,
    completed_milestone_count: 1,
    total_milestone_count: 2,
    current_milestone_id: "SHIPPING",
    next_milestone_id: null,
    next_expected_event_types: ["ShipmentCreated"],
    trace_ids: ["4".repeat(32)],
    started_at: "2026-08-20T11:00:00Z",
    ended_at: "2026-08-20T11:00:30Z",
    duration_ms: 30_000,
    unmapped_event_count: 0,
    anomalies: [
      {
        code: "MISSING_SHIPMENT_AFTER_PAYMENT",
        severity: "HIGH",
        message: "付款完成超過 5 分鐘，仍找不到 ShipmentCreated。",
        event_ids: ["evt-payment-2001-001"],
      },
    ],
    milestones: [
      {
        id: "ORDER",
        label: "訂單建立",
        state: "COMPLETED",
        expected_event_types: ["OrderCreated", "OrderCancelled"],
        actual_event_types: ["OrderCreated"],
        first_event_at: "2026-08-20T11:00:00Z",
        duration_from_previous_ms: null,
        events: [
          {
            event_id: "evt-order-2001-001",
            event_type: "OrderCreated",
            occurred_at: "2026-08-20T11:00:00Z",
            producer: "order-service",
            aggregate_type: "Order",
            aggregate_id: "ORDER-2001",
            trace_id: "4".repeat(32),
          },
        ],
      },
      {
        id: "SHIPPING",
        label: "出貨建立與派送",
        state: "IN_PROGRESS",
        expected_event_types: ["ShipmentCreated"],
        actual_event_types: [],
        first_event_at: null,
        duration_from_previous_ms: null,
        events: [],
      },
    ],
  };

  it("uses the current rolling 72-hour window on an empty Journey route", () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(new Date("2026-08-26T12:00:00Z"));
      renderBusinessJourneyPage("/journey");

      expect(
        new Date(
          (screen.getByTestId("journey-from") as HTMLInputElement).value,
        ).toISOString(),
      ).toBe("2026-08-23T12:00:00.000Z");
      expect(
        new Date(
          (screen.getByTestId("journey-to") as HTMLInputElement).value,
        ).toISOString(),
      ).toBe("2026-08-26T12:00:00.000Z");
      expect(screen.getByText(/時區 .*時間範圍最多 7 天/)).toBeInTheDocument();
    } finally {
      vi.useRealTimers();
    }
  });

  it("renders server-derived milestones and anomaly without deriving journey state in the browser", async () => {
    const getJourney = vi
      .spyOn(api, "businessJourney")
      .mockResolvedValue(journey);

    renderBusinessJourneyPage();

    expect(await screen.findByTestId("journey-status")).toHaveTextContent(
      "IN_PROGRESS",
    );
    expect(screen.getByTestId("journey-event-count")).toHaveTextContent("2");
    expect(screen.getByTestId("journey-profile")).toHaveTextContent(
      "Order Fulfillment · order-fulfillment@v1",
    );
    expect(screen.getByTestId("journey-anomalies")).toHaveTextContent(
      "MISSING_SHIPMENT_AFTER_PAYMENT",
    );
    expect(screen.getByTestId("journey-milestone-order")).toHaveTextContent(
      "OrderCreated",
    );
    expect(screen.getByTestId("journey-milestone-shipping")).toHaveTextContent(
      "前置事件已使此里程碑進入進行中",
    );
    expect(screen.getByTestId("journey-state-help")).toHaveTextContent(
      "狀態由 Profile 依整條事件集合推導",
    );
    expect(screen.getByTestId("journey-query-window")).toHaveTextContent(
      "查詢窗口",
    );
    expect(screen.getByTestId("journey-query-window")).toHaveTextContent(
      "時區",
    );
    expect(getJourney).toHaveBeenCalledWith(
      "ORDER-2001",
      "2026-08-20T11:00:00Z",
      "2026-08-20T11:06:00Z",
    );
  });

  it("keeps the submitted Journey query in browser history and restores the form", async () => {
    vi.spyOn(api, "businessJourney").mockResolvedValue(journey);
    renderBusinessJourneyPage();

    await screen.findByTestId("journey-results");
    fireEvent.change(screen.getByTestId("journey-correlation-id"), {
      target: { value: "ORDER-4002" },
    });
    fireEvent.click(screen.getByTestId("journey-search-submit"));

    await waitFor(() =>
      expect(screen.getByTestId("test-search")).toHaveTextContent(
        "correlation_id=ORDER-4002",
      ),
    );
    fireEvent.click(screen.getByTestId("test-history-back"));
    await waitFor(() =>
      expect(screen.getByTestId("journey-correlation-id")).toHaveValue(
        "ORDER-2001",
      ),
    );
    fireEvent.click(screen.getByTestId("test-history-forward"));
    await waitFor(() =>
      expect(screen.getByTestId("journey-correlation-id")).toHaveValue(
        "ORDER-4002",
      ),
    );
  });
});

describe("JourneyProfilesPage", () => {
  const profile: JourneyProfile = {
    contract_version: 1,
    id: "order-fulfillment",
    version: 1,
    status: "active",
    default: true,
    title: "Order Fulfillment",
    description: "物流訂單流程",
    source_path: "contracts/journeys/order-fulfillment.yaml",
    checksum: "a".repeat(64),
    journey_state_rules: [
      {
        state: "COMPLETED",
        when_any_event_types: ["ShipmentDelivered"],
      },
    ],
    milestones: [
      {
        id: "ORDER",
        label: "訂單建立",
        expected_event_types: ["OrderCreated"],
        state_rules: [
          { state: "COMPLETED", when_any_event_types: ["OrderCreated"] },
        ],
      },
    ],
    anomaly_rules: [
      {
        code: "MISSING_ORDER_CREATED",
        severity: "HIGH",
        message: "付款事件存在，但找不到前置 OrderCreated。",
        trigger_event_types: ["PaymentCompleted"],
        required_any_event_types: ["OrderCreated"],
        evidence_event_types: ["PaymentCompleted"],
        grace_period_seconds: 0,
      },
    ],
    data_quality: { detect_duplicate_event_ids: true },
  };

  it("lists profiles and opens the selected YAML definition in a detail drawer", async () => {
    vi.spyOn(api, "journeyProfiles").mockResolvedValue({ items: [profile] });

    renderJourneyProfilesPage();

    expect(
      await screen.findByTestId("journey-profile-order-fulfillment"),
    ).toHaveTextContent("Order Fulfillment");
    expect(screen.getByTestId("profile-count")).toHaveTextContent("1");
    expect(screen.queryByText(profile.source_path)).not.toBeInTheDocument();
    expect(screen.queryByText("MISSING_ORDER_CREATED")).not.toBeInTheDocument();
    expect(screen.getByTestId("journey-profile-boundary")).toHaveTextContent(
      "指定 Journey 查詢版本、審核與發布",
    );

    fireEvent.click(screen.getByTestId("journey-profile-order-fulfillment"));

    expect(
      screen.getByRole("dialog", { name: "Journey Profile 詳細" }),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId("journey-profile-detail-order-fulfillment"),
    ).toHaveTextContent("MISSING_ORDER_CREATED");
    expect(screen.getByText(profile.source_path)).toBeInTheDocument();
    expect(screen.getByTestId("journey-profile-detail-close")).toHaveFocus();

    fireEvent.click(screen.getByTestId("journey-profile-detail-close"));
    expect(
      screen.queryByTestId("journey-profile-detail"),
    ).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId("profiles-open-journey"));
    expect(screen.getByTestId("test-location")).toHaveTextContent("/journey");
  });
});

describe("ScenarioLabPage", () => {
  const scenario: ScenarioDefinition = {
    id: "S8",
    name: "PAYMENT_FAILED_AND_CANCELLED",
    title: "付款失敗並取消",
    category: "LOGISTICS",
    description: "實際送出隔離事件並回查結果。",
    execution_mode: "LAB_INJECTION",
    synthetic: true,
    expected_event_types: ["OrderCreated", "PaymentFailed", "OrderCancelled"],
    expected_results: ["事件依序入庫"],
  };
  const accepted: ScenarioRun = {
    run_id: "4bd00d41-8b30-476b-82e8-e0474419f7f7",
    scenario,
    correlation_id: "LAB-S8-TEST",
    trace_id: null,
    status: "ACCEPTED",
    execution_mode: "LAB_INJECTION",
    synthetic: true,
    expected_event_types: scenario.expected_event_types,
    actual: {
      trace_id: null,
      event_count: 0,
      event_types: [],
      duplicate_event_ids: [],
      out_of_order: false,
      processing_statuses: [],
      ingestion_failure_count: 0,
      ingestion_failure_types: [],
      max_event_delay_ms: 0,
    },
    checks: [],
    links: {
      timeline: "/timeline?correlation_id=LAB-S8-TEST",
      grafana: "http://localhost:28300/explore",
      tempo: null,
      loki: "http://localhost:28300/explore?correlation_id=LAB-S8-TEST",
    },
    error: null,
    accepted_at: "2026-08-21T00:00:00Z",
    started_at: null,
    completed_at: null,
    duration_ms: null,
    current_step: "等待執行",
  };

  beforeEach(() => {
    vi.spyOn(scenarioApi, "history").mockResolvedValue({ items: [] });
  });

  it("shows identifiers from the single start request without polling", async () => {
    vi.spyOn(scenarioApi, "catalog").mockResolvedValue({ items: [scenario] });
    const run = {
      ...accepted,
      trace_id: "a".repeat(32),
      actual: {
        ...accepted.actual,
        trace_id: "a".repeat(32),
      },
      links: {
        ...accepted.links,
        tempo: "http://localhost:28300/explore?trace_id=test",
      },
    };
    vi.spyOn(scenarioApi, "start").mockResolvedValue(run);
    const runRequest = vi.spyOn(scenarioApi, "run");

    renderScenarioLabPage();
    expect(await screen.findByText("付款失敗並取消")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("run-scenario-s8"));

    await waitFor(() =>
      expect(screen.getByTestId("scenario-run-status")).toHaveTextContent(
        "ACCEPTED",
      ),
    );
    expect(screen.getByTestId("scenario-run-modal")).toHaveAttribute(
      "aria-modal",
      "true",
    );
    expect(screen.getByTestId("scenario-correlation-id")).toHaveTextContent(
      accepted.correlation_id,
    );
    expect(screen.queryByTestId("scenario-trace-id")).not.toBeInTheDocument();
    expect(screen.queryByTestId("scenario-link-tempo")).not.toBeInTheDocument();
    expect(screen.getByTestId("scenario-link-timeline")).toHaveAttribute(
      "href",
      accepted.links.timeline,
    );
    expect(runRequest).not.toHaveBeenCalled();
  });

  it("shows accepted identifiers before domain events create Event IDs", async () => {
    vi.spyOn(scenarioApi, "catalog").mockResolvedValue({ items: [scenario] });
    vi.spyOn(scenarioApi, "start").mockResolvedValue(accepted);
    const runRequest = vi.spyOn(scenarioApi, "run");

    renderScenarioLabPage();
    fireEvent.click(await screen.findByTestId("run-scenario-s8"));

    const dialog = await screen.findByRole("dialog", {
      name: "S8 · 付款失敗並取消",
    });
    expect(dialog).toHaveTextContent(accepted.run_id);
    expect(dialog).toHaveTextContent(accepted.correlation_id);
    expect(dialog).not.toHaveTextContent("Trace ID");
    expect(
      screen.queryByTestId("scenario-event-id-note"),
    ).not.toBeInTheDocument();
    expect(runRequest).not.toHaveBeenCalled();

    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(screen.queryByTestId("scenario-run-modal")).not.toBeInTheDocument();
  });

  it("reopens a persisted completed run from manually loaded history", async () => {
    const completed: ScenarioRun = {
      ...accepted,
      status: "PASSED",
      trace_id: "b".repeat(32),
      actual: {
        ...accepted.actual,
        trace_id: "b".repeat(32),
        event_count: 3,
        event_types: scenario.expected_event_types,
      },
      links: {
        ...accepted.links,
        tempo: "http://localhost:28300/explore?trace_id=completed",
      },
      started_at: "2026-08-21T00:00:00.100Z",
      completed_at: "2026-08-21T00:00:01Z",
      duration_ms: 900,
      current_step: "驗收通過",
    };
    vi.mocked(scenarioApi.history).mockResolvedValue({ items: [completed] });
    vi.spyOn(scenarioApi, "catalog").mockResolvedValue({ items: [scenario] });
    const runRequest = vi.spyOn(scenarioApi, "run");

    renderScenarioLabPage();

    fireEvent.click(
      await screen.findByTestId(`scenario-history-run-${completed.run_id}`),
    );
    const dialog = await screen.findByRole("dialog", {
      name: "S8 · 付款失敗並取消",
    });
    expect(dialog).toHaveTextContent("PASSED");
    expect(dialog).toHaveTextContent(completed.trace_id!);
    expect(screen.getByTestId("scenario-link-tempo")).toHaveAttribute(
      "href",
      completed.links.tempo,
    );
    expect(runRequest).not.toHaveBeenCalled();
  });

  it("keeps Viewer access read-only", async () => {
    vi.spyOn(scenarioApi, "catalog").mockResolvedValue({ items: [scenario] });

    renderScenarioLabPage("VIEWER");

    expect(await screen.findByTestId("run-scenario-s8")).toBeDisabled();
  });
});

describe("QueryShortcutsDrawer", () => {
  const savedSearch: SavedSearch = {
    id: "a11883b0-67f1-4fc2-832f-87d11173041f",
    owner_subject: "tester",
    name: "付款失敗追蹤",
    target: "TIMELINE",
    query: {
      from: "2026-08-20T11:00:00Z",
      to: "2026-08-20T11:06:00Z",
      event_type: "PaymentFailed",
      include_processing_attempts: true,
    },
    open_url:
      "/timeline?event_type=PaymentFailed&from=2026-08-20T11%3A00%3A00Z&to=2026-08-20T11%3A06%3A00Z",
    created_at: "2026-08-22T11:00:00Z",
    updated_at: "2026-08-22T11:00:00Z",
  };

  it("loads built-in presets and the personal list from Timeline", async () => {
    const personal = vi
      .spyOn(api, "savedSearches")
      .mockResolvedValue({ items: [savedSearch] });
    const presets = vi.spyOn(api, "searchPresets").mockResolvedValue({
      items: [
        {
          id: "payment-failed-72h",
          name: "最近付款失敗",
          description: "最近 72 小時的 PaymentFailed events。",
          open_url: "/timeline?event_type=PaymentFailed",
        },
      ],
    });

    renderTimelinePage();
    fireEvent.click(screen.getByTestId("query-shortcuts-open"));

    expect(personal).toHaveBeenCalledTimes(1);
    expect(presets).toHaveBeenCalledTimes(1);
    expect(await screen.findByTestId("query-shortcuts-drawer")).toHaveAttribute(
      "aria-modal",
      "true",
    );
    expect(
      await screen.findByTestId("search-preset-payment-failed-72h"),
    ).toHaveAttribute("href", "/timeline?event_type=PaymentFailed");
    expect(screen.getByTestId("saved-search-row-0")).toHaveTextContent(
      "付款失敗追蹤",
    );
    expect(screen.getByTestId("saved-search-open-0")).toHaveAttribute(
      "href",
      savedSearch.open_url,
    );
  });

  it("deletes only the owner's item and refreshes the personal cache", async () => {
    vi.spyOn(api, "searchPresets").mockResolvedValue({ items: [] });
    const personal = vi
      .spyOn(api, "savedSearches")
      .mockResolvedValueOnce({ items: [savedSearch] })
      .mockResolvedValue({ items: [] });
    const remove = vi
      .spyOn(api, "deleteSavedSearch")
      .mockResolvedValue(undefined);
    vi.spyOn(window, "confirm").mockReturnValue(true);

    renderTimelinePage("VIEWER");
    fireEvent.click(screen.getByTestId("query-shortcuts-open"));
    fireEvent.click(await screen.findByTestId("saved-search-delete-0"));

    await waitFor(() => expect(remove).toHaveBeenCalledWith(savedSearch.id));
    await waitFor(() => expect(personal).toHaveBeenCalledTimes(2));
    expect(screen.queryByTestId("saved-search-row-0")).not.toBeInTheDocument();
  });
});

describe("TimelinePage", () => {
  it("preserves case-link seconds when the query is submitted again", async () => {
    vi.spyOn(api, "searchEvents").mockResolvedValue({
      events: [],
      count: 0,
      truncated: false,
    });
    renderTimelinePage(
      principal.role,
      "/timeline?correlation_id=ORDER-2001&from=2026-08-20T11%3A00%3A10Z&to=2026-08-20T11%3A06%3A10Z",
    );

    const from = screen.getByTestId("timeline-from") as HTMLInputElement;
    const to = screen.getByTestId("timeline-to") as HTMLInputElement;
    expect(from).toHaveAttribute("step", "1");
    expect(to).toHaveAttribute("step", "1");
    expect(from.value).toMatch(/:10(?:\.000)?$/);
    expect(to.value).toMatch(/:10(?:\.000)?$/);

    fireEvent.click(screen.getByTestId("timeline-search-submit"));
    await waitFor(() => {
      const search = new URLSearchParams(
        screen.getByTestId("test-search").textContent ?? "",
      );
      expect(search.get("from")).toBe("2026-08-20T11:00:10.000Z");
      expect(search.get("to")).toBe("2026-08-20T11:06:10.000Z");
    });
  });

  it("uses a controllable now-minus-72-hours default and refreshes it on clear", () => {
    vi.useFakeTimers();
    try {
      vi.setSystemTime(new Date("2026-08-26T12:00:00Z"));
      renderTimelinePage(principal.role, "/timeline");

      expect(
        new Date(
          (screen.getByTestId("timeline-from") as HTMLInputElement).value,
        ).toISOString(),
      ).toBe("2026-08-23T12:00:00.000Z");
      expect(
        new Date(
          (screen.getByTestId("timeline-to") as HTMLInputElement).value,
        ).toISOString(),
      ).toBe("2026-08-26T12:00:00.000Z");
      expect(screen.getByText(/時區 .*時間範圍最多 7 天/)).toBeInTheDocument();

      vi.setSystemTime(new Date("2026-08-26T13:00:00Z"));
      fireEvent.click(screen.getByRole("button", { name: "清除" }));
      expect(
        new Date(
          (screen.getByTestId("timeline-from") as HTMLInputElement).value,
        ).toISOString(),
      ).toBe("2026-08-23T13:00:00.000Z");
      expect(
        new Date(
          (screen.getByTestId("timeline-to") as HTMLInputElement).value,
        ).toISOString(),
      ).toBe("2026-08-26T13:00:00.000Z");
    } finally {
      vi.useRealTimers();
    }
  });

  it("serializes submitted filters and flags into a back-forward-safe URL", async () => {
    const search = vi.spyOn(api, "searchEvents").mockResolvedValue({
      events: [],
      count: 0,
      truncated: false,
    });
    renderTimelinePage("ADMIN");

    fireEvent.change(screen.getByTestId("timeline-correlation-id"), {
      target: { value: "ORDER-2001" },
    });
    fireEvent.click(screen.getByRole("button", { name: "進階條件" }));
    fireEvent.change(screen.getByTestId("timeline-event-type-filter"), {
      target: { value: "PaymentCompleted" },
    });
    fireEvent.click(screen.getByTestId("timeline-include-payload"));
    fireEvent.click(screen.getByTestId("timeline-include-processing-attempts"));
    fireEvent.click(screen.getByTestId("timeline-search-submit"));

    await waitFor(() => expect(search).toHaveBeenCalledTimes(1));
    const firstSearch = new URLSearchParams(
      screen.getByTestId("test-search").textContent ?? "",
    );
    expect(firstSearch.get("correlation_id")).toBe("ORDER-2001");
    expect(firstSearch.get("event_type")).toBe("PaymentCompleted");
    expect(firstSearch.get("from")).toBe("2026-08-20T11:00:00.000Z");
    expect(firstSearch.get("to")).toBe("2026-08-20T11:06:00.000Z");
    expect(firstSearch.get("include_payload")).toBe("true");
    expect(firstSearch.get("include_processing_attempts")).toBe("false");
    expect(
      await screen.findByTestId("timeline-query-window"),
    ).toHaveTextContent("查詢窗口");

    fireEvent.change(screen.getByTestId("timeline-correlation-id"), {
      target: { value: "ORDER-4002" },
    });
    fireEvent.click(screen.getByTestId("timeline-search-submit"));
    await waitFor(() =>
      expect(screen.getByTestId("test-search")).toHaveTextContent(
        "correlation_id=ORDER-4002",
      ),
    );

    fireEvent.click(screen.getByTestId("test-history-back"));
    await waitFor(() =>
      expect(screen.getByTestId("timeline-correlation-id")).toHaveValue(
        "ORDER-2001",
      ),
    );
    expect(screen.getByTestId("timeline-include-payload")).toBeChecked();
    expect(
      screen.getByTestId("timeline-include-processing-attempts"),
    ).not.toBeChecked();

    fireEvent.click(screen.getByTestId("test-history-forward"));
    await waitFor(() =>
      expect(screen.getByTestId("timeline-correlation-id")).toHaveValue(
        "ORDER-4002",
      ),
    );
  });

  it("distinguishes a Pattern no-data result from a no-match", async () => {
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [investigation()],
      next_cursor: null,
    });
    vi.spyOn(api, "investigation").mockResolvedValue(investigation());
    vi.spyOn(api, "patterns").mockResolvedValue([]);
    const summary = vi
      .spyOn(api, "summary")
      .mockResolvedValue(investigationSummary());
    vi.spyOn(api, "analyze").mockResolvedValue({
      investigation_id: "case-1",
      execution_mode: "SYNC",
      analyzed_at: "2026-08-24T12:00:00Z",
      analysis_status: "NO_EVENTS",
      executed_pattern_ids: ["payment-completed-without-shipment"],
      effective_window: null,
      findings: [],
    });

    renderPage();
    fireEvent.click(
      await screen.findByRole("button", { name: /付款後未出貨/ }),
    );
    fireEvent.click(await screen.findByTestId("case-tab-patterns"));
    fireEvent.click(await screen.findByTestId("run-case-pattern-analysis"));

    expect(
      await screen.findByTestId("case-pattern-analysis-status"),
    ).toHaveTextContent("資料不足：沒有 canonical event");
    expect(
      screen.getByTestId("case-pattern-analysis-result"),
    ).toHaveTextContent("不能判定 Pattern 是否命中");
    await waitFor(() => expect(summary).toHaveBeenCalledTimes(2));
  });

  it("labels an evaluated empty Pattern result as no-match", async () => {
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [investigation()],
      next_cursor: null,
    });
    vi.spyOn(api, "investigation").mockResolvedValue(investigation());
    vi.spyOn(api, "patterns").mockResolvedValue([]);
    vi.spyOn(api, "summary").mockResolvedValue(investigationSummary());
    vi.spyOn(api, "analyze").mockResolvedValue({
      investigation_id: "case-1",
      execution_mode: "SYNC",
      analyzed_at: "2026-08-24T12:00:00Z",
      analysis_status: "EVALUATED",
      executed_pattern_ids: ["payment-completed-without-shipment"],
      effective_window: {
        from: "2026-08-20T11:00:00Z",
        to: "2026-08-27T11:00:00Z",
        observed_at: "2026-08-24T12:00:00Z",
        anchor: "EARLIEST_CORRELATION_EVENT",
        source_event_count: 2,
      },
      findings: [],
    });

    renderPage();
    fireEvent.click(
      await screen.findByRole("button", { name: /付款後未出貨/ }),
    );
    fireEvent.click(await screen.findByTestId("case-tab-patterns"));
    fireEvent.click(await screen.findByTestId("run-case-pattern-analysis"));

    expect(
      await screen.findByTestId("case-pattern-analysis-status"),
    ).toHaveTextContent("已評估：未命中");
  });

  it("keeps Pattern source failure distinct from a successful old result", async () => {
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [investigation()],
      next_cursor: null,
    });
    vi.spyOn(api, "investigation").mockResolvedValue(investigation());
    vi.spyOn(api, "patterns").mockResolvedValue([]);
    vi.spyOn(api, "summary").mockResolvedValue(investigationSummary());
    vi.spyOn(api, "analyze").mockRejectedValue(
      new Error("PATTERN_SOURCE_UNAVAILABLE"),
    );

    renderPage();
    fireEvent.click(
      await screen.findByRole("button", { name: /付款後未出貨/ }),
    );
    fireEvent.click(await screen.findByTestId("case-tab-patterns"));
    fireEvent.click(await screen.findByTestId("run-case-pattern-analysis"));

    expect(
      await screen.findByTestId("case-pattern-analysis-error"),
    ).toHaveTextContent("事件來源目前不可用");
    expect(screen.getByTestId("case-pattern-analysis-error")).toHaveTextContent(
      "既有 Finding 不受影響",
    );
  });

  it("saves the active bounded query without the payload expansion flag", async () => {
    vi.spyOn(api, "savedSearches").mockResolvedValue({ items: [] });
    vi.spyOn(api, "searchPresets").mockResolvedValue({ items: [] });
    vi.spyOn(api, "searchEvents").mockResolvedValue({
      events: [],
      count: 0,
      truncated: false,
    });
    const create = vi.spyOn(api, "createSavedSearch").mockResolvedValue({
      id: "a11883b0-67f1-4fc2-832f-87d11173041f",
      owner_subject: "tester",
      name: "ORDER-2001 timeline",
      target: "TIMELINE",
      query: {
        from: "2026-08-20T11:00:00.000Z",
        to: "2026-08-20T11:06:00.000Z",
        correlation_id: "ORDER-2001",
        include_processing_attempts: true,
      },
      open_url: "/timeline?correlation_id=ORDER-2001",
      created_at: "2026-08-22T11:00:00Z",
      updated_at: "2026-08-22T11:00:00Z",
    });

    renderTimelinePage("ADMIN");
    fireEvent.change(screen.getByTestId("timeline-correlation-id"), {
      target: { value: "ORDER-2001" },
    });
    fireEvent.click(screen.getByRole("button", { name: "進階條件" }));
    fireEvent.click(screen.getByTestId("timeline-include-payload"));
    fireEvent.click(screen.getByTestId("timeline-search-submit"));
    fireEvent.click(await screen.findByTestId("query-shortcuts-open"));
    fireEvent.change(screen.getByTestId("saved-search-name"), {
      target: { value: "ORDER-2001 timeline" },
    });
    fireEvent.click(screen.getByTestId("save-search-submit"));

    await waitFor(() =>
      expect(create).toHaveBeenCalledWith(
        "ORDER-2001 timeline",
        "TIMELINE",
        expect.objectContaining({
          correlation_id: "ORDER-2001",
          include_processing_attempts: true,
        }),
      ),
    );
    expect(create.mock.calls[0][2]).not.toHaveProperty("include_payload");
    expect(await screen.findByTestId("saved-search-success")).toHaveTextContent(
      "ORDER-2001 timeline",
    );
  });

  it("saves a relative query window explicitly", async () => {
    vi.spyOn(api, "searchEvents").mockResolvedValue({
      events: [],
      count: 0,
      truncated: false,
    });
    vi.spyOn(api, "savedSearches").mockResolvedValue({ items: [] });
    vi.spyOn(api, "searchPresets").mockResolvedValue({ items: [] });
    const create = vi.spyOn(api, "createSavedSearch").mockResolvedValue({
      id: "a11883b0-67f1-4fc2-832f-87d11173041f",
      owner_subject: "tester",
      name: "最近一天訂單",
      target: "TIMELINE",
      query: {
        time_mode: "RELATIVE",
        relative_window_seconds: 86_400,
        from: "2026-08-20T11:00:00Z",
        to: "2026-08-20T11:06:00Z",
        correlation_id: "ORDER-2001",
        include_processing_attempts: true,
      },
      open_url: "/timeline?correlation_id=ORDER-2001",
      created_at: "2026-08-22T11:00:00Z",
      updated_at: "2026-08-22T11:00:00Z",
    });

    renderTimelinePage();
    fireEvent.change(screen.getByTestId("timeline-correlation-id"), {
      target: { value: "ORDER-2001" },
    });
    fireEvent.click(screen.getByTestId("timeline-search-submit"));
    fireEvent.click(screen.getByTestId("query-shortcuts-open"));
    fireEvent.change(screen.getByTestId("saved-search-name"), {
      target: { value: "最近一天訂單" },
    });
    fireEvent.change(screen.getByTestId("saved-search-time-mode"), {
      target: { value: "RELATIVE" },
    });
    expect(screen.getByTestId("saved-search-relative-window")).toHaveValue(
      "259200",
    );
    fireEvent.change(screen.getByTestId("saved-search-relative-window"), {
      target: { value: "86400" },
    });
    fireEvent.click(screen.getByTestId("save-search-submit"));

    await waitFor(() =>
      expect(create).toHaveBeenCalledWith(
        "最近一天訂單",
        "TIMELINE",
        expect.objectContaining({
          time_mode: "RELATIVE",
          relative_window_seconds: 86_400,
        }),
      ),
    );
  });

  it("searches by an alternate identifier with a bounded time window", async () => {
    const search = vi.spyOn(api, "searchEvents").mockResolvedValue({
      events: [],
      count: 0,
      truncated: false,
    });

    renderTimelinePage();
    fireEvent.change(screen.getByTestId("timeline-identifier-key"), {
      target: { value: "trace_id" },
    });
    fireEvent.change(screen.getByTestId("timeline-correlation-id"), {
      target: { value: "trace-500" },
    });
    fireEvent.click(screen.getByTestId("timeline-search-submit"));

    await waitFor(() =>
      expect(search).toHaveBeenCalledWith(
        expect.objectContaining({
          trace_id: "trace-500",
          from: "2026-08-20T11:00:00.000Z",
          to: "2026-08-20T11:06:00.000Z",
        }),
      ),
    );
    expect(
      await screen.findByText("找不到符合條件的事件。"),
    ).toBeInTheDocument();
    expect(
      screen.queryByTestId("create-investigation"),
    ).not.toBeInTheDocument();
  });

  it("supports advanced allowlist filters and rejects a window over seven days", async () => {
    const search = vi.spyOn(api, "searchEvents").mockResolvedValue({
      events: [],
      count: 0,
      truncated: false,
    });

    renderTimelinePage();
    fireEvent.click(screen.getByRole("button", { name: "進階條件" }));
    fireEvent.change(screen.getByTestId("timeline-event-type-filter"), {
      target: { value: "PaymentCompleted" },
    });
    fireEvent.change(screen.getByTestId("timeline-pattern-id-filter"), {
      target: { value: "payment-completed-without-shipment" },
    });
    fireEvent.change(screen.getByTestId("timeline-alert-id-filter"), {
      target: { value: "grafana-fingerprint-1" },
    });
    fireEvent.change(screen.getByTestId("timeline-severity-filter"), {
      target: { value: "HIGH" },
    });
    fireEvent.click(screen.getByTestId("timeline-search-submit"));
    await waitFor(() =>
      expect(search).toHaveBeenCalledWith(
        expect.objectContaining({
          event_type: "PaymentCompleted",
          pattern_id: "payment-completed-without-shipment",
          alert_id: "grafana-fingerprint-1",
          severity: "HIGH",
        }),
      ),
    );

    fireEvent.change(screen.getByTestId("timeline-to"), {
      target: { value: "2026-08-28T19:00" },
    });
    expect(screen.getByText("查詢時間範圍最多 7 天。")).toBeInTheDocument();
    expect(screen.getByTestId("timeline-search-submit")).toBeDisabled();
  });

  it("expands event metadata, processing, masked payload and observability links", async () => {
    vi.spyOn(api, "searchEvents").mockResolvedValue({
      count: 1,
      truncated: false,
      events: [
        {
          event_id: "evt-payment-1",
          event_type: "PaymentCompleted",
          event_version: 1,
          occurred_at: "2026-08-20T11:00:30Z",
          producer: "payment-service",
          correlation_id: "ORDER-2001",
          causation_id: "evt-order-1",
          trace_id: "55555555555555555555555555555555",
          aggregate_type: "Payment",
          aggregate_id: "PAYMENT-2001",
          sequence: 1,
          kafka_topic: "payment.events",
          kafka_partition: 0,
          kafka_offset: 42,
          service_version: "1.0.0",
          admission_status: "SEARCHABLE_WITH_WARNINGS",
          quality_flags: ["UNKNOWN_EVENT_VERSION"],
          admission_profile: "minimum-envelope-v1",
          ingested_at: "2026-08-20T11:00:31Z",
          payload: { amount: "[REDACTED_AMOUNT]" },
          processing_summary: {
            attempt_count: 2,
            final_status: "SUCCEEDED",
            consumer_groups: ["shipping-service"],
          },
        },
      ],
    });

    renderTimelinePage("ADMIN");
    fireEvent.change(screen.getByTestId("timeline-correlation-id"), {
      target: { value: "ORDER-2001" },
    });
    fireEvent.click(screen.getByRole("button", { name: "進階條件" }));
    fireEvent.click(screen.getByTestId("timeline-include-payload"));
    fireEvent.click(screen.getByTestId("timeline-search-submit"));
    const occurredAt = await screen.findByTestId(
      "timeline-event-0-occurred-at",
    );
    expect(occurredAt).toHaveAttribute("datetime", "2026-08-20T11:00:30Z");
    expect(occurredAt).toHaveTextContent("2026/08/20");
    expect(
      screen.getByTestId("timeline-event-0-quality-warning"),
    ).toHaveTextContent("可查詢・需注意");
    fireEvent.click(
      await screen.findByRole("button", { name: /PaymentCompleted/ }),
    );

    expect(screen.getByTestId("timeline-event-0-detail")).toBeInTheDocument();
    expect(screen.getByText("payment.events / p0 / o42")).toBeInTheDocument();
    expect(screen.getByText("UNKNOWN_EVENT_VERSION")).toBeInTheDocument();
    expect(screen.getByText("minimum-envelope-v1")).toBeInTheDocument();
    expect(screen.getByText(/attempts 2/)).toBeInTheDocument();
    expect(screen.getByText(/REDACTED_AMOUNT/)).toBeInTheDocument();
    const traceLink = screen.getByRole("link", { name: /Tempo trace/ });
    expect(traceLink).toHaveAttribute("target", "_blank");
    expect(traceLink).toHaveAttribute("rel", "noopener noreferrer");
    expect(screen.getByTestId("create-investigation")).toBeInTheDocument();
  });

  it("only exposes the payload expansion control to ADMIN", () => {
    const viewer = renderTimelinePage("VIEWER");
    fireEvent.click(screen.getByRole("button", { name: "進階條件" }));
    expect(
      screen.queryByTestId("timeline-include-payload"),
    ).not.toBeInTheDocument();

    viewer.unmount();
    renderTimelinePage("ADMIN");
    fireEvent.click(screen.getByRole("button", { name: "進階條件" }));
    expect(screen.getByTestId("timeline-include-payload")).toBeInTheDocument();
  });

  it("attaches a Timeline event to an open case and prioritizes the same correlation", async () => {
    const event = {
      event_id: "evt-payment-1",
      event_type: "PaymentCompleted",
      event_version: 1,
      occurred_at: "2026-08-20T11:00:30Z",
      producer: "payment-service",
      correlation_id: "ORDER-2001",
      causation_id: "evt-order-1",
      trace_id: "55555555555555555555555555555555",
      aggregate_type: "Payment",
      aggregate_id: "PAYMENT-2001",
      sequence: 1,
      kafka_topic: "payment.events",
      kafka_partition: 0,
      kafka_offset: 42,
      service_version: "1.0.0",
      admission_status: "SEARCHABLE" as const,
      quality_flags: [],
      admission_profile: "domain-event-json-schema-v1",
      ingested_at: "2026-08-20T11:00:31Z",
    };
    vi.spyOn(api, "searchEvents").mockResolvedValue({
      count: 1,
      truncated: false,
      events: [event],
    });
    const listCases = vi.spyOn(api, "investigations").mockResolvedValue({
      items: [
        investigation({
          id: "case-other",
          case_no: "EH-2026-0002",
          title: "其他案件",
          correlation_id: "ORDER-9999",
        }),
        investigation({ title: "同一筆訂單案件" }),
        investigation({
          id: "case-closed",
          case_no: "EH-2026-0003",
          title: "已結案案件",
          status: "CLOSED",
        }),
      ],
      next_cursor: null,
    });
    const attach = vi.spyOn(api, "attachInvestigationEvent").mockResolvedValue({
      investigation: investigation({
        related_correlation_ids: ["PAYMENT-2001"],
        lock_version: 2,
      }),
      evidence: {
        id: "evidence-1",
        evidence_type: "EVENT",
        reference: event.event_id,
        source: "CLICKHOUSE",
        open_action: "GRAFANA_EVENT",
        collected_at: "2026-08-20T11:02:00Z",
        checksum: "a".repeat(64),
      },
      attached: true,
    });

    renderTimelinePage();
    fireEvent.change(screen.getByTestId("timeline-correlation-id"), {
      target: { value: "ORDER-2001" },
    });
    fireEvent.click(screen.getByTestId("timeline-search-submit"));
    fireEvent.click(
      await screen.findByRole("button", { name: /PaymentCompleted/ }),
    );

    expect(listCases).not.toHaveBeenCalled();
    fireEvent.click(screen.getByTestId("timeline-event-0-attach"));
    expect(
      await screen.findByTestId("event-attachment-modal"),
    ).toHaveTextContent(event.event_id);
    expect(
      await screen.findByTestId("event-attachment-case-0"),
    ).toHaveTextContent("同一筆訂單案件");
    expect(screen.queryByText("已結案案件")).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId("event-attachment-case-0"));
    await waitFor(() =>
      expect(attach).toHaveBeenCalledWith(
        expect.objectContaining({ id: "case-1", lock_version: 1 }),
        event.event_id,
        "2026-08-20T11:00:00.000Z",
        "2026-08-20T11:06:00.000Z",
      ),
    );
    expect(
      await screen.findByTestId("event-attachment-success"),
    ).toHaveTextContent("Event evidence 已加入");
  });
});

describe("PatternsPage", () => {
  it("renders only registered immutable Pattern metadata", async () => {
    vi.spyOn(api, "patterns").mockResolvedValue([
      {
        id: "payment-completed-without-shipment",
        version: 1,
        name: "Payment completed without shipment",
        condition:
          "Detect a paid order that has no shipment within the fixed investigation window.",
        severity: "HIGH",
        window: "PT5M",
        required_event_types: ["PaymentCompleted"],
        expected_event_types: ["ShipmentCreated"],
        exclusion_event_types: [
          "OrderCancelled",
          "PaymentRefunded",
          "PaymentVoided",
        ],
        evidence_query_template_id: "events.by_correlation.v1",
        status: "ACTIVE",
        mutable_at_runtime: false,
        source_path:
          "contracts/patterns/payment-completed-without-shipment.yaml",
        checksum: "a".repeat(64),
        fixture_coverage: { match_count: 1, non_match_count: 2, total: 3 },
      },
    ]);
    vi.spyOn(api, "patternEffectiveness").mockResolvedValue({
      generated_at: "2026-08-24T12:00:00Z",
      window: {
        from: "2026-07-25T12:00:00Z",
        to: "2026-08-24T12:00:00Z",
      },
      items: [
        {
          pattern_id: "payment-completed-without-shipment",
          hit_count: 12,
          last_hit_at: "2026-08-23T10:00:00Z",
          investigation_count: 7,
          confirmed_count: 5,
          false_positive_count: 1,
          needs_review_count: 1,
          unreviewed_count: 5,
          reviewed_count: 7,
          false_positive_rate: 1 / 7,
        },
      ],
    });

    renderPatternsPage();

    expect(await screen.findByTestId("pattern-row-0")).toHaveTextContent(
      "payment-completed-without-shipment",
    );
    expect(screen.getByTestId("pattern-active-count")).toHaveTextContent(
      "1 active",
    );
    expect(screen.getByTestId("pattern-0-hit-count")).toHaveTextContent("12");
    expect(screen.getByTestId("pattern-0-case-count")).toHaveTextContent("7");
    expect(screen.getByTestId("pattern-0-review-count")).toHaveTextContent(
      "7 reviewed / 5 unreviewed",
    );
    expect(
      screen.getByTestId("pattern-0-false-positive-rate"),
    ).toHaveTextContent("14.3%");
    expect(screen.getByTestId("pattern-0-fixture-coverage")).toHaveTextContent(
      "1 match / 2 non-match",
    );
    fireEvent.click(screen.getByTestId("pattern-row-0"));
    const detail = screen.getByRole("dialog", {
      name: "Payment completed without shipment",
    });
    expect(detail).toHaveTextContent("events.by_correlation.v1");
    expect(detail).toHaveTextContent("PaymentVoided");
    expect(detail).toHaveTextContent(
      "contracts/patterns/payment-completed-without-shipment.yaml",
    );
    expect(screen.queryByRole("button", { name: /新增|編輯|刪除/ })).toBeNull();
  });

  it("highlights the Pattern selected by a deep link", async () => {
    vi.spyOn(api, "patterns").mockResolvedValue([
      {
        id: "payment-completed-without-shipment",
        version: 1,
        name: "Payment completed without shipment",
        condition: "Missing shipment inside PT5M.",
        severity: "HIGH",
        window: "PT5M",
        required_event_types: ["PaymentCompleted"],
        expected_event_types: ["ShipmentCreated"],
        exclusion_event_types: [],
        evidence_query_template_id: "events.by_correlation.v1",
        status: "ACTIVE",
        mutable_at_runtime: false,
        source_path:
          "contracts/patterns/payment-completed-without-shipment.yaml",
        checksum: "b".repeat(64),
        fixture_coverage: { match_count: 1, non_match_count: 2, total: 3 },
      },
    ]);
    vi.spyOn(api, "patternEffectiveness").mockResolvedValue({
      generated_at: "2026-08-24T12:00:00Z",
      window: {
        from: "2026-07-25T12:00:00Z",
        to: "2026-08-24T12:00:00Z",
      },
      items: [
        {
          pattern_id: "payment-completed-without-shipment",
          hit_count: 0,
          last_hit_at: null,
          investigation_count: 0,
          confirmed_count: 0,
          false_positive_count: 0,
          needs_review_count: 0,
          unreviewed_count: 0,
          reviewed_count: 0,
          false_positive_rate: null,
        },
      ],
    });

    renderPatternsPage(
      principal.role,
      "/patterns?pattern_id=payment-completed-without-shipment#pattern-payment-completed-without-shipment",
    );

    expect(await screen.findByTestId("pattern-row-0")).toHaveAttribute(
      "data-selected",
      "true",
    );
  });

  it("does not render unavailable Pattern metrics as zero", async () => {
    vi.spyOn(api, "patterns").mockResolvedValue([
      {
        id: "payment-completed-without-shipment",
        version: 1,
        name: "Payment completed without shipment",
        condition: "Missing shipment inside PT5M.",
        severity: "HIGH",
        window: "PT5M",
        required_event_types: ["PaymentCompleted"],
        expected_event_types: ["ShipmentCreated"],
        exclusion_event_types: [],
        evidence_query_template_id: "events.by_correlation.v1",
        status: "ACTIVE",
        mutable_at_runtime: false,
        source_path:
          "contracts/patterns/payment-completed-without-shipment.yaml",
        checksum: "c".repeat(64),
        fixture_coverage: { match_count: 1, non_match_count: 2, total: 3 },
      },
    ]);
    vi.spyOn(api, "patternEffectiveness").mockRejectedValue(
      new Error("PATTERN_EFFECTIVENESS_UNAVAILABLE"),
    );

    renderPatternsPage();

    expect(
      await screen.findByTestId("pattern-effectiveness-error"),
    ).toHaveTextContent("暫時無法使用");
    expect(screen.getByTestId("pattern-0-hit-count")).toHaveTextContent(
      "不可用",
    );
    expect(screen.getByTestId("pattern-0-case-count")).toHaveTextContent(
      "不可用",
    );
  });
});

describe("InvestigationsPage", () => {
  it("renders the case register and opens the right-side detail drawer", async () => {
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [investigation()],
      next_cursor: null,
    });
    vi.spyOn(api, "investigation").mockResolvedValue(investigation());
    vi.spyOn(api, "summary").mockResolvedValue(
      investigationSummary({
        timeline: {
          correlation_id: "ORDER-2001",
          from: "2026-08-20T11:00:00Z",
          to: "2026-08-20T11:06:00Z",
          event_count: 2,
          truncated: false,
          events: [],
        },
      }),
    );

    renderPage();
    expect(await screen.findByText("付款後未出貨")).toBeInTheDocument();
    expect(screen.getByText("01")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /付款後未出貨/ }));
    expect(await screen.findByTestId("case-detail")).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getAllByText("ORDER-2001")).toHaveLength(2),
    );
    expect(
      screen.getByRole("button", { name: "關閉案件詳細" }),
    ).toBeInTheDocument();
    expect(screen.getByTestId("case-incident-window")).toHaveTextContent(
      "TIMELINE_SEARCH",
    );
  });

  it("keeps case data visible and does not report zero events when ClickHouse is unavailable", async () => {
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [investigation()],
      next_cursor: null,
    });
    vi.spyOn(api, "investigation").mockResolvedValue(investigation());
    vi.spyOn(api, "summary").mockResolvedValue(
      investigationSummary({
        partial: true,
        warnings: ["CLICKHOUSE_UNAVAILABLE"],
        source_status: {
          postgres: "OK",
          clickhouse: "UNAVAILABLE",
          technical_observability: "NOT_REQUESTED",
        },
        source_last_success_at: {
          postgres: "2026-08-20T11:01:00Z",
          clickhouse: null,
          technical_observability: null,
        },
      }),
    );

    renderPage();
    fireEvent.click(
      await screen.findByRole("button", { name: /付款後未出貨/ }),
    );

    expect(await screen.findByTestId("case-summary")).toHaveTextContent(
      "CLICKHOUSE_UNAVAILABLE",
    );
    expect(screen.getByTestId("case-event-count")).toHaveTextContent("不可用");
    fireEvent.click(screen.getByTestId("case-tab-timeline"));
    expect(screen.getByTestId("case-timeline-unavailable")).toHaveTextContent(
      "案件與 PostgreSQL 資料仍可使用",
    );
    expect(
      screen.queryByText("目前案件基準時間窗內沒有事件。"),
    ).not.toBeInTheDocument();
  });

  it("explains when durable evidence came from a different Pattern analysis window", async () => {
    const item = investigation({
      incident_from: "2026-08-25T00:40:36Z",
      incident_to: "2026-08-26T00:40:36Z",
      incident_window_source: "MANUAL_DEFAULT",
    });
    const summary = investigationSummary({
      case: item,
      query_window: {
        from: item.incident_from,
        to: item.incident_to,
      },
      timeline: {
        correlation_id: item.correlation_id,
        from: item.incident_from,
        to: item.incident_to,
        event_count: 0,
        truncated: false,
        events: [],
      },
      evidence_references: evidenceManifest().items,
      audit_entries: [
        {
          id: "audit-analysis-window",
          actor_id: "demo-investigator",
          actor_role: "INVESTIGATOR",
          action: "ANALYZE_INVESTIGATION",
          request_id: "request-analysis-window",
          metadata: {
            analysis_status: "EVALUATED",
            executed_pattern_ids: ["payment-completed-without-shipment"],
            finding_count: 1,
            effective_window: {
              from: "2026-08-20T11:00:00Z",
              to: "2026-08-27T11:00:00Z",
              observed_at: "2026-08-26T00:40:36Z",
              anchor: "EARLIEST_CORRELATION_EVENT",
              source_event_count: 2,
            },
          },
          created_at: "2026-08-26T00:40:36Z",
        },
      ],
    });
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [item],
      next_cursor: null,
    });
    vi.spyOn(api, "investigation").mockResolvedValue(item);
    vi.spyOn(api, "summary").mockResolvedValue(summary);
    vi.spyOn(api, "evidence").mockResolvedValue(evidenceManifest());

    renderPage("INVESTIGATOR", "/investigations/case-1");
    await screen.findByTestId("case-summary");
    fireEvent.click(screen.getByTestId("case-tab-timeline"));

    expect(
      await screen.findByTestId("case-evidence-window-notice"),
    ).toHaveTextContent("Evidence 與案件 Event Check 使用不同時間窗");
    expect(screen.getByTestId("case-timeline")).toHaveTextContent(
      "目前案件基準時間窗內沒有事件",
    );
    const timelineLink = screen.getByTestId(
      "case-open-pattern-window-timeline",
    );
    expect(timelineLink).toHaveAttribute(
      "href",
      expect.stringContaining("from=2026-08-20T11%3A00%3A00Z"),
    );
    expect(timelineLink).toHaveAttribute(
      "href",
      expect.stringContaining("to=2026-08-27T11%3A00%3A00Z"),
    );

    fireEvent.click(screen.getByTestId("case-tab-evidence"));
    expect(
      screen.getByTestId("case-evidence-window-notice"),
    ).toBeInTheDocument();
    expect(await screen.findByTestId("evidence-manifest")).toBeInTheDocument();
  });

  it("presents timeline, patterns, evidence and audit in case tabs", async () => {
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [investigation()],
      next_cursor: null,
    });
    vi.spyOn(api, "investigation").mockResolvedValue(investigation());
    vi.spyOn(api, "evidence").mockResolvedValue(evidenceManifest());
    const classifyFinding = vi
      .spyOn(api, "classifyPatternFinding")
      .mockResolvedValue({
        finding_id: "11111111-1111-4111-8111-111111111111",
        status: "CONFIRMED",
        actor_id: "tester",
        actor_role: "INVESTIGATOR",
        updated_at: "2026-08-24T12:00:00Z",
        lock_version: 1,
      });
    vi.spyOn(api, "summary").mockResolvedValue(
      investigationSummary({
        timeline: {
          correlation_id: "ORDER-2001",
          from: "2026-08-20T11:00:00Z",
          to: "2026-08-20T11:06:00Z",
          event_count: 1,
          truncated: false,
          events: [
            {
              event_id: "evt-order-2001-001",
              event_type: "OrderCreated",
              event_version: 1,
              occurred_at: "2026-08-20T11:00:00Z",
              producer: "order-service",
              correlation_id: "ORDER-2001",
              causation_id: null,
              trace_id: "22222222222222222222222222222222",
              aggregate_type: "Order",
              aggregate_id: "ORDER-2001",
              sequence: 1,
              kafka_topic: "order.events",
              kafka_partition: 0,
              kafka_offset: 2000,
              service_version: "1.0.0",
              admission_status: "SEARCHABLE",
              quality_flags: [],
              admission_profile: "domain-event-json-schema-v1",
              ingested_at: "2026-08-20T11:00:01Z",
            },
          ],
        },
        pattern_findings: [
          {
            finding_id: "11111111-1111-4111-8111-111111111111",
            pattern_id: "payment-completed-without-shipment",
            severity: "HIGH",
            matched_conditions: ["PAYMENT_COMPLETED_EXISTS"],
            evidence_references: [],
            recommended_next_query: "查詢 shipping.events",
            feedback: {
              finding_id: "11111111-1111-4111-8111-111111111111",
              status: "UNREVIEWED",
              actor_id: "",
              actor_role: "",
              updated_at: null,
              lock_version: 0,
            },
          },
        ],
        evidence_references: [
          {
            id: "evidence-1",
            evidence_type: "PATTERN_FINDING",
            reference: "payment-completed-without-shipment:v1:ORDER-2001",
            source: "PATTERN_ENGINE",
            open_action: "PATTERN_LIBRARY",
            collected_at: "2026-08-20T11:01:00Z",
            checksum: "abc123",
          },
        ],
        audit_entries: [
          {
            id: "audit-1",
            actor_id: "demo-investigator",
            actor_role: "INVESTIGATOR",
            action: "ANALYZE_INVESTIGATION",
            request_id: "request-1",
            metadata: {},
            created_at: "2026-08-20T11:01:00Z",
          },
        ],
        source_status: {
          postgres: "OK",
          clickhouse: "OK",
          technical_observability: "NOT_REQUESTED",
        },
      }),
    );

    renderPage();
    fireEvent.click(
      await screen.findByRole("button", { name: /付款後未出貨/ }),
    );
    expect(await screen.findByTestId("case-summary")).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("case-tab-timeline"));
    expect(screen.getByTestId("case-timeline")).toHaveTextContent(
      "OrderCreated",
    );
    fireEvent.click(screen.getByTestId("case-tab-patterns"));
    expect(screen.getByTestId("case-patterns")).toHaveTextContent(
      "payment-completed-without-shipment",
    );
    expect(
      screen.getByTestId("pattern-feedback-payment-completed-without-shipment"),
    ).toHaveTextContent("UNREVIEWED");
    fireEvent.click(
      screen.getByRole("button", {
        name: "payment-completed-without-shipment 確認命中",
      }),
    );
    await waitFor(() =>
      expect(classifyFinding).toHaveBeenCalledWith(
        "case-1",
        "11111111-1111-4111-8111-111111111111",
        0,
        "CONFIRMED",
      ),
    );
    fireEvent.click(screen.getByTestId("case-tab-evidence"));
    expect(await screen.findByTestId("evidence-manifest")).toHaveTextContent(
      "PATTERN_FINDING",
    );
    expect(
      screen.getByRole("link", { name: /開啟 Check Model/ }),
    ).toHaveAttribute(
      "href",
      expect.stringContaining("focus=PAYMENT_REQUIRES_SHIPMENT"),
    );
    expect(screen.getByRole("link", { name: /開啟 Alert/ })).toHaveAttribute(
      "href",
      "http://localhost:28332/alerting/grafana/event-quality-delay/view?orgId=1",
    );
    fireEvent.click(screen.getByTestId("case-tab-audit"));
    expect(screen.getByTestId("case-audit")).toHaveTextContent(
      "ANALYZE_INVESTIGATION",
    );
  });

  it("uses the next cursor and keeps row sequence numbers across pages", async () => {
    const list = vi
      .spyOn(api, "investigations")
      .mockResolvedValueOnce({
        items: [investigation()],
        next_cursor: "cursor-page-2",
      })
      .mockResolvedValueOnce({
        items: [
          investigation({
            id: "case-11",
            case_no: "EH-2026-0011",
            title: "第二頁案件",
          }),
        ],
        next_cursor: null,
      });

    renderPage();
    expect(await screen.findByText("付款後未出貨")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "下一頁" }));
    expect(await screen.findByText("第二頁案件")).toBeInTheDocument();
    expect(list).toHaveBeenNthCalledWith(
      2,
      "cursor-page-2",
      expect.objectContaining({ sort_by: "created_at", sort_order: "desc" }),
    );
    expect(screen.getByText("11")).toBeInTheDocument();
  });

  it("loads compound filters and sorting from a shareable URL", async () => {
    const list = vi.spyOn(api, "investigations").mockResolvedValue({
      items: [],
      next_cursor: null,
    });

    renderPage(
      principal.role,
      "/investigations?status=INVESTIGATING&severity=HIGH&priority=P0&assignee=shipping-oncall&tag=urgent&correlation_id=ORDER-2001&sort_by=updated_at&sort_order=asc",
    );

    await waitFor(() =>
      expect(list).toHaveBeenCalledWith(
        undefined,
        expect.objectContaining({
          status: "INVESTIGATING",
          severity: "HIGH",
          priority: "P0",
          assignee: "shipping-oncall",
          tag: "urgent",
          correlation_id: "ORDER-2001",
          sort_by: "updated_at",
          sort_order: "asc",
        }),
      ),
    );
    expect(screen.getByLabelText("Owner")).toHaveValue("shipping-oncall");
    expect(screen.getByLabelText("Tag")).toHaveValue("urgent");
    expect(screen.getByLabelText("Sort by")).toHaveValue("updated_at");
  });

  it("updates collaboration metadata and appends a note without a detail refetch", async () => {
    const initial = investigation();
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [initial],
      next_cursor: null,
    });
    const getDetail = vi.spyOn(api, "investigation").mockResolvedValue(initial);
    vi.spyOn(api, "summary").mockResolvedValue(investigationSummary());
    const patch = vi.spyOn(api, "patchInvestigation").mockResolvedValue(
      investigation({
        assignee: "shipping-oncall",
        priority: "P0",
        tags: ["shipping", "vip"],
        related_correlation_ids: ["SHIPMENT-2001"],
        last_updated_by: "tester",
        lock_version: 2,
      }),
    );
    const note = {
      id: "note-1",
      body: "已確認 shipping consumer lag",
      author_id: "tester",
      author_role: "INVESTIGATOR" as const,
      created_at: "2026-08-20T11:05:00Z",
    };
    vi.spyOn(api, "addInvestigationNote").mockResolvedValue({
      investigation: investigation({
        assignee: "shipping-oncall",
        priority: "P0",
        tags: ["shipping", "vip"],
        related_correlation_ids: ["SHIPMENT-2001"],
        last_updated_by: "tester",
        lock_version: 3,
      }),
      note,
    });

    renderPage();
    fireEvent.click(
      await screen.findByRole("button", { name: /付款後未出貨/ }),
    );
    expect(await screen.findByTestId("case-collaboration")).toBeInTheDocument();
    fireEvent.change(screen.getByTestId("case-owner-input"), {
      target: { value: "shipping-oncall" },
    });
    fireEvent.change(screen.getByTestId("case-priority-select"), {
      target: { value: "P0" },
    });
    fireEvent.change(screen.getByTestId("case-tags-input"), {
      target: { value: "shipping, vip" },
    });
    fireEvent.change(screen.getByTestId("case-related-input"), {
      target: { value: "SHIPMENT-2001" },
    });
    fireEvent.click(screen.getByTestId("case-collaboration-save"));

    await waitFor(() =>
      expect(patch).toHaveBeenCalledWith(initial, {
        assignee: "shipping-oncall",
        priority: "P0",
        tags: ["shipping", "vip"],
        related_correlation_ids: ["SHIPMENT-2001"],
      }),
    );
    expect(await screen.findByText("#vip")).toBeInTheDocument();
    fireEvent.change(screen.getByTestId("case-note-input"), {
      target: { value: note.body },
    });
    fireEvent.click(screen.getByTestId("case-note-submit"));

    expect(await screen.findByText(note.body)).toBeInTheDocument();
    expect(getDetail).toHaveBeenCalledTimes(1);
  });

  it("uses the URL as drawer state and returns to the register when closed", async () => {
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [investigation()],
      next_cursor: null,
    });
    vi.spyOn(api, "investigation").mockResolvedValue(investigation());
    vi.spyOn(api, "summary").mockResolvedValue(investigationSummary());

    renderPage();
    const caseRow = await screen.findByRole("button", {
      name: /付款後未出貨/,
    });
    caseRow.focus();
    fireEvent.click(caseRow);

    const drawer = await screen.findByTestId("case-detail");
    expect(drawer).toHaveAttribute("role", "dialog");
    expect(drawer).toHaveAttribute("aria-modal", "true");
    expect(screen.getByTestId("test-location")).toHaveTextContent(
      "/investigations/case-1",
    );

    fireEvent.keyDown(drawer, { key: "Escape" });

    expect(screen.getByTestId("test-location")).toHaveTextContent(
      "/investigations",
    );
    expect(screen.queryByTestId("case-detail")).not.toBeInTheDocument();
    expect(caseRow).toHaveFocus();
  });

  it("opens a case directly from its durable route", async () => {
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [investigation()],
      next_cursor: null,
    });
    const getDetail = vi
      .spyOn(api, "investigation")
      .mockResolvedValue(investigation());
    vi.spyOn(api, "summary").mockResolvedValue(investigationSummary());

    renderPage("INVESTIGATOR", "/investigations/case-1");

    expect(await screen.findByTestId("case-detail")).toBeInTheDocument();
    expect(getDetail).toHaveBeenCalledWith("case-1");
    expect(screen.getByTestId("test-location")).toHaveTextContent(
      "/investigations/case-1",
    );
  });

  it("keeps a missing case route visible and presents a clear not-found state", async () => {
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [],
      next_cursor: null,
    });
    vi.spyOn(api, "investigation").mockRejectedValue(new Error("NOT_FOUND"));
    vi.spyOn(api, "summary").mockRejectedValue(new Error("NOT_FOUND"));

    renderPage("INVESTIGATOR", "/investigations/missing-case");

    expect(await screen.findByTestId("case-detail-error")).toHaveTextContent(
      "找不到案件",
    );
    expect(screen.getByTestId("test-location")).toHaveTextContent(
      "/investigations/missing-case",
    );
    expect(screen.queryByTestId("case-timeline")).not.toBeInTheDocument();
  });

  it("renders only backend-allowed actions and enters then resumes approval", async () => {
    const initial = investigation();
    const investigating = investigation({
      status: "INVESTIGATING",
      allowed_transitions: ["WAITING_APPROVAL", "RESOLVED", "CLOSED"],
      lock_version: 2,
    });
    const waiting = investigation({
      status: "WAITING_APPROVAL",
      allowed_transitions: ["INVESTIGATING", "RESOLVED", "CLOSED"],
      lock_version: 3,
    });
    const resumed = investigation({
      status: "INVESTIGATING",
      allowed_transitions: ["WAITING_APPROVAL", "RESOLVED", "CLOSED"],
      lock_version: 4,
    });
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [initial],
      next_cursor: null,
    });
    vi.spyOn(api, "investigation").mockResolvedValue(initial);
    vi.spyOn(api, "summary").mockResolvedValue(investigationSummary());
    const patch = vi
      .spyOn(api, "patchInvestigation")
      .mockResolvedValueOnce(investigating)
      .mockResolvedValueOnce(waiting)
      .mockResolvedValueOnce(resumed);

    renderPage("INVESTIGATOR", "/investigations/case-1");
    expect(
      await screen.findByRole("button", { name: "開始調查" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "送交審核" }),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "開始調查" }));

    await waitFor(() =>
      expect(patch).toHaveBeenNthCalledWith(1, initial, {
        status: "INVESTIGATING",
      }),
    );
    fireEvent.click(await screen.findByRole("button", { name: "送交審核" }));
    await waitFor(() =>
      expect(patch).toHaveBeenNthCalledWith(2, investigating, {
        status: "WAITING_APPROVAL",
      }),
    );
    fireEvent.click(await screen.findByRole("button", { name: "返回調查" }));
    await waitFor(() =>
      expect(patch).toHaveBeenNthCalledWith(3, waiting, {
        status: "INVESTIGATING",
      }),
    );
    expect(screen.queryByText("更新狀態")).not.toBeInTheDocument();
  });

  it("collects required resolution evidence only in the resolve flow", async () => {
    const initial = investigation({
      status: "INVESTIGATING",
      allowed_transitions: ["WAITING_APPROVAL", "RESOLVED", "CLOSED"],
    });
    const resolved = investigation({
      status: "RESOLVED",
      allowed_transitions: ["INVESTIGATING", "CLOSED"],
      root_cause: "duplicate payment callback",
      resolution_summary: "callback made idempotent",
      lock_version: 2,
    });
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [initial],
      next_cursor: null,
    });
    vi.spyOn(api, "investigation").mockResolvedValue(initial);
    vi.spyOn(api, "summary").mockResolvedValue(
      investigationSummary({ case: initial }),
    );
    const patch = vi
      .spyOn(api, "patchInvestigation")
      .mockResolvedValue(resolved);

    renderPage("INVESTIGATOR", "/investigations/case-1");
    fireEvent.click(await screen.findByRole("button", { name: "標記已解決" }));
    expect(screen.getByTestId("case-resolution-dialog")).toBeInTheDocument();
    expect(screen.getByTestId("case-resolve-confirm")).toBeDisabled();
    fireEvent.change(screen.getByTestId("case-root-cause-input"), {
      target: { value: "duplicate payment callback" },
    });
    fireEvent.change(screen.getByTestId("case-resolution-input"), {
      target: { value: "callback made idempotent" },
    });
    fireEvent.click(screen.getByTestId("case-resolve-confirm"));

    await waitFor(() =>
      expect(patch).toHaveBeenCalledWith(initial, {
        status: "RESOLVED",
        root_cause: "duplicate payment callback",
        resolution_summary: "callback made idempotent",
      }),
    );
    expect(await screen.findByTestId("case-action-success")).toHaveTextContent(
      "調查結論已保存",
    );
    expect(screen.getByTestId("case-status")).toHaveTextContent("RESOLVED");
    expect(
      screen.getByRole("button", { name: "重新開啟" }),
    ).toBeInTheDocument();
  });

  it("protects an unsaved resolution draft before dismiss and reload", async () => {
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [investigation()],
      next_cursor: null,
    });
    vi.spyOn(api, "investigation").mockResolvedValue(investigation());
    vi.spyOn(api, "summary").mockResolvedValue(investigationSummary());
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);

    renderPage("INVESTIGATOR", "/investigations/case-1");
    fireEvent.click(await screen.findByTestId("case-close-start"));
    fireEvent.change(screen.getByTestId("case-root-cause-input"), {
      target: { value: "consumer stopped" },
    });
    const beforeUnload = new Event("beforeunload", { cancelable: true });
    window.dispatchEvent(beforeUnload);
    expect(beforeUnload.defaultPrevented).toBe(true);

    fireEvent.click(screen.getByTestId("case-close-cancel"));
    expect(confirm).toHaveBeenCalledWith(
      "尚有未送出的調查結論，確定要放棄嗎？",
    );
    expect(screen.getByTestId("case-close-confirmation")).toBeInTheDocument();
    confirm.mockReturnValue(true);
    fireEvent.click(screen.getByTestId("case-close-cancel"));
    expect(
      screen.queryByTestId("case-close-confirmation"),
    ).not.toBeInTheDocument();
  });

  it("reloads a stale case and gives actionable conflict feedback", async () => {
    const initial = investigation();
    const current = investigation({
      status: "INVESTIGATING",
      allowed_transitions: ["WAITING_APPROVAL", "RESOLVED", "CLOSED"],
      lock_version: 2,
    });
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [initial],
      next_cursor: null,
    });
    const getDetail = vi
      .spyOn(api, "investigation")
      .mockResolvedValueOnce(initial)
      .mockResolvedValue(current);
    vi.spyOn(api, "summary").mockResolvedValue(investigationSummary());
    vi.spyOn(api, "patchInvestigation").mockRejectedValue(
      new Error("OPTIMISTIC_LOCK_CONFLICT"),
    );

    renderPage("INVESTIGATOR", "/investigations/case-1");
    fireEvent.click(await screen.findByRole("button", { name: "開始調查" }));

    expect(await screen.findByTestId("case-action-error")).toHaveTextContent(
      "已重新載入最新內容",
    );
    await waitFor(() => expect(getDetail).toHaveBeenCalledTimes(2));
    expect(await screen.findByTestId("case-status")).toHaveTextContent(
      "INVESTIGATING",
    );
  });

  it("requires explicit closure evidence and closes with the entered values", async () => {
    const initial = investigation();
    const closed = investigation({
      status: "CLOSED",
      allowed_transitions: [],
      root_cause: "shipping consumer stopped",
      resolution_summary: "consumer restarted and backlog drained",
      lock_version: 2,
    });
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [initial],
      next_cursor: null,
    });
    vi.spyOn(api, "investigation").mockResolvedValue(initial);
    vi.spyOn(api, "summary").mockResolvedValue(investigationSummary());
    const closeCase = vi
      .spyOn(api, "closeInvestigation")
      .mockResolvedValue(closed);

    renderPage("INVESTIGATOR", "/investigations/case-1");
    const closeStart = await screen.findByTestId("case-close-start");
    expect(
      screen.queryByTestId("case-root-cause-input"),
    ).not.toBeInTheDocument();
    closeStart.focus();
    fireEvent.click(closeStart);
    const closeDialog = screen.getByTestId("case-close-confirmation");
    expect(closeDialog).toBeInTheDocument();
    expect(screen.getByTestId("case-root-cause-input")).toHaveValue("");
    expect(screen.getByTestId("case-resolution-input")).toHaveValue("");
    expect(screen.getByTestId("case-close-confirm")).toBeDisabled();
    fireEvent.keyDown(closeDialog, { key: "Escape" });
    expect(
      screen.queryByTestId("case-close-confirmation"),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId("case-close-start")).toHaveFocus();
    expect(screen.getByTestId("case-status")).toHaveTextContent("OPEN");

    fireEvent.click(screen.getByTestId("case-close-start"));
    fireEvent.change(screen.getByTestId("case-root-cause-input"), {
      target: { value: "shipping consumer stopped" },
    });
    fireEvent.change(screen.getByTestId("case-resolution-input"), {
      target: { value: "consumer restarted and backlog drained" },
    });
    fireEvent.click(screen.getByTestId("case-close-confirm"));

    await waitFor(() =>
      expect(closeCase).toHaveBeenCalledWith(
        initial,
        "shipping consumer stopped",
        "consumer restarted and backlog drained",
      ),
    );
    expect(await screen.findByTestId("case-status")).toHaveTextContent(
      "CLOSED",
    );
  });

  it("does not expose state-changing actions to a Viewer", async () => {
    vi.spyOn(api, "investigations").mockResolvedValue({
      items: [investigation()],
      next_cursor: null,
    });
    vi.spyOn(api, "investigation").mockResolvedValue(investigation());
    vi.spyOn(api, "summary").mockResolvedValue(investigationSummary());
    vi.spyOn(api, "patterns").mockResolvedValue([]);

    renderPage("VIEWER");
    fireEvent.click(
      await screen.findByRole("button", { name: /付款後未出貨/ }),
    );
    await waitFor(() =>
      expect(screen.getByTestId("case-detail")).toBeInTheDocument(),
    );
    expect(
      screen.queryByRole("button", { name: "更新狀態" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "結案" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Root cause")).not.toBeInTheDocument();
    expect(
      screen.queryByTestId("case-collaboration-save"),
    ).not.toBeInTheDocument();
    expect(screen.queryByTestId("case-note-submit")).not.toBeInTheDocument();
    fireEvent.click(screen.getByTestId("case-tab-patterns"));
    expect(
      await screen.findByTestId("case-pattern-readonly"),
    ).toHaveTextContent("Viewer 只能查看");
    expect(
      screen.queryByTestId("run-case-pattern-analysis"),
    ).not.toBeInTheDocument();
  });
});
