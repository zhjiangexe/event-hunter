function fn() {
  // Karate 執行環境可由 -Dkarate.env 覆寫；未指定時使用本機設定。
  var env = karate.env || 'local';
  return {
    env: env,
    // URL 由 JVM system property 注入，讓同一份 feature 可在 local／CI／preview 環境重用。
    apiBaseUrl: karate.properties['api.baseUrl'] || 'http://localhost:28333',
    webBaseUrl: karate.properties['web.baseUrl'] || 'http://localhost:28334',
    orderBaseUrl: karate.properties['order.baseUrl'] || 'http://localhost:28335',
    eventLabBaseUrl: karate.properties['eventLab.baseUrl'] || 'http://localhost:28343',
    kafkaConnectBaseUrl: karate.properties['kafkaConnect.baseUrl'] || 'http://localhost:28324',
    clickhousePocConnectBaseUrl: karate.properties['clickhousePocConnect.baseUrl'] || 'http://localhost:28345',
    redpandaConnectBaseUrl: karate.properties['redpandaConnect.baseUrl'] || 'http://localhost:28325',
    redpandaHttpProxyUrl: karate.properties['redpanda.httpProxyUrl'] || 'http://localhost:28321',
    clickhouseHttpUrl: karate.properties['clickhouse.httpUrl'] || 'http://localhost:28317',
    clickhouseUser: karate.properties['clickhouse.user'] || 'event_hunter',
    clickhousePassword: karate.properties['clickhouse.password'] || 'event_hunter_local_only',
    grafanaBaseUrl: karate.properties['grafana.baseUrl'] || 'http://localhost:28332',
    grafanaUser: karate.properties['grafana.user'] || 'admin',
    grafanaPassword: karate.properties['grafana.password'] || 'admin_local_only',
    lokiBaseUrl: karate.properties['loki.baseUrl'] || 'http://localhost:28327',
    tempoBaseUrl: karate.properties['tempo.baseUrl'] || 'http://localhost:28328',
    // 只供本機 E2E；正式環境必須由 Secret Manager 注入，不得提交真實 secret。
    grafanaWebhookSecret: karate.properties['grafana.webhookSecret'] || 'grafana_webhook_local_only',
    pocRunToken: karate.properties['poc.runToken'] || java.lang.System.getenv('POC_RUN_TOKEN') || 'manual',
    pocTechnicalSourcePartition: Number(karate.properties['poc.technicalSourcePartition'] || java.lang.System.getenv('POC_TECHNICAL_SOURCE_PARTITION') || '-1'),
    pocTechnicalSourceOffset: Number(karate.properties['poc.technicalSourceOffset'] || java.lang.System.getenv('POC_TECHNICAL_SOURCE_OFFSET') || '-1'),
    technicalDlqProjectorBaseUrl: karate.properties['technicalDlqProjector.baseUrl'] || 'http://localhost:28346'
  };
}
