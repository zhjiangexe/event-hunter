@infrastructure
Feature: REQ-EH-009 Order Outbox 到 Event Hunter Timeline 的完整事件管線

  Background:
    # 使用真實 Order API 與 production mapping，不允許為 E2E 建立直接寫 ClickHouse 的 HTTP 後門。
    * def runId = java.util.UUID.randomUUID().toString()
    # Karate v2 會將 Java Instant 物件當成 Date 序列化；明確轉成 RFC3339 字串，避免送出 "Fri Aug ..."。
    * def fromTime = '' + java.time.Instant.ofEpochMilli(java.lang.System.currentTimeMillis() - 60000)
    * def toTime = '' + java.time.Instant.ofEpochMilli(java.lang.System.currentTimeMillis() + 600000)

  Scenario: 建立訂單後可經 Debezium 與目前選定的 ingestion path 查到三個 Domain Event
    # API readiness 必須代表真實 ingestion capability，而不是只有 HTTP process 存活。
    Given url apiBaseUrl
    And path 'health', 'ready'
    When method get
    Then status 200
    And match response.status == 'ready'
    And match response.checks contains
      """
      {
        postgres: 'ready',
        clickhouse: 'ready',
        domain_event_ingestion: 'ready',
        processing_attempt_ingestion: 'ready'
      }
      """

    # 先拒絕 Kafka Connect task 已失敗的假健康環境，避免建立訂單後才空等 timeline。
    Given url kafkaConnectBaseUrl
    And path 'connectors'
    And param expand = 'status'
    When method get
    Then status 200
    And match response['event-hunter-demo-order-outbox-v1'].status.connector.state == 'RUNNING'
    And match response['event-hunter-demo-order-outbox-v1'].status.tasks[*].state == ['RUNNING']
    And match response['event-hunter-demo-payment-outbox-v1'].status.connector.state == 'RUNNING'
    And match response['event-hunter-demo-payment-outbox-v1'].status.tasks[*].state == ['RUNNING']
    And match response['event-hunter-demo-shipping-outbox-v1'].status.connector.state == 'RUNNING'
    And match response['event-hunter-demo-shipping-outbox-v1'].status.tasks[*].state == ['RUNNING']

    Given url orderBaseUrl
    And path 'api', 'v1', 'orders'
    And header Idempotency-Key = 'karate-normal-order-flow-' + runId
    And request { customer_id: 'CUSTOMER-E2E-1', total_amount: 1280, currency: 'TWD' }
    When method post
    Then status 202
    * def correlationId = response.correlation_id

    Given url apiBaseUrl
    And path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200

    # 第一階段最多等 20 秒看到 OrderCreated；若 ingestion consumer 已失聯，應在此快速失敗。
    * configure retry = { count: 20, interval: 1000 }
    Given path 'api', 'v1', 'timelines', correlationId
    And param from = fromTime
    And param to = toTime
    And retry until responseStatus == 200 && response.event_count >= 1
    When method get
    Then status 200
    And match response.events[*].event_type contains 'OrderCreated'

    # 第二階段最多再等 40 秒完成 Payment 與 Shipping；最後一次 response 會保存在 Karate report。
    * configure retry = { count: 40, interval: 1000 }
    Given path 'api', 'v1', 'timelines', correlationId
    And param from = fromTime
    And param to = toTime
    And retry until responseStatus == 200 && response.event_count == 3
    When method get
    Then status 200
    And match response.events[*].event_type == ['OrderCreated', 'PaymentCompleted', 'ShipmentCreated']
    And match each response.events contains { correlation_id: '#(correlationId)' }
    And match each response.events contains { admission_status: 'SEARCHABLE', quality_flags: '#array', admission_profile: '#string' }
