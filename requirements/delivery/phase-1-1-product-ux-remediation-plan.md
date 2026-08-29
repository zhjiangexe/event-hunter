---
document_id: EH-DOC-DEL-003
status: completed
owner: frontend
last_reviewed: 2026-08-29
source_of_truth: false
canonical_topic: phase-1-1-ux-remediation
supersedes: []
---

# Event Hunter Phase 1.1 產品 UX 與資訊可信度修正計畫

> **Completed historical plan.** 本文件保留改版前 Timeline／Journey／Pattern Library 的問題、修復順序與
> 驗收證據。改版後 canonical 資訊架構為 Event Check／Saved Results／Check Models／Investigation Cases／
> Ingestion Issues／Scenario Lab；現況需求與 UI 不應從本計畫的舊頁名反推。

## 1. 文件狀態

- 狀態：`completed`（2026-08-28；`P1.1-UX-01`～`P1.1-UX-13` 的核准範圍均已實作，
  本機 component／contract／backend E2E 與 390px browser regression 為封板依據）
- 建立日期：`2026-08-26`
- 來源：Phase 1.1 全功能頁桌面版、`390 × 844` 行動版實機檢查，以及 frontend／backend contract 比對。
- 目的：修正目前已交付功能中會讓使用者查不到資料、誤判狀態、執行錯誤操作或無法在行動版完成工作的缺口。
- 與既有 sign-off 的關係：`phase-1-1-sign-off.md` 仍代表 `2026-08-24` 本機 release baseline 已通過；
  本文件是後續產品化 remediation，不回溯否定既有測試與簽核。
- 實作規則：本文件先記錄候選工作與順序。開始實作新增能力前，必須核准 scope，配置穩定
  `REQ-EH-*`／工程任務 ID，並同步更新 `traceability.yaml`、`implementation-plan.yaml`、OpenAPI、migration
  與 Karate acceptance feature。

## 2. 稽核結論

目前核心調查路徑已存在：

```text
Smart Search
├── Business Timeline ── Event detail / Observability links
├── Business Journey ─── Milestone / Anomaly
└── Investigation Case ─ Pattern / Evidence / Audit
```

主要問題不是缺少更多大型模組，而是既有頁面尚未完整回答人在調查時真正需要的五個問題：

1. 我現在看到的是哪一段時間，而且資料是否完整？
2. 接下來最合理的操作是什麼，操作後會發生什麼？
3. 這個判斷來自哪些事件、Trace、Pattern 或 Evidence？
4. 我能否保存、分享並在重新整理後重現同一個調查狀態？
5. 在桌面或行動裝置上，我是否都能完成相同核心工作？

因此修正優先級採以下原則：

1. 先修會造成錯誤結論或資料消失假象的時間與可信度問題。
2. 再修會造成錯誤 mutation 或工作流中斷的操作問題。
3. 再補 Journey、Pattern、Case、Scenario 的判讀與 drill-down。
4. 最後統一全站可存取性、語言、錯誤與回饋模式。

## 3. 已確認的實機證據

`390 × 844` viewport 的檢查結果：

| 項目 | 實際結果 | 風險 |
|---|---|---|
| 全域導覽 | 可視寬度約 `335px`，導覽內容約 `1157px` | 當前頁可能在畫面外；使用者無法快速知道所在位置 |
| 行動版身分區 | `.identity` 為 `display: none` | 角色與 Sign out 完全不可用 |
| Business Journey | document client width 約 `375px`，內容 scroll width 約 `471px` | 里程碑右側被 `overflow-x: clip` 裁切 |
| Investigation Cases | 約 `277px` 容器內放置 `1065px` 表格 | 必須水平掃描八欄，無法快速分流案件 |
| Pattern Library | 約 `277px` 容器內放置 `1352px` 表格 | 規則條件與成效無法同時閱讀 |
| Scenario Lab | catalog 頁面高度約 `6255px` | 從前段 Scenario 執行後，結果出現在所有卡片之後 |

另確認以下前後端落差：

- Timeline／Journey 空白頁使用固定 `2026-08-20` fixture 時間。（已由 `EH-P1.1-013` 修正）
- Investigation Summary 修正前在沒有 URL window 時使用 rolling now，而不是案件發生時間。（已由
  `EH-P1.1-014` 改為不可變 Incident Window；新手動案件未指定窗口時的建立預設目前為最近 72 小時。）
- Pattern Analysis 只可由 Timeline 剛建立案件後的 inline 卡片啟動；既有案件 drawer 沒有入口。
- Frontend 固定只執行 `payment-completed-without-shipment`，未使用後端「執行所有適用 Pattern」能力。
- 後端支援 `WAITING_APPROVAL`，前端沒有可選操作；「更新狀態」在執行前也不揭露 target status。
- Journey response 已有 anomaly `event_ids` 與 event `trace_id`，前端未呈現可操作連結或 Trace 摘要。
- Source Health response 已有 `last_success_at`，Overview 未顯示。

## 4. Wave UX-A：立即修正可信度與核心操作

### P1.1-UX-01：動態查詢時間與可重現 URL

**狀態**：`completed`（`2026-08-26`，工程任務 `EH-P1.1-013`）

**目標**：避免使用者因固定 demo 日期查不到資料，並讓同一查詢能分享、重新整理與使用瀏覽器歷史重現。

交付：

- Timeline 與 Journey 空白頁預設為 server/client 現在時間往前 72 小時，不保留 fixture 日期常數。
- 清除條件後回到動態預設時間。
- Timeline 搜尋成功後將完整 allowlisted filters、`from`、`to`、payload／processing flags 寫入 URL。
- Back／forward／refresh 必須還原表單與結果；Saved Search 仍由 typed query state 產生 URL。
- Scenario、Overview、Journey、Pattern、Investigation deep link 必須保留明確 bounded window。
- 時間欄位附近顯示 timezone，結果區顯示實際查詢窗口。

驗收：

- 使用可控 clock 的 frontend tests 證明預設時間不是固定 fixture。
- Browser E2E 驗證 search → URL → reload → back/forward 完整還原。
- Backend 仍拒絕無界、反向或超過七天的時間窗。

完成證據：

- Frontend lint、typecheck、production build 與 `74/74` Vitest／RTL 通過。
- `@eh-p1-1-013` Frontend Karate `2/2`、完整 Frontend Karate `25/25` 通過。
- 相關 Overview／Saved Search Backend Karate `9/9` 通過；既有 Backend Timeline Karate 包含無界與超過七天的拒絕案例。
- 已提交查詢由 URL 擁有；React 表單以 route key 還原草稿，不使用同步 setState effect 複製 URL state。

### P1.1-UX-02：案件 Incident Window 與 partial summary

**狀態**：`completed`（`2026-08-26`，工程任務 `EH-P1.1-014`）

**目標**：案件隔天或重啟後仍能看到當初事件，不因 rolling window 產生「證據消失」假象。

Contract-first 決策：

- Investigation 增加可追溯的 `incident_from`／`incident_to`，或等價的 immutable investigation query window。
- Timeline 手動建案保存當次 bounded window。
- Grafana 自動建案由 alert 發生時間與規則 window 推導，並保存推導來源。
- 舊案件必須定義 deterministic fallback；不得每次以目前時間重新計算。
- 使用者臨時改看其他窗口時，UI 必須區分「案件基準窗口」與「目前檢視窗口」。

交付：

- Case Summary、Evidence 與回到 Timeline 的入口預設共用案件基準窗口。
- Drawer 頂端顯示窗口、來源與調整操作。
- ClickHouse 不可用時仍回傳 PostgreSQL case data；Timeline／technical sections 標示 partial／unavailable，
  不把整個 Summary 直接變成無法使用。
- Summary 顯示 generated time、source last success、truncation 與 retention boundary。
- Pattern Analysis 保留 `EARLIEST_CORRELATION_EVENT` deterministic window；`P1.1-UX-03` 已把該窗口與
  最近執行結果清楚放入同一案件 workspace，但不把 Pattern 的七天證據安全邊界誤稱為 Incident Window。

驗收：

- 新舊案件、延後開啟、restart、ClickHouse unavailable、retention boundary 均有 backend 與 browser E2E。
- 同一案件未改變基準窗口時，Summary、Evidence 與 Timeline deep link 使用相同事件範圍。

完成證據：

- `00008_investigation_incident_window.sql` 可重跑且第二次 backfill 為 `UPDATE 0`；真實 PostgreSQL／API
  restart 後，案件 baseline、source、Summary query window 與 event count 完全一致。
- ClickHouse 實際停止時，Summary 仍以 HTTP 200 保留 PostgreSQL sections，並明確回傳 partial、
  `UNAVAILABLE`、`source_last_success_at` 與近似 90 天 retention boundary；恢復腳本驗證通過。
- Frontend 以 baseline/current-view 分區避免把 unavailable 誤顯示成 0 events，並可帶原窗口回 Timeline。
- Go vet/tests、contract validation、frontend lint/typecheck/build、Vitest／RTL `66/66` 通過；
  `@eh-p1-1-014` Backend／Frontend 各 `2/2`，完整 Backend `104/104`、Frontend browser `23/23` 通過。
- 詳細不變條件、來源與 partial 語意見 `investigation-incident-window-contract.md`。

### P1.1-UX-03：案件內完整 Pattern Analysis

**狀態**：`completed`（`2026-08-26`，工程任務 `EH-P1.1-015`）

**目標**：任何未結案件都能在正式案件工作區執行、查看與重新執行 deterministic Pattern。

交付：

- Cases drawer 的 Patterns 頁籤提供「執行所有適用 Pattern」。
- 進階操作可選指定 Pattern；預設請求不傳固定 `pattern_ids`。
- 顯示 analysis status、effective window、source event count、執行時間與錯誤原因。
- 執行成功後更新 Summary、Patterns、Evidence、Audit，不要求返回 Timeline inline 卡片。
- Timeline 建案成功後提供「開啟案件」並進入同一套 routeable drawer；逐步移除重複的 inline case workspace。

驗收：

- 既有案件、新建案件、NO_EVENTS、NO_MATCH、MATCH、unknown Pattern 與 source failure 都有 E2E。
- API 不再由 frontend 寫死單一物流 Pattern。

完成證據：

- 未傳 `pattern_ids` 時由後端解析所有 ACTIVE immutable definitions，response 與 Audit 都保存
  `executed_pattern_ids`；frontend 只有使用者開啟進階模式時才傳明確選項。
- 任一未結且可寫案件可直接在 Patterns 頁籤執行；Viewer／CLOSED 唯讀。成功後 Summary、Finding、
  Evidence、Audit 與 detail cache 會一起刷新。
- UI 分別顯示「沒有 source event」、「已評估未命中」、「已命中」與可操作的 source timeout／unavailable／
  persistence／unknown Pattern 錯誤；reload 後由 durable Audit 還原最近分析時間、集合與 effective window。
- Timeline 建案後只保留「開啟案件工作區」handoff，移除第二套 Pattern／Evidence 操作介面。
- Pattern 產生的 durable Evidence 若來自不同 effective window，Timeline 空白與 Evidence 頁籤會明確說明
  它不等同 baseline event count，並可直接用 analysis window 開啟 Business Timeline；歷史 fixture 的
  browser E2E 建案也固定傳入相符 incident window，避免再產生假的矛盾案件。
- Go vet/tests、contract validation、frontend lint/typecheck/build 與 Vitest／RTL `69/69` 通過；完整
  Backend Karate `105/105`（含 CLOSED API 邊界）、Frontend browser `23/23` 通過。
- 真實停止 ClickHouse 時回 `PATTERN_SOURCE_UNAVAILABLE`，未保存假的 Finding／Evidence，恢復後 pipeline
  readiness 為 stable/zero lag。

### P1.1-UX-04：明確案件狀態轉換與結案表單

**狀態**：`completed`（`2026-08-26`，工程任務 `EH-P1.1-016`）

**目標**：使用者執行前就知道 target status，且所有後端允許的流程都有清楚 UI。

交付：

- 以明確動作取代「更新狀態」：開始調查、送交審核、標記已解決、重新開啟、結案。
- Backend response 或專用 contract 提供 `allowed_transitions`，避免 frontend 複製 domain state machine。
- Root cause／Resolution 只在 resolved／close flow 顯示，並明確標示是否已保存。
- 未儲存表單在關閉 drawer、切換案件、重新整理前提示。
- 成功、optimistic-lock conflict、forbidden 與 validation error 使用可操作訊息。

驗收：

- 每個合法／非法 transition、Viewer 權限、必填欄位、conflict 與 audit entry 均有測試。
- `WAITING_APPROVAL` 可從 UI 進入與離開。

完成證據：

- `InvestigationCase.AllowedTransitions()` 是唯一合法目標來源；OpenAPI 的 `allowed_transitions` 與
  `contracts/platform/investigation-state-machine.yaml` 已同步，`CLOSED` 回傳空集合。
- 案件 drawer 不再顯示含糊的「更新狀態」，而是依 API 回傳目標顯示開始調查、送交審核、返回調查、
  標記已解決、重新開啟與結案；前端沒有複製 domain transition map。
- Resolve／Close 使用分離的 modal-only 表單，Root cause 與 Resolution summary 空白時不能送出；成功後
  清除草稿並以 live status 說明已保存。Viewer 與 CLOSED 都不顯示 mutation action。
- 有內容的未送出表單在取消、Escape、backdrop／drawer 關閉與重新整理前提示；optimistic-lock conflict
  會重新取得 detail／summary／list，保留他人更新並提供可操作訊息。
- Go tests／vet、contract validation、generated client check、frontend lint／typecheck／build／format 與
  Vitest／RTL `74/74` 通過；專屬 Backend／Chrome 各 `1/1`，完整 Backend Karate `106/106`、Frontend
  browser `25/25` 通過。部署後兩個 ingestion consumer group 均為 Stable、1 member、zero lag。

### P1.1-UX-05：全頁面 390px 核心工作流修復

**狀態**：`completed`

**目標**：行動版不是縮小桌面，而是能完成搜尋、查看、建案與案件處理的介面。

交付：

- 將水平 nav 改成可辨識當前頁的 mobile menu；角色與 Sign out 不得被隱藏。
- 修正 Journey milestone grid 的 min-width／overflow，禁止 document-level horizontal overflow。
- Investigation 與 Pattern 在小螢幕改用摘要卡片；詳細欄位進 drawer，不要求橫向閱讀千像素表格。
- Scenario 執行狀態與結果顯示在所選卡片附近、sticky panel 或 routeable drawer。
- Drawer／modal 使用可視 viewport 高度、safe-area、焦點管理與可觸控關閉操作。
- 所有核心控制符合至少 44px touch target；外部 deep link 也必須可觸控。

驗收：

- `390 × 844` 覆蓋 Login、Overview、Guide、Timeline、Journey、Journey Profiles、Investigations、
  Pattern Library、Scenario Lab、Query Shortcuts 與所有 drawer／modal。
- E2E 斷言 document 無水平 overflow、目前導覽可見、Sign out 可用、核心 CTA 可操作。

## 5. Wave UX-B：補足判讀、導航與產品閉環

### P1.1-UX-06：Journey 決策摘要、Trace 與異常證據

**狀態**：`completed`

**目標**：不用掃描所有里程碑，就能回答「現在在哪裡、下一步是什麼、為何被判為異常」。

交付：

- Backend 提供或明確計算 completed/total、current milestone、next expected event／milestone。
- 顯示同一 correlation 的 distinct Trace 數量與 Trace ID chips，可回 Timeline／Tempo。
- Anomaly 使用 response 的 `event_ids` 產生事件連結；未分類事件數可 drill down 到 Timeline。
- 頂端清楚提示判讀使用的 Profile version 與 query window，不將 window-local 結果宣稱為正式業務狀態。
- Timeline 與 Journey 結果區提供雙向切換，保留原查詢窗口。

驗收：

- Completed、in-progress、failed、compensated、missing predecessor、unmapped event、多 Trace 均有案例。
- Domain 狀態與 next step 不由 frontend 重新猜測。

### P1.1-UX-07：Pattern Library 列表／詳細與成效 drill-down

**狀態**：`completed`

**目標**：將 Registry dump 改成能理解、驗證並套用規則的 Library。

交付：

- Pattern 摘要列表加 routeable detail drawer；桌面使用 semantic table，小螢幕使用 cards。
- 提供 Pattern ID／名稱搜尋、Severity／Status／Event type 篩選與成效排序。
- Detail 顯示人類可讀命中語意、requires／expects／excludes、window、版本、source、checksum 與 fixture coverage。
- Hit count、case count 可開啟對應 Timeline／Investigation 查詢。
- 新增 confirmed／false-positive／needs-review／unreviewed 成效彙總及 rate；數量不足時顯示 sample size，
  不宣稱無代表性的百分比。
- 提供「用此 Pattern 查事件」與「查看相關案件」，不加入 runtime 編輯。

驗收：

- Pattern deep link、找不到 Pattern、metrics unavailable、feedback rate 與 mobile detail drawer 有測試。

### P1.1-UX-08：案件佇列與詳細資訊完整化

**狀態**：`completed`

**目標**：讓人能快速找案件、判斷優先順序、分工並回到相關業務流程。

交付：

- Backend 增加 bounded 的 Case no／title 搜尋；評估並定義 total count 或可替代的結果範圍資訊。
- 列表顯示完整更新時間或相對時間、SLA due/breached、active filter chips 與目前顯示範圍。
- Pagination/cursor state 進 URL；case rows 使用真正 links，支援另開分頁。
- Drawer tab 進 URL，refresh/back/forward 保留 Summary／Timeline／Patterns／Evidence／Audit。
- 顯示 created/closed time、case age、fixed version、workflow ID（有真實值時）與建案來源。
- Related Correlation IDs 連到 Journey／Timeline；tags 與 owner 可作為篩選入口。
- Collaboration fields 提供 dirty、saved、conflict 狀態與 note 字數。

驗收：

- 搜尋、複合篩選、URL pagination、direct drawer tab、related correlation 與 mobile cards 有 E2E。

### P1.1-UX-09：Scenario Lab 分組、執行狀態與 Run History

**狀態**：`completed`

**目標**：讓 Scenario Lab 成為可重複操作與比較的測試工具，而不是只保存最後一次 run 的長頁面。

交付：

- 依 Live services／Lab injection、Reliability／Quality／Failure 等維度分組與篩選。
- CTA 包含 Scenario ID 與模式；Live scenario 在執行前說明會建立哪些持久 demo data。
- 只有被執行的卡片顯示 pending；其他 Scenario 不因單一 start request 全部失效。
- 結果以 selected run drawer／panel 顯示，包含 elapsed time、目前步驟、timeout、Expected／Actual 與 deep links。
- 增加最近 run list／API，可依 Scenario、status、時間與 mode 查詢並重新開啟；是否提供 cancel 先由 runner
  contract 決定，不做假取消。
- Expected／Actual 依型別呈現，不直接輸出難讀的 JSON 字串。
- Viewer 顯示唯讀原因，不只呈現無說明的 disabled button。

驗收：

- S1～S14、Live／Synthetic、pass／fail／timeout、refresh persistence、run history 與 mobile result 有測試。

### P1.1-UX-10：Overview 真實健康度與下一步導航

**狀態**：`completed`

**目標**：Overview 真正回答「目前先處理什麼」並避免假健康訊號。

交付：

- 案件狀態涵蓋 OPEN、INVESTIGATING、WAITING_APPROVAL、RESOLVED、CLOSED，或提供清楚的 active/terminal 分組。
- Source cards 顯示 last success、lag、freshness threshold、reason 與建議處置。
- Smart Search 對 Correlation ID 提供「看 Journey」與「看 Timeline」，不強迫單一路徑。
- Events / 72h 主要進入 Event Hunter 查詢，Quality Dashboard 以明確 external secondary action 呈現。
- Header 的 API ready 改為真實 readiness／degraded state，或移除不具真實資料來源的狀態文案。
- 更新時間顯示完整日期與時間，並提供手動 refresh。

驗收：

- 各 case status、fresh/stale/partial/unavailable、smart-search candidate actions 與 external link label 有測試。

## 6. Wave UX-C：全站一致性、可存取性與說明

### P1.1-UX-11：Shell、Role 與語意導覽

**狀態**：`completed`

- Navigation、case/profile rows 與 routeable CTA 使用 links，不以 button 模擬頁面跳轉。
- 每頁有一個 `h1`，加入 skip-to-content；維持合理 heading hierarchy。
- Login 清楚說明 Viewer／Investigator／Admin 的真實權限，並提供 pending／error 狀態。
- Demo session 過期或 API 回 401 時導回 Login 並保留安全的 return path。
- Mobile navigation 顯示目前頁、角色與 Sign out。

### P1.1-UX-12：錯誤、成功、Undo、Copy 與語言一致性

**狀態**：`completed`

- 將 API error code 映射為人類可理解訊息與 retry／next action；保留 correlation/request ID 供診斷。
- Delete Saved Search 等可逆刪除提供確認或 Undo；結案維持明確 confirmation。
- Collaboration、Pattern feedback、附件、Scenario start 等 mutation 統一顯示 pending/success/failure。
- Event／Trace／Correlation／Aggregate ID、checksum、source path 提供一鍵 Copy。
- 統一中英文：例如 Search timeline、YES、Immutable、Expected／Actual 必須採一致產品詞彙或附說明。
- Input 補上合理的 `name`、`autocomplete`、`inputMode`、`spellCheck` 與 accessible description。

### P1.1-UX-13：功能導覽與真實產品狀態同步

**狀態**：`completed`

- 修正「5 個核心功能」與四個 selector items 的不一致。
- 納入 Overview、Journey Profiles、Scenario Lab 與共用 Query Shortcuts。
- 以使用者問題作為入口，而不是要求先理解產品模組名稱。
- Available／Known gaps 必須由目前 contract 與實作狀態維護，不宣稱未呈現的 completion ratio、
  processing attempts 或 Pattern feedback metrics 已可使用。

已完成子範圍：

- 側欄改名為 `Event Hunter Guide` 並移到 Scenario Lab 下方；導覽數量由實際資料動態計算。
- `/guide` 預設提供 `Getting Started & Integration`，說明完整調查路徑與 Minimum／Recommended／Full
  三種接入層級。
- 說明 Outbox、既有 Kafka、live OTel 與 Grafana 自動建案四條路徑，以及 Canonical Event Envelope
  必備欄位、onboarding checklist 與現行產品邊界。
- 明確標示 Normalization Adapter 使用獨立 consumer group、是 consumer + producer 而非 ClickHouse
  sink；目前也沒有 self-service topic／schema onboarding UI。
- 接入內容已深化成 Integration Runbook：四個資料責任、五個 Timeline admission gates、七個可展開接入
  步驟、四種 failure classification 與五個 SAFE／CONTROLLED 驗證命令；每步均列 repo source of truth、
  驗證方式與完成條件，destructive operation 仍只放在正式 operations runbook。
- 2026-08-26 可讀性補強：在進階 Runbook 前增加三題 Yes／No 快速判斷、單筆事件端到端範例、四種來源
  情境的必要變更、Kafka 白話字典與 Timeline no-data 五步排查；長版內容明確標成進階。
- Business Journey 導覽新增「狀態由完整事件集合推導、actual events 只列本里程碑 expected types」說明；
  Journey 畫面的空里程碑文案也區分「前置事件已啟動，正在等待本階段事件」與「尚未觸發」。

完成證據：

- Frontend lint／typecheck／build／format、generated client checks、Vitest／RTL `74/74` 與 contract
  validation 通過。
- 專屬 `@feature-guide` browser acceptance `1/1`、完整 Frontend Karate `25/25` 通過，包含既有
  `390 × 844` document-level horizontal overflow 驗證。
- 部署後以 `1280 × 720` 瀏覽器實查確認導覽順序、預設頁、三個 tier cards，且 document
  `scrollWidth === clientWidth`。

其餘完成範圍：

- selector 已納入 Overview、Journey Profiles、Scenario Lab 與共用 Query Shortcuts，共九個可分享的導覽入口。
- 每個入口以使用者問題、主要操作、輸入來源與現行限制說明功能；Available／Known gaps 由同一份
  `feature-guide.ts` 資料維護，避免導覽數量與實際頁面再次漂移。

## 6.1 2026-08-28 封板證據

- Shell 使用真正 navigation links、skip-to-content、單一頁面 `h1`、可見的 role／Sign out 與 44px
  mobile controls；390px 下無 document-level horizontal overflow。
- Journey 的 progress、current／next milestone、next expected events 與 distinct Trace IDs 全由 backend
  回傳；前端只呈現並提供 Timeline／Tempo／Investigation 下一步。
- Pattern Library 具搜尋、篩選、排序、routeable drawer、規則／fixture／source 詳細及 reviewed、
  unreviewed、false-positive 成效；小螢幕改為卡片，不要求橫向讀表。
- Investigation 具 bounded case no／title backend search、URL pagination／cursor／drawer tab、完整時間、
  related correlation 行動與 mobile cards。
- Scenario Lab 依執行模式分組，只停用當前啟動按鈕；新增 persisted Run History API、條件篩選、手動
  refresh 與重新開啟結果。Start 仍維持單一 request 回傳 run ID／correlation ID，不恢復 750ms polling。
- Overview 顯示真實 source state、last success、lag、reason、完整產生時間與手動 refresh；全站常見
  backend error code 轉為人類訊息，Saved Search delete 具確認。
- 最終 regression：Go test／vet、Frontend lint／typecheck／production build、Vitest／RTL `80/80`、
  OpenAPI generated-client check 與 contract validation（33 個 operation mappings）通過；新增 Backend Karate
  Journey `6/6`、Investigation `6/6`、Pattern `1/1`、Scenario History `5/5`，Frontend Scenario／390px
  browser acceptance `2/2` 通過。
- E2E 後以 gate timestamp 精確清理，`[E2E]` 案件與本輪 Scenario runs 均為 `0`。

## 7. 依賴與建議執行順序

```text
Wave UX-A
P1.1-UX-01 Dynamic time / URL ──┬── P1.1-UX-02 Incident window / partial
                                ├── P1.1-UX-03 Case Pattern loop
                                └── P1.1-UX-06 Journey summary（Wave B）

P1.1-UX-04 Explicit case state ───── P1.1-UX-08 Case queue/detail（Wave B）

P1.1-UX-05 Responsive foundation ─┬─ P1.1-UX-07 Pattern Library（Wave B）
                                  ├─ P1.1-UX-08 Case queue/detail（Wave B）
                                  └─ P1.1-UX-09 Scenario Lab（Wave B）

Wave UX-C 在元件實作時持續套用，最後再做全站收斂與完整 regression。
```

若一次只能啟動五項，固定順序為：

1. `P1.1-UX-01` 動態時間與 URL。
2. `P1.1-UX-02` 案件 Incident Window／partial。
3. `P1.1-UX-03` 案件 Pattern 閉環。
4. `P1.1-UX-04` 明確狀態轉換。
5. `P1.1-UX-05` 全頁面 responsive blockers。

## 8. 測試與驗收矩陣

| 層級 | 最低要求 |
|---|---|
| Domain/Application | Case transition、incident window、Journey next step、Pattern metrics 等規則具 Go unit tests |
| Repository/Integration | Migration、keyset/search、partial source、restart persistence 具真實 PostgreSQL／ClickHouse 測試 |
| HTTP Contract | OpenAPI、generated client、authorization、time bound、error／partial response 無 drift |
| Backend E2E | 動態窗口、歷史案件、Pattern all-applicable、Journey evidence、Scenario history、source failure |
| Frontend component | URL state、loading/error/success、role visibility、drawer tabs、copy/undo 與可控 clock |
| Browser E2E | Desktop + `390 × 844`，涵蓋九個頁面、共用 drawer/modal、keyboard、back/forward、外部連結 |
| Data isolation | 測試建立的案件、feedback、Saved Search、Scenario run 可辨識並由 gate cleanup 清除 |

每個工作包完成前必須：

1. 正常、空結果、partial、timeout、unauthorized／forbidden、conflict 與 restart 情境按風險覆蓋。
2. 不以 fixture loader 偽造 live service 行為或 observability 成功。
3. 不刪除既有 contract、Karate 或 responsive assertions 來讓新實作通過。
4. 更新本文件狀態、正式 implementation DAG、traceability 與 sign-off addendum。

## 9. 明確不納入本計畫

- Phase 2 Projection Rebuild。
- Phase 3 Sandbox Behavioral Replay。
- Production redrive 或任意事件注入器。
- Journey Profile runtime 編輯、審核與發布 UI。
- Runtime Pattern CRUD、LLM 判斷或任意 SQL。
- Event Hunter 自建 Logs／Metrics／Traces Explore、Kafka Explorer、Topic Registry、Schema Catalog 或 Quality Console。
- 取代 Grafana Alerting、On-call／Escalation 或完整 Incident Management。
- 正式 OIDC／SSO／RBAC 與 staging hardening；仍由 `P1.1-08-02` 的部署決策處理。

## 10. 完成判定

本計畫只有在下列條件全部成立後才能標示 `completed`：

1. Wave UX-A 全部完成，且不再有固定 demo 查詢時間、rolling case window、單一 hard-coded Pattern、
   模糊 case transition 或 390px 核心阻斷。
2. Wave UX-B 的 Journey、Pattern、Case、Scenario、Overview 均能回答各自的主要使用者問題並提供下一步。
3. Wave UX-C 的語意導覽、角色說明、錯誤／成功回饋、Copy 與語言一致性完成。
4. 完整本機 release gate、backend/frontend Karate、responsive suite、restart persistence 與 data cleanup 通過。
5. 由產品／工程共同進行一次桌面與行動版手動 sign-off，並留下可追蹤 evidence。
