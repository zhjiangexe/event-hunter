import type { EvidenceReference, TimelineEvent } from "./api";
import { legacyPatternModelURL } from "./legacy-routes";

export type ObservabilityLink = {
  kind: "event" | "trace" | "logs" | "dashboard";
  label: string;
  href: string;
};

export type EvidenceSourceLink = {
  label: string;
  href: string;
  external: boolean;
};

type ObservableEvent = Pick<
  TimelineEvent,
  "event_id" | "occurred_at" | "producer" | "correlation_id" | "trace_id"
>;

const defaultGrafanaUrl =
  import.meta.env.VITE_GRAFANA_URL || "http://localhost:28332";
const grafanaAlertPathPattern =
  /^\/alerting\/(?:grafana\/)?[A-Za-z0-9_-]+\/(?:view|edit)$/;

export function trustedGrafanaBase(value: string) {
  const url = new URL(value);
  if (
    (url.protocol !== "http:" && url.protocol !== "https:") ||
    url.username ||
    url.password ||
    url.search ||
    url.hash
  ) {
    throw new Error("UNTRUSTED_GRAFANA_ORIGIN");
  }
  return `${url.origin}${url.pathname.replace(/\/$/, "")}`;
}

function eventRange(event: ObservableEvent) {
  const occurredAt = Date.parse(event.occurred_at);
  return {
    from: String(occurredAt - 5 * 60_000),
    to: String(occurredAt + 5 * 60_000),
  };
}

function collectedRange(collectedAt: string) {
  const timestamp = Date.parse(collectedAt);
  return {
    from: String(timestamp - 5 * 60_000),
    to: String(timestamp + 5 * 60_000),
  };
}

function exploreUrl(
  grafanaUrl: string,
  datasource: string,
  datasourceType: string,
  query: Record<string, unknown>,
  range: { from: string; to: string },
) {
  const panes = {
    "event-hunter": {
      datasource,
      queries: [
        {
          refId: "A",
          datasource: { uid: datasource, type: datasourceType },
          ...query,
        },
      ],
      range,
    },
  };
  const search = new URLSearchParams({
    panes: JSON.stringify(panes),
    schemaVersion: "1",
    orgId: "1",
  });
  return `${trustedGrafanaBase(grafanaUrl)}/explore?${search.toString()}`;
}

function sqlLiteral(value: string) {
  return `'${value.replaceAll("'", "''")}'`;
}

export function eventObservabilityLinks(
  event: ObservableEvent,
  grafanaUrl = defaultGrafanaUrl,
): ObservabilityLink[] {
  const range = eventRange(event);
  const links: ObservabilityLink[] = [
    {
      kind: "event",
      label: "Grafana Explore",
      href: exploreUrl(
        grafanaUrl,
        "clickhouse",
        "grafana-clickhouse-datasource",
        {
          rawSql: `SELECT * FROM forensics_events WHERE event_id = ${sqlLiteral(event.event_id)} ORDER BY occurred_at`,
          format: 1,
        },
        range,
      ),
    },
    {
      kind: "logs",
      label: "Loki logs",
      href: exploreUrl(
        grafanaUrl,
        "loki",
        "loki",
        {
          expr: `{service_name=${JSON.stringify(event.producer)}} | correlation_id=${JSON.stringify(event.correlation_id)}`,
          queryType: "range",
        },
        range,
      ),
    },
  ];

  if (event.trace_id) {
    links.push({
      kind: "trace",
      label: "Tempo trace",
      href: exploreUrl(
        grafanaUrl,
        "tempo",
        "tempo",
        { query: event.trace_id, queryType: "traceql" },
        range,
      ),
    });
  }

  links.push({
    kind: "dashboard",
    label: "Quality Dashboard",
    href: qualityDashboardLink(range.from, range.to, grafanaUrl),
  });
  return links;
}

export function qualityDashboardLink(
  from: string,
  to: string,
  grafanaUrl = defaultGrafanaUrl,
) {
  const normalizeTime = (value: string) => {
    const parsed = Date.parse(value);
    return Number.isFinite(parsed) ? String(parsed) : value;
  };
  const dashboardSearch = new URLSearchParams({
    orgId: "1",
    from: normalizeTime(from),
    to: normalizeTime(to),
  });
  return `${trustedGrafanaBase(grafanaUrl)}/d/event-quality?${dashboardSearch.toString()}`;
}

export function traceObservabilityLink(
  traceID: string,
  from: string,
  to: string,
  grafanaUrl = defaultGrafanaUrl,
) {
  return exploreUrl(
    grafanaUrl,
    "tempo",
    "tempo",
    { query: traceID, queryType: "traceql" },
    { from: String(Date.parse(from)), to: String(Date.parse(to)) },
  );
}

export function evidenceSourceLink(
  item: EvidenceReference,
  grafanaUrl = defaultGrafanaUrl,
): EvidenceSourceLink | null {
  const range = collectedRange(item.collected_at);
  switch (item.open_action) {
    case "GRAFANA_EVENT":
      return {
        label: "在 Grafana 開啟 Event",
        href: exploreUrl(
          grafanaUrl,
          "clickhouse",
          "grafana-clickhouse-datasource",
          {
            rawSql: `SELECT * FROM forensics_events WHERE event_id = ${sqlLiteral(item.reference)} ORDER BY occurred_at`,
            format: 1,
          },
          range,
        ),
        external: true,
      };
    case "GRAFANA_TEMPO":
      return {
        label: "在 Tempo 開啟 Trace",
        href: exploreUrl(
          grafanaUrl,
          "tempo",
          "tempo",
          { query: item.reference, queryType: "traceql" },
          range,
        ),
        external: true,
      };
    case "GRAFANA_LOKI":
      return {
        label: "在 Loki 開啟 Logs",
        href: exploreUrl(
          grafanaUrl,
          "loki",
          "loki",
          {
            expr: `{service_name=~".+"} |= ${JSON.stringify(item.reference)}`,
            queryType: "range",
          },
          range,
        ),
        external: true,
      };
    case "GRAFANA_DASHBOARD": {
      const search = new URLSearchParams({
        orgId: "1",
        from: range.from,
        to: range.to,
      });
      return {
        label: "開啟 Quality Dashboard",
        href: `${trustedGrafanaBase(grafanaUrl)}/d/event-quality?${search.toString()}`,
        external: true,
      };
    }
    case "PATTERN_LIBRARY": {
      const patternID = item.reference.split(":v")[0];
      const modelURL = legacyPatternModelURL(item.reference);
      return {
        label: modelURL ? "開啟 Check Model" : "開啟 Legacy Pattern",
        href:
          modelURL ??
          `/patterns?pattern_id=${encodeURIComponent(patternID)}#pattern-${encodeURIComponent(patternID)}`,
        external: false,
      };
    }
    case "GRAFANA_ALERT": {
      if (
        !item.source_locator ||
        !grafanaAlertPathPattern.test(item.source_locator) ||
        !Number.isSafeInteger(item.source_org_id) ||
        item.source_org_id! < 1
      ) {
        return null;
      }
      const search = new URLSearchParams({
        orgId: String(item.source_org_id),
      });
      return {
        label: "在 Grafana 開啟 Alert",
        href: `${trustedGrafanaBase(grafanaUrl)}${item.source_locator}?${search.toString()}`,
        external: true,
      };
    }
    case "NONE":
      return null;
  }
}
