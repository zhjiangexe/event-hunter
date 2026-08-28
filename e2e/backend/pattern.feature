@requires-fixtures
Feature: REQ-EH-003 確定性 Domain Pattern

  Background:
    # Pattern 測試使用固定 canonical fixtures，結果不得依賴 LLM 或執行期可變規則。
    * url apiBaseUrl
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200

    * def clickhouseBasicAuth = 'Basic ' + java.util.Base64.getEncoder().encodeToString((clickhouseUser + ':' + clickhousePassword).getBytes())

  Scenario: Pattern Registry 只公開由 Go 程式碼定義的不可變規則
    Given path 'api', 'v1', 'patterns'
    When method get
    Then status 200
    And match response == '#[1]'
    And match response contains deep
      """
      [{
        id: 'payment-completed-without-shipment',
        version: 1,
        name: 'Payment completed without shipment',
        condition: 'Detect a paid order that has no shipment within the fixed investigation window.',
        severity: 'HIGH',
        window: 'PT5M',
        required_event_types: ['PaymentCompleted'],
        expected_event_types: ['ShipmentCreated'],
        exclusion_event_types: ['OrderCancelled', 'PaymentRefunded', 'PaymentVoided'],
        evidence_query_template_id: 'events.by_correlation.v1',
        status: 'ACTIVE',
        mutable_at_runtime: false
      }]
      """
    And match response[0].source_path == 'contracts/patterns/payment-completed-without-shipment.yaml'
    And match response[0].checksum == '#regex [0-9a-f]{64}'
    And match response[0].fixture_coverage == { match_count: 1, non_match_count: 2, total: 3 }

    # Registry 由版本控制與 CI 管理，MVP 不提供線上建立或修改。
    Given path 'api', 'v1', 'patterns'
    And request { id: 'runtime-rule' }
    When method post
    Then status 405

  @eh-p1-1-015
  Scenario: 未指定 Pattern 時由後端執行所有 ACTIVE Pattern 並同步產生 Finding
    Given path 'api', 'v1', 'investigations'
    And request
      """
      {
        title: '[E2E] 付款完成但未建立出貨',
        severity: 'HIGH',
        correlation_id: 'ORDER-2001',
        start_workflow: false
      }
      """
    When method post
    Then status 201
    * def investigationId = response.id

    Given path 'api', 'v1', 'investigations', investigationId, 'analyze'
    And request { execution_mode: 'SYNC' }
    When method post
    Then status 200
    And match response.execution_mode == 'SYNC'
    And match response.executed_pattern_ids == ['payment-completed-without-shipment']
    And match response.findings[0].pattern_id == 'payment-completed-without-shipment'
    And match response.findings[0].severity == 'HIGH'
    And match response.findings[0].matched_conditions contains 'PAYMENT_COMPLETED_EXISTS'
    And match response.findings[0].matched_conditions contains 'SHIPMENT_CREATED_MISSING_WITHIN_5M'

    Given path 'api', 'v1', 'investigations', investigationId, 'summary'
    When method get
    Then status 200
    * def analysisAudits = karate.filter(response.audit_entries, function(x){ return x.action == 'ANALYZE_INVESTIGATION' })
    And match analysisAudits == '#[1]'
    And match analysisAudits[0].metadata.executed_pattern_ids == ['payment-completed-without-shipment']

  @p1-1-04-01
  Scenario: Pattern 成效由後端固定窗口彙總且可追溯到案件
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] Pattern effectiveness', severity: 'HIGH', correlation_id: 'ORDER-2001' }
    When method post
    Then status 201
    * def investigationId = response.id

    Given path 'api', 'v1', 'investigations', investigationId, 'analyze'
    And request { pattern_ids: ['payment-completed-without-shipment'], execution_mode: 'SYNC' }
    When method post
    Then status 200
    And match response.findings[0].pattern_id == 'payment-completed-without-shipment'

    Given path 'api', 'v1', 'patterns', 'effectiveness'
    When method get
    Then status 200
    And match response.generated_at == '#string'
    And match response.window == { from: '#string', to: '#string' }
    * def metric = karate.filter(response.items, function(x){ return x.pattern_id == 'payment-completed-without-shipment' })[0]
    * match metric contains { pattern_id: 'payment-completed-without-shipment', hit_count: '#number', last_hit_at: '#string', investigation_count: '#number', confirmed_count: '#number', false_positive_count: '#number', needs_review_count: '#number', unreviewed_count: '#number', reviewed_count: '#number' }
    * match metric.false_positive_rate == '#? _ == null || typeof _ == "number"'
    * assert metric.reviewed_count == metric.confirmed_count + metric.false_positive_count + metric.needs_review_count
    * assert metric.unreviewed_count == metric.hit_count - metric.reviewed_count
    * assert metric.hit_count >= 1
    * assert metric.investigation_count >= 1

  @p1-1-04-02
  Scenario: Investigator 可判定 persisted finding 且 stale feedback version 不會覆寫
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] Pattern finding feedback', severity: 'HIGH', correlation_id: 'ORDER-2001' }
    When method post
    Then status 201
    * def investigationId = response.id

    Given path 'api', 'v1', 'investigations', investigationId, 'analyze'
    And request { pattern_ids: ['payment-completed-without-shipment'], execution_mode: 'SYNC' }
    When method post
    Then status 200
    And match response.findings[0].pattern_id == 'payment-completed-without-shipment'

    Given path 'api', 'v1', 'investigations', investigationId
    When method get
    Then status 200
    * def findingId = response.pattern_findings[0].finding_id
    And match findingId == '#uuid'
    And match response.pattern_findings[0].feedback == { finding_id: '#(findingId)', status: 'UNREVIEWED', actor_id: '', actor_role: '', updated_at: null, lock_version: 0 }

    Given path 'api', 'v1', 'investigations', investigationId, 'findings', findingId, 'feedback'
    And header If-Match = '"v0"'
    And header X-Request-ID = 'e2e-pattern-feedback-1'
    And request { status: 'CONFIRMED' }
    When method patch
    Then status 200
    And match header ETag == '"v1"'
    And match response == { finding_id: '#(findingId)', status: 'CONFIRMED', actor_id: 'demo-investigator', actor_role: 'INVESTIGATOR', updated_at: '#string', lock_version: 1 }

    # Viewer 看得到判定，但不能改寫。
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'VIEWER' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'investigations', investigationId, 'findings', findingId, 'feedback'
    And header If-Match = '"v1"'
    And request { status: 'NEEDS_REVIEW' }
    When method patch
    Then status 403
    And match response.code == 'FORBIDDEN'

    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200

    # 相同舊版本不得覆寫剛完成的人工判定。
    Given path 'api', 'v1', 'investigations', investigationId, 'findings', findingId, 'feedback'
    And header If-Match = '"v0"'
    And request { status: 'FALSE_POSITIVE' }
    When method patch
    Then status 409
    And match response.code == 'OPTIMISTIC_LOCK_CONFLICT'

    Given path 'api', 'v1', 'investigations', investigationId, 'summary'
    When method get
    Then status 200
    And match response.pattern_findings[0].feedback.status == 'CONFIRMED'
    And match response.pattern_findings[0].feedback.lock_version == 1
    * def feedbackAudits = karate.filter(response.audit_entries, function(x){ return x.action == 'CLASSIFY_PATTERN_FINDING' })
    And match feedbackAudits == '#[1]'
    And match feedbackAudits[0].metadata contains { finding_id: '#(findingId)', status: 'CONFIRMED', feedback_lock_version: 1 }

  Scenario: 正常訂單流程不會產生缺少出貨 Finding
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] 正常訂單對照案件', severity: 'LOW', correlation_id: 'ORDER-1001' }
    When method post
    Then status 201
    * def investigationId = response.id

    Given path 'api', 'v1', 'investigations', investigationId, 'analyze'
    And request { pattern_ids: ['payment-completed-without-shipment'] }
    When method post
    Then status 200
    And match response.findings == []

  Scenario: 不接受 Registry 以外的 Pattern ID
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] 未知 Pattern 驗證案件', severity: 'LOW', correlation_id: 'ORDER-2001' }
    When method post
    Then status 201
    * def investigationId = response.id

    Given path 'api', 'v1', 'investigations', investigationId, 'analyze'
    And request { pattern_ids: ['runtime-injected-pattern'], execution_mode: 'SYNC' }
    When method post
    Then status 422
    And match response.code == 'UNKNOWN_PATTERN'

  Scenario Outline: 終止型業務事件會排除缺少出貨 Finding
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] Pattern 排除條件 <excludedBy>', severity: 'LOW', correlation_id: '<correlationId>' }
    When method post
    Then status 201
    * def investigationId = response.id

    Given path 'api', 'v1', 'investigations', investigationId, 'analyze'
    And request { pattern_ids: ['payment-completed-without-shipment'], execution_mode: 'SYNC' }
    When method post
    Then status 200
    And match response.findings == []

    # 取消、退款與 void 都代表流程已終止，不應被誤報為出貨異常。
    Examples:
      | correlationId | excludedBy      |
      | ORDER-3001    | OrderCancelled  |
      | ORDER-3002    | PaymentRefunded |
      | ORDER-3003    | PaymentVoided   |

  Scenario: 選用 Temporal 關閉時不可要求 TEMPORAL 執行模式
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] Temporal 關閉驗證案件', severity: 'MEDIUM', correlation_id: 'ORDER-2001' }
    When method post
    Then status 201
    * def investigationId = response.id

    Given path 'api', 'v1', 'investigations', investigationId, 'analyze'
    And request { execution_mode: 'TEMPORAL' }
    When method post
    Then status 409
    And match response.code == 'TEMPORAL_DISABLED'

  @eh-p1-1-015
  Scenario: 已結案案件不得繞過 UI 執行新的 Pattern Analysis
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] Closed Pattern read-only', severity: 'LOW', correlation_id: 'ORDER-2001' }
    When method post
    Then status 201
    * def investigationId = response.id

    Given path 'api', 'v1', 'investigations', investigationId, 'close'
    And header If-Match = '"v0"'
    And request { root_cause: 'analysis boundary test', resolution_summary: 'closed cases remain read-only' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'investigations', investigationId, 'analyze'
    And request { execution_mode: 'SYNC' }
    When method post
    Then status 409
    And match response.code == 'INVALID_TRANSITION'

  @eh-p1-1-011 @infrastructure
  Scenario: 超過 rolling seven days 的歷史事件仍使用相同 server-owned window 重跑
    * def correlationId = 'ORDER-HISTORICAL-PATTERN-E2E'
    # 日期刻意早於 rolling seven days、但仍在 ClickHouse 90-day retention 內。
    * def orderEvent = { event_id: 'evt-historical-pattern-order', event_type: 'OrderCreated', event_version: 1, occurred_at: '2026-07-01T10:00:00Z', producer: 'historical-e2e', correlation_id: '#(correlationId)', causation_id: null, trace_id: '11001100110011001100110011001100', aggregate_type: 'Order', aggregate_id: '#(correlationId)', sequence: 1, kafka_topic: 'event-hunter.historical-fixture', kafka_partition: 98, kafka_offset: 1100, service_version: 'historical-e2e', payload: '{}', ingested_at: '2026-07-01T10:00:01Z' }
    * def paymentEvent = { event_id: 'evt-historical-pattern-payment', event_type: 'PaymentCompleted', event_version: 1, occurred_at: '2026-07-01T10:01:00Z', producer: 'historical-e2e', correlation_id: '#(correlationId)', causation_id: 'evt-historical-pattern-order', trace_id: '11001100110011001100110011001100', aggregate_type: 'Payment', aggregate_id: '#(correlationId)', sequence: 2, kafka_topic: 'event-hunter.historical-fixture', kafka_partition: 98, kafka_offset: 1101, service_version: 'historical-e2e', payload: '{}', ingested_at: '2026-07-01T10:01:01Z' }

    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And header Content-Type = 'application/json'
    And request "ALTER TABLE event_hunter.forensics_events DELETE WHERE event_id IN ('evt-historical-pattern-order','evt-historical-pattern-payment') SETTINGS mutations_sync = 1"
    When method post
    Then status 200

    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And header Content-Type = 'application/json'
    And request 'INSERT INTO event_hunter.forensics_events FORMAT JSONEachRow\n' + karate.toString(orderEvent) + '\n' + karate.toString(paymentEvent)
    When method post
    Then status 200

    # Historical probe 同時鏡像到 candidate，讓 deterministic window 驗收不綁死 active source。
    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And header Content-Type = 'application/json'
    And request "INSERT INTO event_hunter.poc_forensics_events (event_id,event_type,event_version,occurred_at,producer,correlation_id,causation_id,trace_id,aggregate_type,aggregate_id,sequence,kafka_topic,kafka_partition,kafka_offset,payload,payload_sha256,admission_profile,admission_status,quality_flags,ingested_at) SELECT event_id,event_type,event_version,occurred_at,producer,correlation_id,causation_id,trace_id,aggregate_type,aggregate_id,sequence,kafka_topic,kafka_partition,kafka_offset,payload,lower(hex(SHA256(payload))),'synthetic-historical-probe-v1','SEARCHABLE',CAST([],'Array(String)'),ingested_at FROM event_hunter.forensics_events WHERE event_id IN ('evt-historical-pattern-order','evt-historical-pattern-payment')"
    When method post
    Then status 200

    Given url apiBaseUrl
    And path 'api', 'v1', 'investigations'
    And request { title: '[E2E] Historical deterministic Pattern', severity: 'HIGH', correlation_id: '#(correlationId)' }
    When method post
    Then status 201
    * def investigationId = response.id

    Given path 'api', 'v1', 'investigations', investigationId, 'analyze'
    And request { pattern_ids: ['payment-completed-without-shipment'], execution_mode: 'SYNC' }
    When method post
    Then status 200
    And match response.analysis_status == 'EVALUATED'
    And match response.effective_window ==
      """
      {
        from: '2026-07-01T10:00:00Z',
        to: '2026-07-08T10:00:00Z',
        observed_at: '2026-07-08T10:00:00Z',
        anchor: 'EARLIEST_CORRELATION_EVENT',
        source_event_count: 2
      }
      """
    And match response.findings[0].pattern_id == 'payment-completed-without-shipment'
    * def firstWindow = response.effective_window
    * def firstFinding = response.findings[0]

    Given path 'api', 'v1', 'investigations', investigationId, 'analyze'
    And request { pattern_ids: ['payment-completed-without-shipment'], execution_mode: 'SYNC' }
    When method post
    Then status 200
    And match response.effective_window == firstWindow
    And match response.findings[0] == firstFinding

    Given path 'api', 'v1', 'investigations', investigationId, 'close'
    And header If-Match = '"v0"'
    And request { root_cause: 'historical fixture validation', resolution_summary: 'deterministic window verified' }
    When method post
    Then status 200

    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And header Content-Type = 'application/json'
    And request "ALTER TABLE event_hunter.forensics_events DELETE WHERE event_id IN ('evt-historical-pattern-order','evt-historical-pattern-payment') SETTINGS mutations_sync = 1"
    When method post
    Then status 200
    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And header Content-Type = 'application/json'
    And request "ALTER TABLE event_hunter.poc_forensics_events DELETE WHERE event_id IN ('evt-historical-pattern-order','evt-historical-pattern-payment') SETTINGS mutations_sync = 1"
    When method post
    Then status 200

  @eh-p1-1-011
  Scenario: 完全沒有 canonical event 時不得用空 findings 假裝 Pattern no-match
    * def emptyCorrelationId = 'ORDER-NO-EVENTS-' + java.util.UUID.randomUUID()
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] Explicit empty Pattern source', severity: 'LOW', correlation_id: '#(emptyCorrelationId)' }
    When method post
    Then status 201
    * def investigationId = response.id

    Given path 'api', 'v1', 'investigations', investigationId, 'analyze'
    And request { pattern_ids: ['payment-completed-without-shipment'], execution_mode: 'SYNC' }
    When method post
    Then status 200
    And match response.analysis_status == 'NO_EVENTS'
    And match response.effective_window == null
    And match response.findings == []

    Given path 'api', 'v1', 'investigations', investigationId, 'close'
    And header If-Match = '"v0"'
    And request { root_cause: 'no canonical event exists', resolution_summary: 'empty source state verified' }
    When method post
    Then status 200
