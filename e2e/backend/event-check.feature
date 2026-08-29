@requires-fixtures
Feature: Event Check bounded evaluation Snapshot and Case handoff

  Background:
    * url apiBaseUrl
    * def runId = java.util.UUID.randomUUID().toString()
    * def from = '2026-08-20T11:00:00Z'
    * def to = '2026-08-20T11:06:00Z'
    * def pinnedModel = { id: 'order-fulfillment', version: 2 }
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200

  Scenario: Immutable Check Model registry exposes pinned versions and rejects unknown versions
    Given path 'api', 'v1', 'check-models'
    When method get
    Then status 200
    And match response == '#[4]'
    And match response[*].model.model_id contains 'order-fulfillment'
    And match response[*].model.kind contains 'GLOBAL_CHECK'

    Given path 'api', 'v1', 'check-models', 'order-fulfillment', 'versions', 2
    When method get
    Then status 200
    And match response.model.model_id == 'order-fulfillment'
    And match response.model.version == 2
    And match response.checksum == '#regex [0-9a-f]{64}'
    And match response.source_path == 'contracts/check-models/order-fulfillment.yaml'

    Given path 'api', 'v1', 'check-models', 'order-fulfillment', 'versions', 2, 'source'
    When method get
    Then status 200
    And match response.model_id == 'order-fulfillment'
    And match response.version == 2
    And match response.source_path == 'contracts/check-models/order-fulfillment.yaml'
    And match response.checksum == '#regex [0-9a-f]{64}'
    And match response.yaml contains 'model_id: order-fulfillment'
    And match response.yaml contains 'MISSING_SHIPMENT_AFTER_PAYMENT'

    Given path 'api', 'v1', 'check-models', 'order-fulfillment', 'versions', 999
    When method get
    Then status 404

    Given path 'api', 'v1', 'check-models', 'order-fulfillment', 'versions', 999, 'source'
    When method get
    Then status 404

  Scenario Outline: Every supported typed identifier resolves the same explainable bounded scope
    * def identifier = { type: '<identifierType>', value: '<identifierValue>' }
    * if ('<aggregateType>' != '') identifier.qualifier = { aggregate_type: '<aggregateType>' }
    * if ('<businessKeyName>' != '') identifier.qualifier = { business_key_name: '<businessKeyName>' }
    Given path 'api', 'v1', 'event-checks', 'evaluations'
    And request { identifier: '#(identifier)', from: '#(from)', to: '#(to)', model: '#(pinnedModel)' }
    When method post
    Then status 200
    And match response.resolution_status == 'EVALUATED'
    And match response.source_health.status == 'HEALTHY'
    And match response.scope.mode == 'STANDARD_SCOPE'
    And match response.scope.events[*].event_type == ['OrderCreated', 'PaymentCompleted']
    And match response.scope.relationships[*].relation_type contains 'SEED'
    And match response.scope.relationships[*].relation_type contains 'CAUSATION'
    And match response.result.check_status == 'DEVIATED'
    And match response.result.findings[*].code contains 'MISSING_SHIPMENT_AFTER_PAYMENT'
    And match response.event_set_hash == '#regex [0-9a-f]{64}'
    And match response.evaluation_hash == '#regex [0-9a-f]{64}'

    Examples:
      | identifierType | identifierValue                  | aggregateType | businessKeyName |
      | CORRELATION_ID | ORDER-2001                       |               |                 |
      | EVENT_ID       | evt-order-2001-001               |               |                 |
      | TRACE_ID       | 44444444444444444444444444444444 |               |                 |
      | AGGREGATE_ID   | ORDER-2001                       | Order         |                 |
      | BUSINESS_KEY   | ORDER-2001                       |               | order_id        |

  Scenario: Custom scope requires a reason and preserves excluded evidence provenance
    Given path 'api', 'v1', 'event-checks', 'evaluations'
    And request
      """
      {
        identifier: { type: 'CORRELATION_ID', value: 'ORDER-2001' },
        from: '#(from)',
        to: '#(to)',
        model: '#(pinnedModel)',
        scope_adjustments: {
          include: [],
          exclude: [{ event_id: 'evt-payment-2001-001', reason: 'E2E validates explicit analyst scope' }]
        }
      }
      """
    When method post
    Then status 200
    And match response.resolution_status == 'EVALUATED'
    And match response.scope.mode == 'CUSTOM_SCOPE'
    And match response.scope.events[*].event_id == ['evt-order-2001-001']
    And match response.scope.excluded_events == '#[1]'
    And match response.scope.excluded_events[0].event_id == 'evt-payment-2001-001'
    And match response.scope.excluded_events[0].reason == 'E2E validates explicit analyst scope'

    Given path 'api', 'v1', 'event-checks', 'evaluations'
    And request
      """
      {
        identifier: { type: 'CORRELATION_ID', value: 'ORDER-2001' },
        from: '#(from)',
        to: '#(to)',
        model: '#(pinnedModel)',
        scope_adjustments: { include: [], exclude: [{ event_id: 'evt-payment-2001-001', reason: '' }] }
      }
      """
    When method post
    Then status 422

  Scenario: No data is an explicit state and invalid or unbounded requests fail closed
    Given path 'api', 'v1', 'event-checks', 'evaluations'
    And request { identifier: { type: 'CORRELATION_ID', value: 'ORDER-DOES-NOT-EXIST' }, from: '#(from)', to: '#(to)' }
    When method post
    Then status 200
    And match response.resolution_status == 'NO_DATA'
    And match response.scope.events == []
    And match response.result == null

    Given path 'api', 'v1', 'event-checks', 'evaluations'
    And request { identifier: { type: 'CORRELATION_ID', value: 'ORDER-2001' }, from: '2026-08-01T00:00:00Z', to: '2026-08-09T00:00:00Z' }
    When method post
    Then status 422

    Given path 'api', 'v1', 'event-checks', 'evaluations'
    And request { identifier: { type: 'UNKNOWN', value: 'ORDER-2001' }, from: '#(from)', to: '#(to)' }
    When method post
    Then status 422

  Scenario: Snapshot save is idempotent and Case handoff preserves optimistic locking and Finding feedback
    * def evaluationRequest = { identifier: { type: 'CORRELATION_ID', value: 'ORDER-2001' }, from: '#(from)', to: '#(to)', model: '#(pinnedModel)' }
    Given path 'api', 'v1', 'event-checks', 'evaluations'
    And request evaluationRequest
    When method post
    Then status 200
    * def eventSetHash = response.event_set_hash
    * def evaluationHash = response.evaluation_hash

    Given path 'api', 'v1', 'check-snapshots'
    And header Idempotency-Key = 'event-check-e2e-' + runId
    And request { evaluation_request: '#(evaluationRequest)', expected_event_set_hash: '#(eventSetHash)', expected_evaluation_hash: '#(evaluationHash)' }
    When method post
    Then status 201
    And match response.id == '#uuid'
    And match response.event_set_hash == eventSetHash
    And match response.evaluation_hash == evaluationHash
    And match response.event_references == '#[2]'
    And match response.finding_feedback == '#[1]'
    * def snapshotId = response.id
    * def findingId = response.finding_feedback[0].finding_id

    Given path 'api', 'v1', 'check-snapshots'
    And header Idempotency-Key = 'event-check-e2e-' + runId
    And request { evaluation_request: '#(evaluationRequest)', expected_event_set_hash: '#(eventSetHash)', expected_evaluation_hash: '#(evaluationHash)' }
    When method post
    Then status 200
    And match response.id == snapshotId

    Given path 'api', 'v1', 'check-snapshots', snapshotId
    When method get
    Then status 200
    And match response.id == snapshotId
    And match response.result.findings[*].id contains findingId

    Given path 'api', 'v1', 'check-snapshots'
    And param identifier = 'ORDER-2001'
    And param check_status = 'DEVIATED'
    And param page_size = 1
    When method get
    Then status 200
    And match response.page_size == 1
    And match response.items == '#[1]'
    And match response.items[0].id == snapshotId
    And match response.items[0].evaluation_request.identifier.value == 'ORDER-2001'
    And match response.items[0].check_status == 'DEVIATED'
    And match response.items[0].event_count == 2
    And match response.items[0].finding_count == 1
    And match response.items[0].linked_case_count == 0

    Given path 'api', 'v1', 'check-findings', findingId, 'feedback'
    And header If-Match = '"v0"'
    And request { status: 'CONFIRMED' }
    When method patch
    Then status 200
    And match response.status == 'CONFIRMED'
    And match response.lock_version == 1

    Given path 'api', 'v1', 'investigations'
    And request { title: '#("[E2E] Event Check Snapshot " + runId)', severity: 'HIGH', correlation_id: 'ORDER-2001' }
    When method post
    Then status 201
    * def investigationId = response.id
    * def originalEtag = responseHeaders['Etag'][0]

    Given path 'api', 'v1', 'investigations', investigationId, 'check-snapshots'
    And header If-Match = originalEtag
    And request { snapshot_id: '#(snapshotId)' }
    When method post
    Then status 201
    And match response.investigation_id == investigationId
    And match response.snapshot_id == snapshotId
    * def attachedEtag = responseHeaders['Etag'][0]

    Given path 'api', 'v1', 'investigations', investigationId, 'check-snapshots'
    And header If-Match = attachedEtag
    And request { snapshot_id: '#(snapshotId)' }
    When method post
    Then status 200
    And match response.snapshot_id == snapshotId

    Given path 'api', 'v1', 'investigations', investigationId, 'check-snapshots'
    When method get
    Then status 200
    And match response[*].snapshot_id contains snapshotId

    Given path 'api', 'v1', 'check-snapshots'
    And param identifier = 'ORDER-2001'
    And param page_size = 1
    When method get
    Then status 200
    And match response.items[0].id == snapshotId
    And match response.items[0].linked_case_count == 1

    Given path 'api', 'v1', 'investigations', investigationId, 'close'
    And header If-Match = attachedEtag
    And request { root_cause: 'EH-ECM-006 acceptance', resolution_summary: 'Snapshot handoff and feedback verified' }
    When method post
    Then status 200
    And match response.status == 'CLOSED'

  Scenario: Saved Results rejects malformed cursors statuses and page sizes
    Given path 'api', 'v1', 'check-snapshots'
    And param cursor = 'not-a-cursor'
    When method get
    Then status 422
    And match response.code == 'INVALID_CURSOR'

    Given path 'api', 'v1', 'check-snapshots'
    And param check_status = 'FAILED'
    When method get
    Then status 422
    And match response.code == 'INVALID_CHECK_SNAPSHOT_FILTER'

    Given path 'api', 'v1', 'check-snapshots'
    And param page_size = 101
    When method get
    Then status 422
    And match response.code == 'INVALID_CHECK_SNAPSHOT_FILTER'

  Scenario: Viewer can evaluate but cannot save Snapshots or classify Findings
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'VIEWER' }
    When method post
    Then status 200

    * def viewerRequest = { identifier: { type: 'CORRELATION_ID', value: 'ORDER-2001' }, from: '#(from)', to: '#(to)', model: '#(pinnedModel)' }
    Given path 'api', 'v1', 'event-checks', 'evaluations'
    And request viewerRequest
    When method post
    Then status 200
    * def viewerEventSetHash = response.event_set_hash
    * def viewerEvaluationHash = response.evaluation_hash

    Given path 'api', 'v1', 'check-snapshots'
    And header Idempotency-Key = 'event-check-viewer-' + runId
    And request { evaluation_request: '#(viewerRequest)', expected_event_set_hash: '#(viewerEventSetHash)', expected_evaluation_hash: '#(viewerEvaluationHash)' }
    When method post
    Then status 403
    And match response.code == 'FORBIDDEN'
