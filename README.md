# Event Hunter

[產品介紹](https://zhjiangexe.github.io/event-hunter/) · [架構說明](requirements/architecture/current-architecture.md) · [快速啟動](#快速啟動)

Event Hunter 是一套從「業務事件」出發的唯讀調查平台。輸入 Correlation、Trace、Event、Aggregate
ID 或受治理的 Business Key，它會找出有明確關聯的事件，再用版本化 Check Model 判斷實際流程。

它主要回答這幾個問題：

- 這個 ID 關聯到哪些事件，資料是否完整可信？
- 實際事件符合哪條合理路徑，正在等待還是已經違規？
- 是否出現重複、亂序、缺少事件或 ingestion 問題？
- 哪些 Logs 與 Traces 和這筆業務有關？
- 是否需要建立案件，保存證據並交給其他人處理？

Event Hunter 不取代 Grafana，也不控制正式 workflow。Grafana 保存 Logs、Metrics、Traces 與 Alerting；
Event Hunter 保存 canonical events、可重現 Check Snapshot、Investigation Case 與可信 deep links。

## 從事件深入調查

在 Event Check 用 Correlation ID 查出事件後，切換到 **Timeline**；每一筆事件都會帶著自己的
Event ID、Correlation ID、Producer 與 Trace ID，並提供紅框標示的觀測入口：

![Event Check Timeline 的 Grafana、Loki、Tempo 與 Quality Dashboard 觀測入口](artifacts/screenshots/event-check-observability-links-annotated.png)

- **Grafana Explore**：用 `Event ID` 直接查看 ClickHouse 中保存的該筆 canonical event。
- **Loki logs**：用 `Producer` 與 `Correlation ID` 查看服務在事件發布前後留下的執行紀錄。
- **Tempo trace**：用 `Trace ID` 還原同一次請求經過各服務與 Kafka 的跨服務呼叫路徑。
- **Quality Dashboard**：查看該時段的事件量、重複、延遲、legacy contract violation 與 processing DLQ
  聚合；目前 admission quarantine 的逐筆安全摘要以 Ingestion Issues 為準。

## 可以做什麼

| 功能 | 用途 |
|---|---|
| Overview／Smart Search | 使用 Correlation、Trace、Event 或 Aggregate ID 快速定位業務資料 |
| Event Check | 在同一工作區查看 Summary、Timeline、Flow、Findings、Cases，並保存 immutable Snapshot |
| Saved Results | 重新開啟已保存的 Check Snapshot，或建立／加入 Investigation Case |
| Check Models | 查看版本化 Flow Models、Global Checks、測試情境與 checksum 驗證的原始 YAML；這是唯一正式判定來源 |
| Ingestion Issues | 查看格式、admission 與 connector technical DLQ 問題的安全摘要 |
| Investigation Cases | 引用 Snapshot／Evidence，完成指派、Notes、Root cause、Resolution 與 Audit |
| Scenario Lab | 執行 S1～S14 情境，快速產生可供 Event Check 查詢的資料 |
| Event Hunter Guide | 查看操作方式、外部系統接入與調查 Runbook |

目前完成的是 Phase 1.1：Event Check、Check Models、案件協作、Scenario Lab、Grafana deep links 與
OpenTelemetry 整合。Temporal workflow、Projection Rebuild 與 Sandbox Replay 尚未接入，也不是目前
操作 Event Hunter 的必要條件。

## 快速啟動

需要 Docker 與 Docker Compose。第一次啟動會下載並建立多個本機容器。

```bash
git clone https://github.com/zhjiangexe/event-hunter.git
cd event-hunter

# 選用；不建立 .env 也會使用 compose.yaml 的本機預設值
cp .env.example .env

bash scripts/dev-up.sh
```

啟動完成後開啟：

- Event Hunter：<http://localhost:28334/login>
- Grafana：<http://localhost:28332>

Event Hunter 本機登入頁不需要帳號密碼，可直接選擇 `Viewer`、`Investigator` 或 `Admin` 示範角色。
一般操作建議使用 `Investigator`。

常用維護命令：

```bash
bash scripts/dev-status.sh   # 查看服務狀態
bash scripts/dev-down.sh     # 停止容器，但保留資料 volumes
bash scripts/dev-up.sh       # 再次啟動並補齊 migration、topics 與 connectors
```

完整 port、持久化與故障處理請看 [Local development infrastructure](infra/README.md) 和
[Operations Runbook](requirements/operations/operations-runbook.md)。

## 第一次使用

最容易理解 Event Hunter 的方式是先執行一個 Scenario：

1. 登入後開啟 **Scenario Lab**。
2. 執行 `S1 正常訂單出貨`，取得 `Run ID` 與 `Correlation ID`。
3. 在結果 modal 點 **Open Event Check**，或複製 `Correlation ID` 到 Event Check。
4. 依序查看 **Timeline** 的事件事實、**Flow** 的合理路徑及 **Findings**。
5. 從事件詳細資訊開啟 Grafana Logs 或 Trace。
6. 需要協作時先保存 **Check Snapshot**，再加入 **Investigation Case**。

`S1`、`S12`～`S14` 會呼叫真實的 Order／Payment／Shipping demo services；`S2`～`S11` 使用隔離的
synthetic topic 模擬缺少事件、重複、亂序、DLQ、配送與退貨等情境。

如果預期事件完全找不到，先確認 Event Check 的 ID 類型與時間範圍；若 producer 已送出但事件未通過
契約、admission 或 connector 寫入，改到 **Ingestion Issues** 查 safe failure summary。服務在事件送出前
就當機時，不一定會產生 ingestion issue，應改查 Loki／Tempo 與服務 readiness。

## 外部系統如何接入

接入 Event Hunter 的系統需要提供兩類資訊：

1. 將符合 canonical envelope 的 Domain Events 發布到約定的 Kafka topics。
2. 將 OpenTelemetry traces、logs 與 metrics 送到 Event Hunter 使用的 OTel Collector。

最重要的事件識別欄位包括：

- `eventId`：唯一識別單一事件。
- `correlationId`：串起同一段業務旅程。
- `traceId`：連到分散式 Trace。
- `aggregateType`／`aggregateId`：識別事件所屬 Aggregate。
- `eventType`／`eventVersion`／`occurredAt`：描述事件種類、版本與發生時間。

Event Check 查詢 ClickHouse 保存的 read model，不直接即時讀取 Kafka。完整 ingestion、
Outbox、Trace Context 與資料儲存流程請看 [Current Architecture](requirements/architecture/current-architecture.md)；
事件格式以 [Canonical Envelope Schema](contracts/events/canonical-envelope.schema.json)、
[AsyncAPI](contracts/asyncapi.yaml) 與 [Topic Topology](contracts/platform/topic-topology.yaml) 為準。

## 驗證專案

快速執行不會中斷服務的主要檢查：

```bash
python3 scripts/validate-contracts.py
go -C backend test ./...
pnpm --dir frontend install --frozen-lockfile
pnpm --dir frontend test:run
pnpm --dir frontend typecheck
pnpm --dir frontend lint
```

完整 Phase 1 exit gate、Karate E2E、observability 與 restart persistence 測試請參考
[E2E README](e2e/README.md)；部分完整測試會重啟容器，不應在共用環境隨意執行。

## 文件導覽

README 只保留產品入口與快速操作。詳細設計集中在以下文件：

| 文件 | 內容 |
|---|---|
| [Current Architecture](requirements/architecture/current-architecture.md) | 系統元件、資料流、ingestion、OTel、backend 邊界與架構圖 |
| [Infrastructure](infra/README.md) | Compose 服務、ports、migration、持久化與本機環境 |
| [Operations Runbook](requirements/operations/operations-runbook.md) | 啟停、readiness、故障恢復、備份與 release 操作 |
| [Requirements Index](requirements/README.md) | Phase 1／1.1 規劃、產品契約與 traceability |
| [Application Architecture](requirements/architecture/application-screaming-architecture.md) | Investigation context 的 DDD 與 screaming architecture |
| [Data Model](requirements/architecture/data-model.md) | PostgreSQL、ClickHouse 與資料所有權 |
| [Event Check Product Requirements](requirements/product/event-check-and-check-models-requirements.md) | canonical 功能、Check Model、Snapshot、Case handoff 與 legacy migration |
| [HTTP OpenAPI](openapi.yaml) | Event Hunter API 的唯一 HTTP 契約 |
| [Event Contracts](contracts/asyncapi.yaml) | Kafka channels、事件格式與整合契約 |
| [Frontend README](frontend/README.md) | React 開發、API client、測試與登入模式 |
| [E2E README](e2e/README.md) | Karate feature 分層、執行方式與測試產物位置 |

架構圖原始檔也保存在 repository：

- [System Architecture PlantUML](requirements/architecture/diagrams/event-hunter-architecture.puml)
- [Activity Diagram PlantUML](requirements/architecture/diagrams/event-hunter-activity.puml)

## 專案邊界

- Event Hunter 是唯讀調查與案件協作平台，不修改正式訂單、付款、庫存或出貨資料。
- Check Model evaluation 不會執行 Replay、重新發布事件或推進正式業務流程。
- 舊 `/timeline`、`/journey`、`/journey-profiles`、`/patterns` 與 `/saved-searches` 書籤由相容層導向新工作區；無法無損轉換的廣泛 Timeline 篩選暫留 Legacy Event Explorer。
- 正式環境不得沿用 `.env.example` 的本機示範密碼。
- MVP 登入使用 Demo Session；正式部署需替換為 OIDC／企業 Identity Provider。
- Production Redrive 不屬於本專案的一般功能，避免重複扣款、通知、出貨或退款。
