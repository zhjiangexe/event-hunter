import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useLocation, useNavigate } from "react-router-dom";
import {
  api,
  type CheckFindingFeedbackStatus,
  type CheckModelRegistryEntry,
  type CheckSnapshot,
  type CheckSnapshotSummary,
  type CheckSnapshotFindingFeedback,
  type EventCheckEvaluation,
  type EventCheckEvaluationRequest,
  type EventCheckEventReference,
  type EventCheckFinding,
  type EventCheckIdentifierType,
  type EventCheckRelationship,
  type Investigation,
  type Principal,
  type SavedSearchQuery,
  type Severity,
} from "./api";
import { eventObservabilityLinks } from "./observability-links";

type WorkspaceTab = "summary" | "timeline" | "flow" | "findings" | "cases";
type ModelKind = "FLOW" | "GLOBAL_CHECK";
type ModelPanel = "overview" | "versions" | "scenarios";
type ScopeAdjustment = { event_id: string; reason: string };

const identifierTypes: Array<{
  value: EventCheckIdentifierType;
  label: string;
}> = [
  { value: "AUTO", label: "自動辨識" },
  { value: "CORRELATION_ID", label: "Correlation ID" },
  { value: "EVENT_ID", label: "Event ID" },
  { value: "TRACE_ID", label: "Trace ID" },
  { value: "AGGREGATE_ID", label: "Aggregate ID" },
  { value: "BUSINESS_KEY", label: "Business Key" },
];

const workspaceTabs: Array<{ id: WorkspaceTab; label: string }> = [
  { id: "summary", label: "Summary" },
  { id: "timeline", label: "Timeline" },
  { id: "flow", label: "Flow" },
  { id: "findings", label: "Findings" },
  { id: "cases", label: "Cases" },
];

const dateTimeFormatter = new Intl.DateTimeFormat("zh-TW", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hourCycle: "h23",
});

function dynamicWindow() {
  const to = new Date();
  return {
    from: new Date(to.getTime() - 72 * 60 * 60_000).toISOString(),
    to: to.toISOString(),
  };
}

function toLocalInput(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 19);
}

function validDateWindow(from: string, to: string) {
  const start = new Date(from).getTime();
  const end = new Date(to).getTime();
  return (
    Number.isFinite(start) &&
    Number.isFinite(end) &&
    start < end &&
    end - start <= 7 * 24 * 60 * 60_000
  );
}

function pairedAdjustments(
  params: URLSearchParams,
  prefix: "include" | "exclude",
): ScopeAdjustment[] {
  const ids = params.getAll(`${prefix}_event_id`).slice(0, 20);
  const reasons = params.getAll(`${prefix}_reason`).slice(0, 20);
  return ids.flatMap((eventID, index) => {
    const reason = reasons[index]?.trim();
    if (!eventID.trim() || !reason) return [];
    return [{ event_id: eventID.trim(), reason: reason.slice(0, 500) }];
  });
}

function requestFromSearch(search: string): EventCheckEvaluationRequest | null {
  const params = new URLSearchParams(search);
  const identifierValue = params.get("identifier")?.trim() ?? "";
  const identifierType = params.get(
    "identifier_type",
  ) as EventCheckIdentifierType | null;
  const from = params.get("from") ?? "";
  const to = params.get("to") ?? "";
  if (
    !identifierValue ||
    !identifierTypes.some((item) => item.value === identifierType) ||
    !validDateWindow(from, to)
  ) {
    return null;
  }
  const aggregateType = params.get("aggregate_type")?.trim();
  const businessKeyName = params.get("business_key_name")?.trim();
  const modelID = params.get("model_id")?.trim();
  const modelVersion = Number(params.get("model_version"));
  const include = pairedAdjustments(params, "include");
  const exclude = pairedAdjustments(params, "exclude");
  return {
    identifier: {
      type: identifierType!,
      value: identifierValue,
      ...(aggregateType || businessKeyName
        ? {
            qualifier: {
              ...(aggregateType ? { aggregate_type: aggregateType } : {}),
              ...(businessKeyName
                ? { business_key_name: businessKeyName }
                : {}),
            },
          }
        : {}),
    },
    from,
    to,
    ...(modelID && Number.isSafeInteger(modelVersion) && modelVersion > 0
      ? { model: { id: modelID, version: modelVersion } }
      : {}),
    ...(include.length || exclude.length
      ? { scope_adjustments: { include, exclude } }
      : {}),
  };
}

function setAdjustments(
  params: URLSearchParams,
  include: ScopeAdjustment[],
  exclude: ScopeAdjustment[],
) {
  for (const key of [
    "include_event_id",
    "include_reason",
    "exclude_event_id",
    "exclude_reason",
  ]) {
    params.delete(key);
  }
  for (const item of include.slice(0, 20)) {
    params.append("include_event_id", item.event_id);
    params.append("include_reason", item.reason);
  }
  for (const item of exclude.slice(0, 20)) {
    params.append("exclude_event_id", item.event_id);
    params.append("exclude_reason", item.reason);
  }
}

function modelKey(entry: CheckModelRegistryEntry) {
  return `${entry.model.model_id}@${entry.model.version}`;
}

function shortHash(value: string | null | undefined) {
  return value ? `${value.slice(0, 10)}…${value.slice(-8)}` : "—";
}

function eventCheckError(error: unknown, fallback: string) {
  const code = error instanceof Error ? error.message : "UNKNOWN_ERROR";
  const messages: Record<string, string> = {
    EVENT_CHECK_SOURCE_UNAVAILABLE: "Canonical event source 目前無法使用。",
    EVENT_CHECK_SOURCE_TIMEOUT: "事件查詢逾時，請縮小時間範圍後重試。",
    EVALUATION_CHANGED: "事件集合或判斷結果已更新，請重新執行並檢視。",
    OPTIMISTIC_LOCK_CONFLICT: "資料已被其他人更新，已重新載入最新版本。",
    FORBIDDEN: "目前角色沒有執行此操作的權限。",
  };
  return `${messages[code] ?? fallback}（${code}）`;
}

function EventCheckShortcutsDialog({
  open,
  onClose,
  currentQuery,
  principal,
}: {
  open: boolean;
  onClose: () => void;
  currentQuery?: SavedSearchQuery;
  principal: Principal;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [timeMode, setTimeMode] = useState<"ABSOLUTE" | "RELATIVE">("ABSOLUTE");
  const [relativeWindowSeconds, setRelativeWindowSeconds] = useState(259_200);
  const savedSearches = useQuery({
    queryKey: ["saved-searches"],
    queryFn: api.savedSearches,
    enabled: open,
  });
  const save = useMutation({
    mutationFn: () => {
      if (!currentQuery) throw new Error("NO_ACTIVE_QUERY");
      return api.createSavedSearch(name.trim(), "EVENT_CHECK", {
        ...currentQuery,
        time_mode: timeMode,
        relative_window_seconds:
          timeMode === "RELATIVE" ? relativeWindowSeconds : undefined,
      });
    },
    onSuccess: async () => {
      setName("");
      await queryClient.invalidateQueries({ queryKey: ["saved-searches"] });
    },
  });
  const remove = useMutation({
    mutationFn: api.deleteSavedSearch,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["saved-searches"] });
    },
  });

  useEffect(() => {
    if (!open) return;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") onClose();
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, [onClose, open]);

  if (!open) return null;
  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <section
        className="case-drawer query-shortcuts-drawer"
        data-testid="event-check-shortcuts"
        role="dialog"
        aria-modal="true"
        aria-labelledby="event-check-shortcuts-title"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="drawer-header">
          <div>
            <p className="eyebrow">BOUNDED EVENT CHECK SHORTCUTS</p>
            <h3 id="event-check-shortcuts-title">查詢捷徑</h3>
            <p className="muted">
              保存識別碼、時間範圍、Model 與目前分頁；不保存 payload。
            </p>
          </div>
          <button
            type="button"
            className="drawer-close"
            aria-label="關閉查詢捷徑"
            onClick={onClose}
          >
            ×
          </button>
        </div>
        <section className="query-shortcuts-section">
          <div className="card-heading">
            <div>
              <p className="eyebrow">OWNED BY {principal.subject}</p>
              <h4>我的查詢</h4>
            </div>
            <span className="badge">
              {savedSearches.data?.items.length ?? 0}
            </span>
          </div>
          {savedSearches.isLoading && <p className="muted">載入查詢捷徑…</p>}
          {savedSearches.isError && (
            <p className="field-error">無法載入查詢捷徑。</p>
          )}
          {savedSearches.data?.items.length === 0 && (
            <p className="empty-inline">尚未儲存查詢。</p>
          )}
          <div className="saved-search-list">
            {savedSearches.data?.items.map((item, index) => (
              <article
                className="saved-search-row"
                data-testid={`event-check-shortcut-row-${index}`}
                key={item.id}
              >
                <span className="saved-search-target">{item.target}</span>
                <div>
                  <strong>{item.name}</strong>
                  <small>
                    {new Date(item.updated_at).toLocaleString("zh-TW")}
                  </small>
                </div>
                <a
                  data-testid={`event-check-shortcut-open-${index}`}
                  href={item.open_url}
                >
                  開啟 →
                </a>
                <button
                  type="button"
                  className="button-ghost saved-search-delete"
                  data-testid={`event-check-shortcut-delete-${index}`}
                  aria-label={`刪除 ${item.name}`}
                  disabled={remove.isPending}
                  onClick={() => remove.mutate(item.id)}
                >
                  刪除
                </button>
              </article>
            ))}
          </div>
        </section>
        <form
          className="save-search-form query-shortcuts-save"
          onSubmit={(event) => {
            event.preventDefault();
            if (currentQuery && name.trim()) save.mutate();
          }}
        >
          <div className="card-heading">
            <div>
              <p className="eyebrow">CURRENT EVENT CHECK</p>
              <h4>儲存目前條件</h4>
            </div>
            <span className="saved-search-target">EVENT_CHECK</span>
          </div>
          {!currentQuery && (
            <p className="empty-inline">請先執行一次 Event Check。</p>
          )}
          <label>
            名稱
            <input
              data-testid="event-check-shortcut-name"
              value={name}
              maxLength={80}
              disabled={!currentQuery}
              onChange={(event) => setName(event.target.value)}
            />
          </label>
          <label>
            時間模式
            <select
              data-testid="event-check-shortcut-time-mode"
              value={timeMode}
              disabled={!currentQuery}
              onChange={(event) =>
                setTimeMode(event.target.value as "ABSOLUTE" | "RELATIVE")
              }
            >
              <option value="ABSOLUTE">固定目前時間</option>
              <option value="RELATIVE">每次開啟重新計算</option>
            </select>
          </label>
          {timeMode === "RELATIVE" && (
            <label>
              相對範圍
              <select
                data-testid="event-check-shortcut-relative-window"
                value={relativeWindowSeconds}
                onChange={(event) =>
                  setRelativeWindowSeconds(Number(event.target.value))
                }
              >
                <option value={3_600}>最近 1 小時</option>
                <option value={86_400}>最近 24 小時</option>
                <option value={259_200}>最近 72 小時</option>
                <option value={604_800}>最近 7 天</option>
              </select>
            </label>
          )}
          <button
            data-testid="event-check-shortcut-save"
            disabled={!currentQuery || !name.trim() || save.isPending}
          >
            儲存捷徑
          </button>
          {save.isError && (
            <p className="field-error" role="alert">
              {eventCheckError(save.error, "無法儲存查詢捷徑。")}
            </p>
          )}
        </form>
      </section>
    </div>
  );
}

function SearchForm({
  request,
}: {
  request: EventCheckEvaluationRequest | null;
}) {
  const navigate = useNavigate();
  const location = useLocation();
  const initialWindow = useMemo(() => dynamicWindow(), []);
  const params = new URLSearchParams(location.search);
  const [identifierType, setIdentifierType] =
    useState<EventCheckIdentifierType>(
      request?.identifier.type ??
        ((params.get("identifier_type") as EventCheckIdentifierType) || "AUTO"),
    );
  const [identifier, setIdentifier] = useState(
    request?.identifier.value ?? params.get("identifier") ?? "",
  );
  const [from, setFrom] = useState(
    toLocalInput(request?.from ?? params.get("from") ?? initialWindow.from),
  );
  const [to, setTo] = useState(
    toLocalInput(request?.to ?? params.get("to") ?? initialWindow.to),
  );
  const [aggregateType, setAggregateType] = useState(
    request?.identifier.qualifier?.aggregate_type ?? "",
  );
  const [businessKeyName, setBusinessKeyName] = useState(
    request?.identifier.qualifier?.business_key_name ?? "",
  );
  const [validation, setValidation] = useState("");

  const submit = (event: FormEvent) => {
    event.preventDefault();
    const fromISO = new Date(from).toISOString();
    const toISO = new Date(to).toISOString();
    if (!identifier.trim()) {
      setValidation("請輸入一個識別碼。");
      return;
    }
    if (!validDateWindow(fromISO, toISO)) {
      setValidation("起始時間必須早於結束時間，且查詢範圍不可超過 7 天。");
      return;
    }
    const next = new URLSearchParams();
    next.set("identifier_type", identifierType);
    next.set("identifier", identifier.trim());
    next.set("from", fromISO);
    next.set("to", toISO);
    if (aggregateType.trim()) next.set("aggregate_type", aggregateType.trim());
    if (businessKeyName.trim())
      next.set("business_key_name", businessKeyName.trim());
    navigate(`/event-check?${next.toString()}`);
  };

  return (
    <form className="event-check-search card" onSubmit={submit}>
      <div className="event-check-search-grid">
        <label>
          識別碼類型
          <select
            data-testid="event-check-identifier-type"
            value={identifierType}
            onChange={(event) =>
              setIdentifierType(event.target.value as EventCheckIdentifierType)
            }
          >
            {identifierTypes.map((item) => (
              <option value={item.value} key={item.value}>
                {item.label}
              </option>
            ))}
          </select>
        </label>
        <label className="event-check-identifier-field">
          識別碼
          <input
            data-testid="event-check-identifier"
            value={identifier}
            onChange={(event) => setIdentifier(event.target.value)}
            placeholder="例如 ORDER-…、Event ID 或 Trace ID"
          />
        </label>
        <label>
          起始時間
          <input
            data-testid="event-check-from"
            type="datetime-local"
            step="1"
            value={from}
            onChange={(event) => setFrom(event.target.value)}
          />
        </label>
        <label>
          結束時間 / as-of
          <input
            data-testid="event-check-to"
            type="datetime-local"
            step="1"
            value={to}
            onChange={(event) => setTo(event.target.value)}
          />
        </label>
      </div>
      {(identifierType === "AGGREGATE_ID" ||
        identifierType === "BUSINESS_KEY") && (
        <div className="event-check-qualifiers">
          {identifierType === "AGGREGATE_ID" && (
            <label>
              Aggregate type
              <input
                value={aggregateType}
                onChange={(event) => setAggregateType(event.target.value)}
                placeholder="例如 Order"
              />
            </label>
          )}
          {identifierType === "BUSINESS_KEY" && (
            <label>
              Business key name
              <input
                value={businessKeyName}
                onChange={(event) => setBusinessKeyName(event.target.value)}
                placeholder="例如 order_id"
              />
            </label>
          )}
        </div>
      )}
      <div className="event-check-search-actions">
        <span>預設最近 72 小時 · 最長 7 天 · 最多 10,000 events</span>
        <button data-testid="event-check-submit" type="submit">
          執行 Event Check
        </button>
      </div>
      {validation && (
        <p className="field-error" role="alert">
          {validation}
        </p>
      )}
    </form>
  );
}

function SourceHealthPanel({
  sourceHealth,
}: {
  sourceHealth: EventCheckEvaluation["source_health"];
}) {
  return (
    <section
      className="event-check-source-health"
      aria-labelledby="source-health-title"
      data-testid="event-check-source-health"
    >
      <div>
        <p className="eyebrow">SOURCE HEALTH</p>
        <h3 id="source-health-title">資料可信度</h3>
      </div>
      <span
        className={`source-health-pill source-${sourceHealth.status.toLowerCase()}`}
      >
        {sourceHealth.status}
      </span>
      <dl>
        <div>
          <dt>Coverage</dt>
          <dd>
            {dateTimeFormatter.format(new Date(sourceHealth.coverage_from))} →{" "}
            {dateTimeFormatter.format(new Date(sourceHealth.coverage_to))}
          </dd>
        </div>
        <div>
          <dt>Watermark</dt>
          <dd>
            {sourceHealth.watermark
              ? dateTimeFormatter.format(new Date(sourceHealth.watermark))
              : "不可用"}
          </dd>
        </div>
        <div>
          <dt>Result set</dt>
          <dd>{sourceHealth.truncated ? "PARTIAL / 已截斷" : "完整"}</dd>
        </div>
      </dl>
      <div className="source-component-list">
        {(sourceHealth.components ?? []).map((component) => (
          <span key={component.component}>
            {component.component}: <strong>{component.status}</strong>
            <small>{component.detail_code}</small>
          </span>
        ))}
      </div>
    </section>
  );
}

function CustomScopeEditor({
  request,
  disabled,
}: {
  request: EventCheckEvaluationRequest;
  disabled: boolean;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const current = request.scope_adjustments ?? { include: [], exclude: [] };
  const [disposition, setDisposition] = useState<"include" | "exclude">(
    "exclude",
  );
  const [eventID, setEventID] = useState("");
  const [reason, setReason] = useState("");
  const [error, setError] = useState("");

  const update = (include: ScopeAdjustment[], exclude: ScopeAdjustment[]) => {
    const next = new URLSearchParams(location.search);
    next.delete("snapshot_id");
    setAdjustments(next, include, exclude);
    next.set("tab", "timeline");
    navigate(`/event-check?${next.toString()}`);
  };
  const add = (event: FormEvent) => {
    event.preventDefault();
    if (!eventID.trim() || !reason.trim()) {
      setError("Event ID 與調整原因都必填。");
      return;
    }
    const candidate = { event_id: eventID.trim(), reason: reason.trim() };
    const include = current.include.filter(
      (item) => item.event_id !== candidate.event_id,
    );
    const exclude = current.exclude.filter(
      (item) => item.event_id !== candidate.event_id,
    );
    if (disposition === "include") include.push(candidate);
    else exclude.push(candidate);
    update(include, exclude);
  };

  return (
    <details
      className="custom-scope-editor"
      open={Boolean(current.include.length || current.exclude.length)}
    >
      <summary>
        自訂事件範圍
        <span>{current.include.length + current.exclude.length} 項調整</span>
      </summary>
      <p>
        只調整這次檢查，不修改 canonical events。每一筆調整都會保存 Event ID
        與人工原因。
      </p>
      {(current.include.length > 0 || current.exclude.length > 0) && (
        <ul className="scope-adjustment-list">
          {(["include", "exclude"] as const).flatMap((kind) =>
            current[kind].map((item) => (
              <li key={`${kind}:${item.event_id}`}>
                <span>{kind === "include" ? "納入" : "排除"}</span>
                <code>{item.event_id}</code>
                <small>{item.reason}</small>
                {!disabled && (
                  <button
                    type="button"
                    className="button-ghost"
                    aria-label={`移除 ${item.event_id} 的調整`}
                    onClick={() =>
                      update(
                        kind === "include"
                          ? current.include.filter((value) => value !== item)
                          : current.include,
                        kind === "exclude"
                          ? current.exclude.filter((value) => value !== item)
                          : current.exclude,
                      )
                    }
                  >
                    移除
                  </button>
                )}
              </li>
            )),
          )}
        </ul>
      )}
      {!disabled && (
        <form className="scope-adjustment-form" onSubmit={add}>
          <label>
            動作
            <select
              value={disposition}
              onChange={(event) =>
                setDisposition(event.target.value as "include" | "exclude")
              }
            >
              <option value="exclude">排除事件</option>
              <option value="include">納入事件</option>
            </select>
          </label>
          <label>
            Event ID
            <input
              value={eventID}
              onChange={(event) => setEventID(event.target.value)}
            />
          </label>
          <label className="scope-reason-field">
            人工原因
            <input
              value={reason}
              onChange={(event) => setReason(event.target.value)}
              maxLength={500}
            />
          </label>
          <button type="submit">套用並重新檢查</button>
        </form>
      )}
      {error && <p className="field-error">{error}</p>}
    </details>
  );
}

function EventList({
  events,
  excluded,
  relationships,
}: {
  events: Array<
    EventCheckEventReference | CheckSnapshot["event_references"][number]
  >;
  excluded: Array<
    EventCheckEventReference | CheckSnapshot["event_references"][number]
  >;
  relationships: EventCheckRelationship[];
}) {
  const relationByTarget = useMemo(() => {
    const index = new Map<string, EventCheckRelationship[]>();
    for (const relation of relationships) {
      const values = index.get(relation.to_event_id) ?? [];
      values.push(relation);
      index.set(relation.to_event_id, values);
    }
    return index;
  }, [relationships]);

  const rows = [
    ...events.map((event) => ({ event, disposition: "INCLUDED" as const })),
    ...excluded.map((event) => ({ event, disposition: "EXCLUDED" as const })),
  ];
  if (!rows.length) return <p className="empty-inline">此範圍沒有事件。</p>;
  return (
    <div className="check-event-list" data-testid="event-check-event-list">
      {rows.map(({ event, disposition }, index) => (
        <article
          className={`check-event-row disposition-${disposition.toLowerCase()}`}
          data-testid={`event-check-event-${index}`}
          key={`${disposition}:${event.event_id}`}
        >
          <span className="event-index">
            {String(index + 1).padStart(2, "0")}
          </span>
          <div className="check-event-main">
            <div>
              <strong data-testid={`event-check-event-${index}-type`}>
                {event.event_type}
              </strong>
              <span>{disposition}</span>
            </div>
            <time
              dateTime={event.occurred_at}
              data-testid={`event-check-event-${index}-occurred-at`}
            >
              {dateTimeFormatter.format(new Date(event.occurred_at))}
            </time>
            <code data-testid={`event-check-event-${index}-id`}>
              {event.event_id}
            </code>
          </div>
          <dl>
            <div>
              <dt>Aggregate</dt>
              <dd>
                {event.aggregate_type} / {event.aggregate_id}
              </dd>
            </div>
            <div>
              <dt>Correlation</dt>
              <dd>{event.correlation_id}</dd>
            </div>
            <div>
              <dt>Producer</dt>
              <dd>{event.producer}</dd>
            </div>
            <div>
              <dt>Trace</dt>
              <dd>{event.trace_id ?? "—"}</dd>
            </div>
          </dl>
          <div
            className="observability-links check-event-observability"
            data-testid={`event-check-event-${index}-observability`}
          >
            {eventObservabilityLinks(event).map((link) => (
              <a
                href={link.href}
                target="_blank"
                rel="noreferrer"
                data-testid={`event-check-event-${index}-link-${link.kind}`}
                key={link.kind}
              >
                {link.label} ↗
              </a>
            ))}
          </div>
          {"adjustment_reason" in event && event.adjustment_reason && (
            <p className="scope-reason">人工調整：{event.adjustment_reason}</p>
          )}
          {(relationByTarget.get(event.event_id)?.length ?? 0) > 0 && (
            <details className="relationship-detail">
              <summary>
                {relationByTarget.get(event.event_id)!.length} 個關聯原因
              </summary>
              <ul>
                {relationByTarget.get(event.event_id)!.map((relation) => (
                  <li key={relation.ordinal}>
                    <strong>{relation.relation_type}</strong>
                    <span>
                      {relation.from_event_id ?? "查詢起點"} →{" "}
                      {relation.to_event_id}
                    </span>
                    <small>
                      {relation.source_field ??
                        relation.source_rule_id ??
                        "platform resolver"}
                    </small>
                  </li>
                ))}
              </ul>
            </details>
          )}
        </article>
      ))}
    </div>
  );
}

function FindingsPanel({
  findings,
  snapshot,
  principal,
}: {
  findings: EventCheckFinding[];
  snapshot: CheckSnapshot | null;
  principal: Principal;
}) {
  const queryClient = useQueryClient();
  const feedbackByFinding = new Map(
    (snapshot?.finding_feedback ?? []).map((item) => [item.finding_id, item]),
  );
  const classify = useMutation({
    mutationFn: ({
      findingID,
      lockVersion,
      status,
    }: {
      findingID: string;
      lockVersion: number;
      status: CheckFindingFeedbackStatus;
    }) => api.classifyCheckFinding(findingID, lockVersion, status),
    onSuccess: (updated) => {
      if (!snapshot) return;
      queryClient.setQueryData<CheckSnapshot>(
        ["check-snapshot", snapshot.id],
        (current) =>
          current
            ? {
                ...current,
                finding_feedback: (current.finding_feedback ?? []).map(
                  (item) =>
                    item.finding_id === updated.finding_id
                      ? (updated as CheckSnapshotFindingFeedback)
                      : item,
                ),
              }
            : current,
      );
    },
    onError: (error) => {
      if (snapshot && error.message === "OPTIMISTIC_LOCK_CONFLICT") {
        void queryClient.invalidateQueries({
          queryKey: ["check-snapshot", snapshot.id],
        });
      }
    },
  });
  if (!findings.length)
    return <p className="empty-inline">這次檢查沒有 Finding。</p>;
  return (
    <div className="check-finding-list">
      {findings.map((finding, index) => {
        const feedback = finding.id
          ? feedbackByFinding.get(finding.id)
          : undefined;
        return (
          <article
            className="check-finding-card"
            key={`${finding.code}:${index}`}
          >
            <div className="check-finding-heading">
              <div>
                <p className="eyebrow">{finding.rule_kind}</p>
                <h3>{finding.code}</h3>
              </div>
              <span
                className={`severity severity-${finding.severity.toLowerCase()}`}
              >
                {finding.severity}
              </span>
            </div>
            <dl>
              <div>
                <dt>Rule</dt>
                <dd>
                  {finding.rule_id} v{finding.rule_version}
                </dd>
              </div>
              <div>
                <dt>Expectation</dt>
                <dd>{finding.expectation_state ?? "不適用"}</dd>
              </div>
              <div>
                <dt>Feedback</dt>
                <dd>{feedback?.status ?? "尚未保存"}</dd>
              </div>
            </dl>
            <div className="finding-evidence-list">
              {(finding.evidence_references ?? []).map((reference) => (
                <span key={`${reference.type}:${reference.value}`}>
                  <strong>{reference.type}</strong>
                  <code>{reference.value}</code>
                </span>
              ))}
            </div>
            {!snapshot && (
              <p className="muted">
                保存 Check Snapshot 後才會配置 Finding ID 並開放人工判定。
              </p>
            )}
            {snapshot && finding.id && principal.role !== "VIEWER" && (
              <div className="finding-feedback-actions">
                {(["CONFIRMED", "FALSE_POSITIVE", "NEEDS_REVIEW"] as const).map(
                  (status) => (
                    <button
                      type="button"
                      className={
                        feedback?.status === status
                          ? "active"
                          : "button-secondary"
                      }
                      disabled={classify.isPending}
                      onClick={() =>
                        classify.mutate({
                          findingID: finding.id!,
                          lockVersion: feedback?.lock_version ?? 0,
                          status,
                        })
                      }
                      key={status}
                    >
                      {status === "CONFIRMED"
                        ? "確認異常"
                        : status === "FALSE_POSITIVE"
                          ? "標記誤報"
                          : "需要複核"}
                    </button>
                  ),
                )}
              </div>
            )}
          </article>
        );
      })}
      {classify.isError && (
        <p className="field-error">
          {eventCheckError(classify.error, "無法更新 Finding feedback。")}
        </p>
      )}
    </div>
  );
}

type SnapshotReference = Pick<CheckSnapshot, "id">;

type CreateCaseDialogProps = {
  open: boolean;
  snapshot: SnapshotReference | null;
  correlationID: string;
  incidentFrom?: string;
  incidentTo?: string;
  checkStatus: string;
  onClose: () => void;
};

export function CreateCaseDialog(props: CreateCaseDialogProps) {
  if (!props.open || !props.snapshot) return null;
  return (
    <CreateCaseDialogContent
      key={`${props.snapshot.id}:${props.correlationID}:${props.checkStatus}`}
      snapshot={props.snapshot}
      correlationID={props.correlationID}
      incidentFrom={props.incidentFrom}
      incidentTo={props.incidentTo}
      checkStatus={props.checkStatus}
      onClose={props.onClose}
    />
  );
}

function CreateCaseDialogContent({
  snapshot,
  correlationID,
  incidentFrom,
  incidentTo,
  checkStatus,
  onClose,
}: Omit<CreateCaseDialogProps, "open" | "snapshot"> & {
  snapshot: SnapshotReference;
}) {
  const queryClient = useQueryClient();
  const dialogRef = useRef<HTMLFormElement>(null);
  const previousFocus = useRef<HTMLElement | null>(null);
  const [title, setTitle] = useState(
    () => `[Event Check] ${correlationID} · ${checkStatus}`,
  );
  const [severity, setSeverity] = useState<Severity>(() =>
    checkStatus === "DEVIATED" ? "HIGH" : "MEDIUM",
  );
  const [createdCase, setCreatedCase] = useState<Investigation | null>(null);
  const attach = useMutation({
    mutationFn: (item: Investigation) =>
      api.attachInvestigationCheckSnapshot(
        item.id,
        snapshot.id,
        item.lock_version,
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["investigations"] });
      void queryClient.invalidateQueries({ queryKey: ["check-snapshots"] });
    },
  });
  const create = useMutation({
    mutationFn: () =>
      api.createInvestigation({
        title: title.trim(),
        severity,
        correlation_id: correlationID,
        incident_from: incidentFrom,
        incident_to: incidentTo,
      }),
    onSuccess: (item) => {
      setCreatedCase(item);
      attach.mutate(item);
    },
  });
  useEffect(() => {
    previousFocus.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    const dialog = dialogRef.current;
    if (!dialog) return;
    const focusable = () =>
      Array.from(
        dialog.querySelectorAll<HTMLElement>(
          "button:not([disabled]),a[href],input:not([disabled]),select:not([disabled])",
        ),
      );
    focusable()[0]?.focus();
    const keydown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
        return;
      }
      if (event.key !== "Tab") return;
      const items = focusable();
      const first = items[0];
      const last = items[items.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last?.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first?.focus();
      }
    };
    dialog.addEventListener("keydown", keydown);
    return () => {
      dialog.removeEventListener("keydown", keydown);
      previousFocus.current?.focus();
    };
  }, [onClose]);
  return (
    <div className="modal-backdrop">
      <form
        ref={dialogRef}
        className="modal check-case-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="create-check-case-title"
        onSubmit={(event) => {
          event.preventDefault();
          if (!createdCase) create.mutate();
        }}
      >
        <div className="check-case-modal-heading">
          <div>
            <p className="eyebrow">CREATE INVESTIGATION</p>
            <h3 id="create-check-case-title">建立案件</h3>
          </div>
          <button
            className="drawer-close"
            type="button"
            aria-label="關閉建立案件"
            onClick={onClose}
          >
            ×
          </button>
        </div>
        <p className="muted">
          建立新案件後，系統會把 Snapshot <code>{snapshot.id}</code>
          加入案件，不會複製 event payload。
        </p>
        <label>
          案件標題
          <input
            data-testid="event-check-create-case-title"
            value={title}
            maxLength={300}
            disabled={Boolean(createdCase)}
            onChange={(event) => setTitle(event.target.value)}
          />
        </label>
        <label>
          嚴重度
          <select
            data-testid="event-check-create-case-severity"
            value={severity}
            disabled={Boolean(createdCase)}
            onChange={(event) => setSeverity(event.target.value as Severity)}
          >
            {(["LOW", "MEDIUM", "HIGH", "CRITICAL"] as Severity[]).map(
              (value) => (
                <option key={value}>{value}</option>
              ),
            )}
          </select>
        </label>
        {!createdCase && (
          <button
            type="submit"
            data-testid="event-check-create-case-confirm"
            disabled={!title.trim() || create.isPending}
          >
            {create.isPending ? "建立中…" : "建立案件並加入 Snapshot"}
          </button>
        )}
        {create.isError && (
          <p className="field-error">
            {eventCheckError(create.error, "無法建立案件。")}
          </p>
        )}
        {createdCase && attach.isPending && (
          <p className="muted" role="status">
            案件 {createdCase.case_no} 已建立，正在加入 Snapshot…
          </p>
        )}
        {createdCase && attach.isError && (
          <div className="case-warning">
            <strong>案件已建立，但 Snapshot 尚未加入</strong>
            <p>{eventCheckError(attach.error, "加入 Snapshot 失敗。")}</p>
            <button type="button" onClick={() => attach.mutate(createdCase)}>
              重試加入 Snapshot
            </button>
            <a href={`/investigations/${encodeURIComponent(createdCase.id)}`}>
              開啟已建立案件 →
            </a>
          </div>
        )}
        {createdCase && attach.data && (
          <div className="attachment-success" role="status">
            <strong>案件已建立並加入 Snapshot</strong>
            <span>{createdCase.case_no}</span>
            <a
              data-testid="event-check-created-case-open"
              href={`/investigations/${encodeURIComponent(createdCase.id)}`}
            >
              開啟案件工作區 →
            </a>
          </div>
        )}
      </form>
    </div>
  );
}

export function JoinCaseDialog({
  open,
  snapshot,
  correlationID,
  onClose,
}: {
  open: boolean;
  snapshot: SnapshotReference | null;
  correlationID: string;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const dialogRef = useRef<HTMLElement>(null);
  const previousFocus = useRef<HTMLElement | null>(null);
  const candidates = useQuery({
    queryKey: ["investigations", "check-snapshot-candidates"],
    queryFn: () => api.investigations(undefined, {}, 100),
    enabled: open,
  });
  const attach = useMutation({
    mutationFn: (item: Investigation) =>
      api.attachInvestigationCheckSnapshot(
        item.id,
        snapshot!.id,
        item.lock_version,
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ["investigations", "check-snapshot-candidates"],
      });
      void queryClient.invalidateQueries({ queryKey: ["check-snapshots"] });
    },
    onError: (error) => {
      if (error.message === "OPTIMISTIC_LOCK_CONFLICT") {
        void queryClient.invalidateQueries({
          queryKey: ["investigations", "check-snapshot-candidates"],
        });
      }
    },
  });
  useEffect(() => {
    if (!open || !dialogRef.current) return;
    previousFocus.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    const dialog = dialogRef.current;
    const focusable = () =>
      Array.from(
        dialog.querySelectorAll<HTMLElement>(
          "button:not([disabled]),a[href],input:not([disabled]),select:not([disabled])",
        ),
      );
    focusable()[0]?.focus();
    const keydown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
        return;
      }
      if (event.key !== "Tab") return;
      const items = focusable();
      const first = items[0];
      const last = items[items.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last?.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first?.focus();
      }
    };
    dialog.addEventListener("keydown", keydown);
    return () => {
      dialog.removeEventListener("keydown", keydown);
      previousFocus.current?.focus();
    };
  }, [onClose, open]);
  if (!open || !snapshot) return null;
  const items = (candidates.data?.items ?? [])
    .filter((item) => item.status !== "CLOSED")
    .sort(
      (left, right) =>
        Number(right.correlation_id === correlationID) -
        Number(left.correlation_id === correlationID),
    );
  return (
    <div className="modal-backdrop">
      <section
        ref={dialogRef}
        className="modal check-case-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="check-case-title"
        tabIndex={-1}
      >
        <div className="check-case-modal-heading">
          <div>
            <p className="eyebrow">JOIN INVESTIGATION</p>
            <h3 id="check-case-title">加入案件</h3>
          </div>
          <button
            className="drawer-close"
            type="button"
            aria-label="關閉案件選擇"
            onClick={onClose}
          >
            ×
          </button>
        </div>
        <p className="muted">
          將 Snapshot <code>{snapshot.id}</code> 加入所選案件；相同 Correlation
          的案件排在最前面。
        </p>
        {candidates.isLoading && <p className="muted">載入案件…</p>}
        {candidates.isError && <p className="field-error">無法載入案件。</p>}
        <div className="check-case-list">
          {items.map((item) => (
            <article key={item.id}>
              <div>
                <strong>{item.case_no}</strong>
                <span className={`status status-${item.status.toLowerCase()}`}>
                  {item.status}
                </span>
              </div>
              <h4>{item.title}</h4>
              <p>{item.correlation_id}</p>
              <button
                type="button"
                data-testid={`event-check-join-case-${item.id}`}
                disabled={attach.isPending}
                onClick={() => attach.mutate(item)}
              >
                加入此案件
              </button>
            </article>
          ))}
          {!candidates.isLoading && !items.length && (
            <div className="empty-inline">
              <p>目前沒有可加入的案件。</p>
              <a
                href={`/investigations?correlation_id=${encodeURIComponent(correlationID)}`}
              >
                建立或查看 Investigation Case →
              </a>
            </div>
          )}
        </div>
        {attach.data && (
          <div className="attachment-success" role="status">
            <strong>Snapshot 已連結案件</strong>
            <a
              href={`/investigations/${encodeURIComponent(attach.data.investigation_id)}`}
            >
              開啟案件工作區 →
            </a>
          </div>
        )}
        {attach.isError && (
          <p className="field-error">
            {eventCheckError(attach.error, "無法加入案件。")}
          </p>
        )}
      </section>
    </div>
  );
}

export function EventCheckWorkspace({ principal }: { principal: Principal }) {
  const location = useLocation();
  const navigate = useNavigate();
  const request = useMemo(
    () => requestFromSearch(location.search),
    [location.search],
  );
  const params = new URLSearchParams(location.search);
  const shortcutsOpen = params.get("panel") === "query-shortcuts";
  const snapshotID = params.get("snapshot_id");
  const [caseAction, setCaseAction] = useState<"CREATE" | "JOIN" | null>(null);
  const closeCaseAction = useCallback(
    () => setCaseAction(null),
    [setCaseAction],
  );
  const [saveNotice, setSaveNotice] = useState("");
  const saveKeys = useRef(new Map<string, string>());
  const models = useQuery({
    queryKey: ["check-models"],
    queryFn: api.checkModels,
  });
  const evaluation = useQuery({
    queryKey: ["event-check", request],
    queryFn: () => api.evaluateEventCheck(request!),
    enabled: Boolean(request) && !snapshotID,
  });
  const snapshot = useQuery({
    queryKey: ["check-snapshot", snapshotID],
    queryFn: () => api.checkSnapshot(snapshotID!),
    enabled: Boolean(snapshotID),
  });
  const currentSnapshot = snapshot.data ?? null;
  const currentRequest =
    currentSnapshot?.evaluation_request ??
    evaluation.data?.normalized_request ??
    request;
  const result = currentSnapshot?.result ?? evaluation.data?.result ?? null;
  const findings = result?.findings ?? [];
  const flows = result?.flows ?? [];
  const expectations = result?.expectations ?? [];
  const unmappedEventIDs = result?.unmapped_event_ids ?? [];
  const sourceHealth =
    currentSnapshot?.source_health ?? evaluation.data?.source_health;
  const model = currentSnapshot?.model ?? evaluation.data?.model ?? null;
  const eventSetHash =
    currentSnapshot?.event_set_hash ?? evaluation.data?.event_set_hash;
  const evaluationHash =
    currentSnapshot?.evaluation_hash ?? evaluation.data?.evaluation_hash;
  const includedEvents = currentSnapshot
    ? currentSnapshot.event_references.filter(
        (item) => item.disposition === "INCLUDED",
      )
    : (evaluation.data?.scope.events ?? []);
  const excludedEvents = currentSnapshot
    ? currentSnapshot.event_references.filter(
        (item) => item.disposition === "EXCLUDED",
      )
    : (evaluation.data?.scope.excluded_events ?? []);
  const relationships =
    currentSnapshot?.relationships ??
    evaluation.data?.scope.relationships ??
    [];
  const requestedTab = params.get("tab") as WorkspaceTab | null;
  const defaultTab: WorkspaceTab = result && model ? "summary" : "timeline";
  const activeTab = workspaceTabs.some((tab) => tab.id === requestedTab)
    ? requestedTab!
    : defaultTab;
  const flowModels = (models.data ?? []).filter(
    (entry) => entry.model.kind === "FLOW" && entry.model.status === "ACTIVE",
  );
  const canWrite = principal.role !== "VIEWER";
  const correlationID =
    includedEvents[0]?.correlation_id ?? currentRequest?.identifier.value ?? "";
  const shortcutQuery: SavedSearchQuery | undefined = currentRequest
    ? {
        from: currentRequest.from,
        to: currentRequest.to,
        time_mode: "ABSOLUTE",
        include_processing_attempts: false,
        identifier_type: currentRequest.identifier.type,
        identifier_value: currentRequest.identifier.value,
        aggregate_type: currentRequest.identifier.qualifier?.aggregate_type,
        business_key_name:
          currentRequest.identifier.qualifier?.business_key_name,
        model_id: currentRequest.model?.id,
        model_version: currentRequest.model?.version,
        workspace_tab: activeTab,
      }
    : undefined;

  const openShortcuts = () => {
    const next = new URLSearchParams(location.search);
    next.set("panel", "query-shortcuts");
    navigate(`/event-check?${next.toString()}`);
  };
  const closeShortcuts = useCallback(() => {
    const next = new URLSearchParams(location.search);
    next.delete("panel");
    navigate(`/event-check?${next.toString()}`, { replace: true });
  }, [location.search, navigate]);

  const setTab = (tab: WorkspaceTab) => {
    const next = new URLSearchParams(location.search);
    next.set("tab", tab);
    navigate(`/event-check?${next.toString()}`);
  };
  const selectModel = (modelID: string, version: number) => {
    const next = new URLSearchParams(location.search);
    next.delete("snapshot_id");
    next.set("model_id", modelID);
    next.set("model_version", String(version));
    next.set("tab", "summary");
    navigate(`/event-check?${next.toString()}`);
  };
  const selectIdentifierType = (type: EventCheckIdentifierType) => {
    const next = new URLSearchParams(location.search);
    next.delete("snapshot_id");
    next.set("identifier_type", type);
    navigate(`/event-check?${next.toString()}`);
  };
  const save = useMutation({
    mutationFn: ({ afterSave }: { afterSave: "NONE" | "CREATE" | "JOIN" }) => {
      if (!evaluation.data?.event_set_hash || !evaluation.data.evaluation_hash)
        throw new Error("EVALUATION_NOT_READY");
      let key = saveKeys.current.get(evaluation.data.evaluation_hash);
      if (!key) {
        key = globalThis.crypto?.randomUUID?.() ?? `event-check-${Date.now()}`;
        saveKeys.current.set(evaluation.data.evaluation_hash, key);
      }
      return api
        .createCheckSnapshot(
          {
            evaluation_request: evaluation.data.normalized_request,
            expected_event_set_hash: evaluation.data.event_set_hash,
            expected_evaluation_hash: evaluation.data.evaluation_hash,
          },
          key,
        )
        .then((saved) => ({ saved, afterSave }));
    },
    onSuccess: ({ saved, afterSave }) => {
      const next = new URLSearchParams(location.search);
      next.set("snapshot_id", saved.id);
      next.set("tab", afterSave === "NONE" ? "summary" : "cases");
      setSaveNotice(
        "已保存 immutable Check Snapshot；後續新事件不會改寫此結果。",
      );
      navigate(`/event-check?${next.toString()}`, { replace: true });
      if (afterSave !== "NONE") setCaseAction(afterSave);
    },
  });

  return (
    <main className="page event-check-page" data-testid="event-check-page">
      <div className="page-heading event-check-heading">
        <div>
          <p className="eyebrow">BOUNDED CONFORMANCE CHECK</p>
          <h1>Event Check</h1>
          <p className="muted">
            用任一已知 ID 找到有明確關聯的事件，再以版本化 Check Model
            檢查實際流程。
          </p>
        </div>
        <div className="page-heading-actions">
          <a
            href="/event-check/saved-results"
            className="button-secondary"
            data-testid="event-check-saved-results-open"
          >
            Saved Results
          </a>
          <button
            type="button"
            className="button-secondary"
            data-testid="event-check-shortcuts-open"
            onClick={openShortcuts}
          >
            查詢捷徑
          </button>
          <div className="event-check-boundary">
            <strong>唯讀</strong>
            <span>不控制 production workflow</span>
          </div>
        </div>
      </div>
      <SearchForm
        key={request ? JSON.stringify(request) : "empty"}
        request={request}
      />
      {!request && (
        <section className="event-check-empty card">
          <p className="eyebrow">START HERE</p>
          <h2>輸入你手上的識別碼</h2>
          <p>
            可以是 Correlation、Event、Trace、Aggregate 或受治理的 Business
            Key。系統不會只靠字串外觀猜測。
          </p>
        </section>
      )}
      {(evaluation.isLoading || snapshot.isLoading) && (
        <p className="muted" role="status">
          正在解析事件範圍並檢查…
        </p>
      )}
      {(evaluation.isError || snapshot.isError) && (
        <p className="field-error" role="alert">
          {eventCheckError(
            evaluation.error ?? snapshot.error,
            "無法完成 Event Check。",
          )}
        </p>
      )}
      {evaluation.data?.resolution_status ===
        "IDENTIFIER_SELECTION_REQUIRED" && (
        <section className="selection-required card">
          <p className="eyebrow">IDENTIFIER SELECTION REQUIRED</p>
          <h2>這個值可能代表多種識別碼</h2>
          <p>請確認你實際持有的 ID 類型，系統不會偷偷選擇。</p>
          <div>
            {(evaluation.data.identifier_candidates ?? []).map((candidate) => (
              <button
                type="button"
                className="button-secondary"
                key={candidate.type}
                onClick={() => selectIdentifierType(candidate.type)}
              >
                <strong>{candidate.type}</strong>
                <small>
                  {candidate.confidence} · {candidate.reason_code}
                </small>
              </button>
            ))}
          </div>
        </section>
      )}
      {evaluation.data?.resolution_status === "MODEL_SELECTION_REQUIRED" && (
        <section className="selection-required card">
          <p className="eyebrow">MODEL SELECTION REQUIRED</p>
          <h2>請確認主要 Flow Model</h2>
          <p>多個 Model 都可能適用，因此不自動猜測。</p>
          <div>
            {(evaluation.data.model_candidates ?? []).map((candidate) => (
              <button
                type="button"
                className="button-secondary"
                key={`${candidate.model.id}:${candidate.model.version}`}
                onClick={() =>
                  selectModel(candidate.model.id, candidate.model.version)
                }
              >
                <strong>
                  {candidate.model.id} v{candidate.model.version}
                </strong>
                <small>
                  {candidate.confidence} · {candidate.reason_codes.join(" · ")}
                </small>
              </button>
            ))}
          </div>
        </section>
      )}
      {(evaluation.data || currentSnapshot) &&
        sourceHealth &&
        currentRequest && (
          <section
            className="event-check-workspace"
            data-testid="event-check-results"
          >
            <div className="workspace-toolbar card">
              <div>
                <span
                  className={`check-status status-${(result?.check_status ?? evaluation.data?.resolution_status ?? "NO_DATA").toLowerCase()}`}
                  data-testid="event-check-status"
                >
                  {result?.check_status ?? evaluation.data?.resolution_status}
                </span>
                {currentSnapshot && (
                  <span className="snapshot-badge">
                    SNAPSHOT · {currentSnapshot.id.slice(0, 8)}
                  </span>
                )}
                {currentRequest.scope_adjustments && (
                  <span className="custom-scope-badge">CUSTOM SCOPE</span>
                )}
              </div>
              <label>
                主要 Flow Model
                <select
                  data-testid="event-check-model-select"
                  value={model ? `${model.id}@${model.version}` : ""}
                  onChange={(event) => {
                    const selected = flowModels.find(
                      (entry) => modelKey(entry) === event.target.value,
                    );
                    if (selected)
                      selectModel(
                        selected.model.model_id,
                        selected.model.version,
                      );
                  }}
                  disabled={Boolean(currentSnapshot)}
                >
                  <option value="">尚未選擇</option>
                  {flowModels.map((entry) => (
                    <option value={modelKey(entry)} key={modelKey(entry)}>
                      {entry.model.title} · v{entry.model.version}
                    </option>
                  ))}
                </select>
              </label>
              <div className="workspace-actions">
                {currentSnapshot ? (
                  <button
                    type="button"
                    className="button-secondary"
                    data-testid="event-check-rerun"
                    onClick={() => {
                      const next = new URLSearchParams(location.search);
                      next.delete("snapshot_id");
                      navigate(`/event-check?${next.toString()}`);
                    }}
                  >
                    執行最新檢查
                  </button>
                ) : (
                  <button
                    type="button"
                    className="button-secondary"
                    data-testid="event-check-save"
                    disabled={
                      !canWrite || !result || !evaluationHash || save.isPending
                    }
                    onClick={() => save.mutate({ afterSave: "NONE" })}
                  >
                    保存結果
                  </button>
                )}
                <button
                  type="button"
                  data-testid="event-check-create-case"
                  disabled={!canWrite || !result || save.isPending}
                  onClick={() =>
                    currentSnapshot
                      ? setCaseAction("CREATE")
                      : save.mutate({ afterSave: "CREATE" })
                  }
                >
                  建立案件
                </button>
                <button
                  type="button"
                  className="button-secondary"
                  data-testid="event-check-join-case"
                  disabled={!canWrite || !result || save.isPending}
                  onClick={() =>
                    currentSnapshot
                      ? setCaseAction("JOIN")
                      : save.mutate({ afterSave: "JOIN" })
                  }
                >
                  加入案件
                </button>
              </div>
            </div>
            {saveNotice && (
              <p className="success-notice" role="status">
                {saveNotice}
              </p>
            )}
            {save.isError && (
              <p className="field-error">
                {eventCheckError(save.error, "無法保存 Check Snapshot。")}
              </p>
            )}
            <nav
              className="workspace-tabs"
              aria-label="Event Check 結果分頁"
              role="tablist"
            >
              {workspaceTabs.map((tab) => (
                <button
                  type="button"
                  role="tab"
                  data-testid={`event-check-tab-${tab.id}`}
                  aria-selected={activeTab === tab.id}
                  aria-controls="event-check-workspace-panel"
                  className={activeTab === tab.id ? "active" : ""}
                  aria-current={activeTab === tab.id ? "page" : undefined}
                  onClick={() => setTab(tab.id)}
                  key={tab.id}
                >
                  {tab.label}
                  {tab.id === "findings" && result
                    ? ` (${findings.length})`
                    : ""}
                </button>
              ))}
            </nav>
            <div
              className="workspace-panel"
              id="event-check-workspace-panel"
              role="tabpanel"
              tabIndex={0}
            >
              {activeTab === "summary" && result && (
                <div className="check-summary-grid">
                  <section
                    className="check-summary-hero"
                    data-testid="event-check-summary"
                  >
                    <p className="eyebrow">CHECK STATUS</p>
                    <h2>{result.check_status}</h2>
                    <p>
                      {result.business_outcome
                        ? `${result.business_outcome.label} · ${result.business_outcome.category}`
                        : "目前沒有 terminal business outcome。"}
                    </p>
                  </section>
                  <section className="check-summary-metrics">
                    <div>
                      <small>Events</small>
                      <strong data-testid="event-check-event-count">
                        {includedEvents.length}
                      </strong>
                    </div>
                    <div>
                      <small>Findings</small>
                      <strong data-testid="event-check-finding-count">
                        {findings.length}
                      </strong>
                    </div>
                    <div>
                      <small>Unmapped</small>
                      <strong>{unmappedEventIDs.length}</strong>
                    </div>
                    <div>
                      <small>Flows</small>
                      <strong>{flows.length}</strong>
                    </div>
                  </section>
                  {model && (
                    <section
                      className="check-model-summary"
                      data-testid="event-check-model-summary"
                    >
                      <div>
                        <p className="eyebrow">PINNED MODEL</p>
                        <h3>
                          {model.id} v{model.version}
                        </h3>
                      </div>
                      <dl>
                        <div>
                          <dt>Kind</dt>
                          <dd>{model.kind}</dd>
                        </div>
                        <div>
                          <dt>Checksum</dt>
                          <dd title={model.checksum}>
                            {shortHash(model.checksum)}
                          </dd>
                        </div>
                        <div>
                          <dt>Source</dt>
                          <dd>{model.source_path}</dd>
                        </div>
                      </dl>
                    </section>
                  )}
                  <SourceHealthPanel sourceHealth={sourceHealth} />
                  <section className="check-integrity card">
                    <h3>Deterministic evidence</h3>
                    <dl>
                      <div>
                        <dt>Event set hash</dt>
                        <dd title={eventSetHash ?? ""}>
                          {shortHash(eventSetHash)}
                        </dd>
                      </div>
                      <div>
                        <dt>Evaluation hash</dt>
                        <dd title={evaluationHash ?? ""}>
                          {shortHash(evaluationHash)}
                        </dd>
                      </div>
                      <div>
                        <dt>Evaluator</dt>
                        <dd>{result.evaluator_build_version}</dd>
                      </div>
                    </dl>
                  </section>
                  {(evaluation.data?.warnings?.length ?? 0) > 0 && (
                    <section className="case-warning">
                      <strong>Warnings</strong>
                      <ul>
                        {(evaluation.data!.warnings ?? []).map((warning) => (
                          <li key={warning}>{warning}</li>
                        ))}
                      </ul>
                    </section>
                  )}
                </div>
              )}
              {activeTab === "summary" && !result && (
                <p className="empty-inline">
                  尚未取得可評估的 Model；請查看 Timeline 與候選說明。
                </p>
              )}
              {activeTab === "timeline" && (
                <>
                  <CustomScopeEditor
                    request={currentRequest}
                    disabled={!canWrite || Boolean(currentSnapshot)}
                  />
                  <EventList
                    events={includedEvents}
                    excluded={excludedEvents}
                    relationships={relationships}
                  />
                </>
              )}
              {activeTab === "flow" && result && (
                <div className="check-flow-layout">
                  <section>
                    <p className="eyebrow">FLOW RESULTS</p>
                    {flows.map((flow) => (
                      <article
                        className="flow-result-card"
                        data-testid={`event-check-flow-${flow.model.id}`}
                        key={`${flow.model.id}:${flow.role}`}
                      >
                        <div>
                          <strong>
                            {flow.model.id} v{flow.model.version}
                          </strong>
                          <span>{flow.role}</span>
                          <em
                            data-testid={`event-check-flow-${flow.model.id}-status`}
                          >
                            {flow.status}
                          </em>
                        </div>
                        <p>
                          Matched path: {flow.matched_path_id ?? "尚未決定"}
                        </p>
                        <small>
                          候選：
                          {(flow.candidate_path_ids ?? []).join(" · ") || "無"}
                        </small>
                        {flow.outcome && (
                          <p>
                            {flow.outcome.label} · {flow.outcome.category}
                          </p>
                        )}
                      </article>
                    ))}
                  </section>
                  <section>
                    <p className="eyebrow">EXPECTATIONS</p>
                    {expectations.map((expectation) => (
                      <article
                        className="expectation-row"
                        data-testid={`event-check-expectation-${expectation.id}`}
                        key={expectation.id}
                      >
                        <div>
                          <strong>{expectation.id}</strong>
                          <span
                            className={`expectation-state state-${expectation.state.toLowerCase()}`}
                          >
                            {expectation.state}
                          </span>
                        </div>
                        <p>
                          Trigger:{" "}
                          {(expectation.trigger_event_ids ?? []).join(", ") ||
                            "尚未觸發"}
                        </p>
                        <p>
                          Satisfied by:{" "}
                          {(expectation.satisfying_event_ids ?? []).join(
                            ", ",
                          ) || "尚未出現"}
                        </p>
                        <small>
                          Reminder{" "}
                          {expectation.reminder_at
                            ? dateTimeFormatter.format(
                                new Date(expectation.reminder_at),
                              )
                            : "—"}{" "}
                          · Deadline{" "}
                          {expectation.deadline_at
                            ? dateTimeFormatter.format(
                                new Date(expectation.deadline_at),
                              )
                            : "—"}
                        </small>
                      </article>
                    ))}
                  </section>
                </div>
              )}
              {activeTab === "flow" && !result && (
                <p className="empty-inline">需要先選擇適用的 Flow Model。</p>
              )}
              {activeTab === "findings" && (
                <FindingsPanel
                  findings={findings}
                  snapshot={currentSnapshot}
                  principal={principal}
                />
              )}
              {activeTab === "cases" && (
                <section className="check-cases-panel">
                  <p className="eyebrow">INVESTIGATION HANDOFF</p>
                  <h2>
                    {currentSnapshot
                      ? "Snapshot 已可建立或加入案件"
                      : "先固定這次檢查結果"}
                  </h2>
                  <p>
                    {currentSnapshot
                      ? "加入案件只建立 reference；不複製 event payload，也不改寫案件以外的事件資料。"
                      : "案件應引用 immutable Snapshot，避免 late event 讓調查證據在背景改變。"}
                  </p>
                  <div className="workspace-actions">
                    <button
                      type="button"
                      disabled={!canWrite || !result || save.isPending}
                      onClick={() =>
                        currentSnapshot
                          ? setCaseAction("CREATE")
                          : save.mutate({ afterSave: "CREATE" })
                      }
                    >
                      建立案件
                    </button>
                    <button
                      type="button"
                      className="button-secondary"
                      disabled={!canWrite || !result || save.isPending}
                      onClick={() =>
                        currentSnapshot
                          ? setCaseAction("JOIN")
                          : save.mutate({ afterSave: "JOIN" })
                      }
                    >
                      加入案件
                    </button>
                  </div>
                </section>
              )}
            </div>
          </section>
        )}
      <CreateCaseDialog
        open={caseAction === "CREATE"}
        snapshot={currentSnapshot}
        correlationID={correlationID}
        incidentFrom={currentRequest?.from}
        incidentTo={currentRequest?.to}
        checkStatus={result?.check_status ?? "INCONCLUSIVE"}
        onClose={closeCaseAction}
      />
      <JoinCaseDialog
        open={caseAction === "JOIN"}
        snapshot={currentSnapshot}
        correlationID={correlationID}
        onClose={closeCaseAction}
      />
      <EventCheckShortcutsDialog
        open={shortcutsOpen}
        onClose={closeShortcuts}
        currentQuery={shortcutQuery}
        principal={principal}
      />
    </main>
  );
}

function savedResultURL(summary: CheckSnapshotSummary) {
  const request = summary.evaluation_request;
  const query = new URLSearchParams({
    identifier_type: request.identifier.type,
    identifier: request.identifier.value,
    from: request.from,
    to: request.to,
    snapshot_id: summary.id,
    tab: "summary",
  });
  if (request.identifier.qualifier?.aggregate_type) {
    query.set("aggregate_type", request.identifier.qualifier.aggregate_type);
  }
  if (request.identifier.qualifier?.business_key_name) {
    query.set(
      "business_key_name",
      request.identifier.qualifier.business_key_name,
    );
  }
  if (request.model) {
    query.set("model_id", request.model.id);
    query.set("model_version", String(request.model.version));
  }
  return `/event-check?${query.toString()}`;
}

export function SavedCheckResults({ principal }: { principal: Principal }) {
  const [identifierDraft, setIdentifierDraft] = useState("");
  const [statusDraft, setStatusDraft] = useState("");
  const [filters, setFilters] = useState<{
    identifier?: string;
    check_status?: CheckSnapshotSummary["check_status"];
  }>({});
  const [cursors, setCursors] = useState<Array<string | undefined>>([
    undefined,
  ]);
  const [selected, setSelected] = useState<CheckSnapshotSummary | null>(null);
  const [caseAction, setCaseAction] = useState<"CREATE" | "JOIN" | null>(null);
  const currentCursor = cursors[cursors.length - 1];
  const snapshots = useQuery({
    queryKey: ["check-snapshots", filters, currentCursor],
    queryFn: () => api.checkSnapshots(filters, currentCursor, 20),
  });
  const canWrite = principal.role !== "VIEWER";
  const closeCaseAction = useCallback(() => {
    setCaseAction(null);
    setSelected(null);
  }, [setCaseAction, setSelected]);
  const openCaseAction = (
    item: CheckSnapshotSummary,
    action: "CREATE" | "JOIN",
  ) => {
    setSelected(item);
    setCaseAction(action);
  };
  return (
    <main
      className="page saved-check-results-page"
      data-testid="saved-check-results-page"
    >
      <div className="page-heading">
        <div>
          <p className="eyebrow">IMMUTABLE CHECK SNAPSHOTS</p>
          <h1>Saved Results</h1>
          <p className="muted">
            回看已固定的 Event Check 結果。新事件不會改寫
            Snapshot；需要最新狀態時請重新執行檢查。
          </p>
        </div>
        <a href="/event-check" className="button-secondary">
          執行 Event Check
        </a>
      </div>
      <form
        className="card saved-results-filters"
        onSubmit={(event) => {
          event.preventDefault();
          setFilters({
            ...(identifierDraft.trim()
              ? { identifier: identifierDraft.trim() }
              : {}),
            ...(statusDraft
              ? {
                  check_status:
                    statusDraft as CheckSnapshotSummary["check_status"],
                }
              : {}),
          });
          setCursors([undefined]);
        }}
      >
        <label>
          識別碼
          <input
            data-testid="saved-results-identifier"
            value={identifierDraft}
            maxLength={200}
            placeholder="Correlation / Event / Trace / Aggregate ID"
            onChange={(event) => setIdentifierDraft(event.target.value)}
          />
        </label>
        <label>
          Check status
          <select
            data-testid="saved-results-status"
            value={statusDraft}
            onChange={(event) => setStatusDraft(event.target.value)}
          >
            <option value="">全部</option>
            {[
              "NO_DATA",
              "IN_PROGRESS",
              "CONFORMANT",
              "DEVIATED",
              "INCONCLUSIVE",
              "AMBIGUOUS",
            ].map((status) => (
              <option key={status}>{status}</option>
            ))}
          </select>
        </label>
        <button type="submit" data-testid="saved-results-search">
          查詢
        </button>
      </form>
      {snapshots.isLoading && <p className="muted">載入保存結果…</p>}
      {snapshots.isError && (
        <p className="field-error">
          {eventCheckError(snapshots.error, "無法讀取 Saved Results。")}
        </p>
      )}
      {snapshots.data && (
        <section className="card saved-results-list" aria-label="Saved Results">
          <div
            className="saved-result-row saved-result-head"
            aria-hidden="true"
          >
            <span>保存時間／識別碼</span>
            <span>結果／Model</span>
            <span>證據摘要</span>
            <span>操作</span>
          </div>
          {snapshots.data.items.map((item) => (
            <article
              className="saved-result-row"
              data-testid={`saved-result-${item.id}`}
              key={item.id}
            >
              <div>
                <time dateTime={item.created_at}>
                  {dateTimeFormatter.format(new Date(item.created_at))}
                </time>
                <strong>{item.evaluation_request.identifier.value}</strong>
                <small>{item.evaluation_request.identifier.type}</small>
              </div>
              <div>
                <span
                  className={`check-status status-${item.check_status.toLowerCase()}`}
                >
                  {item.check_status}
                </span>
                <small>
                  {item.model.id} · v{item.model.version}
                </small>
                <small>Source {item.source_health_status}</small>
              </div>
              <dl>
                <div>
                  <dt>Events</dt>
                  <dd>{item.event_count}</dd>
                </div>
                <div>
                  <dt>Findings</dt>
                  <dd>{item.finding_count}</dd>
                </div>
                <div>
                  <dt>Cases</dt>
                  <dd>{item.linked_case_count}</dd>
                </div>
              </dl>
              <div className="saved-result-actions">
                <a href={savedResultURL(item)}>查看結果</a>
                {canWrite && (
                  <>
                    <button
                      type="button"
                      data-testid={`saved-result-${item.id}-create-case`}
                      onClick={() => openCaseAction(item, "CREATE")}
                    >
                      建立案件
                    </button>
                    <button
                      type="button"
                      className="button-secondary"
                      data-testid={`saved-result-${item.id}-join-case`}
                      onClick={() => openCaseAction(item, "JOIN")}
                    >
                      加入案件
                    </button>
                  </>
                )}
              </div>
            </article>
          ))}
          {!snapshots.data.items.length && (
            <div className="empty-inline">
              <p>目前條件沒有保存結果。</p>
              <a href="/event-check">執行一次 Event Check →</a>
            </div>
          )}
          <nav className="pagination-actions" aria-label="Saved Results 分頁">
            <button
              type="button"
              className="button-secondary"
              disabled={cursors.length === 1}
              onClick={() => setCursors((items) => items.slice(0, -1))}
            >
              上一頁
            </button>
            <span>第 {cursors.length} 頁</span>
            <button
              type="button"
              className="button-secondary"
              disabled={!snapshots.data.next_cursor}
              onClick={() =>
                snapshots.data.next_cursor &&
                setCursors((items) => [
                  ...items,
                  snapshots.data!.next_cursor ?? undefined,
                ])
              }
            >
              下一頁
            </button>
          </nav>
        </section>
      )}
      <CreateCaseDialog
        open={caseAction === "CREATE"}
        snapshot={selected}
        correlationID={selected?.evaluation_request.identifier.value ?? ""}
        incidentFrom={selected?.evaluation_request.from}
        incidentTo={selected?.evaluation_request.to}
        checkStatus={selected?.check_status ?? "INCONCLUSIVE"}
        onClose={closeCaseAction}
      />
      <JoinCaseDialog
        open={caseAction === "JOIN"}
        snapshot={selected}
        correlationID={selected?.evaluation_request.identifier.value ?? ""}
        onClose={closeCaseAction}
      />
    </main>
  );
}

export function CheckModelsRegistry() {
  const location = useLocation();
  const navigate = useNavigate();
  const detailDialogRef = useRef<HTMLElement>(null);
  const query = new URLSearchParams(location.search);
  const requestedKind = query.get("kind") as ModelKind | null;
  const kind: ModelKind =
    requestedKind === "GLOBAL_CHECK" ? requestedKind : "FLOW";
  const requestedModel = query.get("model_id") ?? query.get("model");
  const requestedVersion = Number(query.get("version"));
  const requestedFocus = query.get("focus");
  const legacyPatternID = query.get("legacy_pattern_id");
  const requestedPanel = query.get("panel") as ModelPanel | null;
  const panel: ModelPanel = ["overview", "versions", "scenarios"].includes(
    requestedPanel ?? "",
  )
    ? requestedPanel!
    : "overview";
  const registry = useQuery({
    queryKey: ["check-models"],
    queryFn: api.checkModels,
  });
  const entries = (registry.data ?? [])
    .filter((entry) => entry.model.kind === kind)
    .sort(
      (left, right) =>
        left.model.model_id.localeCompare(right.model.model_id) ||
        right.model.version - left.model.version,
    );
  const selected =
    entries.find(
      (entry) =>
        entry.model.model_id === requestedModel &&
        entry.model.version === requestedVersion,
    ) ?? null;
  const selectedKey = selected ? modelKey(selected) : "";
  const versions = selected
    ? (registry.data ?? [])
        .filter((entry) => entry.model.model_id === selected.model.model_id)
        .sort((left, right) => right.model.version - left.model.version)
    : [];
  const setKind = (nextKind: ModelKind) =>
    navigate(`/check-models?kind=${nextKind}`);
  const select = (
    entry: CheckModelRegistryEntry,
    nextPanel: ModelPanel = panel,
  ) =>
    navigate(
      `/check-models?${new URLSearchParams({ kind: entry.model.kind, model_id: entry.model.model_id, version: String(entry.model.version), panel: nextPanel }).toString()}`,
    );
  const setPanel = (nextPanel: ModelPanel) =>
    selected && select(selected, nextPanel);
  const closeDetail = () => navigate(`/check-models?kind=${kind}`);

  useEffect(() => {
    if (!selectedKey || !detailDialogRef.current) return;
    const previousFocus = document.activeElement as HTMLElement | null;
    const dialog = detailDialogRef.current;
    const focusable = () =>
      Array.from(
        dialog.querySelectorAll<HTMLElement>(
          'button:not([disabled]), a[href], [tabindex]:not([tabindex="-1"])',
        ),
      );
    focusable()[0]?.focus();
    const keydown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        navigate(`/check-models?kind=${kind}`);
        return;
      }
      if (event.key !== "Tab") return;
      const elements = focusable();
      if (!elements.length) return;
      const first = elements[0];
      const last = elements[elements.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    dialog.addEventListener("keydown", keydown);
    return () => {
      dialog.removeEventListener("keydown", keydown);
      previousFocus?.focus();
    };
  }, [kind, navigate, selectedKey]);
  return (
    <main className="page check-models-page" data-testid="check-models-page">
      <div className="page-heading check-models-heading">
        <div>
          <p className="eyebrow">IMMUTABLE YAML / GIT REGISTRY</p>
          <h1>Check Models</h1>
          <p className="muted">
            Flow Models 描述合理業務路徑；Global Checks
            檢查跨流程事件品質。本頁唯讀。
          </p>
        </div>
        <div className="code-managed-badge">
          <span>Git</span>
          <div>
            <strong>Code managed</strong>
            <small>NO RUNTIME CRUD</small>
          </div>
        </div>
      </div>
      <div
        className="model-kind-tabs"
        role="tablist"
        aria-label="Check Model 類型"
      >
        <button
          type="button"
          role="tab"
          data-testid="check-model-kind-flow"
          aria-selected={kind === "FLOW"}
          className={kind === "FLOW" ? "active" : ""}
          onClick={() => setKind("FLOW")}
        >
          Flow Models
        </button>
        <button
          type="button"
          role="tab"
          data-testid="check-model-kind-global"
          aria-selected={kind === "GLOBAL_CHECK"}
          className={kind === "GLOBAL_CHECK" ? "active" : ""}
          onClick={() => setKind("GLOBAL_CHECK")}
        >
          Global Checks
        </button>
      </div>
      {registry.isLoading && (
        <p className="muted">載入 Check Model Registry…</p>
      )}
      {registry.isError && (
        <p className="field-error">無法讀取 Check Model Registry。</p>
      )}
      {registry.data && (
        <div className="check-models-layout">
          <section
            className="check-model-list card"
            aria-label={`${kind} models`}
          >
            <div className="check-model-list-head">
              <span>Model</span>
              <span>Domain</span>
              <span>Version</span>
              <span>Status</span>
            </div>
            {entries.map((entry) => (
              <button
                type="button"
                data-testid={`check-model-row-${entry.model.model_id}-${entry.model.version}`}
                className={
                  selected && modelKey(selected) === modelKey(entry)
                    ? "selected"
                    : ""
                }
                aria-pressed={Boolean(
                  selected && modelKey(selected) === modelKey(entry),
                )}
                onClick={() => select(entry)}
                key={modelKey(entry)}
              >
                <span>
                  <strong>{entry.model.title}</strong>
                  <code>{entry.model.model_id}</code>
                </span>
                <span data-label="Domain">{entry.model.domain}</span>
                <span data-label="Version">v{entry.model.version}</span>
                <span
                  data-label="Status"
                  className={`model-status status-${entry.model.status.toLowerCase()}`}
                >
                  {entry.model.status}
                </span>
              </button>
            ))}
            {!entries.length && (
              <p className="empty-inline">目前沒有此類型的 Model。</p>
            )}
          </section>
          {selected && (
            <div
              className="modal-backdrop check-model-detail-backdrop"
              onClick={closeDetail}
              data-testid="check-model-detail-backdrop"
            >
              <article
                ref={detailDialogRef}
                className="modal check-model-detail check-model-detail-modal"
                data-testid="check-model-detail"
                role="dialog"
                aria-modal="true"
                aria-labelledby="check-model-detail-title"
                onClick={(event) => event.stopPropagation()}
              >
                <header>
                  <div>
                    <p className="eyebrow">
                      {selected.model.kind} · {selected.model.domain}
                    </p>
                    <h2 id="check-model-detail-title">
                      {selected.model.title}
                    </h2>
                    <code>
                      {selected.model.model_id}@{selected.model.version}
                    </code>
                  </div>
                  <div className="model-detail-header-actions">
                    <span
                      className={`model-status status-${selected.model.status.toLowerCase()}`}
                    >
                      {selected.model.status}
                    </span>
                    <button
                      type="button"
                      className="drawer-close"
                      data-testid="check-model-detail-close"
                      aria-label="關閉 Check Model 詳細資料"
                      onClick={closeDetail}
                    >
                      ×
                    </button>
                  </div>
                </header>
                <p>{selected.model.description}</p>
                {legacyPatternID && (
                  <p className="compatibility-notice" role="status">
                    Legacy Pattern <code>{legacyPatternID}</code> 已對應至此
                    Check Model；醒目項目是唯一正式判定來源。
                  </p>
                )}
                <nav
                  className="model-detail-tabs"
                  aria-label="Model 詳細分頁"
                  role="tablist"
                >
                  {(["overview", "versions", "scenarios"] as const).map(
                    (id) => (
                      <button
                        type="button"
                        role="tab"
                        data-testid={`check-model-panel-${id}`}
                        aria-selected={panel === id}
                        className={panel === id ? "active" : ""}
                        onClick={() => setPanel(id)}
                        key={id}
                      >
                        {id === "overview"
                          ? "Overview"
                          : id === "versions"
                            ? "Versions"
                            : "Test Scenarios"}
                      </button>
                    ),
                  )}
                </nav>
                {panel === "overview" && (
                  <div className="model-overview">
                    <dl>
                      <div>
                        <dt>Checksum</dt>
                        <dd title={selected.checksum}>
                          {shortHash(selected.checksum)}
                        </dd>
                      </div>
                      <div>
                        <dt>Source</dt>
                        <dd>{selected.source_path}</dd>
                      </div>
                      <div>
                        <dt>Aggregate types</dt>
                        <dd>
                          {selected.model.applies_to.aggregate_types.join(
                            " · ",
                          ) || "All"}
                        </dd>
                      </div>
                      <div>
                        <dt>Trigger events</dt>
                        <dd>
                          {selected.model.applies_to.trigger_event_types.join(
                            " · ",
                          ) || "All"}
                        </dd>
                      </div>
                    </dl>
                    <section>
                      <h3>Scope bounds</h3>
                      <ul>
                        <li>
                          {selected.model.scope.max_duration_seconds / 3600}{" "}
                          小時
                        </li>
                        <li>
                          最多{" "}
                          {selected.model.scope.max_events.toLocaleString()}{" "}
                          events
                        </li>
                        <li>
                          最多 {selected.model.scope.max_correlations}{" "}
                          correlations
                        </li>
                        <li>
                          關聯深度 {selected.model.scope.max_relationship_depth}
                        </li>
                      </ul>
                    </section>
                    {selected.model.kind === "FLOW" && (
                      <>
                        <section>
                          <h3>合理路徑</h3>
                          {(selected.model.paths ?? []).map((path) => (
                            <article className="model-rule-card" key={path.id}>
                              <strong>{path.label}</strong>
                              <span>
                                {path.outcome.label} · {path.outcome.category}
                              </span>
                              <small>{path.nodes.join(" → ")}</small>
                            </article>
                          ))}
                        </section>
                        <section>
                          <h3>Expectations</h3>
                          {(selected.model.expectations ?? []).map(
                            (expectation) => (
                              <article
                                id={`check-model-rule-${expectation.id}`}
                                data-testid={`check-model-rule-${expectation.id}`}
                                className={`model-rule-card${requestedFocus === expectation.id ? " focused" : ""}`}
                                key={expectation.id}
                              >
                                <strong>{expectation.label}</strong>
                                <span>
                                  {expectation.severity} ·{" "}
                                  {expectation.finding_code}
                                </span>
                                <small>
                                  Reminder {expectation.reminder_after_seconds}s
                                  · Deadline {expectation.deadline_seconds}s
                                </small>
                              </article>
                            ),
                          )}
                        </section>
                      </>
                    )}
                    {selected.model.kind === "GLOBAL_CHECK" && (
                      <section>
                        <h3>Global rules</h3>
                        {(selected.model.rules ?? []).map((rule) => (
                          <article
                            id={`check-model-rule-${rule.id}`}
                            data-testid={`check-model-rule-${rule.id}`}
                            className={`model-rule-card${requestedFocus === rule.id ? " focused" : ""}`}
                            key={rule.id}
                          >
                            <strong>{rule.id}</strong>
                            <span>{rule.type}</span>
                            <small>
                              {rule.severity} · {rule.finding_code}
                            </small>
                          </article>
                        ))}
                      </section>
                    )}
                  </div>
                )}
                {panel === "versions" && (
                  <div className="model-version-list">
                    {versions.map((version) => (
                      <button
                        type="button"
                        className={
                          modelKey(version) === modelKey(selected)
                            ? "selected"
                            : ""
                        }
                        onClick={() => select(version, "versions")}
                        key={modelKey(version)}
                      >
                        <strong>Version {version.model.version}</strong>
                        <span>{version.model.status}</span>
                        <code>{shortHash(version.checksum)}</code>
                        <small>{version.source_path}</small>
                      </button>
                    ))}
                  </div>
                )}
                {panel === "scenarios" && (
                  <div className="model-scenarios">
                    <p>
                      這些固定案例由 contract fixture 驗證 Model 的
                      deterministic 結果；不是寫死的 UI demo。
                    </p>
                    <code>
                      {selected.model.fixtures?.scenario_file ??
                        "尚未設定 scenario file"}
                    </code>
                    <ol>
                      {(selected.model.fixtures?.case_ids ?? []).map(
                        (caseID) => (
                          <li key={caseID}>
                            <strong>{caseID}</strong>
                          </li>
                        ),
                      )}
                    </ol>
                    {!(selected.model.fixtures?.case_ids.length ?? 0) && (
                      <p className="empty-inline">
                        此 Model 尚未宣告 test scenarios。
                      </p>
                    )}
                  </div>
                )}
                <footer>
                  <span>Authoring: {selected.model.source.authoring}</span>
                  <span>
                    Runtime mutable:{" "}
                    {String(selected.model.source.mutable_at_runtime)}
                  </span>
                </footer>
              </article>
            </div>
          )}
        </div>
      )}
    </main>
  );
}
