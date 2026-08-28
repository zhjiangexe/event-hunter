@infrastructure @grafana-auto-case
Feature: REQ-EH-007 Grafana 真實通知管線自動建立案件

  Background:
    * def uniqueId = java.util.UUID.randomUUID().toString()
    * def correlationId = 'E2E-GRAFANA-AUTO-' + uniqueId
    * def eventId = 'evt-grafana-auto-' + uniqueId
    * def dlqAttemptId = 'attempt-grafana-auto-dlq-' + uniqueId
    * def successAttemptId = 'attempt-grafana-auto-success-' + uniqueId
    * def clickhouseBasicAuth = 'Basic ' + java.util.Base64.getEncoder().encodeToString((clickhouseUser + ':' + clickhousePassword).getBytes())
    * def insertAttempt =
      """
      function(attemptId, attempt, status, reason) {
        var row = {
          attempt_id: attemptId,
          event_id: eventId,
          event_type: 'PaymentCompleted',
          correlation_id: correlationId,
          trace_id: null,
          consumer_group_id: 'shipping-service-v1',
          consumer_service: 'shipping-service',
          attempt: attempt,
          processing_status: status,
          retry_reason: reason,
          retry_topic: status == 'DLQ' ? 'payment.events.dlq' : null,
          kafka_topic: 'payment.events',
          kafka_partition: 98,
          kafka_offset: java.lang.System.currentTimeMillis(),
          started_at: new java.util.Date().toInstant().toString(),
          completed_at: new java.util.Date().toInstant().toString()
        };
        var tables = ['event_processing_attempts', 'poc_event_processing_attempts'];
        for (var i = 0; i < tables.length; i++) {
          var result = karate.call('../helpers/clickhouse-insert.feature', {
            clickhouseHttpUrl: clickhouseHttpUrl,
            authorization: clickhouseBasicAuth,
            table: tables[i],
            row: row
          });
          if (result.responseStatus != 200) {
            karate.fail('ClickHouse attempt insert into ' + tables[i] + ' failed: ' + result.responseStatus + ' ' + karate.toString(result.response));
          }
        }
      }
      """

  Scenario: DLQ business alert 經 provisioned policy 與 HMAC webhook 建案，成功後 resolved 但不自動結案
    # 寫入真實 ClickHouse read model；接下來不直接呼叫 webhook，必須等待 Grafana 自己評估與通知。
    * eval insertAttempt(dlqAttemptId, 3, 'DLQ', 'E2E forced terminal failure')

    Given url apiBaseUrl + '/api/v1/auth/demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200

    * configure retry = { count: 60, interval: 1000 }
    Given url apiBaseUrl + '/api/v1/investigations'
    And param correlation_id = correlationId
    And retry until responseStatus == 200 && response.items.length == 1
    When method get
    Then status 200
    And match response.items == '#[1]'
    And match response.items[0].correlation_id == correlationId
    And match response.items[0].severity == 'HIGH'
    And match response.items[0].status == '#? _ != "CLOSED"'
    * def investigationId = response.items[0].id

    Given url apiBaseUrl + '/api/v1/investigations/' + investigationId + '/evidence-bundle'
    When method get
    Then status 200
    * def firingEvidence = karate.filter(response.items, function(x){ return x.evidence_type == 'GRAFANA_ALERT' && x.source == 'GRAFANA' })
    * assert firingEvidence.length == 1
    And match firingEvidence[0].source_locator == '/alerting/grafana/event-hunter-dlq-investigation/view'
    And match firingEvidence[0].source_org_id == 1
    And match firingEvidence[0].open_action == 'GRAFANA_ALERT'

    # A later success changes the latest terminal status, so Grafana naturally
    # resolves the same alert instance and sends a signed resolved webhook.
    * eval java.lang.Thread.sleep(1100)
    * eval insertAttempt(successAttemptId, 4, 'SUCCEEDED', null)

    * configure retry = { count: 60, interval: 1000 }
    Given url apiBaseUrl + '/api/v1/investigations/' + investigationId + '/evidence-bundle'
    And retry until responseStatus == 200 && karate.filter(response.items, function(x){ return x.evidence_type == 'GRAFANA_ALERT' }).length == 2
    When method get
    Then status 200
    * def allAlertEvidence = karate.filter(response.items, function(x){ return x.evidence_type == 'GRAFANA_ALERT' })
    * assert allAlertEvidence.length == 2

    Given url apiBaseUrl + '/api/v1/investigations/' + investigationId
    When method get
    Then status 200
    And match response.status == '#? _ != "CLOSED"'
