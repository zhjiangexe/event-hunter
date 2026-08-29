---
document_id: EH-DOC-ARCH-003
status: active
owner: platform
last_reviewed: 2026-08-29
source_of_truth: false
canonical_topic: data-model-guide
supersedes: []
---

# Event Hunter Data Model

Event Hunter 的資料模型設計與 runtime 對照。設計會轉成 goose migration、ClickHouse DDL 與 Repository
tests；每個章節會標示已實作或仍屬 target。

Event Check 的 migration 已由 `EH-ECM-003` 實作；精確欄位、一致性與後續遷移邊界以
[Event Check Target Design](event-check-target-design.md) 為準。Evidence Archive 仍是選用 target。

## 1. 資料邊界

```text
PostgreSQL
└── Control Plane：案件、Check Snapshots、Findings、Saved Queries、Scenario runs、稽核

ClickHouse
├── Restricted raw landing：Kafka record，7 天、default-deny
└── Event Plane：canonical events、processing attempts、safe failures、品質 Read Models

Grafana OSS
└── Observability／Query Plane：Dashboard、Explore、Correlations、Alerting 與技術資料 deep link

DuckDB + Parquet
└── Optional Evidence Archive：受治理的離線查詢／匯出，不作線上交易來源或控制面 source of truth
```

應用程式不同步寫入 PostgreSQL 與 ClickHouse。事件由 Kafka ingestion pipeline
寫入 ClickHouse；案件與設定由 Event Hunter API 交易式寫入 PostgreSQL。

Temporal 是 MVP 預設關閉的選用能力。Temporal Server 的 persistence 是 Temporal 內部資料，不直接當成 Event Hunter 的資料表；
`investigation_cases.workflow_id` 與 `replay_sessions.workflow_id` 只保存關聯識別碼。

Grafana OSS 是 Logs／Metrics／Traces 與技術品質告警的主要查詢層。Event Hunter 不保存第二份
完整技術遙測資料；`case_evidence.reference`、Grafana annotation、Alert webhook 與 deep link
只作為案件證據索引。`event_governance` 與 `runtime_quality` 的表格仍保留作為未來設計，MVP
不實作 Event Hunter 的 Catalog／Topic Registry／Quality Console 控制面。

DuckDB／Parquet 只適用於 [ADR-007](../decisions/adr-007-duckdb-parquet-evidence-archive.md) 的後續
Evidence Archive。PostgreSQL 保存 archive catalog；DuckDB 是 typed bounded query／export adapter，不直接
接受前端 SQL，也不取代 PostgreSQL、ClickHouse 或 Grafana telemetry stores。

## 2. Context 與資料表 ownership

| Context | PostgreSQL 表 | ClickHouse 表 | 寫入責任 | MVP 邊界 |
|---|---|---|---|---|
| `event_governance` | `event_definitions`、`event_schema_versions`、`topic_registrations`、`topic_subscriptions` | 無 | 既有 Schema Registry／Kafka tooling；未來才由治理 API 管理 | 暫先不做 Event Hunter 控制面 |
| `runtime_quality` | 無 | safe failure tables、`event_quality_metrics` | ClickHouse MVs／technical projector／品質聚合工作；Grafana 與 Ingestion Issues 查詢 | MVP supporting capability；Event Hunter 不自建通用 Quality Console |
| `investigation` | `investigation_cases`、`case_notes`、`case_evidence`、`grafana_alert_receipts`、`saved_searches`、`scenario_runs`、`audit_logs`；legacy `pattern_*` | 讀取 canonical events／attempts／quality／safe failures | Event Hunter API／Grafana Alert Intake／Scenario Lab；選用 Temporal Activity | canonical Cases 與 Ingestion Issues；Pattern/Journey 僅 compatibility |
| `event_check` | `check_snapshots`、`check_snapshot_event_refs`、`check_snapshot_relations`、`check_findings`、`check_finding_feedback`、`investigation_check_snapshots` | 讀取 `canonical_forensics_events` 與 `canonical_event_processing_attempts`；一般 evaluation 不落庫 | Event Check API／deterministic evaluators | canonical；migration、workspace、compatibility、全量 E2E 與 restart persistence 已完成 |
| `replay_verification` | `replay_sessions` | 讀取事件與離線比較結果 | Temporal Activity／Sandbox Worker | Phase 2／3，暫先不做 |

Context 不直接讀取其他 Context 的 ORM Model 或資料表。跨 Context 的資料交換使用
integration contract、唯讀 Query Port 或 Domain Event；不使用跨 Context SQL JOIN。

目前 PostgreSQL migration 已建立 Cases、Notes、Evidence、Grafana receipts、Saved Searches、Scenario runs、
Audit、legacy Pattern data，以及 Event Check Snapshot／Finding／Case link tables。ClickHouse migration 已建立
legacy promoted tables、restricted raw landing、admission quarantine、technical failure、quality metrics 與兩個
canonical views。Event Catalog、Topic Registry 與 Replay 資料表仍只是未來設計，不應在目前 runtime 建立 CRUD。

Event Check 不改寫上述 Phase 1 歷史。`EH-ECM-006` 已完成 expand／compatibility 與 canonical product
cutover；Journey／Pattern bounded legacy reads 仍保留。舊 direct Case／Pattern schema 的實體 contract
removal 必須是另一個具有 rollback window 的 migration release。

## 3. PostgreSQL Control Plane

### 3.1 `event_definitions`（暫先不做）

事件類型的治理資料。`event_type` 建立後應保持穩定，Owner、Lifecycle 與相容性設定可以更新。

| 欄位 | 建議型別 | 說明 |
|---|---|---|
| `id` | `UUID` | Primary Key |
| `event_type` | `VARCHAR(200)` | 唯一，例如 `PaymentCompleted` |
| `description` | `TEXT` | 業務意義 |
| `producer` | `VARCHAR(200)` | Producer service |
| `owner` | `VARCHAR(200)` | 負責團隊 |
| `compatibility_policy` | `VARCHAR(20)` | `BACKWARD`、`FORWARD`、`FULL`、`NONE` |
| `lifecycle_status` | `VARCHAR(20)` | `DRAFT`、`ACTIVE`、`DEPRECATED` |
| `current_schema_version` | `INTEGER` | 目前啟用 Schema 版本，可為 NULL |
| `lock_version` | `BIGINT` | 樂觀鎖版本 |
| `created_at` | `TIMESTAMPTZ` | 建立時間，UTC |
| `updated_at` | `TIMESTAMPTZ` | 更新時間，UTC |

Constraints：`UNIQUE(event_type)`、`lock_version >= 0`。

### 3.2 `event_schema_versions`（暫先不做）

事件 Schema 的版本紀錄。`schema_body` 建立後不可變；只有 Lifecycle metadata 可以更新。

| 欄位 | 建議型別 | 說明 |
|---|---|---|
| `id` | `UUID` | Primary Key |
| `event_definition_id` | `UUID` | FK → `event_definitions.id` |
| `version` | `INTEGER` | 從 1 開始，對同一事件唯一 |
| `schema_format` | `VARCHAR(20)` | `JSON_SCHEMA`、`AVRO`、`PROTOBUF` |
| `schema_body` | `JSONB` | Schema 本體，不可變 |
| `compatibility_result` | `VARCHAR(20)` | `COMPATIBLE` 或 `INCOMPATIBLE` |
| `lifecycle_status` | `VARCHAR(20)` | 版本生命週期 |
| `lock_version` | `BIGINT` | metadata 更新時使用 |
| `created_at` | `TIMESTAMPTZ` | 建立時間，UTC |

Constraints：`UNIQUE(event_definition_id, version)`。

### 3.3 `topic_registrations`（暫先不做）

Kafka Topic 的治理設定。此表不代表 API 可以直接建立或刪除 Kafka Topic。

| 欄位 | 建議型別 | 說明 |
|---|---|---|
| `id` | `UUID` | Primary Key |
| `topic_name` | `VARCHAR(249)` | Kafka Topic 名稱，唯一 |
| `partition_key` | `VARCHAR(200)` | 例如 `aggregateId` |
| `ordering_guarantee` | `VARCHAR(30)` | `PER_PARTITION`、`PER_AGGREGATE`、`NONE` |
| `retention_days` | `INTEGER` | 保留天數 |
| `pii_classification` | `VARCHAR(20)` | `NONE`、`INTERNAL`、`CONFIDENTIAL`、`RESTRICTED` |
| `owner` | `VARCHAR(200)` | 負責團隊 |
| `lifecycle_status` | `VARCHAR(20)` | `DRAFT`、`ACTIVE`、`DEPRECATED` |
| `lock_version` | `BIGINT` | 樂觀鎖版本 |
| `created_at` | `TIMESTAMPTZ` | 建立時間，UTC |
| `updated_at` | `TIMESTAMPTZ` | 更新時間，UTC |

### 3.4 `topic_subscriptions`（暫先不做）

Topic 與 Consumer group 的治理關係。Kafka 的實際 offset 不放在此表。

| 欄位 | 建議型別 | 說明 |
|---|---|---|
| `id` | `UUID` | Primary Key |
| `topic_registration_id` | `UUID` | FK → `topic_registrations.id` |
| `consumer_group_id` | `VARCHAR(200)` | Kafka Consumer group |
| `consumer_service` | `VARCHAR(200)` | Consumer service |
| `ordering_requirement` | `VARCHAR(30)` | `NONE`、`PER_PARTITION`、`PER_AGGREGATE` |
| `owner` | `VARCHAR(200)` | 負責團隊 |
| `lifecycle_status` | `VARCHAR(20)` | `ACTIVE` 或 `DEPRECATED` |
| `lock_version` | `BIGINT` | 樂觀鎖版本 |
| `created_at` | `TIMESTAMPTZ` | 建立時間，UTC |
| `updated_at` | `TIMESTAMPTZ` | 更新時間，UTC |

Constraints：`UNIQUE(topic_registration_id, consumer_group_id)`。

### 3.5 `investigation_cases`（MVP）

業務級調查案件的 Aggregate root。案件狀態必須依狀態機轉移：

```text
OPEN → INVESTIGATING → WAITING_APPROVAL → RESOLVED → CLOSED
```

| 欄位 | 建議型別 | 說明 |
|---|---|---|
| `id` | `UUID` | Primary Key |
| `case_no` | `VARCHAR(50)` | 人類可讀案件編號，唯一 |
| `title` | `VARCHAR(300)` | 案件標題 |
| `severity` | `VARCHAR(20)` | `LOW`、`MEDIUM`、`HIGH`、`CRITICAL` |
| `status` | `VARCHAR(30)` | 案件狀態 |
| `correlation_id` | `VARCHAR(200)` | 主要業務流程識別碼 |
| `assignee` | `VARCHAR(200)` | 負責人，可為 NULL |
| `root_cause` | `TEXT` | 根因，可為 NULL |
| `resolution_summary` | `TEXT` | 結案處理摘要，可為 NULL |
| `fixed_version` | `VARCHAR(200)` | 修正版本，可為 NULL |
| `notes` | `TEXT` | 人工調查筆記，可為 NULL |
| `workflow_id` | `VARCHAR(200)` | Temporal Workflow ID，可為 NULL |
| `lock_version` | `BIGINT` | 樂觀鎖版本 |
| `created_at` | `TIMESTAMPTZ` | 建立時間，UTC |
| `updated_at` | `TIMESTAMPTZ` | 更新時間，UTC |
| `closed_at` | `TIMESTAMPTZ` | 結案時間，可為 NULL |

Indexes：`case_no` unique、`correlation_id`、`status`、`assignee`、`created_at`。

### 3.6 `pattern_findings`（legacy compatibility）

改版前每次確定性 Pattern 分析產生的不可變 finding。新查詢使用 `check_findings`；本表只支援 deprecated
Pattern／Case compatibility。若相同案件、Pattern、版本與查詢視窗重試，
以 `idempotency_key` 避免重複寫入；新的分析視窗或 Pattern 版本建立新 row，不覆寫舊 finding。

| 欄位 | 建議型別 | 說明 |
|---|---|---|
| `id` | `UUID` | Primary Key |
| `investigation_case_id` | `UUID` | FK → `investigation_cases.id` |
| `pattern_id` | `VARCHAR(200)` | Go registry 中的穩定 Pattern ID |
| `pattern_version` | `INTEGER` | Pattern 程式碼版本 |
| `severity` | `VARCHAR(20)` | finding 嚴重度 |
| `matched_conditions` | `JSONB` | 已命中的確定性條件陣列 |
| `evidence_references` | `JSONB` | 支援 finding 的穩定參照陣列 |
| `recommended_next_query` | `TEXT` | 下一步人工調查建議 |
| `query_template_id` | `VARCHAR(200)` | 使用的固定查詢模板 |
| `query_window_from` | `TIMESTAMPTZ` | 分析時間窗起點 |
| `query_window_to` | `TIMESTAMPTZ` | 分析時間窗終點 |
| `idempotency_key` | `VARCHAR(200)` | 唯一重試鍵 |
| `created_at` | `TIMESTAMPTZ` | 建立時間，UTC |

### 3.7 `case_evidence`（MVP）

案件證據的索引與完整性資訊，不直接把完整 PII payload 塞進案件表。

| 欄位 | 建議型別 | 說明 |
|---|---|---|
| `id` | `UUID` | Primary Key |
| `investigation_case_id` | `UUID` | FK → `investigation_cases.id` |
| `evidence_type` | `VARCHAR(30)` | `EVENT`、`TRACE`、`LOG`、`METRIC`、`GRAFANA_ALERT`、`QUALITY_VIOLATION`、`PATTERN_FINDING`、`REPORT` |
| `reference` | `TEXT` | ClickHouse、Grafana／Tempo／Loki、Trace、Log 或報告的穩定參照；不存完整遙測內容 |
| `checksum` | `VARCHAR(128)` | 證據雜湊，可為 NULL |
| `collected_at` | `TIMESTAMPTZ` | 收集時間，UTC |
| `created_at` | `TIMESTAMPTZ` | 索引建立時間，UTC |

### 3.8 `replay_sessions`（Phase 2／3，暫先不做）

隔離的 Projection Rebuild 或 Sandbox Behavioral Replay 工作。此表不支援 Production Redrive。

| 欄位 | 建議型別 | 說明 |
|---|---|---|
| `id` | `UUID` | Primary Key |
| `investigation_case_id` | `UUID` | FK → `investigation_cases.id` |
| `replay_type` | `VARCHAR(40)` | `PROJECTION_REBUILD` 或 `SANDBOX_BEHAVIORAL_REPLAY` |
| `status` | `VARCHAR(30)` | `PENDING_APPROVAL`、`APPROVED`、`RUNNING`、`SUCCEEDED`、`FAILED`、`CANCELLED` |
| `source_correlation_id` | `VARCHAR(200)` | 歷史事件的 correlation ID |
| `target_service_version` | `VARCHAR(200)` | 隔離環境使用的版本，可為 NULL |
| `sandbox_id` | `VARCHAR(200)` | 隔離環境識別碼，可為 NULL |
| `workflow_id` | `VARCHAR(200)` | Temporal Workflow ID，可為 NULL |
| `comparison_summary` | `JSONB` | 事件、DB、外部呼叫與 Invariant 比較摘要 |
| `lock_version` | `BIGINT` | 樂觀鎖版本 |
| `created_at` | `TIMESTAMPTZ` | 建立時間，UTC |
| `updated_at` | `TIMESTAMPTZ` | 更新時間，UTC |

### 3.9 `audit_logs`（MVP）

平台操作稽核。此表 append-only，不使用 `lock_version`。

| 欄位 | 建議型別 | 說明 |
|---|---|---|
| `id` | `UUID` | Primary Key |
| `actor_id` | `VARCHAR(200)` | 使用者或 Service Account |
| `actor_role` | `VARCHAR(30)` | MVP 為 `VIEWER`、`INVESTIGATOR` 或 `ADMIN` |
| `action` | `VARCHAR(100)` | 例如 `CASE_ANALYSIS_STARTED` |
| `resource_type` | `VARCHAR(100)` | 例如 `investigation_case` |
| `resource_id` | `VARCHAR(200)` | 資源識別碼 |
| `request_id` | `VARCHAR(200)` | API request ID |
| `trace_id` | `VARCHAR(100)` | OpenTelemetry Trace ID，可為 NULL |
| `metadata` | `JSONB` | 不含完整 PII payload 的操作資訊 |
| `created_at` | `TIMESTAMPTZ` | 追加時間，UTC |

### 3.10 `grafana_alert_receipts`（MVP）

Grafana Webhook 的不可變接收紀錄。每個 webhook group 中的 alert 各建立一筆 receipt；相同通知重送
時以 `dedup_key` 取得既有 receipt，不再次建立案件或 Evidence。

| 欄位 | 建議型別 | 說明 |
|---|---|---|
| `id` | `UUID` | Receipt ID |
| `dedup_key` | `VARCHAR(64)` | JCS canonical alert identity 的 SHA-256，唯一 |
| `grafana_org_id` | `BIGINT` | Grafana organization ID |
| `receiver` | `VARCHAR(300)` | Contact Point receiver |
| `group_key` | `TEXT` | Grafana notification group key |
| `fingerprint` | `VARCHAR(300)` | Grafana alert fingerprint |
| `alert_status` | `VARCHAR(20)` | `firing` 或 `resolved` |
| `correlation_id` | `VARCHAR(200)` | 業務流程 ID；不合格告警可為 NULL |
| `severity` | `VARCHAR(20)` | 正規化嚴重度，可為 NULL |
| `labels`／`annotations` | `JSONB` | 經 allowlist／大小限制的告警 metadata |
| `generator_url`／`dashboard_url`／`panel_url` | `TEXT` | Grafana stable reference |
| `investigation_case_id` | `UUID` | 建立或連結的案件，可為 NULL |
| `disposition` | `VARCHAR(30)` | 建案、連結、忽略或記錄 resolved |
| `received_at` | `TIMESTAMPTZ` | 接收時間，UTC |

Receipt 採 append-only，不使用 `lock_version`。`resolved` receipt 只能新增 Evidence，不得自動將案件
設成 `CLOSED`。HMAC secret、簽章 header 與 raw webhook body 不得存入此表。

### 3.11 Event Check tables（migration 00009）

`EH-ECM-003` 已依 [ADR-006](../decisions/adr-006-check-snapshot-and-finding-persistence.md) 建立：

```text
check_snapshots
├── check_snapshot_event_refs
├── check_snapshot_relations
└── check_findings
      └── check_finding_feedback

investigation_cases
└── investigation_check_snapshots ── check_snapshots
```

- `check_snapshots` 固定 normalized request、Model ID／version／checksum、`as_of`、source health、
  `event_set_hash`、`evaluation_hash` 與 deterministic result；不保存 raw payload。
- `check_snapshot_event_refs` 保存 included／excluded event 的最少 metadata、payload checksum、ordinal 與
  adjustment reason，使 ClickHouse TTL 到期後仍能解釋當時輸入。
- `check_snapshot_relations` 保存 same correlation、same aggregate、causation、business key、parent／child 與
  custom include 的可解釋 provenance。
- `check_findings` 泛化目前 `pattern_findings`，保存 Flow Expectation、Model Rule 與 Global Check 結果；
  `check_finding_feedback` 是唯一 mutable current state。
- Snapshot read response 會把 `check_finding_feedback` 疊加成 `finding_feedback` current projection；此 mutable
  projection 不改寫 immutable result，也不納入 `event_set_hash` 或 `evaluation_hash`。
- 即時 evaluation 的 Finding `id` 為 `null`；保存 Snapshot 時配置 UUID。該 storage identity 不納入
  deterministic `evaluation_hash`。
- `investigation_check_snapshots` 是 N:M link；Case attach 使用 Case optimistic lock 並寫 Audit。

Snapshot、event refs、relations 與 findings 都是 append-only。一般 `POST /event-checks/evaluations` 不建立任何
row；只有明確保存或 Case handoff 才重算並核對 hashes 後 transactionally 落庫。精確欄位、constraints、
RBAC 與 legacy backfill 規則見 [Event Check Target Design](event-check-target-design.md)。

## 4. ClickHouse Event Plane

### 4.1 Canonical event read path（正式預設）

官方 ClickHouse Kafka Connect Sink 先將 Domain Topic record 寫入 restricted raw landing，ClickHouse
Materialized View 再依 admission contract 分流到 promoted event 或 quarantine。Runtime API 與 Grafana
一律查 `canonical_forensics_events` view；`forensics_events` 是舊 promoted table，只保留 migration／rollback
證據，沒有正式 writer。Promoted event append-only。

Canonical promoted event 代表 Kafka 中通過 admission 的事件紀錄，不代表某個 Consumer 已成功處理，也不包含單一的
重試次數。同一事件可以被多個 Consumer Group 各自重試，因此 Consumer 的處理嘗試必須以
`consumer_group_id` 為範圍另行觀測；若來源沒有提供 attempt／錯誤資訊，Event Hunter 不應從
重複的 `event_id` 自行推斷為重試。

```sql
ENGINE = MergeTree
PARTITION BY toYYYYMM(occurred_at)
ORDER BY (correlation_id, aggregate_id, occurred_at, event_id)
```

重要欄位：

```text
event_id, event_type, event_version, occurred_at, producer,
correlation_id, causation_id, trace_id, aggregate_type, aggregate_id,
sequence, kafka_topic, kafka_partition, kafka_offset,
payload, ingested_at
```

設計原則：

- `event_id`、`kafka_topic`、`kafka_partition`、`kafka_offset` 用於來源追蹤與重複分析。
- `payload` 依 PII policy 遮罩或拆出受控欄位；查詢預設不回傳完整 payload。
- 時間欄位使用 UTC；高精度需求使用 `DateTime64`。
- 依查詢與保存政策設定 TTL，不以 UPDATE 取代歷史事件。
- 不使用 row-level optimistic locking；事件不可變才是資料一致性策略。

### 4.1.1 `event_processing_attempts`（MVP supporting read model）

若 Event Check Timeline 要顯示 Consumer 重試，consumer 必須送出 processing-attempt event，經相同
Kafka Connect raw landing + Materialized View admission 寫入 promoted table，runtime 由
`canonical_event_processing_attempts` 查詢。它不是 Event Hunter 的交易 Aggregate，也不取代 domain event。

```text
attempt_id, event_id, event_type, correlation_id, trace_id,
consumer_group_id, consumer_service, attempt, processing_status,
retry_reason, retry_topic, kafka_topic, kafka_partition, kafka_offset,
started_at, completed_at, observed_at
```

設計原則：

- `attempt` 必須在 `event_id + consumer_group_id` 範圍內解讀；不同 Consumer Group 不共用計數。
- `attempt_id` 是來源產生的全域冪等識別碼；相同 `attempt_id` 的 sink redelivery 只計算一次。
- 原始事件與 Retry Topic 可能有不同的 Kafka offset，必須保留來源參照。
- `processing_status` 可使用 `STARTED`、`FAILED`、`RETRY_SCHEDULED`、`SUCCEEDED`、`DLQ`。
- 若沒有這個 read model 或等價的 Grafana／OTel 來源，Event Check 只能顯示事件本身，不能宣稱已辨識重試。

### 4.1.2 Ingestion failure read models

`/ingestion-issues` 將三種安全 failure metadata 合併查詢：歷史 `event_ingestion_failures` contract
validation、event／attempt admission quarantine，以及 Kafka Connect technical DLQ projector 的
`ingestion_technical_failures`。原始 record 只存在 restricted raw landing 或受控 DLQ；一般 API 與
`grafana_reader` 都沒有 raw database 權限，也不回傳 exception message／stack trace。

```text
failure_id, source_topic, source_partition, source_offset,
event_id, event_type, correlation_id, error_type, error_code,
error_summary, payload_sha256, failed_at, observed_at
```

這些 read models 提供 schema/admission/technical failure 的確定性來源。相同 transport delivery identity
的重送只計算一次，不使用 row-level lock。業務上合法的失敗事件仍進 Event Check，不應出現在此處。

### 4.2 `event_quality_metrics`（MVP data plane；Grafana 查詢）

事件品質觀測資料，由獨立的品質聚合工作產生。它是給 Grafana Dashboard／Alerting 與
Investigation Summary 查詢的分析 Read Model，不是交易 Aggregate，也不是 ClickHouse 自動產生的
system table。每列粒度固定為「`[window_start, window_end)` × Topic partition × Consumer Group」。

欄位：

```text
metric_id, window_start, window_end, calculated_at,
topic_name, kafka_partition, consumer_group_id,
event_count, duplicate_count, schema_violation_count,
out_of_order_count, dlq_count, max_event_delay_ms,
consumer_lag_messages, max_processing_latency_ms, source
```

資料語意：

- `event_count`、`duplicate_count`、`schema_violation_count`、`out_of_order_count`、`dlq_count`：只統計該窗口內的觀測值；目前 `schema_violation_count` 仍只聚合 legacy `event_ingestion_failures`，ClickHouse admission quarantine 先由 Ingestion Issues 呈現。
- `max_event_delay_ms`：窗口內最大的 `ingested_at - occurred_at`，代表事件到達分析平台的延遲；不是 Consumer lag。
- `consumer_lag_messages`：窗口結束時，Kafka 最新 offset 與該 Consumer Group committed offset 的差；沒有 broker 指標來源時為 `NULL`。
- `max_processing_latency_ms`：窗口內最大的 Consumer `completed_at - started_at`；沒有 processing-attempt 遙測時為 `NULL`。
- `source`：產生資料的確定性工作版本，例如 `quality-worker-v1`，供調查時追溯計算來源。

目前使用 1 分鐘 tumbling window。事件內生指標從 canonical events 與 processing attempts 計算；
Consumer lag 必須另外讀取 Redpanda／Kafka group metrics，不能從
重複事件推測。聚合器以 append-only 寫入 ClickHouse，Grafana 再以 SQL 做 5 分鐘、1 小時等查詢，
不要把每個品質指標寫成 PostgreSQL row update。

## 5. 樂觀鎖與狀態一致性

不是所有資料表都需要樂觀鎖；只有會被更新的 PostgreSQL Control Plane 資料使用 `lock_version`：

| 資料表 | 寫入模型 | 樂觀鎖 |
|---|---|---|
| `event_definitions` | 更新 Owner、狀態與相容性設定 | `lock_version` |
| `event_schema_versions` | Schema body 不可變；metadata 可更新 | metadata 更新時使用 `lock_version` |
| `topic_registrations` | 更新 Topic 設定 | `lock_version` |
| `topic_subscriptions` | 更新 Consumer 設定 | `lock_version` |
| `investigation_cases` | 更新狀態、指派人、根因與結案資訊 | `lock_version` |
| `pattern_findings` | 分析結果 append-only；重試靠 `idempotency_key` 去重 | 不需要 |
| target `check_snapshots`／event refs／relations／`check_findings` | 明確保存後 append-only；由 evaluation hashes 與 actor-scoped idempotency 去重 | 不需要 |
| target `check_finding_feedback` | 人工分類 current state | `lock_version` |
| target `investigation_check_snapshots` | append-only Case link；attach 同時推進 Case version | link 不需要；Case 使用 `lock_version` |
| `case_evidence` | 證據索引 append-only；唯一鍵去重 | 不需要 |
| `replay_sessions` | 更新 Replay 狀態與核准資訊 | `lock_version` |
| `grafana_alert_receipts` | append-only；`dedup_key` 防止 webhook 重送副作用 | 不需要 |
| `audit_logs` | append-only | 不需要 |
| canonical promoted events | append-only；view 依 transport identity 去除 sink redelivery | 不使用 row-level lock |
| canonical processing attempts | append-only；查詢依 `attempt_id` 去除 sink redelivery | 不使用 row-level lock |
| ingestion failure read models | append-only；查詢依來源 topic／partition／offset 去重 | 不使用 row-level lock |
| `event_quality_metrics` | 追加事件或聚合結果 | 不使用 row-level lock |

標準欄位：

```sql
lock_version BIGINT      NOT NULL DEFAULT 0,
updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
```

更新時必須帶入讀取時的版本：

```sql
UPDATE investigation_cases
SET status       = :new_status,
    root_cause   = :root_cause,
    lock_version = lock_version + 1,
    updated_at   = now()
WHERE id = :id
  AND lock_version = :expected_lock_version;
```

若 affected rows 為 `0`，代表資料已被其他操作者更新；API 回傳 `409 Conflict`，
要求重新讀取後再提交。HTTP API 同時使用：

```http
ETag: W/"investigation-123-lock-v7"
If-Match: W/"investigation-123-lock-v7"
```

狀態轉移也必須檢查原狀態，例如只有 `INVESTIGATING` 才能轉成 `WAITING_APPROVAL`。
Go Repository 使用 `pgx`／`sqlc` 將 `WHERE id = $1 AND lock_version = $2` 寫入更新 SQL，
更新筆數為 0 時回傳版本衝突；禁止使用會繞過樂觀鎖的 bulk update。

## 6. Migration 與開發順序

1. 依檔名執行 [`backend/migrations/postgres`](../../backend/migrations/postgres)：control plane baseline 與 Grafana receipt idempotency。
2. 執行 [`backend/migrations/clickhouse`](../../backend/migrations/clickhouse) 內的 DDL：`MergeTree`、partition、排序鍵與 TTL；唯讀查詢帳號由部署設定建立。
3. 建立 canonical event fixture，驗證 Kafka Sink → restricted raw landing → Materialized View admission →
   `canonical_forensics_events` 的欄位映射與 failure routing。
4. 為每個 Repository 補 Integration Test：交易提交、版本衝突、查詢時間範圍與 PII 遮罩。
5. migration 只能透過 CI／部署流程執行，不在應用程式啟動時偷偷修改 Schema。

Event Check 採獨立 expand／backfill／compatibility／contract 順序：新 tables、compatibility adapter 與
canonical cutover 已通過 `EH-ECM-006`。舊 direct Case／Pattern schema 尚未物理刪除；移除時必須另開
migration release，不得把驗收完成誤解為可立即執行不可逆刪除。

第一版不做 PostgreSQL 與 ClickHouse 的雙寫交易；ClickHouse 是事件分析資料面，
若資料需要修正，應重新產生 append-only 事件或建立新的分析版本，不直接覆蓋歷史事件。
