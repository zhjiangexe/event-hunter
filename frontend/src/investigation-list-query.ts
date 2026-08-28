import type {
  InvestigationFilters,
  InvestigationStatus,
  Severity,
} from "./api";

const statuses: InvestigationStatus[] = [
  "OPEN",
  "INVESTIGATING",
  "WAITING_APPROVAL",
  "RESOLVED",
  "CLOSED",
];
const severities: Severity[] = ["LOW", "MEDIUM", "HIGH", "CRITICAL"];
const priorities = ["P0", "P1", "P2", "P3"] as const;

export type InvestigationListQueryState = {
  status?: InvestigationStatus;
  severity?: Severity;
  priority?: (typeof priorities)[number];
  assignee?: string;
  tag?: string;
  correlationId?: string;
  sortBy: "created_at" | "updated_at";
  sortOrder: "asc" | "desc";
  filters: InvestigationFilters;
  key: string;
};

export function parseInvestigationListQuery(
  search: string,
): InvestigationListQueryState {
  const params = new URLSearchParams(search);
  const requestedStatus = params.get("status") as InvestigationStatus;
  const requestedSeverity = params.get("severity") as Severity;
  const requestedPriority = params.get("priority") as
    (typeof priorities)[number] | null;
  const status = statuses.includes(requestedStatus)
    ? requestedStatus
    : undefined;
  const severity = severities.includes(requestedSeverity)
    ? requestedSeverity
    : undefined;
  const priority =
    requestedPriority && priorities.includes(requestedPriority)
      ? requestedPriority
      : undefined;
  const assignee = params.get("assignee")?.trim() || undefined;
  const tag = params.get("tag")?.trim() || undefined;
  const correlationId = params.get("correlation_id")?.trim() || undefined;
  const sortBy =
    params.get("sort_by") === "updated_at" ? "updated_at" : "created_at";
  const sortOrder = params.get("sort_order") === "asc" ? "asc" : "desc";
  const filters = {
    status,
    severity,
    priority,
    assignee,
    tag,
    correlation_id: correlationId,
    sort_by: sortBy,
    sort_order: sortOrder,
  } satisfies InvestigationFilters;

  return {
    status,
    severity,
    priority,
    assignee,
    tag,
    correlationId,
    sortBy,
    sortOrder,
    filters,
    key: [
      status,
      severity,
      priority,
      assignee,
      tag,
      correlationId,
      sortBy,
      sortOrder,
    ].join("|"),
  };
}
