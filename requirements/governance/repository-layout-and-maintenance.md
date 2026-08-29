---
document_id: EH-DOC-GOV-001
status: active
owner: platform
last_reviewed: 2026-08-29
source_of_truth: true
canonical_topic: repository-layout
supersedes: []
---

# Repository layout 與維護規範

本文件定義 Event Hunter 的程式碼、測試、設定與生成物邊界。後續 Agent 在新增功能前應先依此判斷檔案歸屬，避免把可執行規格、runtime 設定與測試報告混在一起。

## 目錄責任

| 路徑 | 責任 | 可提交內容 |
|---|---|---|
| `backend/cmd/` | 各 executable 的 composition root、HTTP/Kafka wiring 與 process lifecycle | Go source、單元測試 |
| `backend/internal/contexts/` | Event Hunter bounded context 的 domain、application、ports 與 adapters | Go source、單元測試 |
| `backend/internal/platform/` | PostgreSQL、ClickHouse、observability、health 等共用技術能力 | Go source、單元測試 |
| `backend/internal/demo/` | order/payment/shipping 示範系統；用來產生真實跨服務事件與 telemetry，不是 Event Hunter domain | Go source、單元測試 |
| `backend/migrations/` | PostgreSQL、ClickHouse 與 demo outbox schema | 依序編號 migration |
| `frontend/src/` | React UI、UI-side query/state helpers 與單元測試 | TypeScript、CSS、測試 |
| `frontend/src/generated/` | 由 OpenAPI 產生的 client/types | 生成後可提交，但不得手改 |
| `contracts/` | HTTP、Kafka、event、canonical Check Models／Event Check、fixture 與 platform policy；journey／pattern 僅 legacy migration input | YAML、JSON Schema、fixture JSON |
| `config/` | 跨服務的非秘密操作模式與受控 cutover 預設；不得存放密碼 | 可 source 的 `.env` profile、模式說明 |
| `infra/` | Collector、Loki、Tempo、Prometheus、Debezium、ClickHouse Kafka Connect Sink 等服務專屬設定 | 可重建 runtime 的設定檔 |
| `e2e/backend/` | 從外部 API／broker／database 驗證完整後端行為的 Karate features | `.feature` |
| `e2e/frontend/` | 真實 browser interaction 的 Karate features | `.feature` |
| `e2e/helpers/` | Karate 共用 JavaScript helper；不得放執行報告 | `.js` |
| `scripts/` | 啟停、migration、fixture、驗證與 E2E entry points | shell／Python scripts |
| `requirements/product/` | 產品範圍與 prototype 對照 | YAML、審查用 HTML |
| `requirements/architecture/` | 現行架構、資料模型、read model 與圖表 | Markdown、PlantUML、PNG |
| `requirements/contracts/` | 產品行為與跨元件語意；不取代 repository `contracts/` 的 executable schemas | Markdown |
| `requirements/decisions/` | 已採用架構決策與替代方案取捨 | ADR Markdown |
| `requirements/delivery/` | 現行 task DAG、開發計畫與已完成交付歷史 | YAML、Markdown |
| `requirements/testing/` | E2E 策略、案例合理性與驗收矩陣 | Markdown |
| `requirements/operations/` | Runbook 與日期化稽核證據 | Markdown |
| `requirements/governance/` | Traceability、文件 catalog 與維護規則 | YAML、Markdown |
| `artifacts/e2e/karate/` | 最新完整 E2E 報告 | 僅保留 `backend/`、`frontend/` 兩個 canonical report |

## 程式碼邊界

Backend 新增功能時採下列依賴方向：

```text
cmd (composition / transport / process lifecycle)
  -> contexts/<context>/adapters/inbound
  -> contexts/<context>/application
       -> contexts/<context>/domain
       -> contexts/<context>/ports
  -> contexts/<context>/adapters/outbound implements ports
  -> platform/<shared-adapter> implements ports
```

- Aggregate root、value object 與不變量放在 `domain`，不得依賴 database、HTTP 或 Kafka。
- Application service 以 use case 命名，負責協調 aggregate、repository 與 unit of work。
- Repository interface 屬於 context port；單一 context 專用的 PostgreSQL／ClickHouse／Kafka 實作優先放在
  `contexts/<context>/adapters`，確實跨 context 共用的實作才放在 `platform`。
- `cmd` 只保留 transport mapping、dependency wiring、readiness 與 graceful shutdown。大型 handler 應按 use case 逐步拆出，不再增加單檔責任。
- `demo` 是受測的外部示範拓撲。Scenario Lab 會驅動它，但不能取代三個服務的 live outbox、Kafka 與 OpenTelemetry 路徑。
- bounded context 根目錄不得放 flat production source；Scenario Lab、Quality 與 Ingestion 已完整使用
  `domain/application/ports/adapters`。`internal/architecture/dependencies_test.go` 會檢查 inward dependencies、
  技術 framework 隔離、flat source 與 `cmd` 內嵌 SQL。
- Frontend 的 URL parsing、observability deep link 與 API mapping 應放在獨立 module；Event Check／Saved
  Results／Check Models 已位於 `event-check-workspace.tsx`，`main.tsx` 只保留 shell 與尚未拆出的頁面 composition。

目前仍應持續拆分的 hotspot 是 `frontend/src/main.tsx` 與 `backend/cmd/api/investigations.go`。既有
`event-check-workspace.tsx`、案件列表 query parser、observability link builder 與 incident-window helper
是後續切片方式的基準；更大範圍拆分不得改 API／UI 行為。

## 測試與報告

測試 source 與執行結果必須分離：

```text
e2e/**/*.feature                  # source
scripts/test-*-e2e.sh             # runner
scripts/lib/karate.sh              # shared Karate standalone setup
artifacts/e2e/karate/backend/      # latest canonical backend report
artifacts/e2e/karate/frontend/     # latest canonical frontend report
target/karate-temp/                # reproducible temporary output
```

- 專案使用 Karate standalone JAR，不使用 Maven／Java test runner。
- Backend 單元測試使用 `go test ./...`；Frontend 單元／UI 測試使用 Vitest。
- E2E feature 不得依賴先前報告或 `target` 內容。
- `bash scripts/clean-generated-artifacts.sh` 清除 compiler/browser 暫存，保留 canonical reports。
- `bash scripts/clean-generated-artifacts.sh --reports` 另外清除 tagged、debug、history 與 legacy Maven reports，但仍保留最新 backend/frontend 完整報告。
- 清理腳本不碰 fixture、database volume、backup 或 runtime persistence data。

## 設定來源與優先順序

設定發生衝突時，依下列來源判定並同步修正：

1. `requirements/product/project-scope.yaml` 與 `contracts/platform/*.yaml`：產品決策、port、topic、failure policy、identity/time 等正式契約。
2. `openapi.yaml`、`contracts/asyncapi.yaml` 與 schemas：HTTP／Kafka payload 的正式介面。
3. `.env.example`：所有 Compose 可調環境變數與本機安全預設的完整清單；真實 secret 只放未提交的 `.env`。
4. `compose.yaml`：container wiring、environment injection、health check、port mapping 與 persistent volume。
5. `infra/**`：單一基礎服務本身的 runtime configuration。
6. `scripts/**`：只可引用以上設定或提供相同 fallback，不得自行發明另一組 port／topic／secret。

特別規則：

- 新增 `${ENV_VAR}` 到 `compose.yaml` 時，必須同步加入 `.env.example`；`scripts/validate-contracts.py` 會驗證。
- 新增或改動 host port 時，先更新 `contracts/platform/port-registry.yaml`，再同步 `.env.example`、Compose、script fallback 與文件。
- Session signing secret、webhook HMAC secret 與資料庫密碼必須分離，不得共用環境變數。
- `frontend/src/generated/*.ts` 由 OpenAPI generation/check script 維護；修改契約後重生，不可直接 patch 生成碼。
- Fixture telemetry 必須標為 synthetic/replayable；live SDK telemetry 不得靠 fixture 假裝完成。

## 變更完成條件

依變更範圍至少執行：

```bash
python3 scripts/validate-contracts.py
docker compose config --quiet
(cd backend && go test ./... && go vet ./...)
(cd frontend && npm test && npm run typecheck && npm run lint && npm run format:check && npm run build)
bash scripts/test-backend-e2e.sh
bash scripts/test-frontend-e2e.sh
bash scripts/clean-generated-artifacts.sh --reports
```

完成後 `artifacts/e2e/karate/` 應只包含 `backend/` 與 `frontend/`，且專案根目錄不應殘留 `target/` 或 `frontend/dist/`。
