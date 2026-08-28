import { describe, expect, it } from "vitest";
import { legacyPatternModelURL, resolveLegacyRoute } from "./legacy-routes";

const now = new Date("2026-08-29T12:00:00Z");

describe("legacy route compatibility", () => {
  it("maps a bounded Timeline identifier to the Event Check Timeline tab", () => {
    const result = resolveLegacyRoute(
      "/timeline",
      "?correlation_id=ORDER-1001&from=2026-08-28T00%3A00%3A00Z&to=2026-08-28T01%3A00%3A00Z&include_processing_attempts=true",
      now,
    );
    expect(result).toMatchObject({ kind: "REDIRECT" });
    if (result?.kind !== "REDIRECT") return;
    expect(result.to).toContain("/event-check?");
    expect(result.to).toContain("identifier_type=CORRELATION_ID");
    expect(result.to).toContain("identifier=ORDER-1001");
    expect(result.to).toContain("tab=timeline");
  });

  it("retains broad Timeline filters instead of silently losing semantics", () => {
    expect(
      resolveLegacyRoute(
        "/timeline",
        "?event_type=PaymentFailed&from=2026-08-28T00%3A00%3A00Z&to=2026-08-28T01%3A00%3A00Z",
        now,
      ),
    ).toMatchObject({ kind: "RETAIN" });
  });

  it("preserves a prefilled legacy time window before an identifier is entered", () => {
    const result = resolveLegacyRoute(
      "/timeline",
      "?from=2026-08-28T00%3A00%3A00Z&to=2026-08-28T01%3A00%3A00Z",
      now,
    );
    expect(result).toMatchObject({ kind: "REDIRECT" });
    if (result?.kind !== "REDIRECT") return;
    expect(result.to).toContain("/event-check?");
    expect(result.to).toContain("from=2026-08-28T00%3A00%3A00Z");
  });

  it("maps Journey and registry routes into their canonical workspaces", () => {
    expect(
      resolveLegacyRoute(
        "/journey",
        "?correlation_id=ORDER-1001&from=2026-08-28T00%3A00%3A00Z&to=2026-08-28T01%3A00%3A00Z",
        now,
      ),
    ).toMatchObject({
      kind: "REDIRECT",
      reason: expect.stringContaining("Flow"),
    });
    expect(resolveLegacyRoute("/journey-profiles", "", now)).toEqual({
      kind: "REDIRECT",
      to: "/check-models?kind=FLOW",
      reason: "Journey Profiles 已併入 Flow Models。",
    });
  });

  it("maps the known Pattern to its canonical expectation", () => {
    const href = legacyPatternModelURL("payment-completed-without-shipment:v1");
    expect(href).toContain("model_id=order-fulfillment");
    expect(href).toContain("focus=PAYMENT_REQUIRES_SHIPMENT");
  });

  it("retains an unknown Pattern until an explicit mapping exists", () => {
    expect(
      resolveLegacyRoute("/patterns", "?pattern_id=unknown-pattern", now),
    ).toMatchObject({ kind: "RETAIN" });
  });
});
