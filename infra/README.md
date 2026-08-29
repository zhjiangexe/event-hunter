# Local development infrastructure

[`compose.yaml`](../compose.yaml) 提供 Event Hunter 的本機依賴與目前已實作的 demo runtime。
所有 image 都固定版本；服務設定、資料庫 migration、Debezium／ClickHouse Kafka Connect connectors、
Materialized Views 與 Grafana provisioning 均有 repo 內檔案作為 source of truth。

## 啟動

需要 Docker Desktop、OrbStack 或其他支援 Docker Compose v2 的 runtime。

```bash
cd event-hunter
cp .env.example .env        # 選用；不複製也會使用 compose.yaml 的本機預設值
bash scripts/dev-up.sh
```

需要開發長時間調查 Workflow 時，才啟動 Temporal profile：

```bash
bash scripts/dev-up.sh --temporal
```

停止容器但保留資料：

```bash
bash scripts/dev-down.sh
```

查看狀態與 log：

```bash
bash scripts/dev-status.sh
docker compose logs -f <service-name>
```

確認 volume 與服務目前可用：

```bash
bash scripts/verify-persistence.sh
```

完整日常檢查、故障恢復、cold backup／restore、retention 與 secret rotation 步驟見
[`requirements/operations/operations-runbook.md`](../requirements/operations/operations-runbook.md)。可先執行唯讀 smoke：

```bash
bash scripts/verify-operations-runbook.sh
```

實際重啟有狀態服務，並比對 PostgreSQL、ClickHouse、Redpanda topic 及 Grafana assets：

```bash
bash scripts/verify-restart-persistence.sh
```

獨立驗證 Quality Worker failure mode 與 Grafana provisioning：

```bash
bash scripts/verify-quality-runtime.sh
bash scripts/verify-grafana-provisioning.sh
```

## 服務與 port

所有主機對外 port 以 [`contracts/platform/port-registry.yaml`](../contracts/platform/port-registry.yaml)
為唯一契約，依「底層依賴先、上層服務後」分配。容器之間仍使用 PostgreSQL `5432`、Kafka `9092`
等 upstream 標準 port；`283xx` 只代表本機主機側或本專案自行實作服務的 listen port。

| 服務 | 本機位置 | 用途 |
|---|---|---|
| PostgreSQL 18 | `localhost:28313` | 案件、Check Snapshots／Findings、Evidence、Saved queries、Scenario runs、Audit |
| ClickHouse | HTTP `28317`、native `28318` | restricted raw landing、canonical events／attempts、safe failures、品質 read models |
| Redpanda Kafka API | `localhost:28319` | 本機 Domain Event broker |
| Redpanda Schema Registry | `http://localhost:28320` | 事件 Schema tooling |
| Redpanda HTTP Proxy／Admin | `28321`／`28322` | Broker HTTP 與管理 API |
| Redpanda Console | `http://localhost:28323` | Topic、Message、Consumer group 開發介面 |
| Prometheus | `http://localhost:28326` | Metrics backend |
| Loki | `http://localhost:28327` | Logs backend |
| Tempo | `http://localhost:28328` | Traces backend |
| OTel Collector | OTLP gRPC `28329`、HTTP `28330`、health `28331` | Telemetry 統一入口 |
| Grafana | `http://localhost:28332` | Dashboard、Explore、Alerting；預設帳密見 `.env.example` |
| Event Hunter API | `http://localhost:28333` | Go API |
| Event Hunter Console | `http://localhost:28334/login` | React investigation console |
| Quality Worker | health `http://localhost:28338/health/ready` | 一分鐘品質聚合 scheduler；人工補算使用 `aggregate` subcommand |
| Temporal（選用） | gRPC `28341`、UI `http://localhost:28342` | 低頻、長時間 Workflow |
| Scenario Lab API | `http://localhost:28343` | S1～S14 Hybrid scenario execution 與 actual-result API；S1／S12～S14 為 live services |
| ClickHouse-first ingestion | REST `http://localhost:28345` | 官方 ClickHouse Sink 的 domain-event／processing-attempt store-all transport |
| Technical DLQ Projector | health `http://localhost:28346/health/ready` | 將官方 Sink DLQ 投影成不含 raw/message/stack 的安全摘要 |

`28310`～`28312` 已避開，因為目前由其他本機服務使用。`28314`～`28316` 為三個 Demo PostgreSQL、
`28324` 為 Debezium Kafka Connect、`28333` 為 Event Hunter API、`28334` 為 React、`28335` 是 Order API；
`28336`／`28337` 保留給目前不對 host 暴露 HTTP 的 Payment／Shipping services。舊 Redpanda Connect
使用的 `28325`／`28344` 已釋出；唯一契約仍以 port registry 為準。

## ClickHouse-first ingestion（正式預設）

`dev-up.sh` 預設啟動獨立 ClickHouse Kafka Connect worker、兩個 connectors 與 raw database，實作
「先保存 Kafka 訊息，再由 ClickHouse admission contract 提升可搜尋事件／attempt」：

```bash
bash scripts/clickhouse-mv-poc-up.sh
bash scripts/test-clickhouse-mv-poc.sh
bash scripts/test-clickhouse-mv-functional-recovery.sh
bash scripts/test-clickhouse-mv-raw-purge.sh
bash scripts/reconcile-ingestion-mode.sh --mode status
bash scripts/reconcile-processing-attempt-ingestion-mode.sh --mode status
```

設定來源位於 `infra/kafka-connect-clickhouse/`，ClickHouse DDL 位於
`backend/migrations/clickhouse/00006_clickhouse_mv_ingestion_poc.sql`，完整範圍與風險見
`requirements/decisions/adr-001-clickhouse-first-ingestion.md`。raw payload 只進入未授權給 `grafana_reader` 的
`event_hunter_poc` database 並保留 7 天；Grafana 只讀安全的 failure summary。
`technical-dlq-projector` 讀取獨立 technical DLQ，成功寫入 `ingestion_technical_failures` 後才 commit；
`/ingestion-issues` 會把 contract、admission 與 technical 三類問題統一呈現，但不具 raw database 權限。

Runtime readers 不直接綁定實體 table，而是讀 `event_hunter.canonical_forensics_events` 與
`event_hunter.canonical_event_processing_attempts`。`config/ingestion-cutover.env` 的 committed default
是 `clickhouse-mv`。舊 Redpanda Connect workers／設定已移除；舊 tables 暫時只作 migration evidence，
不再有 writer。

Event Hunter 的 `/health/live` 只表示 API process 存活；`/health/ready` 會另外驗證 PostgreSQL、
ClickHouse、domain-event／processing-attempt 兩個 Sink connector tasks 與 technical DLQ projector，
避免 worker HTTP 200 掩蓋 connector task failure。

Grafana 啟動時會安裝固定版本的 ClickHouse data source plugin，並自動 provision ClickHouse、
Prometheus、Loki、Tempo、Event Quality dashboard、六條 aggregate quality rules，以及一條
correlation-aware DLQ business rule；另有一條不自動建案的 optional POC admission quarantine rule。
`event_hunter=investigate` 的 alert 由專用 Notification Policy
送到 HMAC-signed Event Hunter Contact Point；HMAC secret 由 `GRAFANA_WEBHOOK_SECRET` 注入，不寫死在
provisioning 檔。ClickHouse 的 `grafana_reader` 是本機唯讀帳號；正式環境不得使用 repo 內的開發密碼。

Order／Payment／Shipping services 使用 OpenTelemetry Go SDK，透過
`OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4318` 輸出 traces、logs 與 metrics。HTTP server
由 `otelhttp` instrumentation 包裝，Kafka client 使用 franz-go `kotel` hooks；outbox 的
`trace_parent`／`trace_state` 由 Debezium connector 映射為 `traceparent`／`tracestate` headers，讓
consumer 延續原始 span context。可用 `bash scripts/test-live-observability.sh` 驗證整條真實服務鏈。

Runtime telemetry 分三類：正式 Compose 僅使用上述 live SDK profile；fixture loader 送出的
synthetic/replayable E2E telemetry 具有 `service.namespace=event-hunter.synthetic` 與固定 fixture
instance，避免與 live Loki stream 混淆或產生跨 stream 的 out-of-order 問題；`otelc` 是未啟用的
後續 optional build profile。`otelc` v1.0 雖已 stable，但其官方支援清單未列 franz-go，而且若同時
啟用其 `net/http`／`database/sql` hooks 與顯式 instrumentation 會造成 duplicate spans。正式
Docker build 因此維持 `otelhttp` + `kotel`，沒有 `otelc` 或 `autoexport` provider。

## 持久化與重啟行為

`bash scripts/dev-down.sh` 只停止並移除容器，不刪除 named volumes。下次執行
`bash scripts/dev-up.sh` 時：

- PostgreSQL 與 ClickHouse 使用 named volumes；migration 由 `scripts/dev-migrate.sh` 可重複套用。
- Redpanda 使用 `redpanda_data`；topic 與 Kafka Connect 的 config／offset／status topics
  保存在 broker volume。`scripts/bootstrap-topics.sh` 與
  `scripts/register-debezium-connectors.sh` 會冪等地補建／更新設定。
- `dev-up.sh` 在 topic bootstrap 後冪等更新兩個 ClickHouse Sink connector configs，確保新 topic／mapping 進入正式訂閱。
- Debezium connector 的 JSON template 保存在 `infra/debezium/`，所以即使 connector metadata
  遺失，也能由啟動流程重新註冊；模板包含 W3C trace context 的 Kafka header mapping。
- Kafka Connect offsets、Prometheus、Loki、Tempo、Grafana、Temporal 的 runtime data 都有 named
  volumes或 broker-backed internal topics；設定檔保存於 `infra/`。
- Loki 的 WAL 與 chunks 位於 `loki_data`，並啟用 `flush_on_shutdown`；因此剛由 live services
  接收、尚未達 idle flush 的 canonical logs，也會在 Compose restart 前落盤。
- Grafana datasource、dashboard、alert rules、Contact Point 與 Notification Policy 由
  `infra/grafana/provisioning/` provision；
  Grafana 自身資料則保存在 `grafana_data`。

因此「容器重啟／`dev-down` 後再啟動」不需要手動重建服務設定。只有刪除 volumes、換機器或
刻意清空 broker data 時，才需要將它視為新環境，重新執行完整的 `dev-up.sh` bootstrap。

## Migration 與資料重置

`scripts/dev-up.sh` 會呼叫 `scripts/dev-migrate.sh`：

- PostgreSQL baseline 尚未存在時，由 `dev-migrate.sh` 解析並執行 migration 的 `-- +goose Up` 區段；目前 runtime 不依賴 goose Go library／binary。
- ClickHouse DDL 使用 `CREATE TABLE IF NOT EXISTS`，每次可重複套用。
- Migration source of truth 是 `backend/migrations/**`；變更後必須通過 contract、fresh-start 與 restart-persistence 驗證。

若確定要刪除所有本機資料，可執行下列破壞性命令；它會清掉 named volumes，無法由 Compose 復原：

```bash
docker compose --profile temporal down --volumes
```

## 仍需後續補強

- 正式環境的 secrets、volume backup、Kafka replication factor 與 Grafana admin policy 仍不可沿用本機預設值。
- fixture loader 是帶有獨立 synthetic resource identity 的可重播測試資料工具，不屬於正式 runtime bootstrap。
- OpenTelemetry Go log signal 目前仍屬 beta；本專案把 exporter 與 `slog` bridge 封裝在共用
  observability runtime，避免業務服務直接依賴 exporter 細節。
