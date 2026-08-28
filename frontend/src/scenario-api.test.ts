import { afterEach, describe, expect, it, vi } from "vitest";
import { scenarioApi } from "./scenario-api";

const fetchMock = vi.fn();
vi.stubGlobal("fetch", fetchMock);

afterEach(() => fetchMock.mockReset());

function respond(body: unknown, status = 200) {
  fetchMock.mockResolvedValueOnce(
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

function requestAt(index = 0): Request {
  return fetchMock.mock.calls[index][0] as Request;
}

describe("Scenario Lab API client", () => {
  it("loads the fixed catalog through the Scenario Lab proxy", async () => {
    respond({ items: [] });

    await scenarioApi.catalog();

    expect(new URL(requestAt().url).pathname).toBe(
      "/scenario-api/api/v1/scenarios",
    );
    expect(requestAt().method).toBe("GET");
  });

  it("starts a selected scenario and retrieves its actual run", async () => {
    const response = {
      run_id: "4bd00d41-8b30-476b-82e8-e0474419f7f7",
      scenario: {
        id: "S8",
        name: "PAYMENT_FAILED_AND_CANCELLED",
        title: "付款失敗並取消",
        category: "LOGISTICS",
        description: "test",
        execution_mode: "LAB_INJECTION",
        synthetic: true,
        expected_event_types: [
          "OrderCreated",
          "PaymentFailed",
          "OrderCancelled",
        ],
        expected_results: ["actual chain"],
      },
      correlation_id: "LAB-S8-TEST",
      trace_id: "a".repeat(32),
      status: "PASSED",
      execution_mode: "LAB_INJECTION",
      synthetic: true,
      expected_event_types: ["OrderCreated", "PaymentFailed", "OrderCancelled"],
      actual: {
        trace_id: "a".repeat(32),
        event_count: 3,
        event_types: ["OrderCreated", "PaymentFailed", "OrderCancelled"],
        duplicate_event_ids: [],
        out_of_order: false,
        processing_statuses: [],
        ingestion_failure_count: 0,
        ingestion_failure_types: [],
        max_event_delay_ms: 10,
      },
      checks: [],
      links: {
        timeline: "/timeline",
        grafana: "/grafana",
        tempo: null,
        loki: "/loki",
      },
      error: null,
      accepted_at: "2026-08-21T00:00:00Z",
      started_at: "2026-08-21T00:00:00Z",
      completed_at: "2026-08-21T00:00:01Z",
    };
    respond({ ...response, status: "ACCEPTED" }, 202);
    respond(response);

    await scenarioApi.start("S8");
    await scenarioApi.run(response.run_id);

    expect(requestAt(0).method).toBe("POST");
    expect(await requestAt(0).clone().json()).toEqual({ scenario_id: "S8" });
    expect(new URL(requestAt(1).url).pathname).toBe(
      `/scenario-api/api/v1/scenario-runs/${response.run_id}`,
    );
  });
});
