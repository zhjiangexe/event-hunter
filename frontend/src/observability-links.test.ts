import { describe, expect, it } from "vitest";
import type { EvidenceReference, TimelineEvent } from "./api";
import {
  evidenceSourceLink,
  eventObservabilityLinks,
  trustedGrafanaBase,
} from "./observability-links";

const event: TimelineEvent = {
  event_id: "evt-'quoted",
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
  admission_status: "SEARCHABLE",
  quality_flags: [],
  admission_profile: "domain-event-json-schema-v1",
  ingested_at: "2026-08-20T11:00:31Z",
};

describe("observability links", () => {
  it("builds encoded Grafana 13 Explore panes for events, traces and logs", () => {
    const links = eventObservabilityLinks(
      event,
      "https://grafana.example.test",
    );

    expect(links.map((link) => link.kind)).toEqual([
      "event",
      "logs",
      "trace",
      "dashboard",
    ]);
    const explore = new URL(links[0].href);
    expect(explore.origin).toBe("https://grafana.example.test");
    expect(explore.searchParams.get("schemaVersion")).toBe("1");
    const panes = JSON.parse(explore.searchParams.get("panes")!);
    expect(panes["event-hunter"].queries[0].rawSql).toContain(
      "FROM canonical_forensics_events",
    );
    expect(panes["event-hunter"].queries[0].rawSql).toContain(
      "event_id = 'evt-''quoted'",
    );
    const trace = new URL(links.find((link) => link.kind === "trace")!.href);
    const tracePanes = JSON.parse(trace.searchParams.get("panes")!);
    expect(tracePanes["event-hunter"].queries[0]).toMatchObject({
      query: "55555555555555555555555555555555",
      queryType: "traceql",
    });
    const logs = new URL(links.find((link) => link.kind === "logs")!.href);
    const logPanes = JSON.parse(logs.searchParams.get("panes")!);
    expect(logPanes["event-hunter"].queries[0].expr).toBe(
      '{service_name="payment-service"} | correlation_id="ORDER-2001"',
    );
  });

  it("rejects non-http origins and credential-bearing URLs", () => {
    expect(() => trustedGrafanaBase("javascript:alert(1)")).toThrow(
      "UNTRUSTED_GRAFANA_ORIGIN",
    );
    expect(() =>
      trustedGrafanaBase("https://admin:secret@grafana.example.test"),
    ).toThrow("UNTRUSTED_GRAFANA_ORIGIN");
  });

  it("builds source links only from the Evidence open-action allowlist", () => {
    const evidence: EvidenceReference = {
      id: "evidence-1",
      evidence_type: "EVENT",
      reference: "evt-'quoted",
      source: "CLICKHOUSE",
      open_action: "GRAFANA_EVENT",
      collected_at: "2026-08-20T11:01:00Z",
      checksum: "a".repeat(64),
    };
    const link = evidenceSourceLink(evidence, "https://grafana.example.test");

    expect(link?.external).toBe(true);
    const panes = JSON.parse(new URL(link!.href).searchParams.get("panes")!);
    expect(panes["event-hunter"].queries[0].rawSql).toContain(
      "FROM canonical_forensics_events",
    );
    expect(panes["event-hunter"].queries[0].rawSql).toContain(
      "event_id = 'evt-''quoted'",
    );
    expect(evidenceSourceLink({ ...evidence, open_action: "NONE" })).toBeNull();
  });

  it("routes trace and log references through their configured Grafana datasources", () => {
    const base: EvidenceReference = {
      id: "evidence-2",
      evidence_type: "TRACE",
      reference: "55555555555555555555555555555555",
      source: "TEMPO",
      open_action: "GRAFANA_TEMPO",
      collected_at: "2026-08-20T11:01:00Z",
      checksum: "b".repeat(64),
    };
    const tracePanes = JSON.parse(
      new URL(
        evidenceSourceLink(base, "https://grafana.example.test")!.href,
      ).searchParams.get("panes")!,
    );
    const logPanes = JSON.parse(
      new URL(
        evidenceSourceLink(
          {
            ...base,
            evidence_type: "LOG",
            source: "LOKI",
            open_action: "GRAFANA_LOKI",
            reference: "ORDER-2001",
          },
          "https://grafana.example.test",
        )!.href,
      ).searchParams.get("panes")!,
    );

    expect(tracePanes["event-hunter"].datasource).toBe("tempo");
    expect(tracePanes["event-hunter"].queries[0].queryType).toBe("traceql");
    expect(logPanes["event-hunter"].datasource).toBe("loki");
    expect(logPanes["event-hunter"].queries[0].expr).toContain("ORDER-2001");
  });

  it("rebases validated alert-rule paths onto the trusted Grafana origin", () => {
    const alert: EvidenceReference = {
      id: "evidence-alert",
      evidence_type: "GRAFANA_ALERT",
      reference: "receipt-1",
      source: "GRAFANA",
      open_action: "GRAFANA_ALERT",
      source_locator: "/alerting/grafana/event-quality-delay/view",
      source_org_id: 7,
      collected_at: "2026-08-20T11:06:00Z",
      checksum: "c".repeat(64),
    };
    const link = evidenceSourceLink(alert, "https://grafana.example.test");

    expect(link?.href).toBe(
      "https://grafana.example.test/alerting/grafana/event-quality-delay/view?orgId=7",
    );
    expect(
      evidenceSourceLink({
        ...alert,
        source_locator: "/login",
      }),
    ).toBeNull();
    expect(
      evidenceSourceLink({
        ...alert,
        source_locator: "https://attacker.example/alerting/demo/view",
      }),
    ).toBeNull();
  });
});
