@requires-fixtures
Feature: REQ-EH-007 經 HMAC 驗證的 Grafana 業務告警接入

  Background:
    # 每個 Scenario 先產生唯一 payload，再對「實際送出的完整字串」簽章，確保測試可重複執行。
    * url apiBaseUrl
    * def firingPayload = read('../../contracts/fixtures/grafana-alert-firing.json')
    * def firingFingerprint = 'eh-firing-' + java.util.UUID.randomUUID().toString()
    * set firingPayload.alerts[0].fingerprint = firingFingerprint
    * def payloadText = karate.toString(firingPayload)
    * def wirePayloadText = payloadText.replace(/\//g, '\\\\/')
    * def unixTimestamp = '' + (java.lang.System.currentTimeMillis() / 1000 | 0)
    * def hmacSha256 =
      """
      function(secret, value) {
        var Mac = Java.type('javax.crypto.Mac');
        var SecretKeySpec = Java.type('javax.crypto.spec.SecretKeySpec');
        var HexFormat = Java.type('java.util.HexFormat');
        var StandardCharsets = Java.type('java.nio.charset.StandardCharsets');
        var mac = Mac.getInstance('HmacSHA256');
        var secretBytes = new java.lang.String(secret).getBytes(StandardCharsets.UTF_8);
        var valueBytes = new java.lang.String(value).getBytes(StandardCharsets.UTF_8);
        mac.init(new SecretKeySpec(secretBytes, 'HmacSHA256'));
        return java.lang.String.valueOf(HexFormat.of().formatHex(mac.doFinal(valueBytes)));
      }
      """
    * def signature = hmacSha256(grafanaWebhookSecret, unixTimestamp + ':' + wirePayloadText)

  Scenario: 合格 firing alert 建立或連結業務案件
    Given path 'api', 'v1', 'integrations', 'grafana', 'alerts'
    And header Content-Type = 'application/json'
    And header X-Grafana-Alerting-Timestamp = unixTimestamp
    And header X-Grafana-Alerting-Signature = signature
    And request wirePayloadText
    When method post
    Then status 202
    And match response.received_alert_count == 1
    And match response.items[0].fingerprint == firingFingerprint
    And match response.items[0].disposition == '#? _ == "CREATED_CASE" || _ == "LINKED_CASE"'
    And match response.items[0].investigation_id == '#uuid'
    * def investigationId = response.items[0].investigation_id

    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'investigations', investigationId, 'evidence-bundle'
    When method get
    Then status 200
    * def alertEvidence = karate.filter(response.items, function(x){ return x.evidence_type == 'GRAFANA_ALERT' && x.source_locator == '/alerting/grafana/event-quality-delay/view' && x.source_org_id == 1 })
    * assert alertEvidence.length > 0
    And match each alertEvidence contains { reference: '#uuid', source: 'GRAFANA', open_action: 'GRAFANA_ALERT', checksum: '#regex [0-9a-f]{64}' }

    Given path 'api', 'v1', 'events', 'search'
    And param from = '2026-08-20T11:00:00Z'
    And param to = '2026-08-20T11:06:00Z'
    And param alert_id = firingFingerprint
    When method get
    Then status 200
    And assert response.count > 0
    And match each response.events contains { correlation_id: 'ORDER-2001' }

  Scenario: 相同通知重送不會建立第二個案件
    Given path 'api', 'v1', 'integrations', 'grafana', 'alerts'
    And header Content-Type = 'application/json'
    And header X-Grafana-Alerting-Timestamp = unixTimestamp
    And header X-Grafana-Alerting-Signature = signature
    And request wirePayloadText
    When method post
    Then status 202
    * def investigationId = response.items[0].investigation_id

    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'investigations', investigationId, 'evidence-bundle'
    When method get
    Then status 200
    * def beforeDuplicateCount = karate.filter(response.items, function(x){ return x.evidence_type == 'GRAFANA_ALERT' }).length

    Given path 'api', 'v1', 'integrations', 'grafana', 'alerts'
    And header Content-Type = 'application/json'
    And header X-Grafana-Alerting-Timestamp = unixTimestamp
    And header X-Grafana-Alerting-Signature = signature
    And request wirePayloadText
    When method post
    Then status 202
    And match response.items[0].disposition == 'DUPLICATE'
    And match response.items[0].reason_code == 'DUPLICATE_NOTIFICATION'

    Given path 'api', 'v1', 'investigations', investigationId, 'evidence-bundle'
    When method get
    Then status 200
    * def afterDuplicateCount = karate.filter(response.items, function(x){ return x.evidence_type == 'GRAFANA_ALERT' }).length
    * assert afterDuplicateCount == beforeDuplicateCount

  Scenario: 無效簽章會在解析業務內容前被拒絕
    Given path 'api', 'v1', 'integrations', 'grafana', 'alerts'
    And header Content-Type = 'application/json'
    And header X-Grafana-Alerting-Timestamp = unixTimestamp
    And header X-Grafana-Alerting-Signature = 'invalid-signature'
    And request wirePayloadText
    When method post
    Then status 401
    And match response.code == 'INVALID_WEBHOOK_SIGNATURE'

  Scenario: resolved alert 只記錄證據，不自動結案
    # 先建立相同 fingerprint 的 firing receipt，resolved 才有確定的案件可以連結。
    Given path 'api', 'v1', 'integrations', 'grafana', 'alerts'
    And header Content-Type = 'application/json'
    And header X-Grafana-Alerting-Timestamp = unixTimestamp
    And header X-Grafana-Alerting-Signature = signature
    And request wirePayloadText
    When method post
    Then status 202
    * def investigationId = response.items[0].investigation_id

    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'investigations', investigationId, 'evidence-bundle'
    When method get
    Then status 200
    * def beforeResolutionCount = karate.filter(response.items, function(x){ return x.evidence_type == 'GRAFANA_ALERT' }).length

    * def resolvedPayload = read('../../contracts/fixtures/grafana-alert-resolved.json')
    * set resolvedPayload.alerts[0].fingerprint = firingFingerprint
    * def resolvedText = karate.toString(resolvedPayload)
    * def resolvedWireText = resolvedText.replace(/\//g, '\\\\/')
    * def resolvedSignature = hmacSha256(grafanaWebhookSecret, unixTimestamp + ':' + resolvedWireText)
    Given path 'api', 'v1', 'integrations', 'grafana', 'alerts'
    And header Content-Type = 'application/json'
    And header X-Grafana-Alerting-Timestamp = unixTimestamp
    And header X-Grafana-Alerting-Signature = resolvedSignature
    And request resolvedWireText
    When method post
    Then status 202
    And match response.items[0].disposition == 'RECORDED_RESOLUTION'
    And match response.items[0].reason_code == 'RESOLUTION_RECORDED'
    And match response.items[0].investigation_id == investigationId

    Given path 'api', 'v1', 'investigations', investigationId, 'evidence-bundle'
    When method get
    Then status 200
    * def afterResolutionCount = karate.filter(response.items, function(x){ return x.evidence_type == 'GRAFANA_ALERT' }).length
    * assert afterResolutionCount == beforeResolutionCount + 1

    Given path 'api', 'v1', 'investigations', investigationId
    When method get
    Then status 200
    And match response.status == '#? _ != "CLOSED"'
