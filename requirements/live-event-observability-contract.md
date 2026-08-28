# Live Event Observability Contract

- 狀態：`implemented`
- 更新日期：2026-08-28
- 對應需求：`REQ-EH-001` Business Timeline、`REQ-EH-009` Demonstrable Outbox-to-Timeline Event Pipeline
- 適用範圍：`order-service`、`payment-service`、`shipping-service` 的 live transactional-outbox 流程

## 1. 目的

使用者從 Business Timeline 開啟 `Loki logs` 時，必須能直接讀懂一個 Domain Event 在服務內經歷的動作，
而不是只看到沒有事件名稱的通用訊息。Logs 也必須能和 Timeline event、Tempo trace 及 Kafka consumer
位置交叉核對。

本契約是 live SDK telemetry 的規格，不適用於 fixture loader 或 Scenario Lab 的 synthetic telemetry。
Synthetic 資料仍須使用獨立 resource identity 與來源標記，不得冒充 live service logs。

## 2. Outbox 與 Kafka 語意

Demo services 不直接把 Domain Event produce 到 Kafka。服務在自己的 PostgreSQL transaction 中寫入
business state 與 outbox row；transaction commit 後，由 Debezium 讀取 outbox 並發布到 Kafka。

因此 log 的語意固定如下：

| 階段 | 訊息 | 真正代表的結果 |
|---|---|---|
| `PREPARING` | `domain event emission starting: <EventType>` | Event envelope 已建立，即將等待示範延遲並寫入 outbox |
| `COMMITTED` | `domain event committed to outbox: <EventType>` | business state 與 outbox row 已在同一個 local transaction 成功 commit；不等同 Kafka 已送達 |
| `FAILED` | `domain event emission failed: <EventType>` | 已準備的事件未形成 committed outbox row |
| Consumer completed | `processed domain event: <EventType>` | 下游服務已處理 Kafka record，且成功 commit 該 consumer position |

`COMMITTED` 之後是否真的到達 Kafka，由 Debezium connector 狀態、Kafka coordinates、processing attempt
與下游 `processed domain event` 共同判斷，不得只靠 producer-side log 宣稱 end-to-end 成功。

## 3. 發布前後與失敗規則

1. `PREPARING` 必須在 `DEMO_EVENT_EMISSION_DELAY` 之前記錄；本機預設為 `2s`。
2. `COMMITTED` 只能在 `tx.Commit()` 成功後記錄。
3. 已記錄 `PREPARING` 後若流程失敗，必須記錄 `FAILED`，不得留下無原因的半段 lifecycle。
4. `FAILED` 的 `event.emission.failure_stage` 只允許：
   - `PRE_EMISSION_DELAY`
   - `OUTBOX_APPEND`
   - `TRANSACTION_COMMIT`
5. Consumer completed log 只能在 Kafka record 成功 commit 後記錄。
6. 所有 log 必須使用 `slog.InfoContext`／`slog.ErrorContext`，讓 `otelslog` 從 active context 注入
   `trace_id` 與 `span_id`。
7. 相同事件的 lifecycle log、canonical event envelope 與 Tempo span 必須使用同一個 trace ID。

## 4. 結構化欄位

### Producer／outbox lifecycle

| 欄位 | 必填 | 說明 |
|---|---|---|
| `event.id` | 是 | Event envelope 的唯一 ID |
| `event.type` | 是 | 例如 `OrderCreated` |
| `event.producer` | 是 | 建立事件的服務 |
| `correlation.id` | 是 | Business Timeline 的主要串接識別碼 |
| `aggregate.type`／`aggregate.id` | 是 | 事件所屬 Aggregate |
| `event.sequence` | 是 | Aggregate 內的序號 |
| `kafka.topic` | 是 | Outbox 預定路由 topic |
| `kafka.partition`／`kafka.offset` | 是 | Producer-side 固定為 `-1`，因 transaction commit 時 Kafka 尚未配置位置 |
| `kafka.position.known` | 是 | Producer-side 固定為 `false` |
| `event.emission.phase` | 是 | `PREPARING`、`COMMITTED` 或 `FAILED` |
| `event.emission.delay_ms` | `PREPARING` 必填 | 本次發布前等待毫秒數 |
| `event.emission.failure_stage` | `FAILED` 必填 | 失敗階段 allowlist |
| `trace_id`／`span_id` | active span 時必填 | 由 OTel log bridge 注入；不得自行生成另一組 ID |

### Kafka consumer completed

Consumer log 必須包含 `event.id`、`event.type`、`correlation.id`、`kafka.topic`、
`kafka.partition`、`kafka.offset`、`kafka.position.known=true`、`trace_id` 與 `span_id`。

OTLP 寫入 Loki 後 dotted keys 會正規化為 underscore，例如 `event.type` 會成為 `event_type`，
`correlation.id` 會成為 `correlation_id`。Event Hunter deep link 的 LogQL 必須使用 Loki 中的正規化名稱。

## 5. Timeline 與 Grafana 使用契約

Timeline event detail 的 `Loki logs` link 必須：

- 使用事件的 `correlation_id`，並限制在可信的 Grafana origin。
- 帶入以事件時間為中心的有界查詢窗口。
- 優先顯示 order／payment／shipping live service streams。
- 讓使用者能依序看到 `starting`、`committed to outbox` 與下游 `processed` 動作。

建議人工診斷 LogQL：

```logql
{service_name=~"order-service|payment-service|shipping-service"}
  | correlation_id="<CORRELATION_ID>"
  |~ "domain event"
```

## 6. 安全與資料治理

- Log 不得寫入完整 event payload、客戶 PII、付款資料、credential、exception stack 或 raw outbox row。
- 錯誤 log 只記錄必要錯誤與固定 failure stage；一般 Viewer 不透過 Event Hunter API 取得 raw logs。
- Event Hunter 只建立 trusted deep link；查閱權限、retention 與遮罩仍由 Grafana／Loki 環境治理。

## 7. 驗收與回歸

| 層級 | 驗收 |
|---|---|
| Go unit | `backend/internal/demo/telemetry/event_lifecycle_test.go` 驗證三種訊息、phase、failure stage、event/correlation 欄位與 2 秒 delay metadata |
| Live integration | `bash scripts/test-live-observability.sh --skip-restart` 建立真實訂單，驗證三個事件同一 trace，Tempo 有三服務，Loki 有三服務 canonical fields 及每個事件的 `PREPARING`／`COMMITTED` lifecycle |
| Restart | `bash scripts/test-live-observability.sh` 重啟 ClickHouse、Loki、Tempo、Collector 與三個服務後重查同一批 telemetry |
| Event pipeline | `e2e/backend/event-pipeline.feature` 驗證 Order → Outbox → Debezium → Kafka → ClickHouse → Timeline 的三事件結果 |
| Frontend | `e2e/frontend/investigation-flow.feature` 驗證 Timeline event detail 的 trusted Loki／Tempo／Grafana links |

最近一次人工 runtime probe（2026-08-28）使用 `ORDER-5142B16E6C050DE8`，產生
`OrderCreated → PaymentCompleted → ShipmentCreated`，Loki 查到具名 lifecycle／consumer logs，且
ClickHouse、Tempo 與 Loki 共用 trace `9c15e072bdf21fb3f93e02858490aee9`。此 ID 僅是本機驗證證據，
不得作為測試 fixture 或長期資料依賴。
