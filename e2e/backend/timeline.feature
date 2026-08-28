@requires-fixtures
Feature: REQ-EH-001 有界的業務時間線

  Background:
    # 需要先載入 contracts/fixtures，並建立具有 timeline:read 權限的 Session。
    * url apiBaseUrl
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'ADMIN' }
    When method post
    Then status 200

  Scenario: 查詢固定業務時間線時預設不暴露 payload
    Given path 'api', 'v1', 'timelines', 'ORDER-1001'
    And param from = '2026-08-20T10:00:00Z'
    And param to = '2026-08-20T10:06:00Z'
    When method get
    Then status 200
    And match response.correlation_id == 'ORDER-1001'
    And match response.event_count == 3
    And match response.truncated == false
    And match response.events[*].event_type == ['OrderCreated', 'PaymentCompleted', 'ShipmentCreated']
    # 即使 fixture 含 payload，沒有明確要求與權限時也必須從回應省略。
    And match each response.events !contains { payload: '#present' }

  Scenario: 只有明確要求時才顯示 Consumer 處理嘗試摘要
    Given path 'api', 'v1', 'timelines', 'ORDER-2001'
    And param from = '2026-08-20T11:00:00Z'
    And param to = '2026-08-20T11:06:00Z'
    And param include_processing_attempts = true
    When method get
    Then status 200
    And match each response.events contains { processing_summary: '##object' }
    * def paymentEvent = response.events.find(x => x.event_id == 'evt-payment-2001-001')
    And match paymentEvent.processing_summary contains
      """
      {
        attempt_count: 3,
        final_status: 'DLQ',
        consumer_groups: ['shipping-service-v1']
      }
      """

  Scenario: Event detail 回傳可追查 metadata 與遮罩後 payload
    Given path 'api', 'v1', 'timelines', 'ORDER-1001'
    And param from = '2026-08-20T10:00:00Z'
    And param to = '2026-08-20T10:06:00Z'
    And param include_payload = true
    And param include_processing_attempts = true
    When method get
    Then status 200
    And match response.events[0] contains
      """
      {
        event_id: 'evt-order-1001-001',
        trace_id: '11111111111111111111111111111111',
        kafka_topic: 'order.events',
        kafka_partition: 0,
        kafka_offset: 1000,
        ingested_at: '#string',
        processing_summary: '##object'
      }
      """
    And match response.events[0].payload.orderId == 'ORDER-1001'
    And match response.events[0].payload.totalAmount == '[REDACTED_AMOUNT]'
    And match response.events[0].payload.customerId == '#regex CUSTOMER-\\*\\*\\*-[0-9a-f]{8}'
    And match response.events[0].occurred_at == '#regex .*Z'
    And match response.events[0].ingested_at == '#regex .*Z'

  Scenario Outline: 非 ADMIN 不可透過 Timeline query flag 取得 payload
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: '<role>' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'timelines', 'ORDER-1001'
    And param from = '2026-08-20T10:00:00Z'
    And param to = '2026-08-20T10:06:00Z'
    And param include_payload = true
    When method get
    Then status 403
    And match response.code == 'FORBIDDEN'

    Examples:
      | role         |
      | VIEWER       |
      | INVESTIGATOR |

  Scenario: 非 ADMIN 不可透過 Event Search query flag 取得 payload
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'VIEWER' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'events', 'search'
    And param from = '2026-08-20T10:00:00Z'
    And param to = '2026-08-20T10:06:00Z'
    And param correlation_id = 'ORDER-1001'
    And param include_payload = true
    When method get
    Then status 403
    And match response.code == 'FORBIDDEN'

  Scenario: 超過七天的時間線範圍會被拒絕
    Given path 'api', 'v1', 'timelines', 'ORDER-1001'
    And param from = '2026-08-01T00:00:00Z'
    And param to = '2026-08-20T00:00:00Z'
    When method get
    Then status 422
    And match response.code == 'QUERY_WINDOW_TOO_LARGE'

  Scenario: Event search 必須提供有界時間範圍
    Given path 'api', 'v1', 'events', 'search'
    And param event_type = 'PaymentCompleted'
    When method get
    Then status 422
    And match response.code == '#string'

  Scenario: Pattern ID 只回傳 Registry 定義的證據事件類型
    Given path 'api', 'v1', 'events', 'search'
    And param from = '2026-08-20T11:00:00Z'
    And param to = '2026-08-20T11:06:00Z'
    And param correlation_id = 'ORDER-2001'
    And param pattern_id = 'payment-completed-without-shipment'
    When method get
    Then status 200
    And assert response.count > 0
    And match each response.events[*].event_type == '#? ["PaymentCompleted", "ShipmentCreated", "OrderCancelled", "PaymentRefunded", "PaymentVoided"].includes(_)'

  Scenario: 未知 Pattern 不得被靜默忽略
    Given path 'api', 'v1', 'events', 'search'
    And param from = '2026-08-20T11:00:00Z'
    And param to = '2026-08-20T11:06:00Z'
    And param pattern_id = 'runtime-injected-pattern'
    When method get
    Then status 422
    And match response.code == 'UNKNOWN_PATTERN'

  Scenario: 最低 Severity 依案件控制面限定 correlation
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] Timeline severity qualifier', severity: 'HIGH', correlation_id: 'ORDER-2001' }
    When method post
    Then status 201

    Given path 'api', 'v1', 'events', 'search'
    And param from = '2026-08-20T11:00:00Z'
    And param to = '2026-08-20T11:06:00Z'
    And param correlation_id = 'ORDER-2001'
    And param severity = 'HIGH'
    When method get
    Then status 200
    And assert response.count > 0
    And match each response.events contains { correlation_id: 'ORDER-2001' }
