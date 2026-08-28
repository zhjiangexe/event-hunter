# Dependency and toolchain upgrade audit

狀態：`verified-local`  
盤點日期：2026-08-27  
原則：應用程式與建置工具可在同一輪完整測試時升級；有持久化資料、跨 major、registry 遷移或
breaking config 的基礎設施，不為了追逐 latest 直接改動正式 Compose pin。

## 本輪已更新並驗證

| 範圍 | 更新後版本 | 決策與驗證 |
|---|---:|---|
| Go toolchain | `1.27.0` | `go test ./...`、`go vet ./...` 與 multi-stage Docker build 通過 |
| Go OpenTelemetry | core/SDK/exporters `1.46.0`、logs `0.22.0`、`otelhttp 0.71.0`、`otelslog 0.20.1` | `go mod tidy` 後測試與建置通過；維持顯式 OTLP HTTP instrumentation |
| pnpm | `11.24.0` | lockfile 由 pnpm 11 重建；只允許 `esbuild` install script |
| frontend test/build | Vite `8.2.1`、Vitest `4.1.11`、plugin-react `6.1.0`、ESLint `10.9.1` | lint、typecheck、74 tests、production build、generated client checks 通過 |
| frontend runtime lock | React `19.2.8`、React Router `7.18.2`、TanStack Query `5.102.3` | 由 lockfile 固定；遵守 pnpm 的 24 小時 minimum-release-age policy |
| container build images | Go `1.27.0-alpine`、Node `26.7-alpine`、Alpine `3.24`、nginx stable `1.30.4-alpine` | backend、frontend 與 ClickHouse POC images 建置通過 |
| Karate standalone | `2.1.2` | `karate.jar` 啟動與既有 shell runner 相容 |

版本依據：[Go releases](https://go.dev/doc/devel/release)、
[OpenTelemetry Go releases](https://github.com/open-telemetry/opentelemetry-go/releases)、
[pnpm releases](https://github.com/pnpm/pnpm/releases/tag/v11.24.0)、
[Karate 2.1.2](https://github.com/karatelabs/karate/releases/tag/v2.1.2)、
[Alpine releases](https://alpinelinux.org/releases/)。

### 刻意沒有硬升到發布頁最末端

- Node upstream 已有 `26.8.0`，但盤點當下官方 registry 尚無 `node:26.8-alpine` manifest，因此 Docker
  build 使用可取得的 `26.7-alpine`；不得填入不存在的 tag。
- Vite `8.2.2`、TanStack Query `5.102.5` 與 React Refresh `0.5.5` 在盤點當下發布未滿 24 小時，
  pnpm supply-chain policy 會拒絕。等待冷卻期後再由 lockfile update 驗證，不停用政策。
- TypeScript `7.0.2` 不符合 `typescript-eslint 8.68.0` 的 `<6.1.0` peer range，因此維持 `5.9.3`；
  這是相容性限制，不是漏升級。

## Compose infrastructure 盤點

下表的「可用最新版」是 2026-08-27 官方 release／image 狀態；本輪只盤點，不修改現行
`compose.yaml` pin，避免下一次啟動時未經 migration rehearsal 就改寫持久化資料。

| 元件 | 現行 pin | 可用最新版／建議目標 | 本輪決策 |
|---|---:|---:|---|
| PostgreSQL | `18.4` | `18.6` | patch candidate；先備份並跑 restart-persistence |
| ClickHouse | `26.6.2.160-stable` | `26.7.5.10-stable` | stateful candidate；先做冷備份／還原演練 |
| Redpanda | `26.2.1` | `26.2.2` | patch candidate；需驗證 topic、consumer offsets 與 connector 恢復 |
| Redpanda Console | `3.10.0` | `3.11.0` | 可獨立升級；需 smoke test integrations UI |
| Debezium Connect | `3.0.0.Final` | `3.6.1.Final` | 延後；跨 minor 且官方 image 已移至 `quay.io/debezium/connect`，需 connector migration |
| Redpanda Connect | 已移除 | 不適用 | 2026-08-27 已由官方 ClickHouse Kafka Connect Sink + Materialized Views 取代，不再列入升級範圍 |
| Prometheus | `3.12.0` | `3.14.0` | patch/minor candidate；先驗證 rule/config |
| Loki | `3.7.5` | `3.7.6` | patch candidate；需 live logs 與 restart 查詢驗證 |
| Tempo | `2.10.7` | `2.10.8` compatible target；`3.0.3` absolute latest | 只考慮 `2.10.8`；3.x breaking migration 另案處理 |
| OTel Collector Contrib | `0.158.0` | `0.159.0` | candidate；先用 target binary validate config，再驗證 trace/log/metric |
| Grafana | `13.1.1` | `13.2.0` | candidate；需驗證 ClickHouse/Loki/Tempo datasources、deep links 與 alert rules |
| Grafana ClickHouse plugin | `4.20.0` | `4.20.0` | 已是最新版 |
| Temporal CLI/dev server | `1.8.2` | `1.8.2` | 已是最新版 |
| ClickHouse Kafka Connect Sink | `1.5.0` | `1.5.0` | 已是最新版，build 仍固定官方 SHA-256 |

官方依據：[PostgreSQL releases](https://www.postgresql.org/docs/release/)、
[Debezium releases](https://debezium.io/releases/)、
[Debezium image registry migration](https://debezium.io/blog/2024/09/18/quay-io-reminder/)、
[Redpanda releases](https://github.com/redpanda-data/redpanda/releases)、
[Redpanda Connect releases](https://github.com/redpanda-data/connect/releases)、
[Grafana releases](https://github.com/grafana/grafana/releases)、
[Loki releases](https://github.com/grafana/loki/releases)、
[Tempo releases](https://github.com/grafana/tempo/releases)、
[Prometheus releases](https://github.com/prometheus/prometheus/releases)、
[Grafana ClickHouse datasource](https://grafana.com/grafana/plugins/grafana-clickhouse-datasource/)。

## 下一輪 infrastructure upgrade gate

每一個 infrastructure pin 必須獨立變更並依序通過：

1. 對目標 image 執行 config／pipeline lint，不先重啟資料服務。
2. PostgreSQL、ClickHouse 與 Redpanda 先做可還原備份；保留舊 image pin 與 rollback 步驟。
3. 只升一個元件，確認 readiness、connector/task 狀態與 consumer lag。
4. 跑 Phase 1 contract、backend Karate、live observability E2E 與 ClickHouse POC。
5. 執行完整 restart-persistence，確認事件、trace、log、案件與 connector offsets 重啟後仍可查。

在上述 gate 完成前，「有較新版本」不等同「已核准更新 production-like local baseline」。
