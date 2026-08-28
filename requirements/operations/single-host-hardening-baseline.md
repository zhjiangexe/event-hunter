---
document_id: EH-DOC-OPS-002
status: active
owner: platform
last_reviewed: 2026-08-28
source_of_truth: true
canonical_topic: single-host-hardening-baseline
supersedes: []
---

# Phase 1.1 Single-host Hardening Baseline

## 支援的部署等級

本基線只支援「內部網路、單機、非 HA 的 staging／pilot」。它允許團隊在接近正式的安全邊界下完成
安裝、升級、TLS、secret rotation、restart、backup 與 restore 演練，但不宣稱為 internet-facing 或
跨機房 production。若要處理正式客戶資料、多團隊存取或 24x7 SLA，必須先完成 OIDC／SSO、完整
RBAC、HA、受管 Kafka／database、Secret Manager、加密備份與正式 RTO／RPO 設計。

## 已採用決策

| 項目 | Phase 1.1 internal pilot 決策 |
|---|---|
| Network | 只有 Event Hunter 與 Grafana 兩個 TLS edge 可發布 host port；database、Kafka、Connect、OTel 與 demo services 只在 Compose network 內可達 |
| TLS | 由部署者提供 certificate／private key；edge 只允許 TLS 1.2／1.3 並送出 HSTS、nosniff 與 Referrer-Policy |
| Secrets | 九個 credential 必須至少 24 字元、互異、不可使用 `local_only`／`CHANGE_ME`；真值只放未提交 env 或部署 secret store |
| Identity | pilot 只允許受控內網人員使用 demo role session；不得作為正式企業身分或跨團隊授權方案 |
| PII | pilot 只允許 synthetic／已去識別資料；現有 payload allowlist 與巢狀遮罩仍必須開啟 |
| RPO | 每日 cold backup，pilot 目標 RPO 24 小時 |
| RTO | 單機重建、restore 與 smoke 的 pilot 目標 RTO 4 小時 |
| Availability | 無 HA；host 故障期間服務不可用，這是此部署等級的明確限制 |

## Retention

| 資料 | 現行政策 |
|---|---|
| canonical events／processing attempts／quality metrics | 90 天 ClickHouse TTL |
| raw landing | 7 天 TTL；只供 admission／故障鑑識，不供一般讀者 |
| ingestion／technical failures | 30～90 天，依 table contract |
| Tempo traces | 7 天 |
| Loki logs | 7 天，compactor retention 已啟用 |
| Prometheus metrics | 7 天 |
| PostgreSQL cases／evidence／audit | pilot 期間保存；pilot 結束後以整個隔離環境銷毀，不接收正式 PII |

PostgreSQL 的逐案件法遵刪除不是 internal pilot 功能。正式資料上線前，必須由資料治理人員核定 legal
hold、Evidence 與 Audit retention，並新增可稽核的 bounded deletion workflow；不可直接沿用「永久保存」。

## 建立與驗證

1. 複製 `.env.hardening.example` 為未提交的 `.env.hardening`，產生互異 secrets 並設定憑證絕對路徑。
2. Private key 權限設為 owner-only，例如 `chmod 600 /path/to/key`。
3. 先執行 fail-closed 驗證：

   ```bash
   python3 scripts/verify-hardening-profile.py --env-file .env.hardening
   ```

4. 建立 fresh staging：

   ```bash
   docker compose --env-file .env.hardening -f compose.yaml -f compose.hardening.yaml up -d --build
   ```

5. 只從設定的兩個 HTTPS URL執行 browser smoke；host 上的 PostgreSQL、ClickHouse、Kafka、Connect、
   Loki、Tempo、OTel、API 與 demo service ports 必須無法直接連入。
6. 依 [Operations Runbook](operations-runbook.md) 執行 restart persistence 與 cold backup。Restore 必須在
   另一個隔離 project name／空白 volumes 演練，禁止覆蓋目前環境。

## 尚未被此基線解決

- OIDC／SSO、SCIM、企業 group-to-role mapping 與 break-glass account。
- 多節點 Kafka、PostgreSQL、ClickHouse、Loki、Tempo 與 Grafana HA。
- 加密、異地、不可變備份及實際跨區 DR。
- 正式容量／load／soak、RTO／RPO 演練與 production on-call 交接。

以上項目不是程式碼可以替部署環境做出的假決策；正式 production sign-off 必須附目標環境證據。
