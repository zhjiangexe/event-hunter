@requires-fixtures
Feature: 擴充訂單、付款、配送與退貨事件情境

  Background:
    * url apiBaseUrl
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'ADMIN' }
    When method post
    Then status 200

  Scenario Outline: Timeline 依事件時間還原擴充情境的完整順序
    * def correlationId = '<correlationId>'
    * def expectedTraceId = '<traceId>'
    * def expectedTypes = karate.fromString('<expectedTypes>')
    Given path 'api', 'v1', 'timelines', correlationId
    And param from = '2026-08-20T14:59:00Z'
    And param to = '2026-08-20T18:50:00Z'
    When method get
    Then status 200
    And match response.event_count == <eventCount>
    And match response.events[*].event_type == expectedTypes
    And match each response.events contains { correlation_id: '#(correlationId)', trace_id: '#(expectedTraceId)' }
    And match each response.events contains { occurred_at: '#regex .*Z', ingested_at: '#regex .*Z' }

    Examples:
      | correlationId | traceId                          | eventCount | expectedTypes                                                                                                                                                      |
      | ORDER-4001    | 40014001400140014001400140014001 | 3          | ["OrderCreated","PaymentFailed","OrderCancelled"]                                                                                                             |
      | ORDER-4002    | 40024002400240024002400240024002 | 6          | ["OrderCreated","PaymentCompleted","ShipmentCreated","ShipmentDispatched","ShipmentInTransit","ShipmentDelivered"]                                      |
      | ORDER-4003    | 40034003400340034003400340034003 | 6          | ["OrderCreated","PaymentCompleted","ShipmentCreated","ShipmentDispatchFailed","ShipmentDispatched","ShipmentDelivered"]                                 |
      | ORDER-4004    | 40044004400440044004400440044004 | 8          | ["OrderCreated","PaymentCompleted","ShipmentCreated","ShipmentDispatched","ShipmentDelivered","ReturnRequested","ReturnReceived","PaymentRefunded"] |

  Scenario: 付款失敗事件保留失敗原因並由取消訂單承接 causation
    Given path 'api', 'v1', 'timelines', 'ORDER-4001'
    And param from = '2026-08-20T15:00:00Z'
    And param to = '2026-08-20T15:01:00Z'
    And param include_payload = true
    When method get
    Then status 200
    * def paymentFailed = response.events.find(x => x.event_type == 'PaymentFailed')
    * def orderCancelled = response.events.find(x => x.event_type == 'OrderCancelled')
    And match paymentFailed contains { aggregate_type: 'Payment', aggregate_id: 'PAYMENT-4001', sequence: 1 }
    And match paymentFailed.payload contains { orderId: 'ORDER-4001', reasonCode: 'CARD_DECLINED', retryable: false, status: 'FAILED' }
    And match orderCancelled.causation_id == paymentFailed.event_id
    And match orderCancelled.sequence == 2

  Scenario: 配送失敗重試後 Shipment aggregate sequence 仍嚴格遞增
    Given path 'api', 'v1', 'timelines', 'ORDER-4003'
    And param from = '2026-08-20T17:00:00Z'
    And param to = '2026-08-20T17:13:00Z'
    And param include_payload = true
    When method get
    Then status 200
    * def shipmentEvents = response.events.filter(x => x.aggregate_type == 'Shipment')
    And match shipmentEvents[*].event_type == ['ShipmentCreated', 'ShipmentDispatchFailed', 'ShipmentDispatched', 'ShipmentDelivered']
    And match shipmentEvents[*].sequence == [1, 2, 3, 4]
    And match shipmentEvents[1].payload contains { reasonCode: 'PROVIDER_TIMEOUT', retryable: true, status: 'DISPATCH_FAILED' }
    And match shipmentEvents[2].causation_id == shipmentEvents[1].event_id

  Scenario: 退貨入庫完成後才產生退款事件
    Given path 'api', 'v1', 'timelines', 'ORDER-4004'
    And param from = '2026-08-20T18:00:00Z'
    And param to = '2026-08-20T18:47:00Z'
    And param include_payload = true
    When method get
    Then status 200
    * def returnRequested = response.events.find(x => x.event_type == 'ReturnRequested')
    * def returnReceived = response.events.find(x => x.event_type == 'ReturnReceived')
    * def refunded = response.events.find(x => x.event_type == 'PaymentRefunded')
    And match returnRequested contains { aggregate_type: 'Return', aggregate_id: 'RETURN-4004', sequence: 1 }
    And match returnReceived contains { aggregate_type: 'Return', aggregate_id: 'RETURN-4004', sequence: 2 }
    And match returnReceived.causation_id == returnRequested.event_id
    And match refunded.causation_id == returnReceived.event_id
    And match refunded.payload contains { orderId: 'ORDER-4004', amount: '[REDACTED_AMOUNT]', reason: '[REDACTED]' }

  @infrastructure
  Scenario: 擴充情境的 synthetic trace 與 logs 可從 Tempo 和 Loki 查詢
    Given url tempoBaseUrl
    And path 'api', 'traces', '40034003400340034003400340034003'
    When method get
    Then status 200
    And assert response.batches.length > 0

    Given url lokiBaseUrl
    And path 'loki', 'api', 'v1', 'query_range'
    And param query = '{service_name="shipping-service",service_namespace="event-hunter.synthetic"} | correlation_id="ORDER-4003"'
    And param start = '1787245140000000000'
    And param end = '1787246040000000000'
    And param limit = 20
    When method get
    Then status 200
    And match response.status == 'success'
    And assert response.data.result.length > 0
    * def observedTypes = response.data.result.map(x => x.stream.event_type)
    And match observedTypes contains 'ShipmentDispatchFailed'
    And match observedTypes contains 'ShipmentDelivered'
