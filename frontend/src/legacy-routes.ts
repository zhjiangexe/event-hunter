export type LegacyRouteResolution =
  | { kind: "REDIRECT"; to: string; reason: string }
  | { kind: "RETAIN"; reason: string };

const identifierMappings = [
  ["correlation_id", "CORRELATION_ID"],
  ["event_id", "EVENT_ID"],
  ["trace_id", "TRACE_ID"],
  ["aggregate_id", "AGGREGATE_ID"],
] as const;

const timelineLosslessKeys = new Set([
  "from",
  "to",
  "include_processing_attempts",
  "panel",
  ...identifierMappings.map(([queryKey]) => queryKey),
]);

const patternMappings: Record<
  string,
  { modelID: string; version: string; focus: string }
> = {
  "payment-completed-without-shipment": {
    modelID: "order-fulfillment",
    version: "2",
    focus: "PAYMENT_REQUIRES_SHIPMENT",
  },
};

function defaultWindow(now: Date) {
  return {
    from: new Date(now.getTime() - 72 * 60 * 60_000).toISOString(),
    to: now.toISOString(),
  };
}

function validWindow(from: string | null, to: string | null) {
  if (!from || !to) return false;
  const start = Date.parse(from);
  const end = Date.parse(to);
  return (
    Number.isFinite(start) &&
    Number.isFinite(end) &&
    start < end &&
    end - start <= 7 * 24 * 60 * 60_000
  );
}

function eventCheckURL(
  identifierType: string,
  identifier: string,
  source: URLSearchParams,
  tab: "summary" | "timeline" | "flow" = "timeline",
  now = new Date(),
) {
  const fallback = defaultWindow(now);
  const from = source.get("from");
  const to = source.get("to");
  const query = new URLSearchParams({
    identifier_type: identifierType,
    identifier,
    from: validWindow(from, to) ? from! : fallback.from,
    to: validWindow(from, to) ? to! : fallback.to,
    tab,
  });
  return `/event-check?${query.toString()}`;
}

export function resolveLegacyRoute(
  pathname: string,
  search: string,
  now = new Date(),
): LegacyRouteResolution | null {
  const params = new URLSearchParams(search);

  if (pathname === "/saved-searches") {
    return {
      kind: "REDIRECT",
      to: "/event-check?panel=query-shortcuts",
      reason: "Saved Searches 已整合至 Event Check。",
    };
  }

  if (pathname === "/journey-profiles") {
    return {
      kind: "REDIRECT",
      to: "/check-models?kind=FLOW",
      reason: "Journey Profiles 已併入 Flow Models。",
    };
  }

  if (pathname === "/journey") {
    const correlationID = params.get("correlation_id")?.trim();
    if (!correlationID) {
      return {
        kind: "REDIRECT",
        to: "/event-check?tab=flow",
        reason: "Business Journey 已整合至 Event Check Flow。",
      };
    }
    return {
      kind: "REDIRECT",
      to: eventCheckURL("CORRELATION_ID", correlationID, params, "flow", now),
      reason: "Business Journey 已整合至 Event Check Flow。",
    };
  }

  if (pathname === "/patterns") {
    const patternID = params.get("pattern_id")?.trim();
    if (!patternID) {
      return {
        kind: "REDIRECT",
        to: "/check-models?kind=FLOW",
        reason: "Pattern Library 已併入 Check Models。",
      };
    }
    const mapped = patternMappings[patternID];
    if (!mapped) {
      return {
        kind: "RETAIN",
        reason: "此舊 Pattern 尚無可驗證的 Check Model 對應。",
      };
    }
    const query = new URLSearchParams({
      kind: "FLOW",
      model_id: mapped.modelID,
      version: mapped.version,
      panel: "overview",
      focus: mapped.focus,
      legacy_pattern_id: patternID,
    });
    return {
      kind: "REDIRECT",
      to: `/check-models?${query.toString()}`,
      reason: "Pattern 已合併為 Check Model expectation。",
    };
  }

  if (pathname !== "/timeline") return null;

  if ([...params.keys()].some((key) => !timelineLosslessKeys.has(key))) {
    return {
      kind: "RETAIN",
      reason: "這個探索查詢含 Event Check 尚未支援的廣泛篩選條件。",
    };
  }
  const identifiers = identifierMappings.flatMap(
    ([queryKey, identifierType]) => {
      const value = params.get(queryKey)?.trim();
      return value ? [{ identifierType, value }] : [];
    },
  );
  if (identifiers.length > 1) {
    return {
      kind: "RETAIN",
      reason: "這個舊查詢同時使用多個識別碼，無法無損轉換。",
    };
  }
  if (identifiers.length === 0) {
    const target = new URLSearchParams();
    const from = params.get("from");
    const to = params.get("to");
    if (validWindow(from, to)) {
      target.set("from", from!);
      target.set("to", to!);
    }
    return {
      kind: "REDIRECT",
      to: target.size ? `/event-check?${target.toString()}` : "/event-check",
      reason: "Business Timeline 已整合至 Event Check。",
    };
  }
  return {
    kind: "REDIRECT",
    to: eventCheckURL(
      identifiers[0].identifierType,
      identifiers[0].value,
      params,
      "timeline",
      now,
    ),
    reason: "Business Timeline 已整合至 Event Check Timeline。",
  };
}

export function legacyPatternModelURL(reference: string) {
  const patternID = reference.split(":v")[0];
  const resolution = resolveLegacyRoute(
    "/patterns",
    `?pattern_id=${encodeURIComponent(patternID)}`,
  );
  return resolution?.kind === "REDIRECT" ? resolution.to : null;
}
