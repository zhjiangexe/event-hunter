import createClient from "openapi-fetch";
import type { components, operations, paths } from "./generated/openapi";

type Schemas = components["schemas"];
type SearchQuery = operations["searchForensicsEvents"]["parameters"]["query"];

export type Role = Schemas["DemoRole"];
export type Severity = Schemas["Severity"];
export type Principal = Schemas["CurrentPrincipal"];
export type TimelineEvent = Schemas["CanonicalEvent"];
export type Timeline = Schemas["BusinessTimeline"];
export type Investigation = Schemas["InvestigationCase"];
export type InvestigationPage = Schemas["InvestigationPage"];
export type PatternFinding = Schemas["PatternFinding"];
export type PatternFindingFeedback = Schemas["PatternFindingFeedback"];
export type PatternFeedbackStatus =
  Schemas["ClassifyPatternFindingRequest"]["status"];
export type PatternResult = Schemas["AnalysisResult"];
export type PatternDefinition = Schemas["PatternDefinition"];
export type PatternEffectivenessSummary =
  Schemas["PatternEffectivenessSummary"];
export type EvidenceReference = Schemas["Evidence"];
export type AuditEntry = Schemas["AuditLogEntry"];
export type EvidenceManifest = Schemas["EvidenceBundleManifest"];
export type InvestigationSummary = Schemas["InvestigationSummary"];
export type InvestigationUpdate = Schemas["UpdateInvestigationRequest"];
export type InvestigationNote = Schemas["InvestigationNote"];
export type AddInvestigationNoteResponse =
  Schemas["AddInvestigationNoteResponse"];
export type AttachInvestigationEventResponse =
  Schemas["AttachInvestigationEventResponse"];
export type EventSearchResult = Schemas["EventSearchResponse"];
export type InvestigationOverview = Schemas["InvestigationOverview"];
export type SourceHealth = Schemas["SourceHealth"];
export type InvestigationStatus = Schemas["InvestigationStatus"];
export type SmartSearchResult = Schemas["SmartSearchResult"];
export type SmartSearchCandidate = Schemas["SmartSearchCandidate"];
export type BusinessJourney = Schemas["BusinessJourney"];
export type JourneyProfile = Schemas["JourneyProfile"];
export type SavedSearch = Schemas["SavedSearch"];
export type SavedSearchQuery = Schemas["SavedSearchQuery"];
export type SavedSearchTarget = Schemas["SavedSearchTarget"];
export type SearchPreset = Schemas["SearchPreset"];
export type IngestionIssue = Schemas["IngestionIssue"];
export type IngestionIssueKind = Schemas["IngestionIssueKind"];
export type IngestionIssuePage = Schemas["IngestionIssuePage"];

type IngestionIssueQuery = NonNullable<
  operations["listIngestionIssues"]["parameters"]["query"]
>;
export type IngestionIssueFilters = Omit<
  IngestionIssueQuery,
  "cursor" | "page_size"
>;

type InvestigationQuery = NonNullable<
  operations["listInvestigations"]["parameters"]["query"]
>;
export type InvestigationFilters = Pick<
  InvestigationQuery,
  | "status"
  | "severity"
  | "assignee"
  | "priority"
  | "tag"
  | "correlation_id"
  | "sort_by"
  | "sort_order"
>;
export type InvestigationReadWindow = {
  from?: string;
  to?: string;
};

// Number inputs remain strings while the user edits them. The API adapter is the
// only place that converts this view state into the generated OpenAPI query type.
export type EventSearchFilters = Omit<
  SearchQuery,
  "event_version" | "kafka_partition" | "kafka_offset"
> & {
  event_version?: string;
  kafka_partition?: string;
  kafka_offset?: string;
};

const runtimeOrigin =
  typeof window === "undefined" ? "http://localhost" : window.location.origin;
const configuredBase = import.meta.env.VITE_API_BASE_URL?.trim() || "/";
const apiBase = new URL(configuredBase, runtimeOrigin)
  .toString()
  .replace(/\/$/, "");

const client = createClient<paths>({
  baseUrl: apiBase,
  credentials: "include",
  fetch: (request) => globalThis.fetch(request),
});

type ClientResult<T> = {
  data?: T;
  error?: unknown;
  response: Response;
};

function apiErrorCode(error: unknown, response: Response): string {
  if (typeof error === "object" && error !== null && "code" in error) {
    const code = (error as { code?: unknown }).code;
    if (typeof code === "string" && code) return code;
  }
  return response.statusText || `HTTP_${response.status}`;
}

function unwrap<T>(result: ClientResult<T>): T {
  if (!result.response.ok) {
    throw new Error(apiErrorCode(result.error, result.response));
  }
  return result.data as T;
}

function optionalInteger(value: string | undefined, name: string) {
  if (value === undefined || value.trim() === "") return undefined;
  const parsed = Number(value);
  if (!Number.isSafeInteger(parsed) || parsed < 0) {
    throw new Error(`INVALID_${name.toUpperCase()}`);
  }
  return parsed;
}

function toSearchQuery(filters: EventSearchFilters): SearchQuery {
  const query: SearchQuery = {
    ...filters,
    event_version: optionalInteger(filters.event_version, "event_version"),
    kafka_partition: optionalInteger(
      filters.kafka_partition,
      "kafka_partition",
    ),
    kafka_offset: optionalInteger(filters.kafka_offset, "kafka_offset"),
  };

  for (const key of Object.keys(query) as Array<keyof SearchQuery>) {
    const value = query[key];
    if (typeof value === "string" && value.trim() === "") delete query[key];
    if (typeof value === "boolean" && !value) delete query[key];
  }
  return query;
}

export const api = {
  createSession: async (role: Role) =>
    unwrap(
      await client.POST("/api/v1/auth/demo-session", {
        body: { role },
      }),
    ),
  deleteSession: async () =>
    unwrap(await client.DELETE("/api/v1/auth/demo-session")),
  me: async () => unwrap(await client.GET("/api/v1/auth/me")),
  patterns: async () => unwrap(await client.GET("/api/v1/patterns")),
  patternEffectiveness: async () =>
    unwrap(await client.GET("/api/v1/patterns/effectiveness")),
  timeline: async (correlationId: string) =>
    unwrap(
      await client.GET("/api/v1/timelines/{correlationId}", {
        params: {
          path: { correlationId },
          query: {
            from: "2026-08-20T11:00:00Z",
            to: "2026-08-20T11:06:00Z",
          },
        },
      }),
    ),
  businessJourney: async (correlationId: string, from: string, to: string) =>
    unwrap(
      await client.GET("/api/v1/business-journeys/{correlationId}", {
        params: {
          path: { correlationId },
          query: { from, to },
        },
      }),
    ),
  journeyProfiles: async () =>
    unwrap(await client.GET("/api/v1/journey-profiles")),
  searchEvents: async (filters: EventSearchFilters) =>
    unwrap(
      await client.GET("/api/v1/events/search", {
        params: { query: toSearchQuery(filters) },
      }),
    ),
  overview: async () =>
    unwrap(await client.GET("/api/v1/investigations/overview")),
  sourceHealth: async () => unwrap(await client.GET("/api/v1/source-health")),
  ingestionIssues: async (
    filters: IngestionIssueFilters,
    cursor?: string,
    pageSize = 20,
  ) =>
    unwrap(
      await client.GET("/api/v1/ingestion-issues", {
        params: { query: { ...filters, cursor, page_size: pageSize } },
      }),
    ),
  identifySearchInput: async (input: string) =>
    unwrap(
      await client.POST("/api/v1/search/identify", {
        body: { input },
      }),
    ),
  savedSearches: async () => unwrap(await client.GET("/api/v1/saved-searches")),
  searchPresets: async () => unwrap(await client.GET("/api/v1/search-presets")),
  createSavedSearch: async (
    name: string,
    target: SavedSearchTarget,
    query: SavedSearchQuery,
  ) =>
    unwrap(
      await client.POST("/api/v1/saved-searches", {
        body: { name, target, query },
      }),
    ),
  deleteSavedSearch: async (id: string) =>
    unwrap(
      await client.DELETE("/api/v1/saved-searches/{savedSearchId}", {
        params: { path: { savedSearchId: id } },
      }),
    ),
  investigations: async (
    cursor?: string,
    filters: InvestigationFilters = {},
    pageSize = 10,
  ) =>
    unwrap(
      await client.GET("/api/v1/investigations", {
        params: { query: { page_size: pageSize, cursor, ...filters } },
      }),
    ),
  investigation: async (id: string) =>
    unwrap(
      await client.GET("/api/v1/investigations/{investigationId}", {
        params: { path: { investigationId: id } },
      }),
    ),
  createInvestigation: async (
    input: Pick<
      Schemas["CreateInvestigationRequest"],
      "title" | "severity" | "correlation_id" | "incident_from" | "incident_to"
    >,
  ) =>
    unwrap(
      await client.POST("/api/v1/investigations", {
        body: { ...input, start_workflow: false },
      }),
    ),
  analyze: async (
    id: string,
    patternIds?: string[],
  ): Promise<PatternResult> => {
    const result = await client.POST(
      "/api/v1/investigations/{investigationId}/analyze",
      {
        params: { path: { investigationId: id } },
        body: {
          ...(patternIds ? { pattern_ids: patternIds } : {}),
          execution_mode: "SYNC",
        },
      },
    );
    if (result.response.status !== 200) {
      throw new Error(apiErrorCode(result.error, result.response));
    }
    return result.data as PatternResult;
  },
  classifyPatternFinding: async (
    investigationId: string,
    findingId: string,
    lockVersion: number,
    status: PatternFeedbackStatus,
  ) =>
    unwrap(
      await client.PATCH(
        "/api/v1/investigations/{investigationId}/findings/{findingId}/feedback",
        {
          params: {
            path: { investigationId, findingId },
            header: { "If-Match": `"v${lockVersion}"` },
          },
          body: { status },
        },
      ),
    ),
  evidence: async (id: string, window: InvestigationReadWindow = {}) =>
    unwrap(
      await client.GET(
        "/api/v1/investigations/{investigationId}/evidence-bundle",
        {
          params: {
            path: { investigationId: id },
            query: window,
          },
        },
      ),
    ),
  summary: async (id: string, window: InvestigationReadWindow = {}) =>
    unwrap(
      await client.GET("/api/v1/investigations/{investigationId}/summary", {
        params: {
          path: { investigationId: id },
          query: window,
        },
      }),
    ),
  patchInvestigation: async (item: Investigation, input: InvestigationUpdate) =>
    unwrap(
      await client.PATCH("/api/v1/investigations/{investigationId}", {
        params: {
          path: { investigationId: item.id },
          header: { "If-Match": `"v${item.lock_version}"` },
        },
        body: input,
      }),
    ),
  addInvestigationNote: async (item: Investigation, body: string) =>
    unwrap(
      await client.POST("/api/v1/investigations/{investigationId}/notes", {
        params: {
          path: { investigationId: item.id },
          header: { "If-Match": `"v${item.lock_version}"` },
        },
        body: { body },
      }),
    ),
  attachInvestigationEvent: async (
    item: Investigation,
    eventId: string,
    from: string,
    to: string,
  ): Promise<AttachInvestigationEventResponse> =>
    unwrap(
      await client.POST(
        "/api/v1/investigations/{investigationId}/evidence/events",
        {
          params: {
            path: { investigationId: item.id },
            header: { "If-Match": `"v${item.lock_version}"` },
          },
          body: { event_id: eventId, from, to },
        },
      ),
    ),
  closeInvestigation: async (
    item: Investigation,
    rootCause: string,
    resolutionSummary: string,
  ) =>
    unwrap(
      await client.POST("/api/v1/investigations/{investigationId}/close", {
        params: {
          path: { investigationId: item.id },
          header: { "If-Match": `"v${item.lock_version}"` },
        },
        body: {
          root_cause: rootCause,
          resolution_summary: resolutionSummary,
        },
      }),
    ),
};
