Feature: REQ-EH-012 唯讀 Business Journey

  Background:
    * url apiBaseUrl
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'VIEWER' }
    When method post
    Then status 200

  @journey-profile-registry
  Scenario: 可列出目前 API build 實際載入的唯讀 Journey Profile
    Given path 'api', 'v1', 'journey-profiles'
    When method get
    Then status 200
    And match response.items == '#[1]'
    And match response.items[0] contains
      """
      {
        id: 'order-fulfillment',
        version: 1,
        status: 'active',
        default: true,
        source_path: 'contracts/journeys/order-fulfillment.yaml'
      }
      """
    And match response.items[0].checksum == '#regex [0-9a-f]{64}'
    And match response.items[0].milestones[*].id == ['ORDER', 'PAYMENT', 'SHIPPING', 'DELIVERY', 'RETURN']
    And match response.items[0].anomaly_rules[*].code contains 'MISSING_SHIPMENT_AFTER_PAYMENT'

  Scenario: 完整送達事件鏈會組成已完成 Journey
    Given path 'api', 'v1', 'business-journeys', 'ORDER-4002'
    And param from = '2026-08-20T16:00:00Z'
    And param to = '2026-08-20T16:21:00Z'
    When method get
    Then status 200
    And match response contains { profile_id: 'order-fulfillment', profile_version: 1, profile_title: 'Order Fulfillment' }
    And match response.status == 'COMPLETED'
    And match response.event_count == 6
    And match response.completed_milestone_count == 4
    And match response.total_milestone_count == 5
    And match response.current_milestone_id == null
    And match response.next_milestone_id == null
    And match response.next_expected_event_types == []
    And match each response.trace_ids == '#regex [0-9a-f]{32}'
    And match response.anomalies == []
    And match response.milestones[0] contains { id: 'ORDER', state: 'COMPLETED', actual_event_types: ['OrderCreated'] }
    And match response.milestones[1] contains { id: 'PAYMENT', state: 'COMPLETED', actual_event_types: ['PaymentCompleted'] }
    And match response.milestones[2] contains { id: 'SHIPPING', state: 'COMPLETED', actual_event_types: ['ShipmentCreated', 'ShipmentDispatched', 'ShipmentInTransit'] }
    And match response.milestones[2].duration_from_previous_ms == 50000
    And match response.milestones[3] contains { id: 'DELIVERY', state: 'COMPLETED', actual_event_types: ['ShipmentDelivered'] }

  Scenario: 付款完成超過五分鐘仍未出貨會顯示進行中與確定性異常
    Given path 'api', 'v1', 'business-journeys', 'ORDER-2001'
    And param from = '2026-08-20T11:00:00Z'
    And param to = '2026-08-20T11:06:00Z'
    When method get
    Then status 200
    And match response.status == 'IN_PROGRESS'
    And match response.event_count == 2
    And match response.completed_milestone_count == 2
    And match response.total_milestone_count == 5
    And match response.current_milestone_id == 'SHIPPING'
    And match response.next_milestone_id == 'DELIVERY'
    And match response.next_expected_event_types contains 'ShipmentCreated'
    And match each response.trace_ids == '#regex [0-9a-f]{32}'
    And match response.milestones[2] contains { id: 'SHIPPING', state: 'IN_PROGRESS', actual_event_types: [] }
    And match response.anomalies[0].code == 'MISSING_SHIPMENT_AFTER_PAYMENT'
    And match response.anomalies[0].severity == 'HIGH'

  Scenario: 查無事件時明確回傳 EMPTY 而不是假成功
    Given path 'api', 'v1', 'business-journeys', 'ORDER-NOT-FOUND'
    And param from = '2026-08-20T11:00:00Z'
    And param to = '2026-08-20T11:06:00Z'
    When method get
    Then status 200
    And match response.status == 'EMPTY'
    And match response.event_count == 0
    And match response.completed_milestone_count == 0
    And match response.total_milestone_count == 5
    And match response.current_milestone_id == null
    And match response.trace_ids == []
    And match response.started_at == null
    And match each response.milestones contains { state: 'NOT_APPLICABLE' }

  Scenario: 只建立 Shipment 尚未送達時維持進行中
    Given path 'api', 'v1', 'business-journeys', 'ORDER-1001'
    And param from = '2026-08-20T10:00:00Z'
    And param to = '2026-08-20T10:06:00Z'
    When method get
    Then status 200
    And match response.status == 'IN_PROGRESS'
    And match response.milestones[2].state == 'COMPLETED'
    And match response.milestones[3].state == 'IN_PROGRESS'

  Scenario: Journey 仍強制最多七天的 bounded time window
    Given path 'api', 'v1', 'business-journeys', 'ORDER-1001'
    And param from = '2026-08-01T00:00:00Z'
    And param to = '2026-08-20T00:00:00Z'
    When method get
    Then status 422
    And match response.code == 'QUERY_WINDOW_TOO_LARGE'
