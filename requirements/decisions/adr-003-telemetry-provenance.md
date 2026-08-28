---
document_id: EH-ADR-003
status: adopted
owner: platform
last_reviewed: 2026-08-28
source_of_truth: true
canonical_topic: telemetry-provenance
supersedes: []
---

# ADR-003：區分 live、synthetic 與 optional telemetry

## 決策

Event Hunter 使用三種明確分流的 telemetry provenance：

- `live SDK telemetry`：order／payment／shipping 服務透過 OTel SDK、`otelhttp`、`kotel` 與 `otelslog`
  自然產生 traces、logs、metrics；這是正式 deep link 與 live E2E 的依據。
- `synthetic fixture telemetry`：可重播的 E2E／展示資料，必須標記 synthetic，不可冒充真實服務執行結果。
- `optional otelc profile`：保留為 compile-time instrumentation 實驗，不取代正式 Docker build。

所有來源都可送入 Event Hunter 使用的 OTel Collector，但查詢、文件與驗收必須保留 provenance。不可只靠
fixture loader 讓 Tempo／Loki 有資料，然後宣告 live service instrumentation 已完成。
