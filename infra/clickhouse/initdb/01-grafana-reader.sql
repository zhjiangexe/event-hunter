-- 僅供本機開發的 Grafana 唯讀帳號。正式環境必須改用 Secret Manager 並限制來源網段。
CREATE USER IF NOT EXISTS grafana_reader
IDENTIFIED WITH plaintext_password BY 'grafana_reader_local_only';

-- 權限套用到 event_hunter database；未來建立的新表也能由 Grafana 查詢。
GRANT SELECT ON event_hunter.* TO grafana_reader;
