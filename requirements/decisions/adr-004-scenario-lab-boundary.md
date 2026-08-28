---
document_id: EH-ADR-004
status: adopted
owner: product
last_reviewed: 2026-08-28
source_of_truth: true
canonical_topic: scenario-lab-boundary
supersedes: []
---

# ADR-004：Scenario Lab 不取代 live domain services

## 決策

Scenario Lab 是可重現情境的 orchestration 與觀察入口，不是 order／payment／shipping 的替代實作。

- Live scenarios 驅動三個 demo services，驗證 HTTP、transactional outbox、Debezium、Kafka、ClickHouse
  與 OpenTelemetry 的自然路徑。
- Isolated scenarios 發送到隔離 topic，用於可控的格式錯誤、缺事件、順序或 failure routing 測試。
- Scenario 名稱、輸入與預期結果可以固定；Actual、checks、事件與 telemetry 必須由後端實際觀察產生。
- Scenario 啟動 API 立即回傳 Run ID 與 Correlation ID；Event ID／Trace ID 只有在事件或 trace 真正產生後
  才可由查詢頁看到。

移除三個 demo services 會失去最重要的跨服務 outbox／Kafka／OTel vertical slice，因此不採用。
