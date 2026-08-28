---
document_id: EH-DOC-HIST-002
status: completed
owner: quality
last_reviewed: 2026-08-28
source_of_truth: false
canonical_topic: phase-1-1-local-sign-off
supersedes: []
---

# Event Hunter Phase 1.1 Local Release Baseline Sign-off

## 決策

- 狀態：`approved`
- 簽核日期：2026-08-24
- 驗證方式：本機完整 Exit Gate；依決策不要求 GitHub／hosted CI
- 執行指令：`bash scripts/test-phase-1-exit.sh --no-start`
- 執行區間：2026-08-24T04:36:06Z ～ 2026-08-24T04:41:29Z
- 結果：`passed`

本文件只簽核可在本機重現的 Phase 1.1 release baseline，不代表
`phase-1-1-development-plan.md` 原始產品化範圍全部完成，也不宣告 production deployment ready。
完整計畫目前仍為 `in_progress`。

Phase 1.1 的阻擋項目 EH-P1.1-007～012 已全部完成。Wave D 與 Wave E 在簽核當下均屬後續產品化
補強，不阻擋本次 release。

尚未由本 sign-off 覆蓋的項目目前只剩 P1.1-08 的正式 staging、TLS、secret rotation、backup／restore、retention／deletion、
OIDC／SSO／RBAC 與 RTO／RPO 演練。

## Post-sign-off addendum（2026-08-24）

- P1.1-04-01 已完成 backend-owned Pattern effectiveness：30 天窗口的 hit、最近命中與案件數已接入
  Pattern Library；Backend 94/94、Frontend 17/17、Frontend unit/component 51/51 及 contract drift checks 通過。
  此完成項不改變完整產品化計畫仍為 `in_progress` 的判定。
- P1.1-04-02 已完成 immutable Pattern Git metadata 與獨立 finding feedback：Backend 95/95、Frontend
  17/17、Frontend unit/component 52/52、Go test/vet 與 contract drift checks 通過；API restart 後仍可查回
  `CONFIRMED v1` 與 audit，gate cleanup 後案件及 feedback 均為 0。未加入 reviewer workflow。
- P1.1-02-03 已完成 backend-owned compound filters、`created_at`／`updated_at` stable keyset sort、
  sort-bound v2 cursor 與可分享 Investigation URL。Go test/vet、Frontend unit/component 53/53、typecheck、
  lint、build、contract/generated client drift、Backend 17 features／97/97 與 Frontend 18/18 均通過；
  cleanup 後案件、feedback、Scenario runs 均為 0，事件管線仍為 RUNNING／Stable／zero lag。
- P1.1-07-01 經盤點確認已由 S1～S11 actual-result scenarios 超額完成，不再重做過期的 S1～S6 候選範圍。
- P1.1-08-01 已新增 `operations-runbook.md`、唯讀／restart smoke checker 與 cold volume backup helper；
  static check、contract validation 及 live read-only smoke 均通過。
- P1.1-QA（Wave E）已完成：Go runtime 升至 1.26.7，Staticcheck／govulncheck／ESLint／pnpm audit／
  Compose policy 全數通過；部署後 API binary 與 Frontend 17/17 E2E 已驗證，測試資料殘留為 0。
- Wave E 完成後另於 2026-08-24T05:35:36Z ～ 05:39:34Z 重跑非破壞性完整 Exit Gate
  （`--no-start --skip-disruptive`）：Backend 92/92、Frontend 17/17、live OTel、ingestion/DLQ、
  quality failure mode、Grafana provisioning 與 10 萬事件效能 profile 全數通過；gate-scoped cleanup 後
  `[E2E]` 案件與該次 Scenario runs 均為 0。破壞性 restart／recovery 證據仍沿用上方正式 sign-off gate。
- EH-POC-002 於 2026-08-27 完成功能完整度補強：新增 Ingestion Issues、safe technical DLQ projector、
  ClickHouse-first outage/restart/backlog recovery 與 bounded raw purge。非壓測驗收為 Backend 18 features／
  108/108、Frontend browser 25/25、Frontend unit/component 77/77、Go test/vet 與 contracts/build checks 全通過；
  該階段 canonical source 與 API readiness 曾回復 committed `legacy`；此歷史結果其後由 EH-POC-003 的
  正式採用決策取代。load/soak/capacity 測試依決策延後。
- EH-POC-003 於 2026-08-27 完成正式採用：domain events 與 processing attempts 均改由官方 ClickHouse
  Kafka Connect Sink 寫入隔離 raw tables，再由 Materialized Views 提升至 canonical read models；兩個
  Redpanda Connect ETL workers／設定／ports 已移除。Redpanda broker 與 Debezium 仍保留。Backend
  108/108、Scenario Lab 16/16、Grafana auto-case 1/1、POC 3/3 與雙路線 outage/backlog recovery 通過；
  load/soak/capacity 與正式 raw-data governance 依決策列為非阻擋後續。
- 2026-08-28 補強 live event lifecycle logging：三個服務會在事件發布前、local outbox transaction commit
  後與失敗時留下具名 structured log，consumer completed log 也直接顯示 event type。Focused Go tests 通過，
  並以 S1 `ORDER-5142B16E6C050DE8` 驗證 OrderCreated → PaymentCompleted → ShipmentCreated 的 Loki
  lifecycle／consumer logs 與 ClickHouse／Tempo 共用 trace。規格與後續可重現 gate 見
  [Live Event Observability Contract](../../contracts/live-event-observability-contract.md)；本補強不改變原始 sign-off 日期。

## 驗收證據

下表保存 2026-08-24 原始本機 sign-off 的當時數值；2026-08-27 的最新非壓測回歸基準以
上方 `EH-POC-002` addendum（Backend 108/108、Frontend 25/25、unit/component 77/77）為準。

| Gate | 結果 | 證據／數值 |
|---|---|---|
| Contract 與 generated clients | Passed | Contract validation、OpenAPI client check、Event Lab scenario API client check、format check |
| Backend 靜態與單元驗證 | Passed | `go vet ./...`、`go test ./...` |
| Frontend 驗證 | Passed | Unit/component 49/49、typecheck、production build |
| Backend acceptance | Passed | Karate 16 features、92/92 scenarios |
| Frontend acceptance | Passed | Karate 17/17 scenarios，含 investigation route/dialog 與 390px responsive gate |
| Live observability | Passed | Order → Payment → Shipping 使用同一 trace；Tempo、Loki、ClickHouse 可交叉核對 |
| Failure/recovery | Passed | Broker outage readiness 503、自動恢復、sink acknowledgement、quality worker failure mode |
| Restart persistence | Passed | PostgreSQL 792→792、ClickHouse 101386→101386、Redpanda topics preserved；API graceful shutdown log verified |
| Performance | Passed | 100,000 events、200 measured requests、0 HTTP errors；Timeline p95 25.89ms、Summary p95 72.20ms |
| E2E isolation | Passed | Gate-scoped cleanup 後 `[E2E]` cases 0、gate Scenario runs 0；interactive fixtures restored |

## 可重現 artifacts

- `build/reports/phase-1-exit-summary.json`
- `build/reports/performance-summary.json`
- `build/reports/performance-fixture-summary.json`
- `artifacts/e2e/karate/backend/karate-summary.html`
- `artifacts/e2e/karate/frontend/karate-summary.html`
- `build/reports/security-quality-summary.json`

上述 artifacts 是本機產物，下一次完整 gate 會更新。若未來啟用 hosted CI，應執行等價 gate 並附加
CI run reference，但不回溯否定本次本機 sign-off。

## 已接受的非阻擋風險

- 本次依使用者決策不使用 GitHub／hosted CI；可重現性由版本化腳本、固定契約與本機 artifacts 提供。
- 本機 Tempo 因單節點 volume 權限相容性明確使用 `user: 0:0`；Compose policy 將其列為 accepted local
  warning。正式部署不得直接沿用，需以 non-root UID/GID 與預先配置的 writable volume 取代。
- Tempo／Loki 的 E2E telemetry 由 retention 管理；資料具 synthetic/live 標示，不做廣泛刪除。
