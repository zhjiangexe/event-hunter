import {
  StrictMode,
  useEffect,
  useMemo,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { createRoot } from "react-dom/client";
import {
  QueryClient,
  QueryClientProvider,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import {
  BrowserRouter,
  NavLink,
  Navigate,
  useLocation,
  useNavigate,
} from "react-router-dom";
import {
  api,
  type BusinessJourney,
  type EvidenceManifest,
  type EventSearchFilters,
  type IngestionIssue,
  type IngestionIssueFilters,
  type IngestionIssueKind,
  type Investigation,
  type InvestigationNote,
  type InvestigationOverview,
  type InvestigationPage,
  type InvestigationSummary,
  type InvestigationStatus,
  type InvestigationUpdate,
  type JourneyProfile,
  type PatternDefinition,
  type PatternResult,
  type Principal,
  type Role,
  type SavedSearch,
  type SavedSearchQuery,
  type SavedSearchTarget,
  type Severity,
  type SmartSearchCandidate,
  type Timeline,
  type TimelineEvent,
} from "./api";
import {
  eventObservabilityLinks,
  evidenceSourceLink,
  qualityDashboardLink,
  traceObservabilityLink,
} from "./observability-links";
import { parseInvestigationListQuery } from "./investigation-list-query";
import { investigationWindowWithEndBoundary } from "./investigation-window";
import {
  scenarioApi,
  type ScenarioRun,
  type ScenarioRunFilters,
  type ScenarioRunPage,
} from "./scenario-api";
import {
  featureGuideByID,
  featureGuides,
  featureGuideWorkflow,
  type IntegrationGuideDefinition,
  type JourneyInterpretationDefinition,
} from "./feature-guide";
import {
  CheckModelsRegistry,
  EventCheckWorkspace,
  SavedCheckResults,
} from "./event-check-workspace";
import { legacyPatternModelURL, resolveLegacyRoute } from "./legacy-routes";
import "./styles.css";

const queryClient = new QueryClient();

const apiErrorMessages: Record<string, string> = {
  UNAUTHENTICATED: "登入狀態已失效，請重新登入。",
  FORBIDDEN: "目前角色沒有執行此操作的權限。",
  NOT_FOUND: "找不到指定資料，可能已刪除或網址已過期。",
  OPTIMISTIC_LOCK_CONFLICT: "資料已被其他人更新，請重新載入後再試。",
  INVALID_TIME_WINDOW: "時間範圍不正確，請確認起訖時間與七天上限。",
  INVESTIGATION_LIST_UNAVAILABLE: "案件資料來源暫時無法使用。",
  PATTERN_EFFECTIVENESS_UNAVAILABLE: "Pattern 成效資料暫時無法使用。",
  SCENARIO_RUNS_UNAVAILABLE: "Scenario 執行歷程暫時無法使用。",
};

function userFacingError(error: unknown, fallback: string) {
  const code = error instanceof Error ? error.message : "UNKNOWN_ERROR";
  return `${apiErrorMessages[code] ?? fallback}（錯誤代碼：${code}）`;
}

const timelineDateFormatter = new Intl.DateTimeFormat("zh-TW", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
});

const timelineTimeFormatter = new Intl.DateTimeFormat("zh-TW", {
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hourCycle: "h23",
});

const dialogFocusableSelector = [
  "a[href]",
  "button:not([disabled])",
  "input:not([disabled])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  '[tabindex]:not([tabindex="-1"])',
].join(",");

function useDialogFocus<T extends HTMLElement>(
  open: boolean,
  onDismiss: () => void,
) {
  const containerRef = useRef<T>(null);
  const dismissRef = useRef(onDismiss);

  useEffect(() => {
    dismissRef.current = onDismiss;
  }, [onDismiss]);

  useEffect(() => {
    if (!open || !containerRef.current) return;
    const container = containerRef.current;
    const previouslyFocused =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    const focusableElements = () =>
      Array.from(
        container.querySelectorAll<HTMLElement>(dialogFocusableSelector),
      ).filter((element) => element.getClientRects().length > 0);
    const initialFocus =
      container.querySelector<HTMLElement>("[data-dialog-initial-focus]") ??
      focusableElements()[0] ??
      container;
    initialFocus.focus();

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        dismissRef.current();
        return;
      }
      if (event.key !== "Tab") return;
      const items = focusableElements();
      if (items.length === 0) {
        event.preventDefault();
        container.focus();
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    container.addEventListener("keydown", handleKeyDown);
    return () => {
      container.removeEventListener("keydown", handleKeyDown);
      if (previouslyFocused?.isConnected) previouslyFocused.focus();
    };
  }, [open]);

  return containerRef;
}

function Login() {
  const navigate = useNavigate();
  const login = useMutation({
    mutationFn: (role: Role) => api.createSession(role),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["me"] });
      navigate("/event-check");
    },
  });
  return (
    <main className="login-shell">
      <section className="login-intro" aria-labelledby="login-product-title">
        <div className="login-brand" aria-label="Event Hunter">
          EH<span>.</span>
        </div>
        <div>
          <p className="eyebrow">BUSINESS EVENT INVESTIGATION</p>
          <h1 id="login-product-title">從事件，找到業務真相。</h1>
          <p className="login-lede">
            用一個業務識別碼重建實際事件流、比對預期路徑，並直達相關 Logs 與
            Traces。
          </p>
        </div>
        <ol className="login-story" aria-label="Event Hunter 工作方式">
          <li>
            <span>01</span>
            <div>
              <strong>找回完整脈絡</strong>
              <small>彙整同一筆業務的 0～N 個事件</small>
            </div>
          </li>
          <li>
            <span>02</span>
            <div>
              <strong>檢查實際路徑</strong>
              <small>用版本化 Check Model 找出缺漏、亂序與失敗</small>
            </div>
          </li>
          <li>
            <span>03</span>
            <div>
              <strong>帶著證據協作</strong>
              <small>保存結果、連結觀測資料並建立調查案件</small>
            </div>
          </li>
        </ol>
        <p className="login-boundary">
          Read-only by design · 不控制正式 workflow，也不重送業務事件
        </p>
      </section>
      <section className="login-card" aria-labelledby="login-role-title">
        <div className="login-card-heading">
          <p className="eyebrow">LOCAL DEMO ACCESS</p>
          <h2 id="login-role-title">選擇角色開始探索</h2>
          <p className="muted">建議使用調查員，體驗完整調查流程。</p>
        </div>
        <div className="role-grid">
          {(["INVESTIGATOR", "VIEWER", "ADMIN"] as Role[]).map((role) => (
            <button
              data-testid={`role-${role.toLowerCase()}`}
              className={
                role === "INVESTIGATOR"
                  ? "role-card role-card-recommended"
                  : "role-card"
              }
              disabled={login.isPending}
              aria-busy={login.isPending && login.variables === role}
              onClick={() => login.mutate(role)}
              key={role}
            >
              <span className="role-card-title">
                {role === "VIEWER"
                  ? "觀察者"
                  : role === "INVESTIGATOR"
                    ? "調查員"
                    : "管理員"}
                {role === "INVESTIGATOR" && <em>建議</em>}
              </span>
              <small>
                {role === "VIEWER"
                  ? "唯讀查看檢查結果、Models 與案件"
                  : role === "INVESTIGATOR"
                    ? "查詢、保存結果、建立案件與分析 finding"
                    : "包含調查能力與完整示範管理權限"}
              </small>
              <b aria-hidden="true">→</b>
              {login.isPending && login.variables === role && (
                <small role="status">登入中…</small>
              )}
            </button>
          ))}
        </div>
        {login.isError && (
          <p className="error">登入失敗，請確認 API 已啟動。</p>
        )}
      </section>
    </main>
  );
}

function Shell({
  principal,
  children,
}: {
  principal: Principal;
  children: React.ReactNode;
}) {
  const location = useLocation();
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const navGroups = [
    {
      label: "總覽",
      items: [{ testID: "nav-dashboard", to: "/dashboard", label: "Overview" }],
    },
    {
      label: "探索與檢查",
      items: [
        { testID: "nav-event-check", to: "/event-check", label: "Event Check" },
        {
          testID: "nav-saved-results",
          to: "/event-check/saved-results",
          label: "Saved Results",
        },
      ],
    },
    {
      label: "問題與調查",
      items: [
        {
          testID: "nav-investigations",
          to: "/investigations",
          label: "Investigation Cases",
        },
        {
          testID: "nav-ingestion-issues",
          to: "/ingestion-issues",
          label: "Ingestion Issues",
        },
      ],
    },
    {
      label: "模型與規則",
      items: [
        {
          testID: "nav-check-models",
          to: "/check-models",
          label: "Check Models",
        },
      ],
    },
    {
      label: "工具與說明",
      items: [
        {
          testID: "nav-scenario-lab",
          to: "/scenario-lab",
          label: "Scenario Lab",
        },
        {
          testID: "nav-feature-guide",
          to: "/guide",
          label: "Event Hunter Guide",
        },
      ],
    },
  ];
  const navItems = navGroups.flatMap((group) => group.items);
  const currentLabel =
    navItems.find((item) =>
      item.to === "/investigations"
        ? location.pathname.startsWith("/investigations")
        : location.pathname === item.to,
    )?.label ?? "Event Hunter";
  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        跳到主要內容
      </a>
      <aside>
        <div className="shell-brand-row">
          <div>
            <p className="brand">
              EH<span>.</span>
            </p>
            <p className="eyebrow">INVESTIGATION CONSOLE</p>
          </div>
          <button
            type="button"
            className="mobile-nav-toggle"
            aria-expanded={mobileMenuOpen}
            aria-controls="primary-navigation"
            onClick={() => setMobileMenuOpen((open) => !open)}
          >
            <span>{currentLabel}</span>
            <span aria-hidden="true">{mobileMenuOpen ? "×" : "☰"}</span>
          </button>
        </div>
        <nav
          id="primary-navigation"
          className={mobileMenuOpen ? "mobile-open" : ""}
        >
          {navGroups.map((group) => (
            <section
              className="nav-group"
              aria-label={group.label}
              key={group.label}
            >
              <span className="nav-group-label">{group.label}</span>
              {group.items.map((item) => (
                <NavLink
                  key={item.to}
                  to={item.to}
                  end={item.to !== "/investigations"}
                  data-testid={item.testID}
                  className={({ isActive }) =>
                    isActive ||
                    (item.to === "/investigations" &&
                      location.pathname.startsWith("/investigations"))
                      ? "nav-active"
                      : ""
                  }
                  onClick={() => setMobileMenuOpen(false)}
                >
                  {item.label}
                </NavLink>
              ))}
            </section>
          ))}
        </nav>
        <div className="identity">
          <small>SESSION ROLE</small>
          <strong>{principal.role}</strong>
          <button
            className="sign-out"
            onClick={async () => {
              await api.deleteSession();
              window.location.href = "/login";
            }}
          >
            Sign out
          </button>
        </div>
      </aside>
      <section className="content" id="main-content" tabIndex={-1}>
        <header>
          <span>業務事件鑑識</span>
          <span className="status-dot" role="status" aria-live="polite">
            ● Authenticated session
          </span>
        </header>
        {children}
      </section>
    </div>
  );
}

export function EventCheckPage({ principal }: { principal: Principal }) {
  return (
    <Shell principal={principal}>
      <EventCheckWorkspace principal={principal} />
    </Shell>
  );
}

export function SavedCheckResultsPage({ principal }: { principal: Principal }) {
  return (
    <Shell principal={principal}>
      <SavedCheckResults principal={principal} />
    </Shell>
  );
}

export function CheckModelsPage({ principal }: { principal: Principal }) {
  return (
    <Shell principal={principal}>
      <CheckModelsRegistry />
    </Shell>
  );
}

function FeatureGuideIntegration({
  integration,
}: {
  integration: IntegrationGuideDefinition;
}) {
  return (
    <section
      className="feature-guide-integration"
      data-testid="integration-guide"
      aria-labelledby="integration-guide-title"
    >
      <div className="integration-heading">
        <p className="eyebrow">EXTERNAL SYSTEM ONBOARDING</p>
        <h4 id="integration-guide-title">外部系統如何接入</h4>
        <p>
          先選擇要解鎖的能力，再準備對應事件、觀測資料與領域設定；不是所有系統一開始都需要
          Full。
        </p>
      </div>

      <section
        className="integration-section integration-quick-start"
        data-testid="integration-quick-start"
      >
        <p className="eyebrow">START HERE · 5 MINUTES</p>
        <h4>先回答三個問題</h4>
        <p className="integration-section-intro">
          不用先讀完整 Runbook。先依 Yes／No
          找到自己的接入類型，再往下看對應情境。
        </p>
        <ol className="integration-decision-list">
          {integration.decisions.map((decision, index) => (
            <li key={decision.question}>
              <span>{index + 1}</span>
              <div>
                <h5>{decision.question}</h5>
                <p>
                  <strong>Yes</strong>
                  {decision.yes}
                </p>
                <p>
                  <strong>No</strong>
                  {decision.no}
                </p>
              </div>
            </li>
          ))}
        </ol>

        <div className="integration-walkthrough">
          <p className="eyebrow">ONE EVENT, END TO END</p>
          <h4>一筆付款事件怎麼出現在 Timeline</h4>
          <ol>
            {integration.walkthrough.map((step, index) => (
              <li key={step.label}>
                <span>{String(index + 1).padStart(2, "0")}</span>
                <div>
                  <strong>{step.label}</strong>
                  <code>{step.example}</code>
                  <p>{step.meaning}</p>
                </div>
              </li>
            ))}
          </ol>
        </div>
      </section>

      <section
        className="integration-section"
        data-testid="integration-change-cases"
      >
        <p className="eyebrow">WHAT DO I CHANGE?</p>
        <h4>依你的來源情況，只做必要的修改</h4>
        <div className="integration-change-list">
          {integration.changeCases.map((item) => (
            <article key={item.situation}>
              <h5>{item.situation}</h5>
              <dl>
                <div>
                  <dt>要 Adapter 嗎？</dt>
                  <dd>{item.adapter}</dd>
                </div>
                <div>
                  <dt>要改什麼？</dt>
                  <dd>{item.changes}</dd>
                </div>
                <div>
                  <dt>第一個成功證據</dt>
                  <dd>{item.firstProof}</dd>
                </div>
              </dl>
            </article>
          ))}
        </div>
      </section>

      <details
        className="integration-glossary"
        data-testid="integration-glossary"
      >
        <summary>名詞看不懂？展開白話字典</summary>
        <dl>
          {integration.glossary.map((item) => (
            <div key={item.term}>
              <dt>{item.term}</dt>
              <dd>
                <strong>{item.plainMeaning}</strong>
                <span>{item.reason}</span>
              </dd>
            </div>
          ))}
        </dl>
      </details>

      <section
        className="integration-section integration-no-data"
        data-testid="integration-no-data"
      >
        <p className="eyebrow">TIMELINE SHOWS NO DATA</p>
        <h4>照這個順序查，不要一次猜所有元件</h4>
        <ol>
          {integration.noDataChecks.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ol>
      </section>

      <section className="integration-section integration-depth">
        <p className="eyebrow">CHOOSE YOUR DEPTH</p>
        <h4>選擇目前真正需要的能力</h4>
        <p className="integration-section-intro">
          Minimum 先讓事件可查；後續需要技術診斷或主動偵測時，再逐層增加能力。
        </p>

        <div className="integration-tier-grid">
          {integration.tiers.map((tier, index) => (
            <section className="integration-tier-card" key={tier.label}>
              <span>{String(index + 1).padStart(2, "0")}</span>
              <h5>{tier.label}</h5>
              <p>{tier.summary}</p>
              <strong>需要</strong>
              <ul>
                {tier.requirements.map((requirement) => (
                  <li key={requirement}>{requirement}</li>
                ))}
              </ul>
              <strong>解鎖</strong>
              <ul>
                {tier.outcomes.map((outcome) => (
                  <li key={outcome}>{outcome}</li>
                ))}
              </ul>
            </section>
          ))}
        </div>
      </section>

      <section className="integration-section">
        <p className="eyebrow">REFERENCE FLOWS</p>
        <h4>四條接入路徑</h4>
        <div className="integration-path-list">
          {integration.paths.map((path) => (
            <article className="integration-path" key={path.label}>
              <h5>{path.label}</h5>
              <p className="integration-flow">{path.flow}</p>
              <p>{path.note}</p>
            </article>
          ))}
        </div>
      </section>

      <section
        className="integration-section"
        data-testid="integration-data-plane"
      >
        <p className="eyebrow">DATA PLANE OWNERSHIP</p>
        <h4>資料真正保存與查詢的位置</h4>
        <p className="integration-section-intro">
          Kafka 負責傳遞，ClickHouse 才是事件查詢庫。Event Check 不直接從 Kafka
          讀取歷史事件。
        </p>
        <div className="integration-store-grid">
          {integration.dataStores.map((store) => (
            <article className="integration-store-card" key={store.label}>
              <h5>{store.label}</h5>
              <strong>{store.role}</strong>
              <p>{store.note}</p>
            </article>
          ))}
        </div>
      </section>

      <section
        className="integration-section"
        data-testid="integration-admission-gates"
      >
        <p className="eyebrow">TIMELINE ADMISSION GATES</p>
        <h4>事件何時才會被 Business Timeline 查到？</h4>
        <p className="integration-section-intro">
          只有欄位「看起來相關」還不夠；以下五關都通過，事件才會成為可查詢的正常事件。
        </p>
        <ol className="integration-gate-list">
          {integration.admissionGates.map((gate, index) => (
            <li key={gate.label}>
              <span>{String(index + 1).padStart(2, "0")}</span>
              <div>
                <h5>{gate.label}</h5>
                <p>{gate.requirement}</p>
                <small>{gate.failure}</small>
              </div>
            </li>
          ))}
        </ol>
      </section>

      <section
        className="integration-section integration-runbook"
        data-testid="integration-runbook"
      >
        <p className="eyebrow">INTEGRATION RUNBOOK</p>
        <h4>進階：從接入決策到正式交接</h4>
        <p className="integration-section-intro">
          已完成上方快速判斷後再使用。依序展開執行；每一步都列出動作、repo
          source of truth、驗證方式與完成條件。
        </p>
        <div className="integration-runbook-steps">
          {integration.runbookSteps.map((step, index) => (
            <details
              key={step.id}
              open={index === 0}
              data-testid={`integration-runbook-step-${step.id}`}
            >
              <summary>
                <span>{String(index + 1).padStart(2, "0")}</span>
                <div>
                  <strong>{step.title}</strong>
                  <small>{step.goal}</small>
                </div>
              </summary>
              <div className="integration-runbook-body">
                <section>
                  <h5>執行動作</h5>
                  <ol>
                    {step.actions.map((action) => (
                      <li key={action}>{action}</li>
                    ))}
                  </ol>
                </section>
                <section>
                  <h5>Source of truth</h5>
                  <div className="integration-source-files">
                    {step.sourceFiles.map((file) => (
                      <code key={file}>{file}</code>
                    ))}
                  </div>
                </section>
                <section>
                  <h5>如何驗證</h5>
                  <ul>
                    {step.verification.map((item) => (
                      <li key={item}>{item}</li>
                    ))}
                  </ul>
                </section>
                <p className="integration-done-when">
                  <strong>完成條件</strong>
                  {step.doneWhen}
                </p>
              </div>
            </details>
          ))}
        </div>
      </section>

      <section
        className="integration-section"
        data-testid="integration-failure-modes"
      >
        <p className="eyebrow">FAILURE CLASSIFICATION</p>
        <h4>先判斷問題發生在哪一層</h4>
        <div className="integration-failure-grid">
          {integration.failureModes.map((mode) => (
            <article className="integration-failure-card" key={mode.label}>
              <h5>{mode.label}</h5>
              <dl>
                <div>
                  <dt>看到什麼</dt>
                  <dd>{mode.signal}</dd>
                </div>
                <div>
                  <dt>去哪裡查</dt>
                  <dd>{mode.lookAt}</dd>
                </div>
                <div>
                  <dt>代表什麼</dt>
                  <dd>{mode.interpretation}</dd>
                </div>
              </dl>
            </article>
          ))}
        </div>
      </section>

      <section
        className="integration-section"
        data-testid="integration-commands"
      >
        <p className="eyebrow">VERIFICATION COMMANDS</p>
        <h4>可重跑的接入驗收</h4>
        <div className="integration-command-list">
          {integration.commands.map((command) => (
            <article key={command.command}>
              <div>
                <span
                  className={`integration-risk integration-risk-${command.risk.toLowerCase()}`}
                >
                  {command.risk}
                </span>
                <strong>{command.label}</strong>
              </div>
              <pre>
                <code>{command.command}</code>
              </pre>
              <p>{command.expected}</p>
            </article>
          ))}
        </div>
      </section>

      <section className="integration-section">
        <p className="eyebrow">CANONICAL EVENT CONTRACT</p>
        <h4>Envelope 必備欄位</h4>
        <div className="integration-field-list">
          {integration.requiredFields.map((field) => (
            <code key={field}>{field}</code>
          ))}
        </div>
        <ul className="integration-field-notes">
          {integration.fieldNotes.map((note) => (
            <li key={note}>{note}</li>
          ))}
        </ul>
      </section>

      <section className="integration-section">
        <p className="eyebrow">ONBOARDING CHECKLIST</p>
        <h4>上線前逐項確認</h4>
        <ol>
          {integration.checklist.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ol>
      </section>

      <section className="integration-section integration-boundaries">
        <p className="eyebrow">CURRENT BOUNDARIES</p>
        <h4>不要誤解成這些能力</h4>
        <ul>
          {integration.boundaries.map((boundary) => (
            <li key={boundary}>{boundary}</li>
          ))}
        </ul>
      </section>
    </section>
  );
}

function JourneyInterpretationGuide({
  interpretation,
}: {
  interpretation: JourneyInterpretationDefinition;
}) {
  return (
    <section
      className="journey-interpretation-guide"
      data-testid="journey-interpretation-guide"
    >
      <p className="eyebrow">HOW TO READ JOURNEY STATE</p>
      <h4>為什麼「進行中」卻沒有該里程碑事件？</h4>
      <ul>
        {interpretation.principles.map((principle) => (
          <li key={principle}>{principle}</li>
        ))}
      </ul>
      <div className="journey-interpretation-example">
        <p>
          <strong>例：目前只有</strong>
          {interpretation.exampleEvents.join(" → ")}
        </p>
        <div>
          {interpretation.exampleResults.map((result) => (
            <article key={result.label}>
              <span>{result.label}</span>
              <strong>{result.state}</strong>
              <p>{result.reason}</p>
            </article>
          ))}
        </div>
      </div>
    </section>
  );
}

export function FeatureGuidePage({ principal }: { principal: Principal }) {
  const location = useLocation();
  const navigate = useNavigate();
  const selectedGuide = featureGuideByID(
    new URLSearchParams(location.search).get("feature"),
  );

  return (
    <Shell principal={principal}>
      <main className="page feature-guide-page" data-testid="feature-guide">
        <div className="page-heading feature-guide-heading">
          <div>
            <p className="eyebrow">PRODUCT FIELD GUIDE</p>
            <h1>Event Hunter Guide</h1>
            <p className="muted">
              從第一次調查到外部系統接入，理解每個能力該在什麼時候使用。
            </p>
          </div>
          <span className="feature-guide-count">
            {featureGuides.length} 篇導覽
          </span>
        </div>

        <section
          className="feature-guide-selector"
          aria-labelledby="guide-selector-title"
        >
          <div>
            <p className="eyebrow">CHOOSE A CAPABILITY</p>
            <h3 id="guide-selector-title">選擇要了解的功能頁</h3>
          </div>
          <label htmlFor="feature-guide-select">
            功能頁
            <select
              id="feature-guide-select"
              data-testid="feature-guide-select"
              value={selectedGuide.id}
              onChange={(event) =>
                navigate(`/guide?feature=${event.target.value}`, {
                  replace: true,
                })
              }
            >
              {featureGuides.map((guide) => (
                <option value={guide.id} key={guide.id}>
                  {guide.label}
                </option>
              ))}
            </select>
          </label>
        </section>

        <div className="feature-guide-layout">
          <article className="feature-guide-detail" key={selectedGuide.id}>
            <div className="feature-guide-title-row">
              <div>
                <span className="feature-layer">{selectedGuide.layer}</span>
                <h3 data-testid="feature-guide-title">{selectedGuide.label}</h3>
              </div>
              <code>{selectedGuide.route}</code>
            </div>
            <p className="feature-guide-purpose">{selectedGuide.purpose}</p>
            <blockquote data-testid="feature-guide-question">
              <span>最適合回答</span>
              {selectedGuide.question}
            </blockquote>

            <div className="feature-guide-io">
              <section>
                <p className="eyebrow">INPUT</p>
                <h4>你需要準備</h4>
                <ul>
                  {selectedGuide.inputs.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
              </section>
              <section>
                <p className="eyebrow">OUTPUT</p>
                <h4>你會得到</h4>
                <ul>
                  {selectedGuide.outputs.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
              </section>
            </div>

            <section className="feature-guide-steps">
              <p className="eyebrow">HOW TO USE</p>
              <h4>建議操作方式</h4>
              <ol>
                {selectedGuide.steps.map((step) => (
                  <li key={step}>{step}</li>
                ))}
              </ol>
            </section>

            {selectedGuide.journeyInterpretation ? (
              <JourneyInterpretationGuide
                interpretation={selectedGuide.journeyInterpretation}
              />
            ) : null}

            {selectedGuide.integration ? (
              <FeatureGuideIntegration
                integration={selectedGuide.integration}
              />
            ) : null}

            <div className="feature-guide-status-grid">
              <section className="feature-guide-capabilities">
                <p className="eyebrow">AVAILABLE NOW</p>
                <h4>目前已具備</h4>
                <ul>
                  {selectedGuide.capabilities.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
              </section>
              <section className="feature-guide-gaps">
                <p className="eyebrow">KNOWN GAPS</p>
                <h4>目前仍缺少</h4>
                <ul>
                  {selectedGuide.gaps.map((item) => (
                    <li key={item}>{item}</li>
                  ))}
                </ul>
              </section>
            </div>

            <button
              type="button"
              data-testid="feature-guide-open"
              className="feature-guide-open"
              onClick={() => navigate(selectedGuide.route)}
            >
              {selectedGuide.actionLabel ?? `前往 ${selectedGuide.label} →`}
            </button>
          </article>

          <section
            className="feature-guide-workflow"
            aria-labelledby="guide-workflow-title"
          >
            <p className="eyebrow">RECOMMENDED FLOW</p>
            <h3 id="guide-workflow-title">一條完整調查路徑</h3>
            <ol>
              {featureGuideWorkflow.map((step, index) => (
                <li key={step}>
                  <span>{String(index + 1).padStart(2, "0")}</span>
                  {step}
                </li>
              ))}
            </ol>
            <div className="feature-guide-note">
              <strong>找不到可查資料？</strong>
              <p>
                先到 Scenario Lab 執行劇本，再從執行結果的 Timeline link
                開始調查。
              </p>
              <button type="button" onClick={() => navigate("/scenario-lab")}>
                開啟 Scenario Lab
              </button>
            </div>
          </section>
        </div>
      </main>
    </Shell>
  );
}

const sourceLabels: Record<string, string> = {
  postgresql: "PostgreSQL",
  clickhouse: "ClickHouse",
  tempo: "Tempo",
  loki: "Loki",
  grafana: "Grafana",
};

function DashboardMetric({
  label,
  value,
  href,
  testID,
}: {
  label: string;
  value: number | null;
  href: string;
  testID: string;
}) {
  return (
    <a className="overview-metric" href={href} data-testid={testID}>
      <span>{label}</span>
      <strong>{value === null ? "不可用" : value.toLocaleString()}</strong>
      <small>查看明細 →</small>
    </a>
  );
}

const smartSearchLabels: Record<
  SmartSearchCandidate["identifier_type"],
  string
> = {
  EVENT_ID: "Event ID",
  TRACE_ID: "Trace ID",
  CORRELATION_ID: "Correlation ID",
  AGGREGATE_ID: "Aggregate ID",
  ALERT_FINGERPRINT: "Grafana alert fingerprint",
};

function SmartSearchPanel({
  window,
}: {
  window?: { from: string; to: string };
}) {
  const navigate = useNavigate();
  const [input, setInput] = useState("");
  const identify = useMutation({
    mutationFn: () => api.identifySearchInput(input),
  });
  const openCandidate = (candidate: SmartSearchCandidate) => {
    if (!identify.data) return;
    const fallbackWindow = dynamicQueryWindow();
    const from = window?.from ?? fallbackWindow.from;
    const to = window?.to ?? fallbackWindow.to;
    if (candidate.identifier_type === "ALERT_FINGERPRINT") {
      const params = new URLSearchParams({
        alert_id: identify.data.normalized_input,
        from,
        to,
      });
      navigate(`/timeline?${params.toString()}`);
      return;
    }
    const params = new URLSearchParams({
      identifier_type: candidate.identifier_type,
      identifier: identify.data.normalized_input,
      from,
      to,
    });
    navigate(`/event-check?${params.toString()}`);
  };

  return (
    <section className="card smart-search-panel" data-testid="smart-search">
      <div>
        <p className="eyebrow">SMART SEARCH</p>
        <h3>從任意已知 ID 開始</h3>
        <p className="muted">
          可輸入 opaque ID，或用 trace: / event: / correlation: / aggregate: /
          alert: 明確指定類型。
        </p>
      </div>
      <form
        className="smart-search-form"
        onSubmit={(event) => {
          event.preventDefault();
          identify.mutate();
        }}
      >
        <input
          aria-label="Smart Search ID"
          data-testid="smart-search-input"
          value={input}
          onChange={(event) => {
            setInput(event.target.value);
            identify.reset();
          }}
          placeholder="ORDER-2001 或 trace:0123…"
          required
        />
        <button
          data-testid="smart-search-identify"
          disabled={identify.isPending}
        >
          辨識
        </button>
      </form>
      {identify.isError && (
        <p className="field-error">
          無法辨識：{(identify.error as Error).message}
        </p>
      )}
      {identify.data?.status === "INVALID" && (
        <p className="field-error" data-testid="smart-search-invalid">
          輸入無法辨識：{identify.data.message}
        </p>
      )}
      {identify.data && identify.data.candidates.length > 0 && (
        <div
          className="smart-search-candidates"
          data-testid="smart-search-candidates"
        >
          <span>
            {identify.data.status === "AMBIGUOUS"
              ? "請選擇這個 ID 的類型："
              : "已確認類型："}
          </span>
          {identify.data.candidates.map((candidate) => (
            <button
              className="button-secondary"
              data-testid={`smart-search-candidate-${candidate.query_parameter}`}
              key={candidate.identifier_type}
              onClick={() => openCandidate(candidate)}
              type="button"
            >
              {smartSearchLabels[candidate.identifier_type]} →
            </button>
          ))}
        </div>
      )}
    </section>
  );
}

export function DashboardPage({ principal }: { principal: Principal }) {
  const overview = useQuery({
    queryKey: ["investigation-overview"],
    queryFn: api.overview,
    refetchInterval: 60_000,
  });
  const data: InvestigationOverview | undefined = overview.data;
  const control = data?.control_plane ?? null;
  const events = data?.events ?? null;
  const qualityLink = data
    ? qualityDashboardLink(data.window.from, data.window.to)
    : "#";

  return (
    <Shell principal={principal}>
      <main className="page overview-page" data-testid="overview-dashboard">
        <div className="page-heading overview-heading">
          <div>
            <p className="eyebrow">OPERATIONAL INVESTIGATION VIEW</p>
            <h1>事件調查總覽</h1>
            <p className="muted">
              最近 72 小時的案件、事件流與資料來源可信度。
            </p>
          </div>
          <div className="page-heading-actions">
            {data && (
              <time dateTime={data.generated_at}>
                更新於 {new Date(data.generated_at).toLocaleString("zh-TW")}
              </time>
            )}
            <button
              type="button"
              className="button-secondary"
              onClick={() => void overview.refetch()}
              disabled={overview.isFetching}
            >
              {overview.isFetching ? "更新中…" : "重新整理"}
            </button>
          </div>
        </div>

        {overview.isLoading && <p className="muted">載入總覽…</p>}
        {overview.isError && (
          <section className="overview-warning" data-testid="overview-error">
            {userFacingError(overview.error, "無法載入調查總覽。")}
          </section>
        )}
        {data?.partial && (
          <section
            className="overview-warning"
            data-testid="overview-partial-warning"
          >
            <strong>目前為部分資料</strong>
            <span>
              {data.warnings.length > 0
                ? data.warnings.join(" · ")
                : "部分來源不是 fresh，數值不可視為完整。"}
            </span>
          </section>
        )}

        <SmartSearchPanel window={data?.window} />

        {data && (
          <>
            <section className="overview-metric-grid">
              <DashboardMetric
                label="Open cases"
                value={control?.cases.open ?? null}
                href="/investigations?status=OPEN"
                testID="overview-open-cases"
              />
              <DashboardMetric
                label="Investigating"
                value={control?.cases.investigating ?? null}
                href="/investigations?status=INVESTIGATING"
                testID="overview-investigating-cases"
              />
              <DashboardMetric
                label="Closed cases"
                value={control?.cases.closed ?? null}
                href="/investigations?status=CLOSED"
                testID="overview-closed-cases"
              />
              <DashboardMetric
                label="Events / 72h"
                value={events?.event_count ?? null}
                href={qualityLink}
                testID="overview-event-count"
              />
            </section>

            <section className="overview-detail-grid">
              <article className="card overview-panel">
                <div className="card-heading">
                  <div>
                    <p className="eyebrow">RECENT ACTIVITY</p>
                    <h3>近 72 小時</h3>
                  </div>
                </div>
                {control ? (
                  <>
                    <dl className="overview-list">
                      <div>
                        <dt>新增案件</dt>
                        <dd>{control.activity.cases_created}</dd>
                      </div>
                      <div>
                        <dt>結案</dt>
                        <dd>{control.activity.cases_closed}</dd>
                      </div>
                      <div>
                        <dt>Grafana alerts</dt>
                        <dd>{control.activity.grafana_alerts}</dd>
                      </div>
                      <div>
                        <dt>Scenario passed</dt>
                        <dd>{control.activity.scenario_passed}</dd>
                      </div>
                      <div>
                        <dt>Scenario failed / timeout</dt>
                        <dd>
                          {control.activity.scenario_failed +
                            control.activity.scenario_timed_out}
                        </dd>
                      </div>
                    </dl>
                    <div className="severity-overview">
                      <h4>Active severity</h4>
                      {(["critical", "high", "medium", "low"] as const).map(
                        (severity) => (
                          <a
                            key={severity}
                            href={`/investigations?severity=${severity.toUpperCase()}`}
                          >
                            <span className={`severity severity-${severity}`}>
                              {severity.toUpperCase()}
                            </span>
                            <strong>{control.severity[severity]}</strong>
                          </a>
                        ),
                      )}
                    </div>
                  </>
                ) : (
                  <p className="empty-inline">PostgreSQL 來源不可用。</p>
                )}
              </article>

              <article className="card overview-panel">
                <div className="card-heading">
                  <div>
                    <p className="eyebrow">TOP EVENT SOURCES</p>
                    <h3>Producer / Event type</h3>
                  </div>
                </div>
                {events || control ? (
                  <div className="overview-breakdowns">
                    <div>
                      <h4>Producer</h4>
                      {events?.top_producers.map((item) => (
                        <a
                          key={item.key}
                          href={`/timeline?producer=${encodeURIComponent(item.key)}&from=${encodeURIComponent(data.window.from)}&to=${encodeURIComponent(data.window.to)}`}
                        >
                          <span>{item.key}</span>
                          <strong>{item.count}</strong>
                        </a>
                      ))}
                    </div>
                    <div>
                      <h4>Event type</h4>
                      {events?.top_event_types.map((item) => (
                        <a
                          key={item.key}
                          href={`/timeline?event_type=${encodeURIComponent(item.key)}&from=${encodeURIComponent(data.window.from)}&to=${encodeURIComponent(data.window.to)}`}
                        >
                          <span>{item.key}</span>
                          <strong>{item.count}</strong>
                        </a>
                      ))}
                    </div>
                    <div>
                      <h4>Pattern</h4>
                      {control?.top_patterns.map((item) => (
                        <a
                          key={item.key}
                          href={
                            legacyPatternModelURL(item.key) ??
                            `/patterns?pattern_id=${encodeURIComponent(item.key)}#pattern-${encodeURIComponent(item.key)}`
                          }
                        >
                          <span>{item.key}</span>
                          <strong>{item.count}</strong>
                        </a>
                      ))}
                    </div>
                  </div>
                ) : (
                  <p className="empty-inline">ClickHouse 來源不可用。</p>
                )}
              </article>
            </section>

            <section className="card overview-panel">
              <div className="card-heading">
                <div>
                  <p className="eyebrow">SOURCE HEALTH</p>
                  <h3>資料可信度</h3>
                </div>
                <a href={qualityLink} target="_blank" rel="noreferrer">
                  Quality Dashboard ↗
                </a>
              </div>
              <div className="source-health-grid">
                {data.sources.map((source) => (
                  <article
                    key={source.name}
                    data-testid={`source-${source.name}`}
                  >
                    <span className={`source-state source-${source.state}`}>
                      {source.state}
                    </span>
                    <strong>{sourceLabels[source.name] ?? source.name}</strong>
                    <small>
                      {source.lag_ms !== null
                        ? `lag ${Math.round(source.lag_ms / 1000)}s`
                        : (source.reason ?? "available")}
                    </small>
                    <small>
                      最近成功：
                      {source.last_success_at
                        ? new Date(source.last_success_at).toLocaleString(
                            "zh-TW",
                          )
                        : "沒有可用紀錄"}
                    </small>
                    {source.reason && <small>原因：{source.reason}</small>}
                  </article>
                ))}
              </div>
            </section>
          </>
        )}
      </main>
    </Shell>
  );
}

type IdentifierKey =
  "correlation_id" | "aggregate_id" | "trace_id" | "event_id";

const identifierOptions: Array<{ value: IdentifierKey; label: string }> = [
  { value: "correlation_id", label: "Correlation ID" },
  { value: "aggregate_id", label: "Aggregate / Business ID" },
  { value: "trace_id", label: "Trace ID" },
  { value: "event_id", label: "Event ID" },
];

const defaultQueryWindowMilliseconds = 72 * 60 * 60_000;
const maximumQueryWindowMilliseconds = 7 * 24 * 60 * 60_000;
const browserTimeZone =
  Intl.DateTimeFormat().resolvedOptions().timeZone || "本機時區";
const queryWindowFormatter = new Intl.DateTimeFormat("zh-TW", {
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hourCycle: "h23",
});

type QueryWindow = { from: string; to: string };

function dynamicQueryWindow(now = new Date()): QueryWindow {
  return {
    from: new Date(
      now.getTime() - defaultQueryWindowMilliseconds,
    ).toISOString(),
    to: now.toISOString(),
  };
}

function queryWindowError(from: string, to: string) {
  const fromTime = Date.parse(from);
  const toTime = Date.parse(to);
  if (!Number.isFinite(fromTime) || !Number.isFinite(toTime)) {
    return "請輸入有效的開始與結束時間。";
  }
  if (toTime <= fromTime) return "結束時間必須晚於開始時間。";
  if (toTime - fromTime > maximumQueryWindowMilliseconds) {
    return "查詢時間範圍最多 7 天。";
  }
  return "";
}

function queryWindowLabel(from: string, to: string) {
  return `${queryWindowFormatter.format(new Date(from))} → ${queryWindowFormatter.format(new Date(to))}`;
}

function investigationTimelineURL(item: Investigation) {
  const query = new URLSearchParams({
    identifier_type: "CORRELATION_ID",
    identifier: item.correlation_id,
    from: item.incident_from,
    to: item.incident_to,
    tab: "timeline",
  });
  return `/event-check?${query.toString()}`;
}

function investigationTimelineWindowURL(
  item: Investigation,
  window: QueryWindow,
) {
  const query = new URLSearchParams({
    identifier_type: "CORRELATION_ID",
    identifier: item.correlation_id,
    from: window.from,
    to: window.to,
    tab: "timeline",
  });
  return `/event-check?${query.toString()}`;
}

function queryWindowsAreEqual(left: QueryWindow, right: QueryWindow) {
  return (
    Date.parse(left.from) === Date.parse(right.from) &&
    Date.parse(left.to) === Date.parse(right.to)
  );
}

function toLocalDateTimeInput(isoValue: string) {
  const date = new Date(isoValue);
  if (!Number.isFinite(date.getTime())) return "";
  const localTime = date.getTime() - date.getTimezoneOffset() * 60_000;
  return new Date(localTime).toISOString().slice(0, 19);
}

const journeyStateLabels: Record<string, string> = {
  EMPTY: "沒有事件",
  IN_PROGRESS: "進行中",
  COMPLETED: "已完成",
  FAILED: "失敗／終止",
  COMPENSATED: "已補償",
  NOT_APPLICABLE: "尚未適用",
};

function durationLabel(durationMS: number | null | undefined) {
  if (durationMS === null || durationMS === undefined) return "—";
  if (durationMS < 1000) return `${durationMS} ms`;
  if (durationMS < 60_000) return `${(durationMS / 1000).toFixed(1)} 秒`;
  return `${(durationMS / 60_000).toFixed(1)} 分鐘`;
}

function optionalSavedSearchInteger(value: string | undefined) {
  if (!value?.trim()) return undefined;
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) && parsed >= 0 ? parsed : undefined;
}

function savedSearchQueryFromFilters(
  filters: EventSearchFilters,
): SavedSearchQuery {
  return {
    from: filters.from,
    to: filters.to,
    correlation_id: filters.correlation_id || undefined,
    event_type: filters.event_type || undefined,
    aggregate_id: filters.aggregate_id || undefined,
    trace_id: filters.trace_id || undefined,
    event_id: filters.event_id || undefined,
    producer: filters.producer || undefined,
    event_version: optionalSavedSearchInteger(filters.event_version),
    causation_id: filters.causation_id || undefined,
    kafka_topic: filters.kafka_topic || undefined,
    kafka_partition: optionalSavedSearchInteger(filters.kafka_partition),
    kafka_offset: optionalSavedSearchInteger(filters.kafka_offset),
    pattern_id: filters.pattern_id || undefined,
    alert_id: filters.alert_id || undefined,
    severity: filters.severity || undefined,
    include_processing_attempts: filters.include_processing_attempts === true,
  };
}

function useQueryShortcutsPanel() {
  const location = useLocation();
  const navigate = useNavigate();
  const open =
    new URLSearchParams(location.search).get("panel") === "query-shortcuts";
  const setOpen = (nextOpen: boolean) => {
    const query = new URLSearchParams(location.search);
    if (nextOpen) query.set("panel", "query-shortcuts");
    else query.delete("panel");
    const search = query.toString();
    navigate(`${location.pathname}${search ? `?${search}` : ""}`, {
      replace: true,
    });
  };
  return { open, setOpen };
}

function savedSearchTimeLabel(item: SavedSearch) {
  if (item.query.time_mode !== "RELATIVE") return "固定時間";
  const seconds = item.query.relative_window_seconds ?? 0;
  if (seconds % 86_400 === 0) return `最近 ${seconds / 86_400} 天`;
  if (seconds % 3_600 === 0) return `最近 ${seconds / 3_600} 小時`;
  return `最近 ${Math.round(seconds / 60)} 分鐘`;
}

function QueryShortcutsDrawer({
  open,
  onClose,
  principalSubject,
  currentTarget,
  currentQuery,
}: {
  open: boolean;
  onClose: () => void;
  principalSubject: string;
  currentTarget: SavedSearchTarget;
  currentQuery?: SavedSearchQuery;
}) {
  const queryClient = useQueryClient();
  const [name, setName] = useState("");
  const [savedName, setSavedName] = useState("");
  const [timeMode, setTimeMode] = useState<"ABSOLUTE" | "RELATIVE">("ABSOLUTE");
  const [relativeWindowSeconds, setRelativeWindowSeconds] = useState(259_200);
  const drawerRef = useDialogFocus<HTMLElement>(open, onClose);
  const savedSearches = useQuery({
    queryKey: ["saved-searches"],
    queryFn: api.savedSearches,
    enabled: open,
  });
  const presets = useQuery({
    queryKey: ["search-presets"],
    queryFn: api.searchPresets,
    enabled: open,
    staleTime: 60_000,
  });
  const remove = useMutation({
    mutationFn: (id: string) => api.deleteSavedSearch(id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["saved-searches"] });
    },
  });
  const save = useMutation({
    mutationFn: () => {
      if (!currentQuery) throw new Error("NO_ACTIVE_QUERY");
      const query: SavedSearchQuery = {
        ...currentQuery,
        time_mode: timeMode,
        relative_window_seconds:
          timeMode === "RELATIVE" ? relativeWindowSeconds : undefined,
      };
      return api.createSavedSearch(name.trim(), currentTarget, query);
    },
    onSuccess: async (created) => {
      await queryClient.invalidateQueries({ queryKey: ["saved-searches"] });
      setSavedName(created.name);
      setName("");
    },
  });

  if (!open) return null;

  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <section
        ref={drawerRef}
        className="case-drawer query-shortcuts-drawer"
        data-testid="query-shortcuts-drawer"
        role="dialog"
        aria-modal="true"
        aria-labelledby="query-shortcuts-title"
        tabIndex={-1}
        onClick={(event) => event.stopPropagation()}
      >
        <div className="drawer-header">
          <div>
            <p className="eyebrow">PERSONAL FORENSIC SHORTCUTS</p>
            <h3 id="query-shortcuts-title">查詢捷徑</h3>
            <p className="muted">重開常用條件，或保存目前的 bounded query。</p>
          </div>
          <button
            className="drawer-close"
            data-testid="query-shortcuts-close"
            data-dialog-initial-focus
            aria-label="關閉查詢捷徑"
            onClick={onClose}
          >
            ×
          </button>
        </div>

        <section className="query-shortcuts-section">
          <div className="card-heading">
            <div>
              <p className="eyebrow">BUILT-IN / RELATIVE</p>
              <h4>常用事件情境</h4>
            </div>
            <span className="badge">{presets.data?.items.length ?? 0}</span>
          </div>
          {presets.isLoading && <p className="muted">載入常用情境…</p>}
          {presets.isError && <p className="field-error">無法載入常用情境。</p>}
          <div className="query-preset-list">
            {presets.data?.items.map((preset) => (
              <a
                data-testid={`search-preset-${preset.id}`}
                href={preset.open_url}
                key={preset.id}
              >
                <div>
                  <strong>{preset.name}</strong>
                  <small>{preset.description}</small>
                </div>
                <span>開啟 →</span>
              </a>
            ))}
          </div>
        </section>

        <section
          className="query-shortcuts-section"
          data-testid="saved-search-list"
        >
          <div className="card-heading">
            <div>
              <p className="eyebrow">OWNED BY {principalSubject}</p>
              <h4>我的搜尋</h4>
            </div>
            <span className="badge">
              {savedSearches.data?.items.length ?? 0}
            </span>
          </div>
          {savedSearches.isLoading && <p className="muted">載入個人搜尋…</p>}
          {savedSearches.isError && (
            <p className="field-error">無法載入個人搜尋。</p>
          )}
          {savedSearches.data?.items.length === 0 && (
            <p className="empty-inline">尚未儲存搜尋。</p>
          )}
          <div className="saved-search-list">
            {savedSearches.data?.items.map((item, index) => (
              <article
                className="saved-search-row"
                data-testid={`saved-search-row-${index}`}
                key={item.id}
              >
                <span className="saved-search-target">{item.target}</span>
                <div>
                  <strong>{item.name}</strong>
                  <small>
                    {savedSearchTimeLabel(item)} ·{" "}
                    {new Date(item.updated_at).toLocaleString("zh-TW")}
                  </small>
                </div>
                <a
                  data-testid={`saved-search-open-${index}`}
                  href={item.open_url}
                >
                  開啟 →
                </a>
                <button
                  type="button"
                  className="button-ghost saved-search-delete"
                  data-testid={`saved-search-delete-${index}`}
                  disabled={remove.isPending}
                  aria-label={`刪除 ${item.name}`}
                  onClick={() => {
                    if (
                      window.confirm(
                        `確定刪除查詢捷徑「${item.name}」？此動作無法復原。`,
                      )
                    ) {
                      remove.mutate(item.id);
                    }
                  }}
                >
                  刪除
                </button>
              </article>
            ))}
          </div>
        </section>

        <form
          className="save-search-form query-shortcuts-save"
          data-testid="save-search-form"
          onSubmit={(event) => {
            event.preventDefault();
            if (currentQuery && name.trim()) save.mutate();
          }}
        >
          <div className="card-heading">
            <div>
              <p className="eyebrow">CURRENT QUERY</p>
              <h4>儲存目前條件</h4>
            </div>
            <span className="saved-search-target">{currentTarget}</span>
          </div>
          {!currentQuery && (
            <p className="empty-inline">請先執行一次查詢，再保存目前條件。</p>
          )}
          <label>
            搜尋名稱
            <input
              data-testid="saved-search-name"
              value={name}
              maxLength={80}
              onChange={(event) => setName(event.target.value)}
              placeholder="例如：最近付款失敗"
              disabled={!currentQuery}
            />
          </label>
          <label>
            時間模式
            <select
              data-testid="saved-search-time-mode"
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
                data-testid="saved-search-relative-window"
                value={relativeWindowSeconds}
                disabled={!currentQuery}
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
            data-testid="save-search-submit"
            disabled={!currentQuery || !name.trim() || save.isPending}
          >
            儲存
          </button>
          {save.isError && (
            <span className="field-error" role="alert">
              儲存失敗：{(save.error as Error).message}
            </span>
          )}
        </form>
        {savedName && (
          <span
            className="save-search-success"
            data-testid="saved-search-success"
            role="status"
            aria-live="polite"
          >
            已儲存「{savedName}」
          </span>
        )}
      </section>
    </div>
  );
}

export function BusinessJourneyPage({ principal }: { principal: Principal }) {
  const location = useLocation();
  const navigate = useNavigate();
  const [fallbackWindow, setFallbackWindow] = useState<QueryWindow>(() =>
    dynamicQueryWindow(),
  );
  const params = new URLSearchParams(location.search);
  const requestedCorrelationID = params.get("correlation_id")?.trim() ?? "";
  const requestedFrom = params.get("from") ?? fallbackWindow.from;
  const requestedTo = params.get("to") ?? fallbackWindow.to;
  const routeStateKey = [
    requestedCorrelationID,
    requestedFrom,
    requestedTo,
  ].join("|");
  return (
    <BusinessJourneyPageContent
      key={routeStateKey}
      principal={principal}
      requestedCorrelationID={requestedCorrelationID}
      requestedFrom={requestedFrom}
      requestedTo={requestedTo}
      onReset={() => {
        setFallbackWindow(dynamicQueryWindow());
        navigate("/journey");
      }}
    />
  );
}

function BusinessJourneyPageContent({
  principal,
  requestedCorrelationID,
  requestedFrom,
  requestedTo,
  onReset,
}: {
  principal: Principal;
  requestedCorrelationID: string;
  requestedFrom: string;
  requestedTo: string;
  onReset: () => void;
}) {
  const navigate = useNavigate();
  const shortcuts = useQueryShortcutsPanel();
  const [correlationID, setCorrelationID] = useState(requestedCorrelationID);
  const [from, setFrom] = useState(() => toLocalDateTimeInput(requestedFrom));
  const [to, setTo] = useState(() => toLocalDateTimeInput(requestedTo));
  const windowError = queryWindowError(from, to);
  const requestedWindowError = queryWindowError(requestedFrom, requestedTo);

  const journey = useQuery({
    queryKey: [
      "business-journey",
      requestedCorrelationID,
      requestedFrom,
      requestedTo,
    ],
    queryFn: () =>
      api.businessJourney(requestedCorrelationID, requestedFrom, requestedTo),
    enabled: Boolean(requestedCorrelationID) && !requestedWindowError,
  });
  const data: BusinessJourney | undefined = journey.data;

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!correlationID.trim() || windowError) return;
    const next = new URLSearchParams({
      correlation_id: correlationID.trim(),
      from: new Date(from).toISOString(),
      to: new Date(to).toISOString(),
    });
    navigate(`/journey?${next.toString()}`);
  };

  return (
    <Shell principal={principal}>
      <main className="page journey-page" data-testid="business-journey-page">
        <div className="page-heading journey-page-heading">
          <div>
            <p className="eyebrow">ORDER / JOURNEY VIEW</p>
            <h1>Business Journey</h1>
            <p className="muted">
              用業務里程碑閱讀同一 correlation 的 canonical events；不建立新
              projection。Expected 事件與狀態由版本化 Journey Profile 定義。
            </p>
          </div>
          <div className="page-heading-actions">
            <button
              className="button-secondary"
              data-testid="query-shortcuts-open"
              onClick={() => shortcuts.setOpen(true)}
            >
              ☆ 查詢捷徑
            </button>
            <button
              className="button-secondary"
              data-testid="journey-open-profiles"
              onClick={() => navigate("/journey-profiles")}
            >
              查看 Profile Registry
            </button>
          </div>
        </div>

        <form className="search-panel journey-search" onSubmit={submit}>
          <span className="search-limit">
            時區 {browserTimeZone} · 時間範圍最多 7 天
          </span>
          <label>
            Correlation ID
            <input
              data-testid="journey-correlation-id"
              value={correlationID}
              onChange={(event) => setCorrelationID(event.target.value)}
              placeholder="例如 ORDER-1001"
              required
            />
          </label>
          <label>
            開始時間
            <input
              data-testid="journey-from"
              type="datetime-local"
              step="1"
              value={from}
              onChange={(event) => setFrom(event.target.value)}
            />
          </label>
          <label>
            結束時間
            <input
              data-testid="journey-to"
              type="datetime-local"
              step="1"
              value={to}
              onChange={(event) => setTo(event.target.value)}
            />
          </label>
          <div className="search-actions">
            <button type="button" className="button-ghost" onClick={onReset}>
              清除
            </button>
            <button
              data-testid="journey-search-submit"
              disabled={!correlationID.trim() || Boolean(windowError)}
            >
              查看 Journey
            </button>
          </div>
          {windowError && <p className="field-error">{windowError}</p>}
        </form>

        {!requestedCorrelationID && (
          <section className="empty-state">
            輸入訂單流程的 Correlation ID，即可查看跨服務里程碑。
          </section>
        )}
        {journey.isLoading && <p className="muted">載入 Business Journey…</p>}
        {journey.isError && (
          <section className="overview-warning" data-testid="journey-error">
            {userFacingError(journey.error, "無法載入 Business Journey。")}
          </section>
        )}

        {data && (
          <div data-testid="journey-results">
            <p className="muted" data-testid="journey-query-window">
              查詢窗口：{queryWindowLabel(data.from, data.to)} · 時區{" "}
              {browserTimeZone}
            </p>
            <section className="journey-summary card">
              <div>
                <p className="eyebrow">{data.correlation_id}</p>
                <h3>{journeyStateLabels[data.status] ?? data.status}</h3>
                <p className="muted" data-testid="journey-profile">
                  Profile: {data.profile_title} · {data.profile_id}@v
                  {data.profile_version}
                </p>
                <p
                  className="journey-state-help"
                  data-testid="journey-state-help"
                >
                  狀態由 Profile 依整條事件集合推導；下方事件只列各里程碑自己的
                  Expected event。
                </p>
              </div>
              <dl>
                <div>
                  <dt>事件</dt>
                  <dd data-testid="journey-event-count">{data.event_count}</dd>
                </div>
                <div>
                  <dt>里程碑進度</dt>
                  <dd data-testid="journey-progress">
                    {data.completed_milestone_count} /{" "}
                    {data.total_milestone_count}
                  </dd>
                </div>
                <div>
                  <dt>跨服務耗時</dt>
                  <dd>{durationLabel(data.duration_ms)}</dd>
                </div>
                <div>
                  <dt>未分類事件</dt>
                  <dd>{data.unmapped_event_count}</dd>
                </div>
              </dl>
              <span
                className={`journey-status journey-status-${data.status.toLowerCase()}`}
                data-testid="journey-status"
              >
                {data.status}
              </span>
            </section>

            {data.status !== "EMPTY" && (
              <section
                className="journey-progress-card card"
                data-testid="journey-progress-card"
              >
                <div>
                  <p className="eyebrow">SERVER-DERIVED PROGRESS</p>
                  <h3>
                    {data.current_milestone_id
                      ? `目前：${data.current_milestone_id}`
                      : data.status === "COMPLETED"
                        ? "主要旅程已完成"
                        : "目前沒有進行中的里程碑"}
                  </h3>
                  {data.next_expected_event_types.length > 0 && (
                    <p className="muted">
                      正在等待：{data.next_expected_event_types.join(" / ")}
                    </p>
                  )}
                  {data.next_milestone_id && (
                    <p className="journey-state-help">
                      Profile 下一個定義：{data.next_milestone_id}
                      ；它可能是選用支線，不代表一定會發生。
                    </p>
                  )}
                </div>
                <div className="journey-traces">
                  <strong>跨服務 traces</strong>
                  {data.trace_ids.length > 0 ? (
                    data.trace_ids.map((traceID) => (
                      <a
                        key={traceID}
                        href={traceObservabilityLink(
                          traceID,
                          data.from,
                          data.to,
                        )}
                        target="_blank"
                        rel="noreferrer"
                      >
                        Tempo · {traceID.slice(0, 12)}… ↗
                      </a>
                    ))
                  ) : (
                    <span className="muted">事件未帶 trace ID</span>
                  )}
                </div>
                <div className="journey-next-actions">
                  <a
                    href={`/timeline?${new URLSearchParams({ correlation_id: data.correlation_id, from: data.from, to: data.to, include_processing_attempts: "true" }).toString()}`}
                  >
                    查看完整 Timeline →
                  </a>
                  {data.anomalies.length > 0 && (
                    <a
                      href={`/investigations?correlation_id=${encodeURIComponent(data.correlation_id)}`}
                    >
                      查看／建立調查案件 →
                    </a>
                  )}
                </div>
              </section>
            )}

            {data.status === "EMPTY" && (
              <section className="empty-state">
                此時間範圍沒有事件。請同時確認 Overview 的 ClickHouse Source
                Health。
              </section>
            )}

            {data.anomalies.length > 0 && (
              <section
                className="journey-anomalies"
                data-testid="journey-anomalies"
              >
                {data.anomalies.map((anomaly) => (
                  <article key={anomaly.code}>
                    <span
                      className={`severity severity-${anomaly.severity.toLowerCase()}`}
                    >
                      {anomaly.severity}
                    </span>
                    <div>
                      <strong>{anomaly.code}</strong>
                      <p>{anomaly.message}</p>
                    </div>
                  </article>
                ))}
              </section>
            )}

            <section
              className="journey-milestones"
              data-testid="journey-milestones"
            >
              {data.milestones.map((milestone, index) => (
                <article
                  className={`journey-milestone milestone-${milestone.state.toLowerCase()}`}
                  data-testid={`journey-milestone-${milestone.id.toLowerCase()}`}
                  key={milestone.id}
                >
                  <span className="milestone-index">{index + 1}</span>
                  <div className="milestone-content">
                    <div className="card-heading">
                      <div>
                        <p className="eyebrow">{milestone.id}</p>
                        <h3>{milestone.label}</h3>
                      </div>
                      <span className="milestone-state">
                        {journeyStateLabels[milestone.state] ?? milestone.state}
                      </span>
                    </div>
                    <p className="muted milestone-expected">
                      Expected: {milestone.expected_event_types.join(" / ")}
                    </p>
                    {milestone.duration_from_previous_ms !== null && (
                      <small>
                        距前一里程碑{" "}
                        {durationLabel(milestone.duration_from_previous_ms)}
                      </small>
                    )}
                    {milestone.events.length > 0 ? (
                      <div className="milestone-events">
                        {milestone.events.map((event) => {
                          const timelineParams = new URLSearchParams({
                            event_id: event.event_id,
                            from: data.from,
                            to: data.to,
                          });
                          return (
                            <a
                              href={`/timeline?${timelineParams.toString()}`}
                              key={event.event_id}
                            >
                              <strong>{event.event_type}</strong>
                              <span>{event.producer}</span>
                              <time dateTime={event.occurred_at}>
                                {new Date(event.occurred_at).toLocaleString(
                                  "zh-TW",
                                )}
                              </time>
                            </a>
                          );
                        })}
                      </div>
                    ) : (
                      <p className="empty-inline">
                        {milestone.state === "IN_PROGRESS"
                          ? `前置事件已使此里程碑進入進行中；目前正在等待 ${milestone.expected_event_types.join(" / ")}。`
                          : milestone.state === "NOT_APPLICABLE"
                            ? `尚未收到 ${milestone.expected_event_types.join(" / ")}；此里程碑目前尚未觸發。`
                            : "此里程碑尚無自己的實際事件。"}
                      </p>
                    )}
                  </div>
                </article>
              ))}
            </section>
          </div>
        )}
        <QueryShortcutsDrawer
          open={shortcuts.open}
          onClose={() => shortcuts.setOpen(false)}
          principalSubject={principal.subject}
          currentTarget="JOURNEY"
          currentQuery={
            requestedCorrelationID && !requestedWindowError
              ? {
                  from: requestedFrom,
                  to: requestedTo,
                  correlation_id: requestedCorrelationID,
                  include_processing_attempts: false,
                }
              : undefined
          }
        />
      </main>
    </Shell>
  );
}

function profileGracePeriodLabel(seconds: number) {
  if (seconds === 0) return "立即判斷";
  if (seconds % 60 === 0) return `${seconds / 60} 分鐘`;
  return `${seconds} 秒`;
}

function JourneyProfileDetail({ profile }: { profile: JourneyProfile }) {
  return (
    <article data-testid={`journey-profile-detail-${profile.id}`}>
      <header className="journey-profile-card-heading">
        <div>
          <p className="eyebrow">{profile.id}</p>
          <h3>{profile.title}</h3>
          <p className="muted">{profile.description}</p>
        </div>
        <div className="journey-profile-badges">
          <span className={`profile-status profile-status-${profile.status}`}>
            {profile.status.toUpperCase()}
          </span>
          {profile.default && <span className="profile-default">DEFAULT</span>}
        </div>
      </header>

      <dl className="journey-profile-metadata">
        <div>
          <dt>Profile version</dt>
          <dd>v{profile.version}</dd>
        </div>
        <div>
          <dt>Contract version</dt>
          <dd>v{profile.contract_version}</dd>
        </div>
        <div>
          <dt>Milestones</dt>
          <dd>{profile.milestones.length}</dd>
        </div>
        <div>
          <dt>Anomaly rules</dt>
          <dd>{profile.anomaly_rules.length}</dd>
        </div>
      </dl>

      <section className="journey-profile-section">
        <div className="card-heading">
          <div>
            <p className="eyebrow">EXPECTED FLOW</p>
            <h4>里程碑順序</h4>
          </div>
        </div>
        <ol className="profile-milestone-list">
          {profile.milestones.map((milestone) => (
            <li key={milestone.id}>
              <span>{milestone.id}</span>
              <div>
                <strong>{milestone.label}</strong>
                <small>{milestone.expected_event_types.join(" / ")}</small>
              </div>
            </li>
          ))}
        </ol>
      </section>

      <section className="journey-profile-section">
        <div className="card-heading">
          <div>
            <p className="eyebrow">DETERMINISTIC CHECKS</p>
            <h4>異常規則</h4>
          </div>
        </div>
        {profile.anomaly_rules.length > 0 ? (
          <div className="profile-anomaly-list">
            {profile.anomaly_rules.map((rule) => (
              <div key={rule.code}>
                <span
                  className={`severity severity-${rule.severity.toLowerCase()}`}
                >
                  {rule.severity}
                </span>
                <div>
                  <strong>{rule.code}</strong>
                  <p>{rule.message}</p>
                  <small>
                    Trigger: {rule.trigger_event_types.join(" / ")} · Grace:{" "}
                    {profileGracePeriodLabel(rule.grace_period_seconds)}
                  </small>
                </div>
              </div>
            ))}
          </div>
        ) : (
          <p className="empty-inline">此版本沒有異常規則。</p>
        )}
      </section>

      <footer className="journey-profile-provenance">
        <div>
          <span>Source</span>
          <code>{profile.source_path}</code>
        </div>
        <div>
          <span>YAML SHA-256</span>
          <code title={profile.checksum}>{profile.checksum}</code>
        </div>
        <div>
          <span>Data quality</span>
          <strong>
            Duplicate event ID detection:{" "}
            {profile.data_quality.detect_duplicate_event_ids ? "ON" : "OFF"}
          </strong>
        </div>
      </footer>
    </article>
  );
}

function journeyProfileKey(profile: JourneyProfile) {
  return `${profile.id}@v${profile.version}`;
}

function journeyProfileSourceName(sourcePath: string) {
  return sourcePath.split("/").at(-1) ?? sourcePath;
}

function JourneyProfileDetailDrawer({
  open,
  profile,
  requestedKey,
  loading,
  onClose,
}: {
  open: boolean;
  profile?: JourneyProfile;
  requestedKey: string;
  loading: boolean;
  onClose: () => void;
}) {
  const drawerRef = useDialogFocus<HTMLElement>(open, onClose);
  if (!open) return null;

  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <section
        ref={drawerRef}
        className="case-drawer journey-profile-drawer"
        data-testid="journey-profile-detail"
        role="dialog"
        aria-modal="true"
        aria-labelledby="journey-profile-detail-title"
        tabIndex={-1}
        onClick={(event) => event.stopPropagation()}
      >
        <div className="drawer-header journey-profile-drawer-header">
          <div>
            <p className="eyebrow">
              {profile ? journeyProfileKey(profile) : requestedKey}
            </p>
            <h3 id="journey-profile-detail-title">Journey Profile 詳細</h3>
          </div>
          <button
            className="drawer-close"
            data-testid="journey-profile-detail-close"
            data-dialog-initial-focus
            aria-label="關閉 Journey Profile 詳細"
            onClick={onClose}
          >
            ×
          </button>
        </div>
        {loading && <p className="muted">載入 Journey Profile 詳細…</p>}
        {!loading && !profile && (
          <section
            className="overview-warning"
            data-testid="journey-profile-detail-not-found"
          >
            找不到 Journey Profile「{requestedKey}」。請關閉詳細畫面後重新選擇。
          </section>
        )}
        {profile && <JourneyProfileDetail profile={profile} />}
      </section>
    </div>
  );
}

export function JourneyProfilesPage({ principal }: { principal: Principal }) {
  const location = useLocation();
  const navigate = useNavigate();
  const profiles = useQuery({
    queryKey: ["journey-profiles"],
    queryFn: api.journeyProfiles,
  });
  const items = profiles.data?.items ?? [];
  const activeCount = items.filter(
    (profile) => profile.status === "active",
  ).length;
  const requestedKey =
    new URLSearchParams(location.search).get("profile")?.trim() ?? "";
  const selectedProfile = items.find(
    (profile) => journeyProfileKey(profile) === requestedKey,
  );
  const selectProfile = (profile: JourneyProfile) => {
    const query = new URLSearchParams(location.search);
    query.set("profile", journeyProfileKey(profile));
    navigate(`${location.pathname}?${query.toString()}`);
  };
  const closeProfile = () => {
    const query = new URLSearchParams(location.search);
    query.delete("profile");
    const search = query.toString();
    navigate(`${location.pathname}${search ? `?${search}` : ""}`);
  };

  return (
    <Shell principal={principal}>
      <main
        className="page journey-profiles-page"
        data-testid="journey-profile-registry"
      >
        <div className="page-heading journey-profile-page-heading">
          <div>
            <p className="eyebrow">GIT-MANAGED / READ-ONLY</p>
            <h1>Journey Profile Registry</h1>
            <p className="muted">
              查看目前 API build 實際載入的流程定義。Profile 仍由 YAML、contract
              validation 與 code review 發布；此頁不會直接改動 production 規則。
            </p>
          </div>
          <button
            data-testid="profiles-open-journey"
            onClick={() => navigate("/journey")}
          >
            前往 Business Journey
          </button>
        </div>

        <section
          className="journey-profile-summary"
          aria-label="Profile summary"
        >
          <div>
            <span>Loaded profiles</span>
            <strong data-testid="profile-count">{items.length}</strong>
          </div>
          <div>
            <span>Active</span>
            <strong>{activeCount}</strong>
          </div>
          <div>
            <span>Runtime policy</span>
            <strong>Immutable</strong>
          </div>
        </section>

        <section
          className="profile-boundary-note"
          data-testid="journey-profile-boundary"
        >
          <strong>目前可以做：</strong> 檢視已載入版本、里程碑與異常規則。
          <strong> 尚未提供：</strong> 畫面編輯、指定 Journey
          查詢版本、審核與發布。
        </section>

        {profiles.isLoading && <p className="muted">載入 Journey Profiles…</p>}
        {profiles.isError && (
          <section
            className="overview-warning"
            data-testid="journey-profile-error"
          >
            無法載入 Journey Profiles：{(profiles.error as Error).message}
          </section>
        )}
        {!profiles.isLoading && !profiles.isError && items.length === 0 && (
          <section className="empty-state">
            目前 API build 沒有 Journey Profile。
          </section>
        )}
        {items.length > 0 && (
          <section
            className="journey-profile-registry-list card"
            aria-label="Journey Profile 列表"
          >
            <div className="journey-profile-table-head" aria-hidden="true">
              <span>Profile</span>
              <span>Version</span>
              <span>Status</span>
              <span>Default</span>
              <span>Milestones</span>
              <span>Rules</span>
              <span>Source</span>
            </div>
            <div className="journey-profile-table-body">
              {items.map((profile) => {
                const selected = journeyProfileKey(profile) === requestedKey;
                return (
                  <button
                    type="button"
                    className={`journey-profile-table-row${selected ? " selected" : ""}`}
                    data-testid={`journey-profile-${profile.id}`}
                    aria-label={`查看 ${profile.title} v${profile.version} 詳細`}
                    aria-pressed={selected}
                    key={journeyProfileKey(profile)}
                    onClick={() => selectProfile(profile)}
                  >
                    <span className="journey-profile-table-identity">
                      <strong>{profile.title}</strong>
                      <small>{profile.id}</small>
                    </span>
                    <span data-label="Version">v{profile.version}</span>
                    <span data-label="Status">
                      <span
                        className={`profile-status profile-status-${profile.status}`}
                      >
                        {profile.status.toUpperCase()}
                      </span>
                    </span>
                    <span data-label="Default">
                      {profile.default ? "YES" : "—"}
                    </span>
                    <span data-label="Milestones">
                      {profile.milestones.length}
                    </span>
                    <span data-label="Rules">
                      {profile.anomaly_rules.length}
                    </span>
                    <span
                      className="journey-profile-table-source"
                      data-label="Source"
                      title={profile.source_path}
                    >
                      {journeyProfileSourceName(profile.source_path)}
                    </span>
                  </button>
                );
              })}
            </div>
          </section>
        )}
        <JourneyProfileDetailDrawer
          open={Boolean(requestedKey)}
          profile={selectedProfile}
          requestedKey={requestedKey}
          loading={profiles.isLoading}
          onClose={closeProfile}
        />
      </main>
    </Shell>
  );
}

const timelineStringFilterKeys = [
  "producer",
  "event_type",
  "event_id",
  "trace_id",
  "correlation_id",
  "aggregate_id",
  "alert_id",
  "causation_id",
  "kafka_topic",
  "pattern_id",
  "event_version",
  "kafka_partition",
  "kafka_offset",
] as const;

const timelineConditionKeys = [
  ...timelineStringFilterKeys,
  "severity",
] as const;

function timelineFiltersFromSearch(
  search: string,
  fallbackWindow: QueryWindow,
  canIncludePayload: boolean,
): EventSearchFilters {
  const query = new URLSearchParams(search);
  const filters: EventSearchFilters = {
    from: query.get("from") || fallbackWindow.from,
    to: query.get("to") || fallbackWindow.to,
    include_processing_attempts:
      query.get("include_processing_attempts") !== "false",
    include_payload:
      canIncludePayload && query.get("include_payload") === "true",
  };
  for (const key of timelineStringFilterKeys) {
    const value = query.get(key)?.trim();
    if (value) filters[key] = value;
  }
  const severity = query.get("severity");
  if (["LOW", "MEDIUM", "HIGH", "CRITICAL"].includes(severity ?? "")) {
    filters.severity = severity as Severity;
  }
  return filters;
}

function hasTimelineCondition(filters: EventSearchFilters) {
  return timelineConditionKeys.some((key) => Boolean(filters[key]));
}

function timelineFiltersToSearch(
  filters: EventSearchFilters,
  canIncludePayload: boolean,
) {
  const query = new URLSearchParams({
    from: filters.from,
    to: filters.to,
    include_processing_attempts: String(
      filters.include_processing_attempts === true,
    ),
  });
  for (const key of timelineConditionKeys) {
    const value = filters[key];
    if (value !== undefined && value !== "") query.set(key, String(value));
  }
  if (canIncludePayload && filters.include_payload === true) {
    query.set("include_payload", "true");
  }
  return query.toString();
}

type TimelineAdvancedFilters = {
  event_type: string;
  event_version: string;
  producer: string;
  causation_id: string;
  kafka_topic: string;
  kafka_partition: string;
  kafka_offset: string;
  pattern_id: string;
  alert_id: string;
  severity: string;
};

function timelineAdvancedFilters(
  filters?: EventSearchFilters | null,
): TimelineAdvancedFilters {
  return {
    event_type: filters?.event_type ?? "",
    event_version: filters?.event_version ?? "",
    producer: filters?.producer ?? "",
    causation_id: filters?.causation_id ?? "",
    kafka_topic: filters?.kafka_topic ?? "",
    kafka_partition: filters?.kafka_partition ?? "",
    kafka_offset: filters?.kafka_offset ?? "",
    pattern_id: filters?.pattern_id ?? "",
    alert_id: filters?.alert_id ?? "",
    severity: filters?.severity ?? "",
  };
}

function TimelineSearchForm({
  onSearch,
  onReset,
  initialFilters,
  canIncludePayload,
}: {
  onSearch: (filters: EventSearchFilters) => void;
  onReset: () => void;
  initialFilters?: EventSearchFilters | null;
  canIncludePayload: boolean;
}) {
  const initialIdentifier = identifierOptions.find((option) =>
    Boolean(initialFilters?.[option.value]),
  );
  const initialAdvanced = timelineAdvancedFilters(initialFilters);
  const [identifierKey, setIdentifierKey] = useState<IdentifierKey>(
    initialIdentifier?.value ?? "correlation_id",
  );
  const [identifierValue, setIdentifierValue] = useState(
    initialIdentifier ? (initialFilters?.[initialIdentifier.value] ?? "") : "",
  );
  const [from, setFrom] = useState(() =>
    toLocalDateTimeInput(initialFilters?.from ?? dynamicQueryWindow().from),
  );
  const [to, setTo] = useState(() =>
    toLocalDateTimeInput(initialFilters?.to ?? dynamicQueryWindow().to),
  );
  const [showAdvanced, setShowAdvanced] = useState(
    () =>
      Object.values(initialAdvanced).some(Boolean) ||
      (canIncludePayload && initialFilters?.include_payload === true) ||
      initialFilters?.include_processing_attempts === false,
  );
  const [includePayload, setIncludePayload] = useState(
    canIncludePayload && initialFilters?.include_payload === true,
  );
  const [includeProcessingAttempts, setIncludeProcessingAttempts] = useState(
    initialFilters?.include_processing_attempts !== false,
  );
  const [advanced, setAdvanced] = useState(initialAdvanced);

  const windowError = queryWindowError(from, to);
  const hasCondition =
    Boolean(identifierValue.trim()) ||
    Object.values(advanced).some((value) => Boolean(value.trim()));
  const canSubmit = hasCondition && !windowError;

  const updateAdvanced = (key: keyof typeof advanced, value: string) =>
    setAdvanced((current) => ({ ...current, [key]: value }));

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!canSubmit) return;
    const filters: EventSearchFilters = {
      from: new Date(from).toISOString(),
      to: new Date(to).toISOString(),
      include_payload: includePayload,
      include_processing_attempts: includeProcessingAttempts,
      ...advanced,
      severity: (advanced.severity || undefined) as Severity | undefined,
    };
    if (identifierValue.trim()) filters[identifierKey] = identifierValue.trim();
    onSearch(filters);
  };

  const reset = () => {
    const nextWindow = dynamicQueryWindow();
    setIdentifierKey("correlation_id");
    setIdentifierValue("");
    setFrom(toLocalDateTimeInput(nextWindow.from));
    setTo(toLocalDateTimeInput(nextWindow.to));
    setAdvanced(timelineAdvancedFilters());
    setShowAdvanced(false);
    setIncludePayload(false);
    setIncludeProcessingAttempts(true);
    onReset();
  };

  return (
    <form className="search-panel" onSubmit={submit}>
      <div className="search-panel-heading">
        <div>
          <p className="eyebrow">BOUNDED EVENT SEARCH</p>
          <h3>查詢條件</h3>
        </div>
        <span className="search-limit">
          時區 {browserTimeZone} · 時間範圍最多 7 天
        </span>
      </div>
      <div className="basic-search-grid">
        <label htmlFor="timeline-identifier-key">
          識別碼類型
          <select
            id="timeline-identifier-key"
            data-testid="timeline-identifier-key"
            value={identifierKey}
            onChange={(event) =>
              setIdentifierKey(event.target.value as IdentifierKey)
            }
          >
            {identifierOptions.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        <label htmlFor="timeline-identifier-value">
          {identifierOptions.find((option) => option.value === identifierKey)
            ?.label ?? "識別碼"}
          <input
            id="timeline-identifier-value"
            data-testid="timeline-correlation-id"
            value={identifierValue}
            onChange={(event) => setIdentifierValue(event.target.value)}
            placeholder="例如 ORDER-2001"
          />
        </label>
        <label htmlFor="timeline-from">
          開始時間
          <input
            id="timeline-from"
            data-testid="timeline-from"
            type="datetime-local"
            step="1"
            value={from}
            onChange={(event) => setFrom(event.target.value)}
          />
        </label>
        <label htmlFor="timeline-to">
          結束時間
          <input
            id="timeline-to"
            data-testid="timeline-to"
            type="datetime-local"
            step="1"
            value={to}
            onChange={(event) => setTo(event.target.value)}
          />
        </label>
      </div>
      <div className="search-actions">
        <button
          type="button"
          className="button-secondary"
          aria-expanded={showAdvanced}
          aria-controls="timeline-advanced-search"
          onClick={() => setShowAdvanced((current) => !current)}
        >
          {showAdvanced ? "收合進階條件" : "進階條件"}
        </button>
        <button type="button" className="button-ghost" onClick={reset}>
          清除
        </button>
        <button data-testid="timeline-search-submit" disabled={!canSubmit}>
          Search timeline
        </button>
      </div>
      {showAdvanced && (
        <div id="timeline-advanced-search" className="advanced-search-grid">
          <label>
            Event type
            <input
              data-testid="timeline-event-type-filter"
              value={advanced.event_type}
              onChange={(event) =>
                updateAdvanced("event_type", event.target.value)
              }
              placeholder="PaymentCompleted"
            />
          </label>
          <label>
            Event version
            <input
              type="number"
              min="1"
              value={advanced.event_version}
              onChange={(event) =>
                updateAdvanced("event_version", event.target.value)
              }
              placeholder="1"
            />
          </label>
          <label>
            Producer / Service
            <input
              value={advanced.producer}
              onChange={(event) =>
                updateAdvanced("producer", event.target.value)
              }
              placeholder="payment-service"
            />
          </label>
          <label>
            Causation ID
            <input
              value={advanced.causation_id}
              onChange={(event) =>
                updateAdvanced("causation_id", event.target.value)
              }
            />
          </label>
          <label>
            Kafka topic
            <input
              value={advanced.kafka_topic}
              onChange={(event) =>
                updateAdvanced("kafka_topic", event.target.value)
              }
            />
          </label>
          <label>
            Partition
            <input
              type="number"
              min="0"
              value={advanced.kafka_partition}
              onChange={(event) =>
                updateAdvanced("kafka_partition", event.target.value)
              }
            />
          </label>
          <label>
            Offset
            <input
              type="number"
              min="0"
              value={advanced.kafka_offset}
              onChange={(event) =>
                updateAdvanced("kafka_offset", event.target.value)
              }
            />
          </label>
          <label>
            Pattern ID
            <input
              data-testid="timeline-pattern-id-filter"
              value={advanced.pattern_id}
              onChange={(event) =>
                updateAdvanced("pattern_id", event.target.value)
              }
              placeholder="payment-completed-without-shipment"
            />
          </label>
          <label>
            Grafana Alert fingerprint
            <input
              data-testid="timeline-alert-id-filter"
              value={advanced.alert_id}
              onChange={(event) =>
                updateAdvanced("alert_id", event.target.value)
              }
              placeholder="Grafana alert fingerprint"
            />
          </label>
          <label>
            最低案件 Severity
            <select
              data-testid="timeline-severity-filter"
              value={advanced.severity}
              onChange={(event) =>
                updateAdvanced("severity", event.target.value)
              }
            >
              <option value="">不限</option>
              <option value="LOW">LOW</option>
              <option value="MEDIUM">MEDIUM</option>
              <option value="HIGH">HIGH</option>
              <option value="CRITICAL">CRITICAL</option>
            </select>
          </label>
          {canIncludePayload && (
            <label className="checkbox-field">
              <input
                type="checkbox"
                data-testid="timeline-include-payload"
                checked={includePayload}
                onChange={(event) => setIncludePayload(event.target.checked)}
              />
              包含遮罩後 payload（ADMIN）
            </label>
          )}
          <label className="checkbox-field">
            <input
              type="checkbox"
              data-testid="timeline-include-processing-attempts"
              checked={includeProcessingAttempts}
              onChange={(event) =>
                setIncludeProcessingAttempts(event.target.checked)
              }
            />
            包含 Kafka 處理重試摘要
          </label>
        </div>
      )}
      {windowError && <p className="field-error">{windowError}</p>}
    </form>
  );
}

function TimelineEventCard({
  event,
  index,
  onAttach,
}: {
  event: TimelineEvent;
  index: number;
  onAttach?: (event: TimelineEvent) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const links = eventObservabilityLinks(event);
  const occurredAt = new Date(event.occurred_at);
  const occurredDate = timelineDateFormatter.format(occurredAt);
  const occurredTime = timelineTimeFormatter.format(occurredAt);
  return (
    <article className="event-card">
      <button
        type="button"
        className="event-row"
        aria-expanded={expanded}
        aria-controls={`timeline-event-detail-${event.event_id}`}
        onClick={() => setExpanded((current) => !current)}
      >
        <span className="event-index">
          {String(index + 1).padStart(2, "0")}
        </span>
        <span className="event-summary">
          <strong data-testid={`timeline-event-${index}-type`}>
            {event.event_type}
          </strong>
          <span className="event-summary-meta">
            <small>
              {event.producer} · sequence {event.sequence} · v
              {event.event_version}
            </small>
            {event.admission_status === "SEARCHABLE_WITH_WARNINGS" && (
              <span
                className="event-quality-badge is-warning"
                data-testid={`timeline-event-${index}-quality-warning`}
                title={event.quality_flags.join(", ")}
              >
                可查詢・需注意
              </span>
            )}
          </span>
        </span>
        <time
          className="event-occurred-at"
          dateTime={event.occurred_at}
          aria-label={`${occurredDate} ${occurredTime}`}
          data-testid={`timeline-event-${index}-occurred-at`}
        >
          <span className="event-date">{occurredDate}</span>
          <span className="event-clock">{occurredTime}</span>
        </time>
        <span className="event-expand">{expanded ? "收合" : "詳細"}</span>
      </button>
      {expanded && (
        <div
          id={`timeline-event-detail-${event.event_id}`}
          data-testid={`timeline-event-${index}-detail`}
          className="event-detail"
        >
          <dl className="event-metadata">
            <div>
              <dt>Event ID</dt>
              <dd>{event.event_id}</dd>
            </div>
            <div>
              <dt>Aggregate</dt>
              <dd>
                {event.aggregate_type} / {event.aggregate_id}
              </dd>
            </div>
            <div>
              <dt>Causation ID</dt>
              <dd>{event.causation_id || "—"}</dd>
            </div>
            <div>
              <dt>Trace ID</dt>
              <dd>{event.trace_id || "—"}</dd>
            </div>
            <div>
              <dt>Kafka</dt>
              <dd>
                {event.kafka_topic} / p{event.kafka_partition} / o
                {event.kafka_offset}
              </dd>
            </div>
            <div>
              <dt>Service version</dt>
              <dd>{event.service_version || "—"}</dd>
            </div>
            <div>
              <dt>Ingestion quality</dt>
              <dd>
                <span
                  className={`event-quality-badge${
                    event.admission_status === "SEARCHABLE_WITH_WARNINGS"
                      ? " is-warning"
                      : ""
                  }`}
                >
                  {event.admission_status === "SEARCHABLE_WITH_WARNINGS"
                    ? "Searchable with warnings"
                    : "Searchable"}
                </span>
              </dd>
            </div>
            <div>
              <dt>Quality flags</dt>
              <dd>{event.quality_flags.join(", ") || "None"}</dd>
            </div>
            <div>
              <dt>Admission profile</dt>
              <dd>{event.admission_profile}</dd>
            </div>
            <div>
              <dt>Ingested at</dt>
              <dd>{event.ingested_at || "—"}</dd>
            </div>
          </dl>
          {event.processing_summary && (
            <section className="processing-summary">
              <strong>Processing summary</strong>
              <p>
                attempts {event.processing_summary.attempt_count} · status{" "}
                {event.processing_summary.final_status || "N/A"}
              </p>
              <small>
                consumers:{" "}
                {event.processing_summary.consumer_groups?.join(", ") || "none"}
              </small>
            </section>
          )}
          {event.payload && (
            <section className="event-payload">
              <strong>Masked payload</strong>
              <pre>{JSON.stringify(event.payload, null, 2)}</pre>
            </section>
          )}
          {onAttach && (
            <div className="event-case-action">
              <div>
                <strong>案件證據</strong>
                <small>只保存 Event reference，不複製 payload。</small>
              </div>
              <button
                type="button"
                className="button-secondary"
                data-testid={`timeline-event-${index}-attach`}
                onClick={() => onAttach(event)}
              >
                ＋ 加入案件
              </button>
            </div>
          )}
          <nav className="observability-links" aria-label="事件觀測連結">
            {links.map((link) => (
              <a
                key={link.kind}
                data-testid={`timeline-event-${index}-link-${link.kind}`}
                href={link.href}
                target="_blank"
                rel="noopener noreferrer"
              >
                {link.label} ↗
              </a>
            ))}
          </nav>
        </div>
      )}
    </article>
  );
}

function EvidenceManifestPanel({ manifest }: { manifest: EvidenceManifest }) {
  return (
    <section data-testid="evidence-manifest" className="evidence-manifest">
      <div className="reference-heading">
        <div>
          <p className="eyebrow">
            EVIDENCE BUNDLE · SCHEMA V{manifest.schema_version}
          </p>
          <h3>{manifest.items.length} references</h3>
        </div>
        <span
          className={manifest.partial ? "status status-partial" : "badge"}
          data-testid="evidence-manifest-state"
        >
          {manifest.partial ? "PARTIAL" : "COMPLETE"}
        </span>
      </div>
      <dl className="evidence-integrity">
        <div>
          <dt>Algorithm</dt>
          <dd data-testid="evidence-checksum-algorithm">
            {manifest.checksum_algorithm}
          </dd>
        </div>
        <div>
          <dt>Manifest checksum</dt>
          <dd>{manifest.manifest_sha256}</dd>
        </div>
      </dl>
      <div className="source-status-grid" data-testid="evidence-source-status">
        {Object.entries(manifest.source_status).map(([source, status]) => (
          <span key={source}>
            {source}: <strong>{status}</strong>
          </span>
        ))}
      </div>
      {(manifest.partial || manifest.warnings.length > 0) && (
        <div className="case-warning" data-testid="evidence-warnings">
          <strong>Partial evidence</strong>
          <p>{manifest.warnings.join("；") || "部分證據資料不完整。"}</p>
        </div>
      )}
      <div className="evidence-reference-list">
        {manifest.items.map((item, index) => {
          const sourceLink = evidenceSourceLink(item);
          return (
            <article className="case-reference" key={item.id}>
              <div className="reference-heading">
                <div>
                  <strong>{item.evidence_type}</strong>
                  <small className="evidence-source">{item.source}</small>
                </div>
                <time>
                  {new Date(item.collected_at).toLocaleString("zh-TW")}
                </time>
              </div>
              <p className="reference-value">{item.reference}</p>
              <div className="evidence-reference-footer">
                <small>checksum: {item.checksum || "not supplied"}</small>
                {sourceLink && (
                  <a
                    data-testid={`evidence-source-link-${index}`}
                    href={sourceLink.href}
                    target={sourceLink.external ? "_blank" : undefined}
                    rel={
                      sourceLink.external ? "noopener noreferrer" : undefined
                    }
                  >
                    {sourceLink.label} {sourceLink.external ? "↗" : "→"}
                  </a>
                )}
              </div>
            </article>
          );
        })}
        {manifest.items.length === 0 && (
          <p className="empty-inline">尚無 Evidence reference。</p>
        )}
      </div>
    </section>
  );
}

const ingestionIssueKindLabels: Record<IngestionIssueKind, string> = {
  CONTRACT_VALIDATION: "Contract validation",
  ADMISSION_QUARANTINE: "Admission quarantine",
  TECHNICAL_DLQ: "Technical DLQ",
};

function nullableValue(value: string | number | null | undefined) {
  return value === null || value === undefined || value === "" ? "—" : value;
}

export function IngestionIssuesPage({ principal }: { principal: Principal }) {
  const navigate = useNavigate();
  const initialWindow = useMemo(() => dynamicQueryWindow(), []);
  const [from, setFrom] = useState(() =>
    toLocalDateTimeInput(initialWindow.from),
  );
  const [to, setTo] = useState(() => toLocalDateTimeInput(initialWindow.to));
  const [kind, setKind] = useState<IngestionIssueKind | "">("");
  const [errorCode, setErrorCode] = useState("");
  const [sourceTopic, setSourceTopic] = useState("");
  const [correlationID, setCorrelationID] = useState("");
  const [filters, setFilters] = useState<IngestionIssueFilters>({
    from: initialWindow.from,
    to: initialWindow.to,
  });
  const filterKey = JSON.stringify(filters);
  const [pagination, setPagination] = useState({
    filterKey,
    page: 1,
    cursors: [] as string[],
  });
  const [selected, setSelected] = useState<IngestionIssue | null>(null);
  const page = pagination.filterKey === filterKey ? pagination.page : 1;
  const cursors =
    pagination.filterKey === filterKey ? pagination.cursors : ([] as string[]);
  const cursor = page > 1 ? cursors[page - 2] : undefined;
  const pageSize = 20;
  const drawerRef = useDialogFocus<HTMLElement>(Boolean(selected), () =>
    setSelected(null),
  );
  const issues = useQuery({
    queryKey: [
      "ingestion-issues",
      filters.from ?? "",
      filters.to ?? "",
      filters.kind ?? "",
      filters.error_code ?? "",
      filters.source_topic ?? "",
      filters.correlation_id ?? "",
      cursor ?? "",
      pageSize,
    ],
    queryFn: () => api.ingestionIssues(filters, cursor, pageSize),
  });
  const windowError = queryWindowError(from, to);

  const applyFilters = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (windowError) return;
    const nextFilters: IngestionIssueFilters = {
      from: new Date(from).toISOString(),
      to: new Date(to).toISOString(),
      kind: kind || undefined,
      error_code: errorCode.trim() || undefined,
      source_topic: sourceTopic.trim() || undefined,
      correlation_id: correlationID.trim() || undefined,
    };
    const nextKey = JSON.stringify(nextFilters);
    setFilters(nextFilters);
    setPagination({ filterKey: nextKey, page: 1, cursors: [] });
    setSelected(null);
  };
  const resetFilters = () => {
    const nextWindow = dynamicQueryWindow();
    const nextFilters: IngestionIssueFilters = {
      from: nextWindow.from,
      to: nextWindow.to,
    };
    setFrom(toLocalDateTimeInput(nextWindow.from));
    setTo(toLocalDateTimeInput(nextWindow.to));
    setKind("");
    setErrorCode("");
    setSourceTopic("");
    setCorrelationID("");
    setFilters(nextFilters);
    setPagination({
      filterKey: JSON.stringify(nextFilters),
      page: 1,
      cursors: [],
    });
    setSelected(null);
  };
  const goNext = () => {
    if (!issues.data?.next_cursor) return;
    setPagination((current) => ({
      filterKey,
      page: page + 1,
      cursors: [
        ...(current.filterKey === filterKey ? current.cursors : []).slice(
          0,
          page - 1,
        ),
        issues.data!.next_cursor!,
      ],
    }));
  };
  const items = issues.data?.items ?? [];

  return (
    <Shell principal={principal}>
      <main className="page">
        <div className="page-heading">
          <p className="eyebrow">INGESTION OPERATIONS</p>
          <h1>Ingestion Issues</h1>
          <p className="muted">
            集中查看事件契約、Admission 與 connector 技術問題；此頁不顯示原始
            payload。
          </p>
        </div>
        <form className="search-panel" onSubmit={applyFilters}>
          <div className="search-panel-heading">
            <div>
              <p className="eyebrow">SAFE FAILURE SEARCH</p>
              <h3>問題篩選</h3>
            </div>
            <span className="search-limit">預設最近 72 小時 · 最多 7 天</span>
          </div>
          <div className="ingestion-issue-filter-grid">
            <label>
              類型
              <select
                data-testid="ingestion-issue-kind"
                value={kind}
                onChange={(event) =>
                  setKind(event.target.value as IngestionIssueKind | "")
                }
              >
                <option value="">全部</option>
                {Object.entries(ingestionIssueKindLabels).map(
                  ([value, label]) => (
                    <option value={value} key={value}>
                      {label}
                    </option>
                  ),
                )}
              </select>
            </label>
            <label>
              Error code
              <input
                value={errorCode}
                onChange={(event) => setErrorCode(event.target.value)}
                placeholder="例如 SCHEMA_VIOLATION"
              />
            </label>
            <label>
              Source topic
              <input
                value={sourceTopic}
                onChange={(event) => setSourceTopic(event.target.value)}
                placeholder="例如 order.events"
              />
            </label>
            <label>
              Correlation ID
              <input
                value={correlationID}
                onChange={(event) => setCorrelationID(event.target.value)}
                placeholder="若來源可解析"
              />
            </label>
            <label>
              開始時間
              <input
                data-testid="ingestion-issue-from"
                type="datetime-local"
                step="1"
                value={from}
                onChange={(event) => setFrom(event.target.value)}
              />
            </label>
            <label>
              結束時間
              <input
                data-testid="ingestion-issue-to"
                type="datetime-local"
                step="1"
                value={to}
                onChange={(event) => setTo(event.target.value)}
              />
            </label>
          </div>
          {windowError && <p className="field-error">{windowError}</p>}
          <div className="search-actions">
            <button
              type="button"
              className="button-secondary"
              onClick={resetFilters}
            >
              重設
            </button>
            <button
              data-testid="ingestion-issue-search"
              disabled={Boolean(windowError)}
            >
              查詢問題
            </button>
          </div>
        </form>

        <section className="card ingestion-issue-list">
          <div className="card-heading">
            <div>
              <p className="eyebrow">SAFE READ MODEL</p>
              <h3>接入問題列表</h3>
            </div>
            <span className="badge">
              第 {page} 頁 · {items.length} issues
            </span>
          </div>
          {issues.isLoading && <p className="muted">正在載入接入問題…</p>}
          {issues.isError && (
            <p className="field-error" data-testid="ingestion-issue-error">
              接入問題載入失敗：{issues.error.message}
            </p>
          )}
          {!issues.isLoading && !issues.isError && items.length === 0 && (
            <p className="empty-state">此時間範圍與條件下沒有接入問題。</p>
          )}
          <div className="ingestion-issue-table">
            <div className="ingestion-issue-table-head">
              <span>#</span>
              <span>類型／錯誤</span>
              <span>來源</span>
              <span>識別資訊</span>
              <span>發生時間</span>
            </div>
            {items.map((item, index) => (
              <button
                className={`ingestion-issue-row ${selected?.id === item.id ? "selected" : ""}`}
                data-testid={`ingestion-issue-row-${index}`}
                key={`${item.kind}-${item.id}`}
                onClick={() => setSelected(item)}
              >
                <span className="case-number" data-label="#">
                  {String((page - 1) * pageSize + index + 1).padStart(2, "0")}
                </span>
                <span className="ingestion-issue-identity">
                  <em
                    className={`ingestion-kind ingestion-kind-${item.kind.toLowerCase()}`}
                  >
                    {ingestionIssueKindLabels[item.kind]}
                  </em>
                  <strong>{item.error_code}</strong>
                </span>
                <span>
                  <strong>
                    {item.source_topic ?? item.dlq_topic ?? "未知來源"}
                  </strong>
                  <small>{item.pipeline}</small>
                </span>
                <span>
                  <strong>
                    {item.correlation_id ??
                      item.event_type ??
                      "只有 transport metadata"}
                  </strong>
                  <small>
                    {item.event_id ?? item.payload_sha256.slice(0, 12)}
                  </small>
                </span>
                <time>
                  {new Date(item.occurred_at).toLocaleString("zh-TW")}
                </time>
              </button>
            ))}
          </div>
          <div className="pagination">
            <span>第 {page} 頁</span>
            <div>
              <button
                className="button-secondary"
                disabled={page === 1 || issues.isFetching}
                onClick={() =>
                  setPagination((current) => ({
                    filterKey,
                    page: Math.max(1, page - 1),
                    cursors:
                      current.filterKey === filterKey ? current.cursors : [],
                  }))
                }
              >
                上一頁
              </button>
              <button
                className="button-secondary"
                disabled={!issues.data?.next_cursor || issues.isFetching}
                onClick={goNext}
              >
                下一頁
              </button>
            </div>
          </div>
        </section>

        {selected && (
          <div className="drawer-backdrop" onClick={() => setSelected(null)}>
            <section
              ref={drawerRef}
              className="case-drawer ingestion-issue-drawer"
              data-testid="ingestion-issue-detail"
              role="dialog"
              aria-modal="true"
              aria-labelledby="ingestion-issue-detail-title"
              tabIndex={-1}
              onClick={(event) => event.stopPropagation()}
            >
              <div className="drawer-header">
                <div>
                  <p className="eyebrow">{selected.kind}</p>
                  <h3 id="ingestion-issue-detail-title">
                    {selected.error_code}
                  </h3>
                  <p className="muted">
                    {new Date(selected.occurred_at).toLocaleString("zh-TW")}
                  </p>
                </div>
                <button
                  className="drawer-close"
                  data-dialog-initial-focus
                  aria-label="關閉接入問題詳細資料"
                  onClick={() => setSelected(null)}
                >
                  ×
                </button>
              </div>
              <p className="ingestion-safe-notice">
                安全摘要不保存 raw payload、exception message 或 stack
                trace。Payload SHA-256 可供受控維運流程核對原始訊息。
              </p>
              <dl className="ingestion-issue-details">
                <div>
                  <dt>Issue ID</dt>
                  <dd>{selected.id}</dd>
                </div>
                <div>
                  <dt>Pipeline</dt>
                  <dd>{selected.pipeline}</dd>
                </div>
                <div>
                  <dt>Event ID</dt>
                  <dd>{nullableValue(selected.event_id)}</dd>
                </div>
                <div>
                  <dt>Event type</dt>
                  <dd>{nullableValue(selected.event_type)}</dd>
                </div>
                <div>
                  <dt>Correlation ID</dt>
                  <dd>{nullableValue(selected.correlation_id)}</dd>
                </div>
                <div>
                  <dt>Source topic</dt>
                  <dd>{nullableValue(selected.source_topic)}</dd>
                </div>
                <div>
                  <dt>Source partition / offset</dt>
                  <dd>
                    {nullableValue(selected.source_partition)} /{" "}
                    {nullableValue(selected.source_offset)}
                  </dd>
                </div>
                <div>
                  <dt>DLQ topic</dt>
                  <dd>{nullableValue(selected.dlq_topic)}</dd>
                </div>
                <div>
                  <dt>DLQ partition / offset</dt>
                  <dd>
                    {nullableValue(selected.dlq_partition)} /{" "}
                    {nullableValue(selected.dlq_offset)}
                  </dd>
                </div>
                <div>
                  <dt>Admission profile</dt>
                  <dd>{nullableValue(selected.admission_profile)}</dd>
                </div>
                <div>
                  <dt>Connector / task</dt>
                  <dd>
                    {nullableValue(selected.connector_name)} /{" "}
                    {nullableValue(selected.connector_task)}
                  </dd>
                </div>
                <div>
                  <dt>Failure stage</dt>
                  <dd>{nullableValue(selected.failure_stage)}</dd>
                </div>
                <div>
                  <dt>Exception class</dt>
                  <dd>{nullableValue(selected.exception_class)}</dd>
                </div>
                <div className="ingestion-hash">
                  <dt>Payload SHA-256</dt>
                  <dd>{selected.payload_sha256}</dd>
                </div>
              </dl>
              {selected.correlation_id && (
                <button
                  className="ingestion-timeline-link"
                  onClick={() =>
                    navigate(
                      `/event-check?${new URLSearchParams({
                        identifier_type: "CORRELATION_ID",
                        identifier: selected.correlation_id!,
                        from: filters.from ?? initialWindow.from,
                        to: filters.to ?? initialWindow.to,
                        tab: "timeline",
                      }).toString()}`,
                    )
                  }
                >
                  在 Event Check 開啟 →
                </button>
              )}
            </section>
          </div>
        )}
      </main>
    </Shell>
  );
}

export function TimelinePage({ principal }: { principal: Principal }) {
  const location = useLocation();
  const navigate = useNavigate();
  const shortcuts = useQueryShortcutsPanel();
  const timelineQueryClient = useQueryClient();
  const [fallbackWindow, setFallbackWindow] = useState<QueryWindow>(() =>
    dynamicQueryWindow(),
  );
  const canIncludePayload = principal.role === "ADMIN";
  const compatibility = resolveLegacyRoute(location.pathname, location.search);
  const urlFilters = useMemo(
    () =>
      timelineFiltersFromSearch(
        location.search,
        fallbackWindow,
        canIncludePayload,
      ),
    [canIncludePayload, fallbackWindow, location.search],
  );
  const activeFilters =
    hasTimelineCondition(urlFilters) &&
    !queryWindowError(urlFilters.from, urlFilters.to)
      ? urlFilters
      : null;
  const [showModal, setShowModal] = useState(false);
  const [investigation, setInvestigation] = useState<Investigation | null>(
    null,
  );
  const [title, setTitle] = useState("");
  const [severity, setSeverity] = useState<Severity>("HIGH");
  const [attachmentEvent, setAttachmentEvent] = useState<TimelineEvent | null>(
    null,
  );
  const createDialogRef = useDialogFocus<HTMLFormElement>(showModal, () =>
    setShowModal(false),
  );
  const attachmentDialogRef = useDialogFocus<HTMLElement>(
    Boolean(attachmentEvent),
    () => setAttachmentEvent(null),
  );
  const [attachmentResult, setAttachmentResult] = useState<{
    investigation: Investigation;
    attached: boolean;
  } | null>(null);
  const timeline = useQuery({
    queryKey: ["timeline-search", activeFilters],
    queryFn: async () => {
      const result = await api.searchEvents(activeFilters!);
      return {
        correlation_id: activeFilters!.correlation_id || "事件搜尋結果",
        event_count: result.count,
        truncated: result.truncated,
        events: result.events,
      } as Timeline;
    },
    enabled: Boolean(activeFilters),
  });
  const create = useMutation({
    mutationFn: () =>
      api.createInvestigation({
        title,
        severity,
        correlation_id: activeFilters?.correlation_id || "",
        ...investigationWindowWithEndBoundary(
          activeFilters?.from,
          activeFilters?.to,
        ),
      }),
    onSuccess: (item) => {
      setInvestigation(item);
      setShowModal(false);
    },
  });
  const attachmentCandidates = useQuery({
    queryKey: ["investigations", "attachment-candidates"],
    queryFn: () => api.investigations(undefined, {}, 100),
    enabled: Boolean(attachmentEvent),
  });
  const attachEvent = useMutation({
    mutationFn: (item: Investigation) =>
      api.attachInvestigationEvent(
        item,
        attachmentEvent!.event_id,
        activeFilters!.from,
        activeFilters!.to,
      ),
    onSuccess: (result) => {
      const updated = result.investigation;
      timelineQueryClient.setQueryData<Investigation>(
        ["investigation", updated.id],
        (previous) => ({
          ...updated,
          evidence: result.attached
            ? [result.evidence, ...(previous?.evidence ?? updated.evidence)]
            : (previous?.evidence ?? updated.evidence),
          pattern_findings:
            previous?.pattern_findings ?? updated.pattern_findings,
          collaboration_notes:
            previous?.collaboration_notes ?? updated.collaboration_notes,
        }),
      );
      timelineQueryClient.setQueriesData<InvestigationPage>(
        { queryKey: ["investigations"] },
        (page) =>
          page
            ? {
                ...page,
                items: page.items.map((item) =>
                  item.id === updated.id ? { ...item, ...updated } : item,
                ),
              }
            : page,
      );
      void timelineQueryClient.invalidateQueries({
        queryKey: ["evidence", updated.id],
      });
      void timelineQueryClient.invalidateQueries({
        queryKey: ["summary", updated.id],
      });
      setAttachmentResult({
        investigation: updated,
        attached: result.attached,
      });
    },
  });
  const canModify = principal.role !== "VIEWER";
  const attachableCases = (attachmentCandidates.data?.items ?? [])
    .filter((item) => item.status !== "CLOSED")
    .sort((left, right) => {
      const leftMatches =
        left.correlation_id === attachmentEvent?.correlation_id;
      const rightMatches =
        right.correlation_id === attachmentEvent?.correlation_id;
      return Number(rightMatches) - Number(leftMatches);
    });
  return (
    <Shell principal={principal}>
      <main className="page">
        <div className="page-heading timeline-page-heading">
          <div>
            <p className="eyebrow">LEGACY COMPATIBILITY EXPLORER</p>
            <h1>Legacy Event Explorer</h1>
            <p className="muted">
              保留 event type、producer 與 Kafka metadata 等廣泛探索條件。
            </p>
          </div>
          <button
            className="button-secondary"
            data-testid="query-shortcuts-open"
            onClick={() => shortcuts.setOpen(true)}
          >
            ☆ 查詢捷徑
          </button>
        </div>
        {compatibility?.kind === "RETAIN" && (
          <section className="compatibility-notice card" role="status">
            <strong>此頁僅供無法無損遷移的舊查詢</strong>
            <p>{compatibility.reason}</p>
            <a href="/event-check">使用正式 Event Check →</a>
          </section>
        )}
        <TimelineSearchForm
          key={timelineFiltersToSearch(urlFilters, canIncludePayload)}
          initialFilters={urlFilters}
          canIncludePayload={canIncludePayload}
          onSearch={(filters) =>
            navigate(
              `/timeline?${timelineFiltersToSearch(filters, canIncludePayload)}`,
            )
          }
          onReset={() => {
            setFallbackWindow(dynamicQueryWindow());
            navigate("/timeline");
          }}
        />
        {timeline.isLoading && <p className="muted">正在查詢事件…</p>}
        {timeline.isError && (
          <p className="error">查詢失敗：{timeline.error.message}</p>
        )}
        {timeline.data && (
          <section data-testid="timeline-results" className="timeline-card">
            <p className="muted" data-testid="timeline-query-window">
              查詢窗口：
              {queryWindowLabel(activeFilters!.from, activeFilters!.to)} · 時區{" "}
              {browserTimeZone}
            </p>
            <div className="card-heading">
              <div>
                <p className="eyebrow">{timeline.data.correlation_id}</p>
                <h3>
                  事件序列{" "}
                  <span data-testid="timeline-event-count">
                    {timeline.data.event_count}
                  </span>
                </h3>
              </div>
              <div className="result-actions">
                {canModify && activeFilters?.correlation_id && (
                  <button
                    data-testid="create-investigation"
                    className="button-secondary"
                    onClick={() => setShowModal(true)}
                  >
                    ＋ 建立調查案件
                  </button>
                )}
              </div>
            </div>
            <div className="timeline-list">
              {timeline.data.events.length === 0 && (
                <p className="empty-state">找不到符合條件的事件。</p>
              )}
              {timeline.data.events.map((event, index) => (
                <TimelineEventCard
                  key={event.event_id}
                  event={event}
                  index={index}
                  onAttach={
                    canModify && activeFilters
                      ? (selectedEvent) => {
                          attachEvent.reset();
                          setAttachmentResult(null);
                          setAttachmentEvent(selectedEvent);
                        }
                      : undefined
                  }
                />
              ))}
            </div>
            {timeline.data.truncated && (
              <p className="search-warning">
                結果已達查詢上限，請縮小時間範圍或增加篩選條件。
              </p>
            )}
          </section>
        )}
        {investigation && (
          <section data-testid="investigation-detail" className="detail-card">
            <div className="card-heading">
              <div>
                <p className="eyebrow">{investigation.case_no}</p>
                <h3>{investigation.title}</h3>
              </div>
              <span className="badge">{investigation.status}</span>
            </div>
            <p className="muted">案件已連結 {investigation.correlation_id}</p>
            <div className="detail-actions">
              <button
                data-testid="open-created-investigation"
                onClick={() =>
                  navigate(
                    `/investigations/${encodeURIComponent(investigation.id)}`,
                  )
                }
              >
                開啟案件工作區
              </button>
            </div>
            <p className="muted">
              Pattern、Evidence 與 Audit
              都在可分享、可重新整理的案件詳細頁中操作。
            </p>
          </section>
        )}
        {showModal && (
          <div className="modal-backdrop">
            <form
              ref={createDialogRef}
              data-testid="investigation-create-modal"
              className="modal"
              role="dialog"
              aria-modal="true"
              aria-labelledby="investigation-create-title"
              tabIndex={-1}
              onSubmit={(event) => {
                event.preventDefault();
                create.mutate();
              }}
            >
              <p className="eyebrow">NEW INVESTIGATION</p>
              <h3 id="investigation-create-title">建立業務調查案件</h3>
              <label>
                案件標題
                <input
                  data-dialog-initial-focus
                  data-testid="investigation-title"
                  value={title}
                  onChange={(event) => setTitle(event.target.value)}
                  required
                />
              </label>
              <label>
                Severity
                <select
                  data-testid="investigation-severity"
                  value={severity}
                  onChange={(event) =>
                    setSeverity(event.target.value as Severity)
                  }
                >
                  <option>HIGH</option>
                  <option>MEDIUM</option>
                  <option>LOW</option>
                </select>
              </label>
              <div className="modal-actions">
                <button
                  type="button"
                  className="button-secondary"
                  onClick={() => setShowModal(false)}
                >
                  取消
                </button>
                <button data-testid="investigation-create-submit" type="submit">
                  建立案件
                </button>
              </div>
            </form>
          </div>
        )}
        {attachmentEvent && (
          <div className="modal-backdrop">
            <section
              ref={attachmentDialogRef}
              className="modal attachment-modal"
              data-testid="event-attachment-modal"
              role="dialog"
              aria-modal="true"
              aria-labelledby="event-attachment-title"
              tabIndex={-1}
            >
              <div className="attachment-modal-heading">
                <div>
                  <p className="eyebrow">ATTACH EVENT EVIDENCE</p>
                  <h3 id="event-attachment-title">加入案件</h3>
                </div>
                <button
                  type="button"
                  className="drawer-close"
                  aria-label="關閉加入案件"
                  onClick={() => setAttachmentEvent(null)}
                >
                  ×
                </button>
              </div>
              <div className="attachment-event-summary">
                <strong>{attachmentEvent.event_type}</strong>
                <code>{attachmentEvent.event_id}</code>
                <small>{attachmentEvent.correlation_id}</small>
              </div>
              {attachmentResult ? (
                <div
                  className="attachment-success"
                  data-testid="event-attachment-success"
                  role="status"
                  aria-live="polite"
                >
                  <strong>
                    {attachmentResult.attached
                      ? "Event evidence 已加入"
                      : "此 Event 已存在於案件"}
                  </strong>
                  <p>
                    {attachmentResult.investigation.case_no} ·{" "}
                    {attachmentResult.investigation.title}
                  </p>
                  <div className="modal-actions">
                    <button
                      type="button"
                      className="button-secondary"
                      onClick={() => setAttachmentEvent(null)}
                    >
                      留在 Timeline
                    </button>
                    <button
                      type="button"
                      onClick={() => navigate("/investigations")}
                    >
                      前往案件列表
                    </button>
                  </div>
                </div>
              ) : (
                <>
                  <p className="muted">
                    選擇未結案案件；相同 correlation 的案件會排在前面。
                  </p>
                  {attachmentCandidates.isLoading && (
                    <p className="muted">載入案件…</p>
                  )}
                  {attachmentCandidates.isError && (
                    <p className="field-error">案件清單載入失敗。</p>
                  )}
                  <div className="attachment-case-list">
                    {attachableCases.map((item, index) => (
                      <button
                        type="button"
                        key={item.id}
                        data-testid={`event-attachment-case-${index}`}
                        disabled={attachEvent.isPending}
                        onClick={() => attachEvent.mutate(item)}
                      >
                        <span>
                          <strong>{item.title}</strong>
                          <small>{item.case_no}</small>
                        </span>
                        <span>
                          {item.correlation_id ===
                            attachmentEvent.correlation_id && (
                            <em>相同 correlation</em>
                          )}
                          <small>
                            {item.priority} · {item.status}
                          </small>
                        </span>
                      </button>
                    ))}
                    {!attachmentCandidates.isLoading &&
                      attachableCases.length === 0 && (
                        <p className="empty-inline">目前沒有可加入的案件。</p>
                      )}
                  </div>
                  {attachEvent.isError && (
                    <p
                      className="field-error"
                      data-testid="event-attachment-error"
                    >
                      加入失敗：{attachEvent.error.message}
                    </p>
                  )}
                </>
              )}
            </section>
          </div>
        )}
        <QueryShortcutsDrawer
          open={shortcuts.open}
          onClose={() => shortcuts.setOpen(false)}
          principalSubject={principal.subject}
          currentTarget="TIMELINE"
          currentQuery={
            activeFilters
              ? savedSearchQueryFromFilters(activeFilters)
              : undefined
          }
        />
      </main>
    </Shell>
  );
}

type InvestigationTab =
  "summary" | "timeline" | "patterns" | "evidence" | "audit";

type ResolutionAction = "RESOLVE" | "CLOSE";

function investigationTransitionLabel(
  current: InvestigationStatus,
  target: InvestigationStatus,
) {
  if (target === "INVESTIGATING") {
    if (current === "OPEN") return "開始調查";
    if (current === "RESOLVED") return "重新開啟";
    return "返回調查";
  }
  if (target === "WAITING_APPROVAL") return "送交審核";
  if (target === "RESOLVED") return "標記已解決";
  if (target === "CLOSED") return "結案";
  return target;
}

function investigationActionError(error: Error) {
  switch (error.message) {
    case "OPTIMISTIC_LOCK_CONFLICT":
      return "案件已被其他人更新，已重新載入最新內容；請確認後再執行一次。";
    case "RESOLUTION_FIELDS_REQUIRED":
      return "Root cause 與 Resolution summary 皆為必填，請補齊後重試。";
    case "INVALID_STATE_TRANSITION":
    case "CLOSE_OPERATION_REQUIRED":
      return "這個狀態操作已不再適用，已重新載入案件的可用動作。";
    case "FORBIDDEN":
      return "目前角色沒有修改案件的權限；請改用 Investigator 或 Admin。";
    default:
      return `案件操作失敗：${error.message}`;
  }
}

const investigationTabs: { id: InvestigationTab; label: string }[] = [
  { id: "summary", label: "Summary" },
  { id: "timeline", label: "Timeline" },
  { id: "patterns", label: "Patterns" },
  { id: "evidence", label: "Evidence" },
  { id: "audit", label: "Audit" },
];

function parseCollaborationValues(value: string) {
  return value
    .split(/[,，\n]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function CaseCollaborationPanel({
  item,
  canWrite,
  onCaseChanged,
}: {
  item: Investigation;
  canWrite: boolean;
  onCaseChanged: (
    updated: Investigation,
    appendedNote?: InvestigationNote,
  ) => void;
}) {
  const [owner, setOwner] = useState(item.assignee ?? "");
  const [priority, setPriority] = useState(item.priority);
  const [tags, setTags] = useState(item.tags.join(", "));
  const [related, setRelated] = useState(
    item.related_correlation_ids.join(", "),
  );
  const [noteBody, setNoteBody] = useState("");
  const updateCollaboration = useMutation({
    mutationFn: () =>
      api.patchInvestigation(item, {
        assignee: owner,
        priority,
        tags: parseCollaborationValues(tags),
        related_correlation_ids: parseCollaborationValues(related),
      }),
    onSuccess: (updated) => onCaseChanged(updated),
  });
  const appendNote = useMutation({
    mutationFn: () => api.addInvestigationNote(item, noteBody),
    onSuccess: (result) => {
      setNoteBody("");
      onCaseChanged(result.investigation, result.note);
    },
  });

  return (
    <section className="case-collaboration" data-testid="case-collaboration">
      <div className="collaboration-heading">
        <div>
          <p className="eyebrow">CASE COLLABORATION</p>
          <h4>Owner、SLA 與協作紀錄</h4>
        </div>
        <span
          className={`sla-status sla-${item.sla_status.toLowerCase()}`}
          data-testid="case-sla-status"
        >
          {item.sla_status}
        </span>
      </div>
      <dl className="collaboration-facts">
        <div>
          <dt>Priority</dt>
          <dd>{item.priority}</dd>
        </div>
        <div>
          <dt>SLA due</dt>
          <dd>{new Date(item.sla_due_at).toLocaleString("zh-TW")}</dd>
        </div>
        <div>
          <dt>Last updated by</dt>
          <dd>{item.last_updated_by}</dd>
        </div>
      </dl>
      {canWrite && item.status !== "CLOSED" && (
        <div className="collaboration-form">
          <label>
            Owner / Assignee
            <input
              data-testid="case-owner-input"
              value={owner}
              maxLength={200}
              onChange={(event) => setOwner(event.target.value)}
            />
          </label>
          <label>
            Priority
            <select
              data-testid="case-priority-select"
              value={priority}
              onChange={(event) =>
                setPriority(event.target.value as Investigation["priority"])
              }
            >
              {(["P0", "P1", "P2", "P3"] as const).map((value) => (
                <option key={value} value={value}>
                  {value}
                </option>
              ))}
            </select>
          </label>
          <label className="collaboration-wide">
            Tags（逗號分隔）
            <input
              data-testid="case-tags-input"
              value={tags}
              onChange={(event) => setTags(event.target.value)}
            />
          </label>
          <label className="collaboration-wide">
            Related Correlation IDs（逗號分隔）
            <input
              data-testid="case-related-input"
              value={related}
              onChange={(event) => setRelated(event.target.value)}
            />
          </label>
          <button
            data-testid="case-collaboration-save"
            disabled={updateCollaboration.isPending}
            onClick={() => updateCollaboration.mutate()}
          >
            儲存協作欄位
          </button>
        </div>
      )}
      {item.tags.length > 0 && (
        <div className="case-tags" data-testid="case-tags">
          {item.tags.map((tag) => (
            <span key={tag}>#{tag}</span>
          ))}
        </div>
      )}
      {item.related_correlation_ids.length > 0 && (
        <div className="related-correlations">
          <small>Related</small>
          {item.related_correlation_ids.map((correlationID) => (
            <code key={correlationID}>{correlationID}</code>
          ))}
        </div>
      )}
      {(updateCollaboration.isError || appendNote.isError) && (
        <p className="field-error" data-testid="case-collaboration-error">
          協作資料更新失敗：
          {((updateCollaboration.error || appendNote.error) as Error).message}
        </p>
      )}
      <div className="case-notes">
        <div className="case-notes-heading">
          <strong>Append-only notes</strong>
          <span>{item.collaboration_notes.length}</span>
        </div>
        {canWrite && item.status !== "CLOSED" && (
          <div className="case-note-compose">
            <textarea
              data-testid="case-note-input"
              maxLength={2000}
              placeholder="記錄已確認的事實、交接或下一步…"
              value={noteBody}
              onChange={(event) => setNoteBody(event.target.value)}
            />
            <button
              data-testid="case-note-submit"
              disabled={appendNote.isPending || noteBody.trim() === ""}
              onClick={() => appendNote.mutate()}
            >
              追加筆記
            </button>
          </div>
        )}
        <div className="case-note-list" data-testid="case-note-list">
          {item.collaboration_notes.map((note) => (
            <article key={note.id}>
              <p>{note.body}</p>
              <small>
                {note.author_id} · {note.author_role} ·{" "}
                {new Date(note.created_at).toLocaleString("zh-TW")}
              </small>
            </article>
          ))}
          {item.collaboration_notes.length === 0 && (
            <p className="empty-inline">尚無協作筆記。</p>
          )}
        </div>
      </div>
    </section>
  );
}

type PatternAnalysisView = {
  analyzedAt: string;
  analysisStatus: PatternResult["analysis_status"];
  executedPatternIDs: string[];
  effectiveWindow: PatternResult["effective_window"];
  findingCount: number;
};

function stringArray(value: unknown): string[] {
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string")
    : [];
}

function numberValue(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function effectivePatternWindow(
  value: unknown,
): PatternResult["effective_window"] {
  if (!value || typeof value !== "object") return null;
  const candidate = value as Record<string, unknown>;
  if (
    typeof candidate.from !== "string" ||
    typeof candidate.to !== "string" ||
    typeof candidate.observed_at !== "string" ||
    candidate.anchor !== "EARLIEST_CORRELATION_EVENT" ||
    typeof candidate.source_event_count !== "number"
  ) {
    return null;
  }
  return {
    from: candidate.from,
    to: candidate.to,
    observed_at: candidate.observed_at,
    anchor: candidate.anchor,
    source_event_count: candidate.source_event_count,
  };
}

function latestPatternAnalysis(
  summary: InvestigationSummary,
): PatternAnalysisView | null {
  const audit = summary.audit_entries.find(
    (entry) => entry.action === "ANALYZE_INVESTIGATION",
  );
  if (!audit) return null;
  const status = audit.metadata.analysis_status;
  if (status !== "EVALUATED" && status !== "NO_EVENTS") return null;
  return {
    analyzedAt: audit.created_at,
    analysisStatus: status,
    executedPatternIDs: stringArray(
      audit.metadata.executed_pattern_ids ?? audit.metadata.pattern_ids,
    ),
    effectiveWindow: effectivePatternWindow(audit.metadata.effective_window),
    findingCount: numberValue(audit.metadata.finding_count),
  };
}

function CaseEvidenceWindowNotice({
  item,
  summary,
}: {
  item: Investigation;
  summary: InvestigationSummary;
}) {
  const analysis = latestPatternAnalysis(summary);
  if (
    summary.evidence_references.length === 0 ||
    !analysis?.effectiveWindow ||
    queryWindowsAreEqual(
      { from: item.incident_from, to: item.incident_to },
      analysis.effectiveWindow,
    )
  ) {
    return null;
  }

  return (
    <div className="case-warning" data-testid="case-evidence-window-notice">
      <strong>Evidence 與案件 Event Check 使用不同時間窗</strong>
      <p>
        Evidence 是分析時保存的不可變參照，不代表事件一定落在案件基準窗口內。
        最近一次 Pattern Analysis 使用{" "}
        {queryWindowLabel(
          analysis.effectiveWindow.from,
          analysis.effectiveWindow.to,
        )}
        ，共讀取 {analysis.effectiveWindow.source_event_count} 個 source
        events。
      </p>
      <a
        data-testid="case-open-pattern-window-timeline"
        href={investigationTimelineWindowURL(item, analysis.effectiveWindow)}
      >
        在 Event Check 開啟分析時間窗 →
      </a>
    </div>
  );
}

function patternAnalysisError(error: Error): string {
  switch (error.message) {
    case "PATTERN_SOURCE_TIMEOUT":
      return "事件來源查詢逾時，沒有產生新的判定；請稍後重試。";
    case "PATTERN_SOURCE_UNAVAILABLE":
      return "事件來源目前不可用，既有 Finding 不受影響；請在來源恢復後重試。";
    case "PATTERN_PERSISTENCE_UNAVAILABLE":
      return "分析完成前無法保存 Finding、Evidence 與 Audit，因此本次沒有套用結果。";
    case "UNKNOWN_PATTERN":
      return "選取的 Pattern 已不存在或未啟用，請重新載入 Registry。";
    default:
      return `Pattern Analysis 失敗：${error.message}`;
  }
}

function CasePatternAnalysisPanel({
  item,
  summary,
  principal,
}: {
  item: Investigation;
  summary: InvestigationSummary;
  principal: Principal;
}) {
  const queryClient = useQueryClient();
  const registry = useQuery({
    queryKey: ["patterns"],
    queryFn: api.patterns,
  });
  const [selectionMode, setSelectionMode] = useState<"ALL" | "SELECTED">("ALL");
  const [selectedPatternIDs, setSelectedPatternIDs] = useState<string[]>([]);
  const analyze = useMutation({
    mutationFn: () =>
      api.analyze(
        item.id,
        selectionMode === "SELECTED" ? selectedPatternIDs : undefined,
      ),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["summary", item.id] }),
        queryClient.invalidateQueries({ queryKey: ["evidence", item.id] }),
        queryClient.invalidateQueries({
          queryKey: ["investigation", item.id],
        }),
      ]);
    },
  });
  const currentAnalysis: PatternAnalysisView | null = analyze.data
    ? {
        analyzedAt: analyze.data.analyzed_at,
        analysisStatus: analyze.data.analysis_status,
        executedPatternIDs: analyze.data.executed_pattern_ids,
        effectiveWindow: analyze.data.effective_window,
        findingCount: analyze.data.findings.length,
      }
    : latestPatternAnalysis(summary);
  const canRun = principal.role !== "VIEWER" && item.status !== "CLOSED";
  const selectedModeWithoutPattern =
    selectionMode === "SELECTED" && selectedPatternIDs.length === 0;

  return (
    <section
      className="case-pattern-analysis"
      data-testid="case-pattern-analysis"
    >
      <div className="reference-heading">
        <div>
          <p className="eyebrow">DETERMINISTIC ANALYSIS</p>
          <h4>案件 Pattern Analysis</h4>
        </div>
        <span className="badge">Server-owned Registry</span>
      </div>
      <p className="muted">
        預設由後端執行所有 ACTIVE Pattern；分析成功後會同步刷新
        Finding、Evidence 與 Audit。
      </p>
      {canRun ? (
        <>
          <button
            type="button"
            data-testid="run-case-pattern-analysis"
            disabled={analyze.isPending || selectedModeWithoutPattern}
            onClick={() => analyze.mutate()}
          >
            {analyze.isPending
              ? "分析中…"
              : selectionMode === "ALL"
                ? "執行所有適用 Pattern"
                : `執行已選 Pattern（${selectedPatternIDs.length}）`}
          </button>
          <details className="pattern-analysis-advanced">
            <summary>進階：指定 Pattern</summary>
            <label className="checkbox-field">
              <input
                type="radio"
                name={`pattern-mode-${item.id}`}
                checked={selectionMode === "ALL"}
                onChange={() => setSelectionMode("ALL")}
              />
              所有 ACTIVE Pattern（建議）
            </label>
            <label className="checkbox-field">
              <input
                type="radio"
                name={`pattern-mode-${item.id}`}
                checked={selectionMode === "SELECTED"}
                onChange={() => setSelectionMode("SELECTED")}
              />
              只執行下列選取項目
            </label>
            {registry.isLoading && <p className="muted">載入 Registry…</p>}
            {registry.isError && (
              <p className="field-error">
                Registry 載入失敗：{(registry.error as Error).message}
              </p>
            )}
            {(registry.data ?? []).map((pattern: PatternDefinition) => (
              <label className="checkbox-field" key={pattern.id}>
                <input
                  type="checkbox"
                  data-testid={`case-pattern-select-${pattern.id}`}
                  disabled={selectionMode !== "SELECTED"}
                  checked={selectedPatternIDs.includes(pattern.id)}
                  onChange={(event) =>
                    setSelectedPatternIDs((current) =>
                      event.target.checked
                        ? [...current, pattern.id]
                        : current.filter((id) => id !== pattern.id),
                    )
                  }
                />
                {pattern.name} · v{pattern.version}
              </label>
            ))}
            {selectedModeWithoutPattern && (
              <p className="field-error">請至少選取一個 Pattern。</p>
            )}
          </details>
        </>
      ) : (
        <p className="muted" data-testid="case-pattern-readonly">
          {item.status === "CLOSED"
            ? "案件已結案，只能查看既有分析結果。"
            : "Viewer 只能查看既有分析結果。"}
        </p>
      )}
      {analyze.isError && (
        <p className="field-error" data-testid="case-pattern-analysis-error">
          {patternAnalysisError(analyze.error as Error)}
        </p>
      )}
      {currentAnalysis ? (
        <div
          className="pattern-analysis-result"
          data-testid="case-pattern-analysis-result"
          role="status"
          aria-live="polite"
        >
          <div className="reference-heading">
            <strong data-testid="case-pattern-analysis-status">
              {currentAnalysis.analysisStatus === "NO_EVENTS"
                ? "資料不足：沒有 canonical event"
                : currentAnalysis.findingCount === 0
                  ? "已評估：未命中"
                  : `已命中 ${currentAnalysis.findingCount} 個 Finding`}
            </strong>
            <time>
              {new Date(currentAnalysis.analyzedAt).toLocaleString("zh-TW")}
            </time>
          </div>
          <p>
            執行：
            {currentAnalysis.executedPatternIDs.join("、") ||
              "舊資料未保存執行清單"}
          </p>
          {currentAnalysis.effectiveWindow ? (
            <p data-testid="case-pattern-effective-window">
              Event-time window：
              {queryWindowLabel(
                currentAnalysis.effectiveWindow.from,
                currentAnalysis.effectiveWindow.to,
              )}
              {" · "}
              {currentAnalysis.effectiveWindow.source_event_count} source events
            </p>
          ) : (
            <p>沒有 source event，因此本次不能判定 Pattern 是否命中。</p>
          )}
        </div>
      ) : (
        <p className="empty-inline">尚未執行 Pattern Analysis。</p>
      )}
    </section>
  );
}

export function InvestigationsPage({ principal }: { principal: Principal }) {
  const caseQueryClient = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const [rootCause, setRootCause] = useState("");
  const [resolution, setResolution] = useState("");
  const [resolutionAction, setResolutionAction] =
    useState<ResolutionAction | null>(null);
  const [caseActionNotice, setCaseActionNotice] = useState("");
  const detailRouteMatch = location.pathname.match(
    /^\/investigations\/([^/]+)$/,
  );
  // React Router already exposes a decoded pathname. Investigation IDs are
  // opaque server identifiers, so do not decode a second time (which can also
  // throw for a malformed percent escape in a manually entered URL).
  const selectedId = detailRouteMatch?.[1] ?? null;
  const readWindowQuery = new URLSearchParams(location.search);
  const requestedTab = readWindowQuery.get("tab");
  const activeTab: InvestigationTab = [
    "summary",
    "timeline",
    "patterns",
    "evidence",
    "audit",
  ].includes(requestedTab ?? "")
    ? (requestedTab as InvestigationTab)
    : "summary";
  const setActiveTab = (tab: InvestigationTab) => {
    const next = new URLSearchParams(location.search);
    if (tab === "summary") next.delete("tab");
    else next.set("tab", tab);
    navigate(`${location.pathname}?${next.toString()}`);
  };
  const readFrom = readWindowQuery.get("from") || undefined;
  const readTo = readWindowQuery.get("to") || undefined;
  const readWindow =
    readFrom && readTo ? { from: readFrom, to: readTo } : undefined;
  const listQuery = parseInvestigationListQuery(location.search);
  const {
    query: caseQuery,
    status,
    severity: severityFilter,
    priority: priorityFilter,
    assignee: assigneeFilter,
    tag: tagFilter,
    correlationId: correlationFilter,
    sortBy,
    sortOrder,
    filters: investigationFilters,
    key: filterKey,
  } = listQuery;
  const requestedPage = Number(readWindowQuery.get("page") ?? "1");
  const page =
    Number.isSafeInteger(requestedPage) && requestedPage > 0
      ? requestedPage
      : 1;
  const cursor = readWindowQuery.get("cursor") || undefined;
  const list = useQuery({
    queryKey: ["investigations", cursor, investigationFilters],
    queryFn: () => api.investigations(cursor, investigationFilters),
  });
  const detail = useQuery({
    queryKey: ["investigation", selectedId],
    queryFn: () => api.investigation(selectedId!),
    enabled: Boolean(selectedId),
  });
  const summary = useQuery({
    queryKey: ["summary", selectedId, readFrom, readTo],
    queryFn: () => api.summary(selectedId!, readWindow),
    enabled: Boolean(selectedId),
  });
  const evidence = useQuery({
    queryKey: ["evidence", selectedId, readFrom, readTo],
    queryFn: () => api.evidence(selectedId!, readWindow),
    enabled: Boolean(selectedId) && activeTab === "evidence",
  });
  const cacheUpdatedInvestigation = (
    updated: Investigation,
    appendedNote?: InvestigationNote,
  ) => {
    const detailKey = ["investigation", updated.id] as const;
    caseQueryClient.setQueryData<Investigation>(detailKey, (previous) => ({
      ...updated,
      pattern_findings: previous?.pattern_findings ?? updated.pattern_findings,
      evidence: previous?.evidence ?? updated.evidence,
      collaboration_notes: appendedNote
        ? [appendedNote, ...(previous?.collaboration_notes ?? [])]
        : (previous?.collaboration_notes ?? updated.collaboration_notes),
    }));
    caseQueryClient.setQueriesData<InvestigationPage>(
      { queryKey: ["investigations"] },
      (previous) =>
        previous
          ? {
              ...previous,
              items: previous.items.map((item) =>
                item.id === updated.id ? { ...item, ...updated } : item,
              ),
            }
          : previous,
    );
    caseQueryClient.setQueriesData<InvestigationSummary>(
      { queryKey: ["summary", updated.id] },
      (previous) =>
        previous
          ? {
              ...previous,
              case: {
                ...updated,
                collaboration_notes: appendedNote
                  ? [appendedNote, ...previous.case.collaboration_notes]
                  : previous.case.collaboration_notes,
              },
            }
          : previous,
    );
    void caseQueryClient.invalidateQueries({
      queryKey: ["summary", updated.id],
    });
  };
  const update = useMutation({
    mutationFn: ({ input }: { input: InvestigationUpdate; notice: string }) =>
      api.patchInvestigation(detail.data!, input),
    onSuccess: (updated, command) => {
      cacheUpdatedInvestigation(updated);
      setCaseActionNotice(command.notice);
      setResolutionAction(null);
      setRootCause("");
      setResolution("");
    },
    onError: (error) => {
      if (
        error.message === "OPTIMISTIC_LOCK_CONFLICT" ||
        error.message === "INVALID_STATE_TRANSITION" ||
        error.message === "CLOSE_OPERATION_REQUIRED"
      ) {
        void caseQueryClient.invalidateQueries({
          queryKey: ["investigation", selectedId],
        });
        void caseQueryClient.invalidateQueries({
          queryKey: ["summary", selectedId],
        });
        void caseQueryClient.invalidateQueries({
          queryKey: ["investigations"],
        });
      }
    },
  });
  const close = useMutation({
    mutationFn: () =>
      api.closeInvestigation(detail.data!, rootCause, resolution),
    onSuccess: (updated) => {
      cacheUpdatedInvestigation(updated);
      setCaseActionNotice("案件已結案，後續內容改為唯讀。");
      setResolutionAction(null);
      setRootCause("");
      setResolution("");
    },
    onError: (error) => {
      if (error.message === "OPTIMISTIC_LOCK_CONFLICT") {
        void caseQueryClient.invalidateQueries({
          queryKey: ["investigation", selectedId],
        });
        void caseQueryClient.invalidateQueries({
          queryKey: ["summary", selectedId],
        });
        void caseQueryClient.invalidateQueries({
          queryKey: ["investigations"],
        });
      }
    },
  });
  const classifyFinding = useMutation({
    mutationFn: ({
      findingId,
      lockVersion,
      status,
    }: {
      findingId: string;
      lockVersion: number;
      status: "CONFIRMED" | "FALSE_POSITIVE" | "NEEDS_REVIEW";
    }) =>
      api.classifyPatternFinding(selectedId!, findingId, lockVersion, status),
    onSuccess: () => {
      void caseQueryClient.invalidateQueries({
        queryKey: ["summary", selectedId],
      });
      void caseQueryClient.invalidateQueries({
        queryKey: ["investigation", selectedId],
      });
    },
  });
  const pageSize = 10;
  const items = list.data?.items ?? [];
  const hasNextPage = Boolean(list.data?.next_cursor);
  const goNext = () => {
    if (!list.data?.next_cursor) return;
    const next = new URLSearchParams(location.search);
    next.set("page", String(page + 1));
    next.set("cursor", list.data.next_cursor);
    navigate(`${location.pathname}?${next.toString()}`);
  };
  const resolutionDraftDirty =
    resolutionAction !== null &&
    (rootCause.trim() !== "" || resolution.trim() !== "");
  const resetResolutionDraft = () => {
    setRootCause("");
    setResolution("");
    setResolutionAction(null);
  };
  const discardResolutionDraft = () => {
    if (
      resolutionDraftDirty &&
      !window.confirm("尚有未送出的調查結論，確定要放棄嗎？")
    ) {
      return false;
    }
    resetResolutionDraft();
    return true;
  };
  useEffect(() => {
    if (!resolutionDraftDirty) return;
    const warnBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", warnBeforeUnload);
    return () => window.removeEventListener("beforeunload", warnBeforeUnload);
  }, [resolutionDraftDirty]);
  const selectInvestigation = (id: string) => {
    if (!discardResolutionDraft()) return;
    setCaseActionNotice("");
    update.reset();
    close.reset();
    const next = new URLSearchParams(location.search);
    next.delete("tab");
    navigate(`/investigations/${encodeURIComponent(id)}?${next.toString()}`);
  };
  const closeDrawer = () => {
    if (!discardResolutionDraft()) return;
    setCaseActionNotice("");
    navigate(`/investigations${location.search}`);
  };
  const applyListFilters = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const next = new URLSearchParams();
    for (const name of [
      "query",
      "status",
      "severity",
      "priority",
      "assignee",
      "tag",
      "correlation_id",
      "sort_by",
      "sort_order",
    ]) {
      const value = String(data.get(name) ?? "").trim();
      if (value) next.set(name, value);
    }
    navigate(`/investigations?${next.toString()}`);
  };
  const drawerDialogRef = useDialogFocus<HTMLElement>(
    Boolean(selectedId),
    closeDrawer,
  );
  const closeConfirmationRef = useDialogFocus<HTMLElement>(
    resolutionAction !== null,
    discardResolutionDraft,
  );
  const executeTransition = (target: InvestigationStatus) => {
    if (!detail.data || !detail.data.allowed_transitions.includes(target)) {
      return;
    }
    setCaseActionNotice("");
    update.reset();
    close.reset();
    if (target === "RESOLVED") {
      setResolutionAction("RESOLVE");
      return;
    }
    if (target === "CLOSED") {
      setResolutionAction("CLOSE");
      return;
    }
    if (target !== "INVESTIGATING" && target !== "WAITING_APPROVAL") return;
    update.mutate({
      input: { status: target },
      notice: `案件狀態已更新為 ${target}。`,
    });
  };
  const submitResolution = () => {
    if (
      !detail.data ||
      !resolutionAction ||
      rootCause.trim() === "" ||
      resolution.trim() === ""
    ) {
      return;
    }
    if (resolutionAction === "CLOSE") {
      close.mutate();
      return;
    }
    update.mutate({
      input: {
        status: "RESOLVED",
        root_cause: rootCause.trim(),
        resolution_summary: resolution.trim(),
      },
      notice: "案件已標記為已解決，調查結論已保存。",
    });
  };
  return (
    <Shell principal={principal}>
      <main className="page">
        <div className="page-heading">
          <p className="eyebrow">CASE MANAGEMENT</p>
          <h1>Investigation Cases</h1>
          <p className="muted">
            集中查看案件狀態、Pattern finding 與證據摘要。
          </p>
        </div>
        <section className="case-list card">
          <div className="card-heading">
            <div>
              <p className="eyebrow">CASE REGISTER</p>
              <h3>案件列表</h3>
              {[
                caseQuery,
                status,
                severityFilter,
                priorityFilter,
                assigneeFilter,
                tagFilter,
                correlationFilter,
              ].some(Boolean) && (
                <small className="muted">複合篩選已啟用</small>
              )}
            </div>
            <span className="badge">
              第 {page} 頁 · {items.length} cases
            </span>
          </div>
          <form
            className="case-list-filters"
            data-testid="case-list-filters"
            key={filterKey}
            onSubmit={applyListFilters}
          >
            <label className="case-list-query">
              案件編號／標題
              <input
                name="query"
                defaultValue={caseQuery ?? ""}
                maxLength={100}
                autoComplete="off"
                placeholder="例如 MANUAL-4002 或付款異常"
              />
            </label>
            <label>
              Status
              <select name="status" defaultValue={status ?? ""}>
                <option value="">全部</option>
                {[
                  "OPEN",
                  "INVESTIGATING",
                  "WAITING_APPROVAL",
                  "RESOLVED",
                  "CLOSED",
                ].map((value) => (
                  <option key={value}>{value}</option>
                ))}
              </select>
            </label>
            <label>
              Severity
              <select name="severity" defaultValue={severityFilter ?? ""}>
                <option value="">全部</option>
                {["LOW", "MEDIUM", "HIGH", "CRITICAL"].map((value) => (
                  <option key={value}>{value}</option>
                ))}
              </select>
            </label>
            <label>
              Priority
              <select name="priority" defaultValue={priorityFilter ?? ""}>
                <option value="">全部</option>
                {["P0", "P1", "P2", "P3"].map((value) => (
                  <option key={value}>{value}</option>
                ))}
              </select>
            </label>
            <label>
              Owner
              <input name="assignee" defaultValue={assigneeFilter ?? ""} />
            </label>
            <label>
              Tag
              <input name="tag" defaultValue={tagFilter ?? ""} />
            </label>
            <label>
              Correlation ID
              <input
                name="correlation_id"
                defaultValue={correlationFilter ?? ""}
              />
            </label>
            <label>
              Sort by
              <select name="sort_by" defaultValue={sortBy}>
                <option value="created_at">建立時間</option>
                <option value="updated_at">更新時間</option>
              </select>
            </label>
            <label>
              Order
              <select name="sort_order" defaultValue={sortOrder}>
                <option value="desc">新到舊</option>
                <option value="asc">舊到新</option>
              </select>
            </label>
            <div className="case-list-filter-actions">
              <button type="submit">套用</button>
              <button
                type="button"
                className="button-secondary"
                onClick={() => navigate("/investigations")}
              >
                清除
              </button>
            </div>
          </form>
          <div className="case-table">
            <div className="case-table-head">
              <span>#</span>
              <span>案件</span>
              <span>狀態</span>
              <span>Severity</span>
              <span>Priority</span>
              <span>Correlation ID</span>
              <span>負責人</span>
              <span>更新時間</span>
            </div>
            {list.isLoading && <p className="muted">載入案件…</p>}
            {list.isError && (
              <p className="field-error" data-testid="case-list-error">
                {userFacingError(list.error, "案件列表載入失敗。")}
              </p>
            )}
            {items.map((item, index) => (
              <button
                className={`case-table-row ${selectedId === item.id ? "selected" : ""}`}
                data-testid={`case-row-${index}`}
                key={item.id}
                onClick={() => selectInvestigation(item.id)}
              >
                <span className="case-number">
                  {String((page - 1) * pageSize + index + 1).padStart(2, "0")}
                </span>
                <span className="case-name" data-label="案件">
                  <strong>{item.title}</strong>
                  <small>{item.case_no}</small>
                </span>
                <span data-label="狀態">
                  <em className={`status status-${item.status.toLowerCase()}`}>
                    {item.status}
                  </em>
                </span>
                <span data-label="Severity">
                  <em
                    className={`severity severity-${item.severity.toLowerCase()}`}
                  >
                    {item.severity}
                  </em>
                </span>
                <span className="case-priority" data-label="Priority">
                  {item.priority}
                </span>
                <span className="case-correlation" data-label="Correlation ID">
                  {item.correlation_id}
                </span>
                <span data-label="負責人">{item.assignee || "未指派"}</span>
                <span className="case-updated" data-label="更新時間">
                  {new Date(item.updated_at).toLocaleString("zh-TW")}
                </span>
              </button>
            ))}
          </div>
          <div className="pagination">
            <span>第 {page} 頁</span>
            <div>
              <button
                className="button-secondary"
                disabled={page === 1 || list.isFetching}
                onClick={() => navigate(-1)}
              >
                上一頁
              </button>
              <button
                className="button-secondary"
                disabled={!hasNextPage || list.isFetching}
                onClick={goNext}
              >
                下一頁
              </button>
            </div>
          </div>
        </section>
        {selectedId && (
          <div className="drawer-backdrop" onClick={closeDrawer}>
            <section
              ref={drawerDialogRef}
              className="case-drawer"
              data-testid="case-detail"
              role="dialog"
              aria-modal="true"
              aria-labelledby={detail.data ? "case-detail-title" : undefined}
              aria-label={detail.data ? undefined : "案件詳細"}
              tabIndex={-1}
              onClick={(event) => event.stopPropagation()}
            >
              <div className="drawer-header drawer-header-loading">
                {detail.data ? (
                  <div>
                    <p className="eyebrow">{detail.data.case_no}</p>
                    <h3 id="case-detail-title">{detail.data.title}</h3>
                  </div>
                ) : (
                  <p className="muted">載入案件詳細…</p>
                )}
                <button
                  className="drawer-close"
                  data-testid="case-detail-close"
                  aria-label="關閉案件詳細"
                  onClick={closeDrawer}
                >
                  ×
                </button>
              </div>
              {detail.isError && (
                <p className="field-error" data-testid="case-detail-error">
                  {(detail.error as Error).message === "NOT_FOUND"
                    ? "找不到案件。"
                    : `案件詳細載入失敗：${(detail.error as Error).message}`}
                </p>
              )}
              {detail.data && (
                <>
                  <div className="case-state-line">
                    <span className="muted">{detail.data.correlation_id}</span>
                    <span className="badge" data-testid="case-status">
                      {detail.data.status}
                    </span>
                    <span
                      className={`severity severity-${detail.data.severity.toLowerCase()}`}
                    >
                      {detail.data.severity}
                    </span>
                    <span className="case-priority">
                      {detail.data.priority}
                    </span>
                    <span
                      className={`sla-status sla-${detail.data.sla_status.toLowerCase()}`}
                    >
                      {detail.data.sla_status}
                    </span>
                  </div>
                  <section
                    className="case-incident-window"
                    data-testid="case-incident-window"
                  >
                    <div>
                      <small>
                        案件基準窗口 · {detail.data.incident_window_source}
                      </small>
                      <strong>
                        {queryWindowLabel(
                          detail.data.incident_from,
                          detail.data.incident_to,
                        )}
                      </strong>
                      <a
                        data-testid="case-open-baseline-timeline"
                        href={investigationTimelineURL(detail.data)}
                      >
                        在 Event Check 開啟／調整
                      </a>
                    </div>
                    {readWindow && (
                      <div data-testid="case-current-window">
                        <small>目前檢視窗口（不會修改案件基準）</small>
                        <strong>
                          {queryWindowLabel(readWindow.from, readWindow.to)}
                        </strong>
                      </div>
                    )}
                  </section>
                  {principal.role !== "VIEWER" &&
                    detail.data.status !== "CLOSED" && (
                      <div
                        className="detail-actions"
                        data-testid="case-state-actions"
                      >
                        {detail.data.allowed_transitions.map((target) => (
                          <button
                            key={target}
                            className={
                              target === "CLOSED"
                                ? "button-secondary"
                                : undefined
                            }
                            data-testid={
                              target === "CLOSED"
                                ? "case-close-start"
                                : `case-transition-${target.toLowerCase()}`
                            }
                            onClick={(event) => {
                              event.currentTarget.focus();
                              executeTransition(target);
                            }}
                            disabled={update.isPending || close.isPending}
                          >
                            {investigationTransitionLabel(
                              detail.data.status,
                              target,
                            )}
                          </button>
                        ))}
                      </div>
                    )}
                  {caseActionNotice && (
                    <p
                      className="success-message"
                      role="status"
                      data-testid="case-action-success"
                    >
                      {caseActionNotice}
                    </p>
                  )}
                  {(update.isError || close.isError) && (
                    <p className="field-error" data-testid="case-action-error">
                      {investigationActionError(
                        (update.error || close.error) as Error,
                      )}
                    </p>
                  )}
                </>
              )}
              <nav className="case-tabs" aria-label="案件詳細頁籤">
                {investigationTabs.map((tab) => (
                  <button
                    key={tab.id}
                    type="button"
                    data-testid={`case-tab-${tab.id}`}
                    aria-current={activeTab === tab.id ? "page" : undefined}
                    className={activeTab === tab.id ? "active" : ""}
                    onClick={() => setActiveTab(tab.id)}
                  >
                    {tab.label}
                  </button>
                ))}
              </nav>
              {summary.isLoading && <p className="muted">載入案件整合資料…</p>}
              {summary.isError && (
                <p className="field-error" data-testid="case-summary-error">
                  案件整合資料載入失敗：{(summary.error as Error).message}
                </p>
              )}
              {summary.data && activeTab === "summary" && (
                <section className="case-tab-panel" data-testid="case-summary">
                  <div className="case-metrics">
                    <div>
                      <small>Events</small>
                      <strong data-testid="case-event-count">
                        {summary.data.source_status.clickhouse === "OK"
                          ? summary.data.timeline.event_count
                          : "不可用"}
                      </strong>
                    </div>
                    <div>
                      <small>Findings</small>
                      <strong>{summary.data.pattern_findings.length}</strong>
                    </div>
                    <div>
                      <small>Evidence</small>
                      <strong>{summary.data.evidence_references.length}</strong>
                    </div>
                  </div>
                  {detail.data && (
                    <CaseCollaborationPanel
                      key={`${detail.data.id}:${detail.data.lock_version}`}
                      item={detail.data}
                      canWrite={principal.role !== "VIEWER"}
                      onCaseChanged={cacheUpdatedInvestigation}
                    />
                  )}
                  <dl className="case-summary-meta">
                    <div>
                      <dt>Owner / Assignee</dt>
                      <dd>{detail.data?.assignee || "未指派"}</dd>
                    </div>
                    <div>
                      <dt>Updated</dt>
                      <dd>
                        {detail.data
                          ? new Date(detail.data.updated_at).toLocaleString(
                              "zh-TW",
                            )
                          : "—"}
                      </dd>
                    </div>
                    <div>
                      <dt>Root cause</dt>
                      <dd>{detail.data?.root_cause || "尚未填寫"}</dd>
                    </div>
                    <div>
                      <dt>Resolution</dt>
                      <dd>{detail.data?.resolution_summary || "尚未填寫"}</dd>
                    </div>
                    <div>
                      <dt>Summary generated</dt>
                      <dd data-testid="case-summary-generated-at">
                        {new Date(summary.data.generated_at).toLocaleString(
                          "zh-TW",
                        )}
                      </dd>
                    </div>
                    <div>
                      <dt>Event retention boundary</dt>
                      <dd data-testid="case-retention-boundary">
                        {new Date(
                          summary.data.event_retention_boundary,
                        ).toLocaleString("zh-TW")}
                      </dd>
                    </div>
                    <div>
                      <dt>Timeline result</dt>
                      <dd data-testid="case-timeline-truncation">
                        {summary.data.source_status.clickhouse !== "OK"
                          ? "來源不可用"
                          : summary.data.timeline.truncated
                            ? "已達查詢上限，結果已截斷"
                            : "完整（未截斷）"}
                      </dd>
                    </div>
                    <div>
                      <dt>ClickHouse last success</dt>
                      <dd data-testid="case-clickhouse-last-success">
                        {summary.data.source_last_success_at.clickhouse
                          ? new Date(
                              summary.data.source_last_success_at.clickhouse,
                            ).toLocaleString("zh-TW")
                          : "本次查詢無可驗證成功時間"}
                      </dd>
                    </div>
                  </dl>
                  <div className="source-status-grid">
                    {Object.entries(summary.data.source_status).map(
                      ([source, status]) => (
                        <span key={source}>
                          {source}: <strong>{status}</strong>
                        </span>
                      ),
                    )}
                  </div>
                  {(summary.data.partial ||
                    summary.data.warnings.length > 0) && (
                    <div className="case-warning">
                      <strong>Partial data</strong>
                      <p>
                        {summary.data.warnings.join("；") ||
                          "部分資料來源目前不可用。"}
                      </p>
                    </div>
                  )}
                </section>
              )}
              {summary.data && activeTab === "timeline" && (
                <section className="case-tab-panel" data-testid="case-timeline">
                  {summary.data.source_status.clickhouse !== "OK" ? (
                    <div
                      className="case-warning"
                      data-testid="case-timeline-unavailable"
                    >
                      <strong>事件來源目前不可用</strong>
                      <p>
                        案件與 PostgreSQL 資料仍可使用；請稍後重試事件時間線。
                      </p>
                    </div>
                  ) : (
                    <>
                      <p className="muted">
                        {summary.data.timeline.event_count} events · correlation{" "}
                        {summary.data.timeline.correlation_id}
                      </p>
                      <div className="timeline-list case-timeline-list">
                        {summary.data.timeline.events.map((event, index) => (
                          <TimelineEventCard
                            key={event.event_id}
                            event={event}
                            index={index}
                          />
                        ))}
                      </div>
                      {summary.data.timeline.events.length === 0 && (
                        <>
                          {detail.data && (
                            <CaseEvidenceWindowNotice
                              item={detail.data}
                              summary={summary.data}
                            />
                          )}
                          <p className="empty-inline">
                            目前案件基準時間窗內沒有事件。
                          </p>
                        </>
                      )}
                    </>
                  )}
                </section>
              )}
              {summary.data && activeTab === "patterns" && (
                <section className="case-tab-panel" data-testid="case-patterns">
                  {detail.data && (
                    <CasePatternAnalysisPanel
                      key={detail.data.id}
                      item={detail.data}
                      summary={summary.data}
                      principal={principal}
                    />
                  )}
                  {summary.data.pattern_findings.map((finding) => (
                    <article
                      className="case-reference"
                      data-testid={`case-pattern-finding-${finding.pattern_id}`}
                      key={finding.finding_id ?? finding.pattern_id}
                    >
                      <div className="reference-heading">
                        <strong>{finding.pattern_id}</strong>
                        <span
                          className={`severity severity-${finding.severity.toLowerCase()}`}
                        >
                          {finding.severity}
                        </span>
                      </div>
                      <p>{finding.matched_conditions.join(" · ")}</p>
                      <small>
                        {finding.recommended_next_query || "無建議查詢"}
                      </small>
                      {finding.feedback && (
                        <div
                          className="pattern-feedback"
                          data-testid={`pattern-feedback-${finding.pattern_id}`}
                        >
                          <span>
                            人工判定：<strong>{finding.feedback.status}</strong>
                          </span>
                          {finding.feedback.updated_at && (
                            <small>
                              {finding.feedback.actor_id} ·{" "}
                              {new Date(
                                finding.feedback.updated_at,
                              ).toLocaleString("zh-TW")}
                            </small>
                          )}
                          {principal.role !== "VIEWER" &&
                            finding.finding_id && (
                              <div className="pattern-feedback-actions">
                                {(
                                  [
                                    ["CONFIRMED", "確認命中"],
                                    ["FALSE_POSITIVE", "標記誤報"],
                                    ["NEEDS_REVIEW", "需要複核"],
                                  ] as const
                                ).map(([status, label]) => (
                                  <button
                                    type="button"
                                    className="button-secondary"
                                    key={status}
                                    disabled={classifyFinding.isPending}
                                    aria-label={`${finding.pattern_id} ${label}`}
                                    onClick={() =>
                                      classifyFinding.mutate({
                                        findingId: finding.finding_id!,
                                        lockVersion:
                                          finding.feedback!.lock_version,
                                        status,
                                      })
                                    }
                                  >
                                    {label}
                                  </button>
                                ))}
                              </div>
                            )}
                        </div>
                      )}
                    </article>
                  ))}
                  {classifyFinding.isError && (
                    <p
                      className="field-error"
                      data-testid="pattern-feedback-error"
                    >
                      Pattern 判定失敗：
                      {(classifyFinding.error as Error).message}
                    </p>
                  )}
                  {summary.data.pattern_findings.length === 0 && (
                    <p className="empty-inline">尚無 Pattern finding。</p>
                  )}
                </section>
              )}
              {activeTab === "evidence" && (
                <section className="case-tab-panel" data-testid="case-evidence">
                  {detail.data && summary.data && (
                    <CaseEvidenceWindowNotice
                      item={detail.data}
                      summary={summary.data}
                    />
                  )}
                  {evidence.isLoading && (
                    <p className="muted">載入 Evidence manifest…</p>
                  )}
                  {evidence.isError && (
                    <p
                      className="field-error"
                      data-testid="case-evidence-error"
                    >
                      Evidence 載入失敗：{(evidence.error as Error).message}
                    </p>
                  )}
                  {evidence.data && (
                    <EvidenceManifestPanel manifest={evidence.data} />
                  )}
                </section>
              )}
              {summary.data && activeTab === "audit" && (
                <section className="case-tab-panel" data-testid="case-audit">
                  {summary.data.audit_entries.map((entry) => (
                    <article className="audit-entry" key={entry.id}>
                      <span className="audit-marker" />
                      <div>
                        <strong>{entry.action}</strong>
                        <p>
                          {entry.actor_id} · {entry.actor_role}
                        </p>
                        <time>
                          {new Date(entry.created_at).toLocaleString("zh-TW")}
                        </time>
                      </div>
                    </article>
                  ))}
                  {summary.data.audit_entries.length === 0 && (
                    <p className="empty-inline">尚無 Audit entry。</p>
                  )}
                </section>
              )}
            </section>
          </div>
        )}
        {resolutionAction && detail.data && (
          <div className="modal-backdrop" onClick={discardResolutionDraft}>
            <section
              ref={closeConfirmationRef}
              className="modal close-confirmation"
              role="dialog"
              aria-modal="true"
              aria-labelledby="case-resolution-dialog-title"
              data-testid={
                resolutionAction === "CLOSE"
                  ? "case-close-confirmation"
                  : "case-resolution-dialog"
              }
              tabIndex={-1}
              onClick={(event) => event.stopPropagation()}
            >
              <div>
                <p className="eyebrow">
                  {resolutionAction === "CLOSE"
                    ? "CONFIRM CASE CLOSURE"
                    : "RESOLVE INVESTIGATION"}
                </p>
                <h3 id="case-resolution-dialog-title">
                  {resolutionAction === "CLOSE"
                    ? "確認結案？"
                    : "記錄調查結論並標記已解決"}
                </h3>
                <p className="muted">
                  {resolutionAction === "CLOSE"
                    ? "結案後不可再修改案件或追加筆記。請確認以下內容已記錄實際調查結果。"
                    : "案件將進入 RESOLVED，之後仍可重新開啟；以下調查結論會立即保存。"}
                </p>
              </div>
              <div className="resolution-fields">
                <label>
                  Root cause
                  <textarea
                    data-dialog-initial-focus
                    data-testid="case-root-cause-input"
                    value={rootCause}
                    onChange={(event) => setRootCause(event.target.value)}
                  />
                </label>
                <label>
                  Resolution summary
                  <textarea
                    data-testid="case-resolution-input"
                    value={resolution}
                    onChange={(event) => setResolution(event.target.value)}
                  />
                </label>
              </div>
              {(rootCause.trim() === "" || resolution.trim() === "") && (
                <p className="field-error" role="alert">
                  Root cause 與 Resolution summary 皆為必填。
                </p>
              )}
              {(update.isError || close.isError) && (
                <p className="field-error" role="alert">
                  {investigationActionError(
                    (update.error || close.error) as Error,
                  )}
                </p>
              )}
              <div className="modal-actions">
                <button
                  type="button"
                  className="button-secondary"
                  data-testid="case-close-cancel"
                  onClick={discardResolutionDraft}
                >
                  取消
                </button>
                <button
                  type="button"
                  data-testid={
                    resolutionAction === "CLOSE"
                      ? "case-close-confirm"
                      : "case-resolve-confirm"
                  }
                  disabled={
                    update.isPending ||
                    close.isPending ||
                    rootCause.trim() === "" ||
                    resolution.trim() === ""
                  }
                  onClick={submitResolution}
                >
                  {resolutionAction === "CLOSE"
                    ? "確認結案"
                    : "保存並標記已解決"}
                </button>
              </div>
            </section>
          </div>
        )}
        <button className="back-link" onClick={() => navigate("/event-check")}>
          ← 回到 Event Check
        </button>
      </main>
    </Shell>
  );
}

export function PatternsPage({ principal }: { principal: Principal }) {
  const location = useLocation();
  const navigate = useNavigate();
  const [patternQuery, setPatternQuery] = useState("");
  const [severityFilter, setSeverityFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [eventTypeFilter, setEventTypeFilter] = useState("");
  const [sortBy, setSortBy] = useState("name");
  const patterns = useQuery({
    queryKey: ["patterns"],
    queryFn: api.patterns,
  });
  const effectiveness = useQuery({
    queryKey: ["pattern-effectiveness"],
    queryFn: api.patternEffectiveness,
  });
  const items = useMemo(() => patterns.data ?? [], [patterns.data]);
  const metricsByPattern = useMemo(
    () =>
      new Map(
        (effectiveness.data?.items ?? []).map((item) => [
          item.pattern_id,
          item,
        ]),
      ),
    [effectiveness.data?.items],
  );
  const selectedPatternID = new URLSearchParams(location.search).get(
    "pattern_id",
  );
  const selectedPattern = items.find((item) => item.id === selectedPatternID);
  const selectedPatternExists = Boolean(selectedPattern);
  const eventTypes = Array.from(
    new Set(
      items.flatMap((pattern) => [
        ...pattern.required_event_types,
        ...pattern.expected_event_types,
        ...pattern.exclusion_event_types,
      ]),
    ),
  ).sort();
  const filteredItems = useMemo(
    () =>
      items
        .filter((pattern) => {
          const normalizedQuery = patternQuery.trim().toLowerCase();
          return (
            (!normalizedQuery ||
              pattern.id.toLowerCase().includes(normalizedQuery) ||
              pattern.name.toLowerCase().includes(normalizedQuery) ||
              pattern.condition.toLowerCase().includes(normalizedQuery)) &&
            (!severityFilter || pattern.severity === severityFilter) &&
            (!statusFilter || pattern.status === statusFilter) &&
            (!eventTypeFilter ||
              [
                ...pattern.required_event_types,
                ...pattern.expected_event_types,
                ...pattern.exclusion_event_types,
              ].includes(eventTypeFilter))
          );
        })
        .sort((left, right) => {
          if (sortBy === "hits") {
            return (
              (metricsByPattern.get(right.id)?.hit_count ?? 0) -
              (metricsByPattern.get(left.id)?.hit_count ?? 0)
            );
          }
          if (sortBy === "false-positive") {
            return (
              (metricsByPattern.get(right.id)?.false_positive_rate ?? -1) -
              (metricsByPattern.get(left.id)?.false_positive_rate ?? -1)
            );
          }
          return left.id.localeCompare(right.id);
        }),
    [
      eventTypeFilter,
      items,
      metricsByPattern,
      patternQuery,
      severityFilter,
      sortBy,
      statusFilter,
    ],
  );
  const closePattern = () => {
    const query = new URLSearchParams(location.search);
    query.delete("pattern_id");
    navigate(`${location.pathname}${query.size ? `?${query.toString()}` : ""}`);
  };
  const patternDrawerRef = useDialogFocus<HTMLElement>(
    Boolean(selectedPatternID),
    closePattern,
  );
  const openPattern = (pattern: PatternDefinition) => {
    const query = new URLSearchParams(location.search);
    query.set("pattern_id", pattern.id);
    navigate(
      `${location.pathname}?${query.toString()}#pattern-${encodeURIComponent(pattern.id)}`,
    );
  };
  return (
    <Shell principal={principal}>
      <main className="page" data-testid="pattern-library">
        <div className="page-heading pattern-page-heading">
          <div>
            <p className="eyebrow">DETERMINISTIC DOMAIN RULES</p>
            <h1>Pattern Library</h1>
            <p className="muted">
              固定、唯讀、可測試的 Domain Pattern；不提供線上編輯或 LLM
              自動推理。
            </p>
          </div>
          <div className="code-managed-badge">
            <span>⌘</span>
            <div>
              <small>MANAGEMENT</small>
              <strong>Code + CI</strong>
            </div>
          </div>
        </div>
        <section className="card pattern-registry">
          <div className="card-heading">
            <div>
              <p className="eyebrow">REGISTERED PATTERNS</p>
              <h3>固定 Pattern</h3>
            </div>
            <span className="badge" data-testid="pattern-active-count">
              {items.filter((item) => item.status === "ACTIVE").length} active ·
              read-only
            </span>
          </div>
          {patterns.isLoading && (
            <p className="muted">載入 Pattern Registry…</p>
          )}
          {patterns.isError && (
            <p className="field-error" data-testid="pattern-library-error">
              {userFacingError(patterns.error, "Pattern Registry 載入失敗。")}
            </p>
          )}
          {effectiveness.isError && (
            <p
              className="field-error"
              data-testid="pattern-effectiveness-error"
            >
              {userFacingError(effectiveness.error, "Pattern 成效資料不可用。")}
            </p>
          )}
          {effectiveness.data && (
            <p className="muted" data-testid="pattern-effectiveness-window">
              成效窗口：
              {new Date(effectiveness.data.window.from).toLocaleDateString(
                "zh-TW",
              )}
              ～
              {new Date(effectiveness.data.window.to).toLocaleDateString(
                "zh-TW",
              )}
            </p>
          )}
          <div className="pattern-filters" data-testid="pattern-filters">
            <label>
              搜尋 Pattern
              <input
                value={patternQuery}
                onChange={(event) => setPatternQuery(event.target.value)}
                placeholder="ID、名稱或條件"
                autoComplete="off"
              />
            </label>
            <label>
              Severity
              <select
                value={severityFilter}
                onChange={(event) => setSeverityFilter(event.target.value)}
              >
                <option value="">全部</option>
                {["LOW", "MEDIUM", "HIGH", "CRITICAL"].map((value) => (
                  <option key={value}>{value}</option>
                ))}
              </select>
            </label>
            <label>
              Status
              <select
                value={statusFilter}
                onChange={(event) => setStatusFilter(event.target.value)}
              >
                <option value="">全部</option>
                {["DRAFT", "ACTIVE", "DEPRECATED"].map((value) => (
                  <option key={value}>{value}</option>
                ))}
              </select>
            </label>
            <label>
              Event type
              <select
                value={eventTypeFilter}
                onChange={(event) => setEventTypeFilter(event.target.value)}
              >
                <option value="">全部</option>
                {eventTypes.map((value) => (
                  <option key={value}>{value}</option>
                ))}
              </select>
            </label>
            <label>
              排序
              <select
                value={sortBy}
                onChange={(event) => setSortBy(event.target.value)}
              >
                <option value="name">Pattern ID</option>
                <option value="hits">命中數</option>
                <option value="false-positive">False-positive rate</option>
              </select>
            </label>
          </div>
          {!patterns.isLoading &&
            selectedPatternID &&
            !selectedPatternExists && (
              <p
                className="field-error"
                data-testid="selected-pattern-not-found"
              >
                找不到指定的 Pattern：{selectedPatternID}
              </p>
            )}
          <div className="pattern-table">
            <div className="pattern-table-head">
              <span>Pattern</span>
              <span>命中</span>
              <span>案件</span>
              <span>人工判定</span>
              <span>False positive</span>
              <span>Severity</span>
              <span>狀態</span>
            </div>
            {filteredItems.map((pattern, index) => {
              const metric = metricsByPattern.get(pattern.id);
              const metricsUnavailable =
                effectiveness.isError ||
                (!effectiveness.isLoading && metric === undefined);
              return (
                <button
                  type="button"
                  id={`pattern-${pattern.id}`}
                  className={`pattern-table-row${pattern.id === selectedPatternID ? " pattern-selected" : ""}`}
                  data-selected={
                    pattern.id === selectedPatternID ? "true" : "false"
                  }
                  data-testid={`pattern-row-${index}`}
                  key={`${pattern.id}:v${pattern.version}`}
                  onClick={() => openPattern(pattern)}
                >
                  <div className="pattern-name" data-label="Pattern">
                    <strong data-testid={`pattern-${index}-id`}>
                      {pattern.id}
                    </strong>
                    <small>
                      {pattern.name} · v{pattern.version} · {pattern.window}
                    </small>
                    <small data-testid={`pattern-${index}-fixture-coverage`}>
                      fixtures: {pattern.fixture_coverage.match_count} match /{" "}
                      {pattern.fixture_coverage.non_match_count} non-match
                    </small>
                  </div>
                  <strong
                    data-testid={`pattern-${index}-hit-count`}
                    data-label="命中"
                  >
                    {effectiveness.isLoading
                      ? "載入中"
                      : metricsUnavailable
                        ? "不可用"
                        : metric!.hit_count.toLocaleString()}
                  </strong>
                  <strong
                    data-testid={`pattern-${index}-case-count`}
                    data-label="案件"
                  >
                    {effectiveness.isLoading
                      ? "載入中"
                      : metricsUnavailable
                        ? "不可用"
                        : metric!.investigation_count.toLocaleString()}
                  </strong>
                  <span
                    data-testid={`pattern-${index}-review-count`}
                    data-label="人工判定"
                  >
                    {effectiveness.isLoading
                      ? "載入中"
                      : metricsUnavailable
                        ? "不可用"
                        : `${metric!.reviewed_count} reviewed / ${metric!.unreviewed_count} unreviewed`}
                  </span>
                  <span
                    data-testid={`pattern-${index}-false-positive-rate`}
                    data-label="False positive"
                  >
                    {effectiveness.isLoading
                      ? "載入中"
                      : metricsUnavailable
                        ? "不可用"
                        : metric!.false_positive_rate === null
                          ? "尚無判定"
                          : `${(metric!.false_positive_rate * 100).toFixed(1)}%`}
                  </span>
                  <span
                    className={`severity severity-${pattern.severity.toLowerCase()}`}
                    data-label="Severity"
                  >
                    {pattern.severity}
                  </span>
                  <span
                    className="pattern-status"
                    data-testid={`pattern-${index}-status`}
                    data-label="狀態"
                  >
                    {pattern.status}
                  </span>
                </button>
              );
            })}
          </div>
          {!patterns.isLoading &&
            !patterns.isError &&
            filteredItems.length === 0 && (
              <p className="empty-inline">目前條件沒有 Pattern。</p>
            )}
        </section>
        <section className="card pattern-boundary">
          <div className="boundary-icon">✓</div>
          <div>
            <p className="eyebrow">EXECUTION BOUNDARY</p>
            <h3>唯讀調查，不修改正式系統</h3>
            <p className="muted">
              Pattern 只讀取 ClickHouse 事件與技術觀測參照，產生 Finding 與
              Evidence reference；不修改正式訂單、付款、庫存，也不執行 Replay。
            </p>
          </div>
        </section>
        {selectedPatternID && (
          <div className="drawer-backdrop" onClick={closePattern}>
            <section
              ref={patternDrawerRef}
              className="case-drawer pattern-drawer"
              role="dialog"
              aria-modal="true"
              aria-labelledby="pattern-detail-title"
              tabIndex={-1}
              onClick={(event) => event.stopPropagation()}
            >
              <div className="drawer-header">
                <div>
                  <p className="eyebrow">PATTERN DETAIL</p>
                  <h3 id="pattern-detail-title">
                    {selectedPattern?.name ?? selectedPatternID}
                  </h3>
                </div>
                <button
                  className="drawer-close"
                  data-dialog-initial-focus
                  data-testid="pattern-detail-close"
                  aria-label="關閉 Pattern 詳細"
                  onClick={closePattern}
                >
                  ×
                </button>
              </div>
              {!selectedPattern && (
                <p className="field-error">
                  找不到指定的 Pattern：{selectedPatternID}
                </p>
              )}
              {selectedPattern &&
                (() => {
                  const metric = metricsByPattern.get(selectedPattern.id);
                  const window = dynamicQueryWindow();
                  const timeline = new URLSearchParams({
                    from: window.from,
                    to: window.to,
                    include_processing_attempts: "true",
                  });
                  if (selectedPattern.required_event_types[0]) {
                    timeline.set(
                      "event_type",
                      selectedPattern.required_event_types[0],
                    );
                  }
                  return (
                    <div className="pattern-detail-content">
                      <div className="pattern-detail-badges">
                        <span
                          className={`severity severity-${selectedPattern.severity.toLowerCase()}`}
                        >
                          {selectedPattern.severity}
                        </span>
                        <span className="pattern-status">
                          {selectedPattern.status}
                        </span>
                        <span className="badge">
                          v{selectedPattern.version}
                        </span>
                      </div>
                      <section>
                        <h4>規則條件</h4>
                        <p>{selectedPattern.condition}</p>
                        <dl className="pattern-rule-list">
                          <div>
                            <dt>Requires</dt>
                            <dd>
                              {selectedPattern.required_event_types.join(
                                ", ",
                              ) || "—"}
                            </dd>
                          </div>
                          <div>
                            <dt>Expects</dt>
                            <dd>
                              {selectedPattern.expected_event_types.join(
                                ", ",
                              ) || "—"}
                            </dd>
                          </div>
                          <div>
                            <dt>Excludes</dt>
                            <dd>
                              {selectedPattern.exclusion_event_types.join(
                                ", ",
                              ) || "—"}
                            </dd>
                          </div>
                          <div>
                            <dt>Window</dt>
                            <dd>{selectedPattern.window}</dd>
                          </div>
                        </dl>
                      </section>
                      <section>
                        <h4>近 30 天成效</h4>
                        {metric ? (
                          <dl className="pattern-effectiveness-detail">
                            <div>
                              <dt>Hits</dt>
                              <dd>{metric.hit_count}</dd>
                            </div>
                            <div>
                              <dt>Cases</dt>
                              <dd>{metric.investigation_count}</dd>
                            </div>
                            <div>
                              <dt>Confirmed</dt>
                              <dd>{metric.confirmed_count}</dd>
                            </div>
                            <div>
                              <dt>False positive</dt>
                              <dd>{metric.false_positive_count}</dd>
                            </div>
                            <div>
                              <dt>Needs review</dt>
                              <dd>{metric.needs_review_count}</dd>
                            </div>
                            <div>
                              <dt>Unreviewed</dt>
                              <dd>{metric.unreviewed_count}</dd>
                            </div>
                          </dl>
                        ) : (
                          <p className="muted">成效資料不可用。</p>
                        )}
                      </section>
                      <section>
                        <h4>治理與驗證</h4>
                        <p>
                          <strong>Evidence query：</strong>
                          <code>
                            {selectedPattern.evidence_query_template_id}
                          </code>
                        </p>
                        <p>
                          <strong>Source：</strong>
                          <code>{selectedPattern.source_path}</code>
                        </p>
                        <p title={selectedPattern.checksum}>
                          <strong>SHA-256：</strong>
                          <code>{selectedPattern.checksum}</code>
                        </p>
                        <p>
                          <strong>Fixtures：</strong>
                          {selectedPattern.fixture_coverage.match_count} match /{" "}
                          {selectedPattern.fixture_coverage.non_match_count}{" "}
                          non-match
                        </p>
                      </section>
                      <div className="pattern-detail-actions">
                        <a
                          href={`/event-check?${new URLSearchParams({
                            identifier_type: "EVENT_ID",
                            identifier: timeline.get("event_id") ?? "",
                            from: timeline.get("from") ?? "",
                            to: timeline.get("to") ?? "",
                            tab: "timeline",
                          }).toString()}`}
                        >
                          用主要事件執行 Event Check →
                        </a>
                        <a href="/investigations">查看 Investigation Cases →</a>
                      </div>
                    </div>
                  );
                })()}
            </section>
          </div>
        )}
      </main>
    </Shell>
  );
}

function ScenarioRunModal({
  run,
  pending,
  error,
  onClose,
}: {
  run: ScenarioRun | null;
  pending: boolean;
  error: Error | null;
  onClose: () => void;
}) {
  const [copiedIdentifier, setCopiedIdentifier] = useState<string | null>(null);
  const dialogRef = useDialogFocus<HTMLElement>(true, onClose);
  const status = error ? "START_FAILED" : (run?.status ?? "STARTING");
  const terminal = ["PASSED", "FAILED", "TIMED_OUT"].includes(status);

  const copyIdentifier = async (label: string, value: string) => {
    try {
      await navigator.clipboard.writeText(value);
      setCopiedIdentifier(label);
    } catch {
      setCopiedIdentifier(null);
    }
  };

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <section
        ref={dialogRef}
        className="modal scenario-run-modal"
        data-testid="scenario-run-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="scenario-run-modal-title"
        tabIndex={-1}
        onClick={(event) => event.stopPropagation()}
      >
        <div className="scenario-run-modal-header">
          <div>
            <p className="eyebrow">CURRENT SCENARIO RUN</p>
            <h3 id="scenario-run-modal-title">
              {run
                ? `${run.scenario.id} · ${run.scenario.title}`
                : "正在啟動 Scenario"}
            </h3>
          </div>
          <button
            type="button"
            className="drawer-close"
            data-dialog-initial-focus
            aria-label="關閉 Scenario 執行資訊"
            onClick={onClose}
          >
            ×
          </button>
        </div>

        <div className="scenario-run-modal-status">
          <span
            className={`scenario-run-status status-${status.toLowerCase()}`}
            data-testid="scenario-run-status"
            role="status"
            aria-live="polite"
          >
            {status}
          </span>
          <span className="muted">
            {pending && !run
              ? "正在取得 Run ID 與 Correlation ID…"
              : run
                ? "情境已接受；可使用 Correlation ID 到其他功能查詢後續結果。"
                : "Scenario 無法啟動。"}
          </span>
        </div>

        {error && <p className="field-error">無法啟動：{error.message}</p>}

        {run && (
          <>
            <dl className="scenario-identifier-list">
              <div>
                <dt>Run ID</dt>
                <dd>
                  <code data-testid="scenario-run-id">{run.run_id}</code>
                  <button
                    type="button"
                    className="button-secondary"
                    onClick={() => void copyIdentifier("run", run.run_id)}
                  >
                    {copiedIdentifier === "run" ? "已複製" : "複製"}
                  </button>
                </dd>
              </div>
              <div>
                <dt>Correlation ID</dt>
                <dd>
                  <code data-testid="scenario-correlation-id">
                    {run.correlation_id}
                  </code>
                  <button
                    type="button"
                    className="button-secondary"
                    onClick={() =>
                      void copyIdentifier("correlation", run.correlation_id)
                    }
                  >
                    {copiedIdentifier === "correlation" ? "已複製" : "複製"}
                  </button>
                </dd>
              </div>
              {terminal && run.trace_id && (
                <div>
                  <dt>Trace ID</dt>
                  <dd>
                    <code data-testid="scenario-trace-id">{run.trace_id}</code>
                    <button
                      type="button"
                      className="button-secondary"
                      onClick={() =>
                        void copyIdentifier("trace", run.trace_id!)
                      }
                    >
                      {copiedIdentifier === "trace" ? "已複製" : "複製"}
                    </button>
                  </dd>
                </div>
              )}
            </dl>

            {terminal && (
              <section
                className="scenario-completed-summary"
                data-testid="scenario-result"
              >
                <div>
                  <strong>{run.current_step}</strong>
                  <span>
                    {run.duration_ms === null
                      ? "Duration unavailable"
                      : `${run.duration_ms} ms`}
                  </span>
                </div>
                <div
                  className="scenario-events"
                  data-testid="scenario-actual-events"
                >
                  {run.actual.event_types.map((eventType) => (
                    <code key={eventType}>{eventType}</code>
                  ))}
                  {run.actual.event_types.length === 0 && (
                    <span className="muted">沒有 admitted domain events</span>
                  )}
                </div>
                {run.checks.map((check) => (
                  <p key={check.id}>
                    <strong>{check.passed ? "PASS" : "FAIL"}</strong> ·{" "}
                    {check.label}
                  </p>
                ))}
              </section>
            )}

            <div className="scenario-links">
              <a data-testid="scenario-link-timeline" href={run.links.timeline}>
                Open Event Check →
              </a>
              <a
                data-testid="scenario-link-loki"
                href={run.links.loki}
                target="_blank"
                rel="noreferrer"
              >
                Open Logs ↗
              </a>
              {terminal && (
                <a
                  data-testid="scenario-link-grafana"
                  href={run.links.grafana}
                  target="_blank"
                  rel="noreferrer"
                >
                  Open Grafana ↗
                </a>
              )}
              {terminal && run.links.tempo && (
                <a
                  data-testid="scenario-link-tempo"
                  href={run.links.tempo}
                  target="_blank"
                  rel="noreferrer"
                >
                  Open Trace ↗
                </a>
              )}
            </div>
          </>
        )}
      </section>
    </div>
  );
}

export function ScenarioLabPage({ principal }: { principal: Principal }) {
  const scenarioQueryClient = useQueryClient();
  const [acceptedRun, setAcceptedRun] = useState<ScenarioRun | null>(null);
  const [showRunModal, setShowRunModal] = useState(false);
  const [historyStatus, setHistoryStatus] = useState<
    NonNullable<ScenarioRunFilters["status"]> | ""
  >("");
  const [historyMode, setHistoryMode] = useState<
    NonNullable<ScenarioRunFilters["execution_mode"]> | ""
  >("");
  const [historyScenario, setHistoryScenario] = useState("");
  const catalog = useQuery({
    queryKey: ["scenario-catalog"],
    queryFn: scenarioApi.catalog,
  });
  const start = useMutation({
    mutationFn: scenarioApi.start,
    onMutate: () => {
      setAcceptedRun(null);
      setShowRunModal(true);
    },
    onSuccess: (run) => {
      setAcceptedRun(run);
      scenarioQueryClient.setQueriesData<ScenarioRunPage>(
        { queryKey: ["scenario-history"] },
        (previous) =>
          previous
            ? {
                items: [
                  run,
                  ...previous.items.filter(
                    (item) => item.run_id !== run.run_id,
                  ),
                ],
              }
            : previous,
      );
    },
  });
  const items = useMemo(() => catalog.data?.items ?? [], [catalog.data?.items]);
  const historyFilters = {
    status: historyStatus || undefined,
    execution_mode: historyMode || undefined,
    scenario_id: historyScenario || undefined,
    page_size: 20,
  };
  const history = useQuery({
    queryKey: ["scenario-history", historyFilters],
    queryFn: () => scenarioApi.history(historyFilters),
    retry: false,
  });
  const groups = useMemo(
    () =>
      ["LIVE_SERVICES", "LAB_INJECTION"].map((mode) => ({
        mode,
        items: items.filter((scenario) => scenario.execution_mode === mode),
      })),
    [items],
  );
  const canRun = principal.role !== "VIEWER";

  return (
    <Shell principal={principal}>
      <main className="page" data-testid="scenario-lab">
        <div className="page-heading scenario-heading">
          <div>
            <p className="eyebrow">DETERMINISTIC PIPELINE EXERCISES</p>
            <h1>Scenario Lab</h1>
            <p className="muted">
              劇本與預期固定；actual 與 PASS／FAIL 來自後端實際執行及資料回查。
            </p>
          </div>
          <div className="scenario-mode-legend">
            <span>LIVE_SERVICES：真實三服務鏈</span>
            <span>LAB_INJECTION：隔離 synthetic topic</span>
          </div>
        </div>

        {catalog.isLoading && <p className="muted">載入 Scenario catalog…</p>}
        {catalog.isError && (
          <p className="field-error" data-testid="scenario-catalog-error">
            {userFacingError(catalog.error, "Scenario Lab 尚未就緒。")}
          </p>
        )}
        <section
          className="scenario-catalog-groups"
          data-testid="scenario-catalog"
        >
          {groups.map((group) => (
            <section className="scenario-catalog-group" key={group.mode}>
              <div className="card-heading">
                <div>
                  <p className="eyebrow">{group.mode}</p>
                  <h3>
                    {group.mode === "LIVE_SERVICES"
                      ? "真實服務鏈情境"
                      : "隔離事件注入情境"}
                  </h3>
                </div>
                <span className="badge">{group.items.length} scenarios</span>
              </div>
              <div className="scenario-grid">
                {group.items.map((scenario) => (
                  <article className="scenario-card" key={scenario.id}>
                    <div className="scenario-card-heading">
                      <span className="scenario-id">{scenario.id}</span>
                      <span
                        className={`scenario-mode ${scenario.synthetic ? "synthetic" : "live"}`}
                      >
                        {scenario.execution_mode}
                      </span>
                    </div>
                    <p className="eyebrow">{scenario.category}</p>
                    <h3>{scenario.title}</h3>
                    <p className="muted">{scenario.description}</p>
                    <div className="scenario-events">
                      {scenario.expected_event_types.length > 0 ? (
                        scenario.expected_event_types.map((eventType) => (
                          <code key={eventType}>{eventType}</code>
                        ))
                      ) : (
                        <code>Invalid PaymentCompleted → ingestion DLQ</code>
                      )}
                    </div>
                    <ul>
                      {scenario.expected_results.map((result) => (
                        <li key={result}>{result}</li>
                      ))}
                    </ul>
                    <button
                      data-testid={`run-scenario-${scenario.id.toLowerCase()}`}
                      disabled={
                        !canRun ||
                        (start.isPending && start.variables === scenario.id)
                      }
                      title={
                        canRun
                          ? "啟動此固定情境"
                          : "VIEWER 為唯讀角色，不能啟動情境"
                      }
                      onClick={() => start.mutate(scenario.id)}
                    >
                      {start.isPending && start.variables === scenario.id
                        ? "啟動中…"
                        : "執行情境"}
                    </button>
                  </article>
                ))}
              </div>
            </section>
          ))}
        </section>

        <section
          className="card scenario-history"
          data-testid="scenario-history"
        >
          <div className="card-heading">
            <div>
              <p className="eyebrow">RECENT RUNS</p>
              <h3>近期執行歷程</h3>
              <p className="muted">
                顯示後端已保存的實際狀態；此區不會定時輪詢或補查 trace ID。
              </p>
            </div>
            <button
              type="button"
              className="button-secondary"
              data-testid="scenario-history-refresh"
              onClick={() => void history.refetch()}
              disabled={history.isFetching}
            >
              {history.isFetching ? "更新中…" : "手動更新"}
            </button>
          </div>
          <div className="scenario-history-filters">
            <label>
              Scenario
              <select
                data-testid="scenario-history-scenario-filter"
                value={historyScenario}
                onChange={(event) => setHistoryScenario(event.target.value)}
              >
                <option value="">全部</option>
                {items.map((scenario) => (
                  <option value={scenario.id} key={scenario.id}>
                    {scenario.id} · {scenario.title}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Status
              <select
                data-testid="scenario-history-status-filter"
                value={historyStatus}
                onChange={(event) =>
                  setHistoryStatus(event.target.value as typeof historyStatus)
                }
              >
                <option value="">全部</option>
                {["ACCEPTED", "RUNNING", "PASSED", "FAILED", "TIMED_OUT"].map(
                  (status) => (
                    <option key={status}>{status}</option>
                  ),
                )}
              </select>
            </label>
            <label>
              Mode
              <select
                data-testid="scenario-history-mode-filter"
                value={historyMode}
                onChange={(event) =>
                  setHistoryMode(event.target.value as typeof historyMode)
                }
              >
                <option value="">全部</option>
                <option>LIVE_SERVICES</option>
                <option>LAB_INJECTION</option>
              </select>
            </label>
          </div>
          {history.isError && (
            <p className="field-error">
              {userFacingError(history.error, "執行歷程載入失敗。")}
            </p>
          )}
          <div className="scenario-history-list">
            {(history.data?.items ?? []).map((run) => (
              <button
                type="button"
                data-testid={`scenario-history-run-${run.run_id}`}
                key={run.run_id}
                onClick={() => {
                  setAcceptedRun(run);
                  setShowRunModal(true);
                }}
              >
                <span className="scenario-id">{run.scenario.id}</span>
                <span>
                  <strong>{run.scenario.title}</strong>
                  <small>{run.correlation_id}</small>
                </span>
                <span>{run.current_step}</span>
                <span
                  className={`scenario-run-status status-${run.status.toLowerCase()}`}
                >
                  {run.status}
                </span>
                <time dateTime={run.accepted_at}>
                  {new Date(run.accepted_at).toLocaleString("zh-TW")}
                </time>
              </button>
            ))}
            {!history.isLoading &&
              !history.isError &&
              (history.data?.items.length ?? 0) === 0 && (
                <p className="empty-inline">目前條件沒有執行紀錄。</p>
              )}
          </div>
        </section>

        {start.isError && (
          <p className="field-error" data-testid="scenario-start-error">
            {userFacingError(start.error, "無法啟動 Scenario。")}
          </p>
        )}
        {showRunModal && (
          <ScenarioRunModal
            run={acceptedRun}
            pending={start.isPending}
            error={start.isError ? (start.error as Error) : null}
            onClose={() => setShowRunModal(false)}
          />
        )}
      </main>
    </Shell>
  );
}

function App() {
  return <Login />;
}

function Bootstrap() {
  const me = useQuery({ queryKey: ["me"], queryFn: api.me, retry: false });
  if (me.isLoading) return <div className="loading">Loading Event Hunter…</div>;
  return <AppWithPrincipal principal={me.data ?? null} />;
}
function AppWithPrincipal({ principal }: { principal: Principal | null }) {
  const [current] = useState(principal);
  const location = useLocation();
  if (!current) return <App />;
  const legacyRoute = resolveLegacyRoute(location.pathname, location.search);
  if (legacyRoute?.kind === "REDIRECT")
    return <Navigate replace to={legacyRoute.to} />;
  if (location.pathname === "/event-check/saved-results")
    return <SavedCheckResultsPage principal={current} />;
  if (location.pathname === "/event-check")
    return <EventCheckPage principal={current} />;
  if (location.pathname === "/check-models")
    return <CheckModelsPage principal={current} />;
  if (location.pathname === "/dashboard")
    return <DashboardPage principal={current} />;
  if (location.pathname === "/guide")
    return <FeatureGuidePage principal={current} />;
  if (location.pathname === "/journey")
    return <BusinessJourneyPage principal={current} />;
  if (location.pathname === "/journey-profiles")
    return <JourneyProfilesPage principal={current} />;
  if (location.pathname === "/ingestion-issues")
    return <IngestionIssuesPage principal={current} />;
  if (
    location.pathname === "/investigations" ||
    /^\/investigations\/[^/]+$/.test(location.pathname)
  )
    return <InvestigationsPage principal={current} />;
  if (location.pathname === "/patterns")
    return <PatternsPage principal={current} />;
  if (location.pathname === "/scenario-lab")
    return <ScenarioLabPage principal={current} />;
  if (location.pathname === "/timeline")
    return <TimelinePage principal={current} />;
  return <Navigate replace to="/event-check" />;
}

const rootElement = document.getElementById("root");
if (rootElement)
  createRoot(rootElement).render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <Bootstrap />
        </BrowserRouter>
      </QueryClientProvider>
    </StrictMode>,
  );
