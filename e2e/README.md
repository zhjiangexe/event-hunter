# Event Hunter E2E Contracts

Karate feature files are executable acceptance contracts. They assume the canonical fixtures under
`../contracts/fixtures` have already been loaded into the local ClickHouse test database.

```text
e2e/backend   HTTP contract、RBAC、Pattern、樂觀鎖、Summary、Evidence manifest
e2e/frontend  由 data-testid 鎖定的瀏覽器 happy path
e2e/poc       明確執行的 ClickHouse-first ingestion infrastructure acceptance
artifacts/e2e/karate  Karate HTML／JSON 測試報告（不與 feature files 混放）
```

日常只保留 `backend/` 與 `frontend/` 兩份最新完整報告。Karate browser profile、編譯輸出與歷史
tag/debug reports 可安全清除，不包含 fixture 或 persistence volume：

```bash
bash scripts/clean-generated-artifacts.sh            # target、dist、Python cache
bash scripts/clean-generated-artifacts.sh --reports  # 再移除非 canonical 歷史報告與 legacy Maven 產物
```

Runtime properties:

```text
-Dapi.baseUrl=http://localhost:28333
-Dweb.baseUrl=http://localhost:28334
-Dorder.baseUrl=http://localhost:28335
-DeventLab.baseUrl=http://localhost:28343
-DkafkaConnect.baseUrl=http://localhost:28324
-DredpandaConnect.baseUrl=http://localhost:28325
-Dclickhouse.httpUrl=http://localhost:28317
-Dclickhouse.user=event_hunter
-Dclickhouse.password=event_hunter_local_only
-Dgrafana.baseUrl=http://localhost:28332
-Dgrafana.user=admin
-Dgrafana.password=admin_local_only
-Dgrafana.webhookSecret=grafana_webhook_local_only
```

## 執行 Backend E2E

本專案使用 Karate feature files 搭配 standalone `karate.jar`，不建立 Java test class、Maven project
或 Gradle project。Karate v2 CLI 需要 Java 17+。

```bash
curl -L -o /tmp/karate-2.1.2.jar \
  https://github.com/karatelabs/karate/releases/download/v2.1.2/karate-2.1.2.jar

java -jar /tmp/karate-2.1.2.jar run \
  --no-pom \
  --env=local \
  --configdir=e2e \
  --threads=1 \
  --output=artifacts/e2e/karate/backend-<feature> \
  e2e/backend/<feature>.feature
```

逐一指定 feature 執行時，應分別執行 `e2e/backend/*.feature`；這樣可以讓有界的
`event-pipeline.feature` retry window 與其他 feature 的結果分開記錄。該 feature 先驗證三個
Debezium tasks、兩個 ClickHouse Sink connectors 與 technical DLQ projector，再以 20 秒等待首筆
Order event、40 秒等待完整三服務事件鏈；`scripts/test-backend-e2e.sh` 會先執行同一 capability-level
readiness gate。
所有輸出都應位於
`artifacts/e2e/karate/`，`e2e/` 僅保留 feature files 與 Karate config。

`@requires-fixtures` 場景的固定資料：

- `ORDER-1001`：正常 Order → Payment → Shipment，共 3 筆事件。
- `ORDER-2001`：Order → Payment，超過 5 分鐘仍無 Shipment，共 2 筆事件。
- `ORDER-3001`～`ORDER-3003`：付款後取消、退款或 void，皆不應產生缺少出貨 finding。
- `ORDER-4001`：付款拒絕後取消訂單，驗證 `PaymentFailed` 與跨 Aggregate causation。
- `ORDER-4002`：建立出貨、派車、運送中到簽收的完整物流生命週期。
- `ORDER-4003`：派車失敗後重試成功並送達，驗證 Shipment sequence 仍單調遞增。
- `ORDER-4004`：送達後申請退貨、退貨入庫再退款，驗證逆物流與付款補償順序。

實作不得為了通過測試而建立測試專用業務分支；測試資料應經與正式 ingestion 相同的欄位映射寫入。

Infrastructure scenarios：

- `event-pipeline.feature` 從真實 Demo Order API 經 Outbox、Debezium、Redpanda、ClickHouse Sink／Materialized Views 到 Timeline。
- `quality-metrics.feature` 執行前先載入 `quality-window.json`，再執行 production `quality-worker aggregate --from ... --to ...`。
- `grafana-alert-webhook.feature` 從 JSON fixture 產生每次唯一 payload，並對實際送出的完整字串計算與 Grafana 相同的 timestamped HMAC-SHA256。
- `grafana-auto-case.feature` 不直接呼叫 webhook：它寫入唯一 terminal DLQ attempt，等待 provisioned Grafana rule → policy → HMAC Contact Point 自動建案，再寫入 success attempt 驗證 resolved Evidence 與不自動結案。
- `observability-deep-links.feature` 透過 Grafana API smoke-test 4 個 datasource UID、Event Quality Dashboard UID 與實際 Alert rule UID，避免 UI 產生指向不存在資產的連結。
- `event-scenarios.feature` 驗證四組擴充業務序列、payload／causation／aggregate sequence，並直接確認 Tempo trace 與 Loki synthetic logs。
- `payload-security.feature` 以隔離 ClickHouse probe 驗證 ADMIN-only payload authorization 與巢狀 object／array 遞迴遮罩，結束後同步刪除 probe，避免污染總覽與後續測試。
- `scenario-lab.feature` 實際執行 S1～S14：S1、S12～S14 呼叫三個 live services；S2～S11 經隔離 `event-lab.events`，所有 PASS／FAIL 由 ClickHouse actual、processing attempts 或 ingestion failures 回查判定。
- `ingestion-issues.feature` 以唯一 topic 建立 contract、admission、technical 三類安全摘要，驗證 filter、keyset pagination、7 天上限與回應中沒有 raw payload、exception message 或 stack trace。
- `investigation-boundaries.feature` 驗證 cursor 分頁、query 邊界、案件狀態機、Viewer 全 mutation surface 唯讀與一致 404；自行建立的 OPEN 案件會在 scenario 尾端透過正式 API 結案。

`e2e/poc/clickhouse-mv-ingestion.feature` 與 `clickhouse-mv-processing-attempt.feature` 不屬於預設 API backend suite。它們由
`bash scripts/test-clickhouse-mv-poc.sh` 明確啟動正式 ingestion dependencies，驗證八筆 admission 輸入的全量
raw landing、4 筆 promotion、4 筆 quarantine（包含已知事件缺少 required payload keys 的
`SCHEMA_VIOLATION`），另送一筆 tombstone 驗證真實 technical DLQ；同時覆蓋
raw RBAC、processing-attempt valid／failure 分流、transport identity 守恆與 connector restart。
報告只寫入 `artifacts/e2e/karate/clickhouse-mv-poc/`。

Ingestion 功能恢復與有界 raw purge：

```bash
bash scripts/test-clickhouse-mv-functional-recovery.sh
bash scripts/test-clickhouse-mv-candidate-only-recovery.sh
bash scripts/test-clickhouse-mv-raw-purge.sh
bash scripts/load-clickhouse-mv-candidate-fixtures.sh
```

最後一個命令只保留既有自動化相容性；實際會呼叫 canonical fixture loader，依
`contracts/platform/ingestion-mapping.yaml` 直接寫入正式 promoted tables，不再從 legacy tables 鏡像。

`test-clickhouse-mv-candidate-only-recovery.sh` 要求 domain-event 與 processing-attempt canonical sources
都已切到 `clickhouse-mv`，並驗證新建 baseline 與 ClickHouse outage backlog 都只出現在 promoted
tables、舊 tables 維持零新增、API readiness 為 200→503→200。兩類資料皆由同一個官方 ClickHouse
Kafka Connect worker 的兩個獨立 connectors 傳輸。

完整 `scripts/test-backend-e2e.sh` 保留每個 window 300 requests 的產品限制，只在測試期間把 fixed window
縮短為 10 秒，避免 108 scenarios 共用同一個本機 IP 時互相污染；腳本結束會自動還原原 runtime window。
fixture mirror 腳本只處理九組固定 synthetic correlations，不能當成 live ingestion 驗收。

目前 Backend 基準為 18 個 feature、108 個 runtime scenarios；案例選擇與分層原則見
`../requirements/backend-e2e-test-plan.md`。

品質聚合的 fixture → production worker → Karate acceptance 可用單一腳本重現：

```bash
bash scripts/test-quality-e2e.sh
```

## 執行 Frontend E2E

Frontend UI acceptance 使用同一個 standalone Karate v2 JAR：

```bash
bash scripts/test-frontend-e2e.sh
```

預設使用 `/tmp/event-hunter-karate-2.1.2.jar`，也可用 `KARATE_JAR` 覆寫；報告預設寫入
`artifacts/e2e/karate/frontend/`。

案件 production navigation 可獨立回歸：

```bash
java -jar /tmp/event-hunter-karate-2.1.2.jar run --no-pom --configdir=e2e \
  --output=artifacts/e2e/karate/frontend-eh010 --tags=@eh-p1-1-010 \
  e2e/frontend/investigation-flow.feature
```

此標籤覆蓋 list → detail URL、direct URL、reload、back／forward、drawer close、404，以及
空白結案欄位的 cancel／confirm；目前標籤基準為 4/4 passed。`@eh-p1-1-013` 另覆蓋 Timeline／Journey
空白頁最近 72 小時預設，以及 Timeline search → URL → reload → back／forward；`@eh-p1-1-014` 覆蓋案件 baseline/current-view window 與
Grafana window source；`@eh-p1-1-015` 覆蓋案件內 all-active Pattern、Evidence/Audit refresh 與 reload
還原；`@eh-p1-1-016` 覆蓋 API `allowed_transitions`、WAITING_APPROVAL 進出、Resolve／Close 必填表單、
重新開啟與 terminal CLOSED。可用以下命令獨立回歸：

```bash
java -jar /tmp/event-hunter-karate-2.1.2.jar run --no-pom --configdir=e2e \
  --output=artifacts/e2e/karate/backend-eh016 --tags=@eh-p1-1-016 \
  e2e/backend/investigation-boundaries.feature
java -jar /tmp/event-hunter-karate-2.1.2.jar run --no-pom --configdir=e2e \
  --output=artifacts/e2e/karate/frontend-eh016 --tags=@eh-p1-1-016 \
  e2e/frontend/investigation-flow.feature
```

目前完整 Backend 基準為 108/108、Frontend 基準為 25/25 passed。

`@p1-1-02-03` 覆蓋後端複合篩選、stable keyset sorting、cursor 排序綁定與可分享 Investigation URL；
目前 Backend 標籤基準為 7/7、Frontend 標籤基準為 1/1。

`@eh-p1-1-012` 另覆蓋 dialog 的 role／aria-modal、初始焦點、Escape 與 focus restore，以及 390px 下
Timeline、Overview、Investigations、Pattern Library、Scenario Lab 的 document-level overflow。

## E2E 資料清理

完整 release gate 會記錄起始時間，完成後執行：

```bash
bash scripts/cleanup-e2e-data.sh --since <RFC3339-gate-start>
```

測試建立的案件 title 一律使用 `[E2E]` 前綴。清理腳本只刪除明確標記或 gate 時窗內的測試資料，
涵蓋案件子資料、Grafana webhook receipts、Scenario runs／Saved searches、三個 demo service 資料庫及
ClickHouse probe rows；Tempo／Loki 依 retention 管理，不以廣泛刪除破壞 live telemetry。清理完成後由
release gate 還原 interactive fixtures。HTML／JSON 報告只寫入 `artifacts/e2e/karate/`，不得混入 feature 目錄。

若要清除舊版測試留下、尚未一致加上 `[E2E]` 標記的本機資料，可在確認切點晚於固定 fixture
（目前 fixture 全部位於 2026-08-20）後使用：

```bash
bash scripts/cleanup-e2e-data.sh --since <fixture-safe-cutoff> --all-since
```

`--all-since` 會刪除切點後建立的全部案件及其子資料，僅供本機維護；release gate 預設仍使用較保守的
marker-based 模式。
