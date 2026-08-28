@infrastructure
Feature: Phase 1.1 safe Ingestion Issues read surface

  Background:
    * def clickhouseBasicAuth = 'Basic ' + java.util.Base64.getEncoder().encodeToString((clickhouseUser + ':' + clickhousePassword).getBytes())
    * def token = java.util.UUID.randomUUID().toString().replace('-', '')
    * def sourceTopic = 'e2e.ingestion.' + token
    * def correlationId = 'E2E-INGESTION-' + token
    * def payloadHash = token + token
    * def technicalId = token + token
    * def now = java.time.Instant.now()
    * def isoInstant = java.time.format.DateTimeFormatter.ISO_INSTANT
    * def contractAt = isoInstant.format(now.minusSeconds(3))
    * def admissionAt = isoInstant.format(now.minusSeconds(2))
    * def technicalAt = isoInstant.format(now.minusSeconds(1))
    * def from = isoInstant.format(now.minusSeconds(60))
    * def to = isoInstant.format(now.plusSeconds(60))

  Scenario: contract admission and technical failures are searchable without sensitive details
    * def contractRow = { source_topic: '#(sourceTopic)', source_partition: 0, source_offset: 100, event_id: '#("evt-contract-" + token)', event_type: 'OrderCreated', correlation_id: '#(correlationId)', error_type: 'SCHEMA_VIOLATION', error_code: 'SCHEMA_VIOLATION', error_summary: 'must-never-leak-secret-value', payload_sha256: '#(payloadHash)', failed_at: '#(contractAt)' }
    * def admissionRow = { source_topic: '#(sourceTopic)', source_partition: 0, source_offset: 101, event_id: '#("evt-admission-" + token)', event_type: 'OrderCreated', correlation_id: '#(correlationId)', error_code: 'MISSING_OR_INVALID_REQUIRED_FIELD', payload_sha256: '#(payloadHash)', admission_profile: 'minimum-envelope-v1', failed_at: '#(admissionAt)' }
    * def technicalRow = { failure_id: '#(technicalId)', dlq_topic: 'event-hunter.poc-clickhouse-sink.dlq', dlq_partition: 0, dlq_offset: 102, source_topic: '#(sourceTopic)', source_partition: 0, source_offset: 102, connector_name: 'event-hunter-poc-raw-landing', connector_task: 0, failure_stage: 'TASK_PUT', exception_class: 'java.lang.RuntimeException', payload_sha256: '#(payloadHash)', observed_at: '#(technicalAt)' }

    * call read('../helpers/clickhouse-insert.feature') { authorization: '#(clickhouseBasicAuth)', table: 'event_ingestion_failures', row: '#(contractRow)' }
    * match responseStatus == 200
    * call read('../helpers/clickhouse-insert.feature') { authorization: '#(clickhouseBasicAuth)', table: 'poc_event_admission_failures', row: '#(admissionRow)' }
    * match responseStatus == 200
    * call read('../helpers/clickhouse-insert.feature') { authorization: '#(clickhouseBasicAuth)', table: 'ingestion_technical_failures', row: '#(technicalRow)' }
    * match responseStatus == 200

    Given url apiBaseUrl
    And path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'ingestion-issues'
    And param from = from
    And param to = to
    And param source_topic = sourceTopic
    And param page_size = 2
    When method get
    Then status 200
    And match response.page_size == 2
    And match response.items == '#[2]'
    And match response.next_cursor == '#string'
    And match response.items[*].kind contains only ['TECHNICAL_DLQ', 'ADMISSION_QUARANTINE']
    * def firstPageJSON = karate.toString(response)
    * assert !firstPageJSON.includes('raw_payload')
    * assert !firstPageJSON.includes('error_summary')
    * assert !firstPageJSON.includes('must-never-leak-secret-value')
    * assert !firstPageJSON.includes('exception_message')
    * assert !firstPageJSON.includes('stacktrace')
    * def nextCursor = response.next_cursor

    Given path 'api', 'v1', 'ingestion-issues'
    And param from = from
    And param to = to
    And param source_topic = sourceTopic
    And param page_size = 2
    And param cursor = nextCursor
    When method get
    Then status 200
    And match response.items == '#[1]'
    And match response.items[0].kind == 'CONTRACT_VALIDATION'
    And match response.next_cursor == null

    Given path 'api', 'v1', 'ingestion-issues'
    And param from = from
    And param to = to
    And param kind = 'TECHNICAL_DLQ'
    And param source_topic = sourceTopic
    When method get
    Then status 200
    And match response.items == '#[1]'
    And match response.items[0] contains { kind: 'TECHNICAL_DLQ', error_code: 'CONNECTOR_TASK_FAILURE', source_topic: '#(sourceTopic)', source_offset: 102, failure_stage: 'TASK_PUT', exception_class: 'java.lang.RuntimeException' }

    # Exact source topic makes cleanup recoverable and prevents E2E data from entering product results.
    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And header Content-Type = 'application/json'
    And request "ALTER TABLE event_hunter.event_ingestion_failures DELETE WHERE source_topic = '" + sourceTopic + "' SETTINGS mutations_sync = 1"
    When method post
    Then status 200

    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And header Content-Type = 'application/json'
    And request "ALTER TABLE event_hunter.poc_event_admission_failures DELETE WHERE source_topic = '" + sourceTopic + "' SETTINGS mutations_sync = 1"
    When method post
    Then status 200

    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And header Content-Type = 'application/json'
    And request "ALTER TABLE event_hunter.ingestion_technical_failures DELETE WHERE source_topic = '" + sourceTopic + "' SETTINGS mutations_sync = 1"
    When method post
    Then status 200

  Scenario: invalid kind cursor and over-wide windows are rejected deterministically
    Given url apiBaseUrl
    And path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'VIEWER' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'ingestion-issues'
    And param kind = 'UNKNOWN'
    When method get
    Then status 422
    And match response.code == 'INVALID_INGESTION_ISSUE_KIND'

    Given path 'api', 'v1', 'ingestion-issues'
    And param cursor = 'not-an-opaque-cursor'
    When method get
    Then status 422
    And match response.code == 'INVALID_CURSOR'

    Given path 'api', 'v1', 'ingestion-issues'
    And param from = '2026-08-01T00:00:00Z'
    And param to = '2026-08-09T00:00:00Z'
    When method get
    Then status 422
    And match response.code == 'INVALID_INGESTION_ISSUE_FILTER'
