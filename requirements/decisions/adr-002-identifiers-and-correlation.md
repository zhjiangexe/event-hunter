---
document_id: EH-ADR-002
status: adopted
owner: platform
last_reviewed: 2026-08-28
source_of_truth: true
canonical_topic: identifier-and-correlation-model
supersedes: []
---

# ADR-002：識別碼與關聯模型

## 決策

Event Hunter 保存來源系統提供的識別碼，不把它們壓成同一個概念：

| 識別碼 | 生命週期 | 產生者 |
|---|---|---|
| Event ID | 一筆不可變事件 | producer／outbox |
| Trace ID | 一次 distributed trace | OpenTelemetry SDK／上游 request context |
| Correlation ID | 一段商業旅程，可跨多次 request 與 trace | 業務系統 |
| Aggregate ID | 一個 domain aggregate | 業務系統 |
| Scenario Run ID | 一次 Scenario Lab 執行 | Event Hunter Scenario Lab |

因此同一個 Correlation ID 可以對應多個 Trace ID，每個 Trace 又可以包含多個 spans 和多筆事件；同一筆
Event ID 則只能代表一筆 canonical event。查詢層可沿任一識別碼切入，但不得假設 Correlation ID 與
Trace ID 永遠一對一。

Scenario Lab 的啟動 response 只保證立即回傳 Run ID 與 Correlation ID。Trace ID 必須等 live service
實際建立 trace／event 後才存在，前端不為了補 Trace ID 執行隱藏式 750ms polling。
