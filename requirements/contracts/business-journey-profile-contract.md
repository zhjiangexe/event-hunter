---
document_id: EH-DOC-CON-001
status: superseded
owner: product
last_reviewed: 2026-08-29
source_of_truth: false
canonical_topic: business-journey-profile
supersedes: []
---

# Business Journey Profile Contract

> **Superseded compatibility contract.** Canonical flow authoring and evaluation 已移至
> [Event Check 與 Check Models 產品需求](../product/event-check-and-check-models-requirements.md)及
> [Event Check Target Design](../architecture/event-check-target-design.md)。本文件只用來解釋 deprecated
> `/journey`、`/journey-profiles` API／route 與 migration evidence，不得用來新增 Journey Profile。

更新日期：2026-08-26

## 目的

Business Journey 不會從目前事件「預測未來」，也不會自行猜測每個領域的正確流程。它用一份經
domain owner／平台治理人員審查、可版本控管的 Journey Profile，將查詢時間窗內已存在的 canonical
events 解讀成 milestone、expected event、狀態與確定性缺漏。

第一份正式 profile 是
[`contracts/journeys/order-fulfillment.yaml`](../../contracts/journeys/order-fulfillment.yaml)。目前 runtime
只選用唯一的 `active + default` profile；`/journey-profiles` 提供目前 API build 實際載入版本的唯讀列表與詳細抽屜，
多 profile 的查詢版本選擇規則與線上編輯／審核／發布 UI 尚未開放。

## Authoring 與 runtime 邊界

```text
contracts/journeys/*.yaml              人工審查、Git-managed authoring source
        │
        ├── journey-profile.schema.json 結構與欄位限制
        ├── validate-contracts.py       event schema、唯一 ID、唯一 default 與 drift 驗證
        ▼
generate-journey-registry.py
        ▼
domain/journeys/registry_generated.go  production runtime 的 immutable registry
        ▼
business_journey.Service               查詢 canonical events 並依規則組織結果
        │
        └── journey_profiles.Service   列出目前 build 的唯讀 registry 與來源校驗資訊
```

正式 Docker image 不在啟動時讀取 workspace YAML。修改 YAML 後必須重新產生 registry、通過驗證並
重新建置映像，因此設定變更可審查、可重現，也不會因 mount 遺失而靜默改變流程語意。
Generator 另將 repo-relative `source_path` 與原始 YAML bytes 的 SHA-256 寫入 registry，讓 UI 與 API
能辨識目前載入的是哪份來源；checksum 是來源識別資訊，不是線上發布簽章。

```bash
python3 scripts/generate-journey-registry.py
python3 scripts/generate-journey-registry.py --check
python3 scripts/validate-contracts.py
```

## 規則語意

- `milestones`：定義顯示順序、穩定 ID、label、此 milestone 認得的 `expected_event_types`，以及狀態規則。
- `journey_state_rules`：依 YAML 順序比對；第一條符合者決定整體狀態。沒有規則符合但已有事件時為 `IN_PROGRESS`；沒有事件時為 `EMPTY`。
- `when_any_event_types`：至少出現其中一種事件才符合。
- `unless_any_event_types`：若其中任一事件已出現，該規則不符合，可表達「派送失敗但後續已恢復」。
- `anomaly_rules`：trigger 已出現、`required_any_event_types` 全部缺少，且 grace period 已成熟時才產生 anomaly。
- `grace_period_seconds`：以時間窗結束時間和最早 trigger event 的 event time 比較；`0` 代表立即判斷。
- `data_quality.detect_duplicate_event_ids`：啟用跨領域通用的重複 event ID 檢查。

狀態規則與里程碑事件清單是兩個不同概念：狀態規則可查看同一 correlation 的完整事件集合，
但 milestone 的 `actual_event_types`／`events` 只收錄該 milestone 的 `expected_event_types`。例如只有
`OrderCreated → PaymentCompleted → ShipmentCreated` 時，Delivery 的規則因 `ShipmentCreated` 而成為
`IN_PROGRESS`，但 Delivery 尚無自己的 `ShipmentDelivered`；Return 未出現 `ReturnRequested`／
`ReturnReceived` 時為 `NOT_APPLICABLE`。前端必須說明這是「前置事件已啟動、正在等待本階段事件」或
「支線尚未觸發」，不得只用「沒有實際事件」造成資料遺失的誤解。

Profile 內引用的每個 event type 都必須已有 `contracts/events/*.schema.json`；拼錯名稱或引用未治理事件
會讓 contract validation 失敗。API 回應包含 `profile_id`、`profile_version`、`profile_title`，讓使用者
知道此次 Journey 是依哪個版本解讀。

`GET /api/v1/journey-profiles` 與前端 `/journey-profiles` 會列出 profile version、status、default、
milestones、anomaly rules、data-quality flags、source path 與 checksum。前端列表只呈現可比較的摘要欄位；
點選後以 `?profile={id}@v{version}` 開啟右側詳細抽屜，顯示完整里程碑、規則與 YAML provenance，因此可
直接連結、重新整理並配合瀏覽器返回。這是觀測目前 runtime contract 的 read model，不提供 mutation，
也不會在瀏覽器內重新推導 Journey 狀態。

## 治理責任

- Domain owner 定義「合理流程、完成／失敗／補償條件、grace period」。
- Event platform owner 確認 canonical event schema、命名與資料可取得性。
- Event Hunter 只執行確定性規則，不宣稱 YAML 是 production system 的真正狀態機，也不補送缺少事件。
- 每次 profile 變更至少補 Go 規則測試；若改變使用者可觀察結果，同步更新 Karate Business Journey E2E。

## 目前限制與後續擴充

目前只有 `order-fulfillment@v1` 且為唯一 default；唯讀 Registry 已可在 UI 檢視。要支援保險理賠、倉儲入庫或其他 journey 時，可以新增
YAML，但在啟用第二份 active profile 前，仍需先定義可靠的 profile selection contract（例如顯式
`profile_id`、受治理的 domain key，或事件 envelope metadata）。系統不會只靠 correlation ID 字串猜測
領域。Runtime profile 編輯、版本選擇、審核與發布仍是後續產品化工作。
