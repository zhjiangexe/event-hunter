# Event Hunter 本機運維 Runbook

## 1. 適用範圍與安全界線

本文件是 Phase 1.1 本機 Compose 環境的可執行 Runbook，涵蓋啟停、健康檢查、故障定位、restart
persistence、cold backup／restore、retention 與 secret rotation。它不是正式 production DR 設計；正式環境
仍需獨立核定 HA、Kafka replication factor、Secret Manager、加密備份、RTO／RPO 與存取稽核。

操作分級：

| 等級 | 操作 | 是否中斷／破壞資料 |
|---|---|---|
| Safe | status、logs、readiness、runbook smoke | 不寫入、不重啟 |
| Controlled | `dev-up`、connector re-register、service restart、fixture reload | 可能短暫中斷或追加 synthetic data |
| Destructive | restore、`down --volumes`、修改 retention 後清理 | 會覆寫或刪除資料，執行前必須確認備份與目標 volume |

設定的 source of truth 是 `compose.yaml`、`.env.example`、`infra/`、`backend/migrations/`、
`contracts/platform/` 與 `scripts/`。不得只在 container 內手改設定，否則重建後會消失。

## 2. 標準啟動、停止與日常確認

首次啟動或重新建置：

```bash
cp .env.example .env
bash scripts/dev-up.sh
```

日常狀態與唯讀 smoke：

```bash
bash scripts/dev-status.sh
bash scripts/verify-operations-runbook.sh
```

Smoke 的成功條件是：Compose 設定可解析、具名 volumes 存在、API／Event Lab／Frontend 可達、三個
Debezium tasks 與兩個 ClickHouse Sink tasks 均為 `RUNNING`、technical projector ready、Grafana 的四個
datasources／dashboard／七條 alert rules、HMAC Contact Point 與 Notification Policy 均可查詢。

停止但保留資料：

```bash
bash scripts/dev-down.sh
```

`dev-down.sh` 不帶 `--volumes`；下次 `dev-up.sh` 會重套冪等 migration、補建 topics、註冊 connectors
並還原互動式 fixtures。

## 3. 健康與 readiness 判讀

```bash
curl -i http://localhost:28333/health/live
curl -i http://localhost:28333/health/ready
curl -i http://localhost:28343/health/ready
bash scripts/verify-event-pipeline-readiness.sh
```

- `/health/live=200` 只表示 API process 存活。
- `/health/ready=200` 才表示 PostgreSQL、ClickHouse、domain-event ingestion 與 processing-attempt ingestion
  均可用；依賴中斷時預期回 `503`，不能把它當成應用 crash。
- Overview 的 source status 可把 Tempo／Loki／Grafana 的 partial/unavailable 與真的零筆資料區分。

常用唯讀診斷：

```bash
docker compose logs --since 10m event-hunter-api
docker compose logs --since 10m kafka-connect
docker compose logs --since 10m kafka-connect-clickhouse-poc technical-dlq-projector
curl -s http://localhost:28324/connectors?expand=status
curl -s http://localhost:28345/connectors?expand=status
curl -s http://localhost:28346/health/ready
```

## 4. 受控重啟與保存驗證

完整受控演練會中斷本機服務，必須明確確認：

```bash
bash scripts/verify-operations-runbook.sh --restart --yes
```

它會依序重啟 stateful stores、connectors／consumers、demo services、control plane，重新註冊 Debezium，
再比較 PostgreSQL row count、ClickHouse row count、Redpanda topics，並要求 API 留下 graceful shutdown
完成訊息。不得以 `docker compose down --volumes` 代替 restart。

## 5. 常見故障與回復順序

| 症狀 | 先確認 | 建議回復 |
|---|---|---|
| API live=200、ready=503 | `verify-event-pipeline-readiness.sh` 與 API logs | 先恢復 PostgreSQL／ClickHouse／Redpanda，再恢復 Connect；不要先重建 API 掩蓋根因 |
| Debezium task FAILED／不存在 | `GET :28324/connectors?expand=status` | `bash scripts/register-debezium-connectors.sh`，再跑 readiness checker |
| Sink task FAILED／offset 不前進 | `GET :28345/connectors?expand=status`、broker 與 ClickHouse | 先修 ClickHouse／converter，再執行 `bash scripts/register-clickhouse-mv-poc.sh` 並重驗 readiness |
| Timeline 有 event、Tempo／Loki no data | event 的 `trace_id`、live/synthetic namespace、查詢時間窗 | 先確認 OTel Collector、Tempo、Loki；用 `bash scripts/test-live-observability.sh --skip-restart` 產生 live probe |
| Quality Dashboard 無新窗口 | Quality Worker `/health/ready` 與 logs | 恢復 ClickHouse 後重啟 worker；需要補算時使用 `quality-worker backfill`，不可寫假 metrics |
| Grafana link 開啟但資產不存在 | provisioning files 與 Grafana API | `bash scripts/verify-grafana-provisioning.sh`；修 repo provisioning 後再重建 Grafana |
| Scenario Lab `TIMED_OUT` | run checks、event pipeline readiness、ClickHouse failures | 先修 ingestion，再重跑；不可直接把 run status 改成 PASSED |
| Ingestion Issues 出現 `TECHNICAL_DLQ` | issue drawer 的 source/DLQ coordinates、connector/task/stage/class | 先修 converter／Sink 或 ClickHouse；以 SHA-256 和受控原始來源核對，不把 raw payload 複製到一般案件或 Grafana |

### 用事件 lifecycle logs 判斷卡點

從 Timeline event detail 開啟 `Loki logs`，或在 Grafana Explore 使用：

```logql
{service_name=~"order-service|payment-service|shipping-service"}
  | correlation_id="<CORRELATION_ID>"
  |~ "domain event"
```

| 看到的最後證據 | 判讀與下一步 |
|---|---|
| 只有 `domain event emission starting` | 查同一 `event_id` 是否有 `FAILED` 與 `event_emission_failure_stage`；優先檢查 request cancellation、outbox DB 與 transaction commit |
| 有 `committed to outbox`，沒有下游 `processed domain event` | Local transaction 已成功；檢查 Debezium connector、Kafka topic、consumer group lag，不要誤判為服務沒有產生事件 |
| 有下游 `processed domain event`，Timeline 仍無事件 | Kafka delivery 已成立；檢查 ClickHouse Sink、raw admission、quarantine 與 canonical views |
| Timeline 與 Loki 都有資料，Tempo 無 trace | 核對同一 `trace_id` 後檢查 OTel Collector trace exporter 與 Tempo；不要改用新的 correlation ID 掩蓋 context propagation 問題 |

Producer-side `kafka_partition=-1`／`kafka_offset=-1` 且 `kafka_position_known=false` 是正確狀態，因為
Debezium 尚未為 committed outbox row 配置 Kafka 位置。完整語意見
[Live Event Observability Contract](live-event-observability-contract.md)。

故障注入只在允許中斷時執行：

```bash
bash scripts/test-ingestion-recovery.sh --yes
bash scripts/test-ingestion-acknowledgement.sh --yes
```

兩支腳本都有 EXIT recovery；即使測試失敗也會嘗試恢復被停止的 Redpanda／ClickHouse 與 ClickHouse Sink。

`dev-up.sh` 會在載入 fixtures 前要求兩個正式 Sink connectors、所有 tasks 與 technical projector 都 ready，
並確認 domain／attempt canonical views 均指向 `clickhouse-mv`。HTTP process 存活但 task `FAILED` 不算 ready。

### ClickHouse-first ingestion 的受控操作

此路線是正式預設。工具名稱中的 `poc`／`candidate` 是 offsets 與既有自動化的相容識別：

```bash
bash scripts/clickhouse-mv-poc-up.sh
bash scripts/test-clickhouse-mv-poc.sh
bash scripts/test-clickhouse-mv-functional-recovery.sh
bash scripts/test-clickhouse-mv-candidate-only-recovery.sh
bash scripts/test-clickhouse-mv-raw-purge.sh
bash scripts/reconcile-ingestion-mode.sh --mode status
bash scripts/reconcile-processing-attempt-ingestion-mode.sh --mode status
```

`test-clickhouse-mv-functional-recovery.sh`／`test-clickhouse-mv-candidate-only-recovery.sh` 會暫停 ClickHouse，
預期 API readiness 回 503，再重啟 ClickHouse、官方 Sink 與 projector，驗證 domain 與 attempts 的 outage
backlog 均可恢復。兩者只可在允許中斷的本機／CI 執行；不會切回歷史 tables。Redpanda broker不可停止，
因為新 Sink、Debezium 與 demo consumers 仍透過 Kafka topics 傳遞資料。

完整 Backend E2E 會在保留每 window 300 requests 的前提下，暫時把本機 fixed window 縮短成 10 秒，
結束後自動還原原設定。`load-domain-fixtures.py` 只載入固定 synthetic fixtures 到 promoted tables；
正式 live 驗收仍須新建訂單並觀察 Kafka → raw → promoted → canonical Timeline。

raw landing 不提供一般查詢。人工清除先 dry-run：

```bash
bash scripts/purge-clickhouse-mv-raw.sh \
  --from 2026-08-20T00:00:00Z \
  --to 2026-08-21T00:00:00Z

bash scripts/purge-clickhouse-mv-raw.sh \
  --from 2026-08-20T00:00:00Z \
  --to 2026-08-21T00:00:00Z \
  --execute --yes
```

單次窗口最多 24 小時，且結束時間必須距今至少一小時；命令只刪 `event_hunter_poc` raw landing，不刪
promoted/failure read models。未來若提供 privileged raw reader，必須使用獨立角色、限時核准、來源限制、
Secret Manager 與 query audit，不得擴大 `grafana_reader`。

## 6. Cold backup

完整本機備份採「停止服務後逐一封存具名 volume」，避免 PostgreSQL／ClickHouse／Redpanda 在 snapshot
期間仍寫入而產生不一致。先確認目前沒有不可中斷的工作：

```bash
bash scripts/dev-down.sh
bash scripts/backup-local-volumes.sh --output backups/event-hunter-20260824
```

備份工具拒絕覆寫既有目錄，也會拒絕在任何 Compose service 仍運行時執行；它只處理 allowlist 中的
`event-hunter_*_data` volumes，並產生 `SHA256SUMS`、resolved Compose 與 `.env.example` snapshot。
備份可能包含 CONFIDENTIAL payload、案件與本機 credentials，必須加密保存且限制存取。完成後執行：

```bash
bash scripts/dev-up.sh
bash scripts/verify-operations-runbook.sh
```

## 7. Restore 演練

Restore 會覆寫現有本機資料，必須在隔離環境或已確認可丟棄的 workspace 執行。最低程序：

1. 驗證 `shasum -a 256 -c SHA256SUMS` 全部通過。
2. 執行 `bash scripts/dev-down.sh`，並再次確認 `docker compose --profile temporal ps -q` 為空。
3. 對照 backup manifest，只重建明確的 `event-hunter_<logical-name>` 具名 volumes；不得對不明 volume
   使用 wildcard 或遞迴刪除。
4. 將對應 `*.tar.gz` 解到空 volume 根目錄；不可疊加到仍有資料的 volume。
5. 執行 `bash scripts/dev-up.sh`，再跑 `verify-operations-runbook.sh --restart --yes`。
6. 核對案件、Timeline、Scenario Lab、Grafana assets，以及至少一條既有 trace/log deep link。

本 repo 暫不提供一鍵 destructive restore script，避免誤把工作中的 volume 清空。若只是建立全新 demo
環境，優先讓 migration + topic bootstrap + fixture loader 重建，不需要還原舊 runtime telemetry。

## 8. Retention 與容量

| 資料 | 目前本機政策 | Source of truth |
|---|---|---|
| Canonical events、attempts、contract failures、quality／consumer metrics | 90 天 TTL | ClickHouse migrations、`data-classification.yaml` |
| ClickHouse-first raw landing | 7 天 TTL；另有 24 小時有界 purge | `00006_clickhouse_mv_ingestion_poc.sql`、`purge-clickhouse-mv-raw.sh` |
| Admission quarantine、technical DLQ safe summary | 30 天 TTL | ClickHouse migrations `00006`、`00008` |
| Kafka domain／lab／DLQ topics | 7 天 | `topic-topology.yaml` |
| Prometheus | 7 天 | `compose.yaml` |
| Tempo traces | 7 天 | `infra/tempo/tempo.yaml` |
| Loki logs | 目前未設 time-based retention | `infra/loki/loki-config.yaml` |
| PostgreSQL cases／audit／Scenario runs | 尚未設自動 TTL | PostgreSQL migrations |

Loki 與 PostgreSQL 的 production retention 是已知產品化決策點，不得默認永久保存。變更 retention 前先核定
法遵、調查時窗、Evidence 可重現性與容量；ClickHouse 資料超過 TTL 後，Pattern 無法重建原始證據，會回
`NO_EVENTS`。

## 9. Secret rotation

`.env.example` 只有本機預設值。正式 secret 不得 commit；應由 Secret Manager／Vault 注入。輪替順序：

1. 建立新 secret 並更新所有 producer/consumer 端，不先撤銷舊值。
2. 對受影響服務執行受控 recreate；僅 `restart` 不會重新讀取 Compose environment。
3. 跑 `verify-operations-runbook.sh` 與 live observability probe。
4. 確認新連線、Grafana datasource、webhook HMAC 正常後才撤銷舊值。

`POSTGRES_PASSWORD`／demo DB 密碼／ClickHouse 密碼的 rotation 不只是改 `.env`，還要先在資料庫建立或
修改 credential；`DEMO_SESSION_SECRET` 會使既有 session 失效；`GRAFANA_WEBHOOK_SECRET` 必須與 Grafana
Contact Point 同步。Grafana ClickHouse reader 目前由 init SQL 建立，本機更改後需同步 provisioning password。

## 10. Release 與交接

一般變更先跑非破壞 gate；release 候選再跑完整 gate：

```bash
bash scripts/test-phase-1-exit.sh --no-start --skip-disruptive
bash scripts/test-phase-1-exit.sh --no-start
```

完整 gate 成功後確認 `build/reports/phase-1-exit-summary.json` 為 `passed`，且 E2E cleanup 後沒有
`[E2E]` 案件或本輪 Scenario runs 殘留。Phase 1.1 的正式基準與已接受風險記錄在
`requirements/phase-1-1-sign-off.md`。
