@poc @infrastructure
Feature: ClickHouse-first store-all and promote-searchable ingestion POC

  Background:
    * def runToken = pocRunToken
    * def marker = 'E2E-POC-' + runToken
    * def clickhouseBasicAuth = 'Basic ' + java.util.Base64.getEncoder().encodeToString((clickhouseUser + ':' + clickhousePassword).getBytes())
    * def toBase64 = function(value){ return java.util.Base64.getEncoder().encodeToString(value.getBytes()) }
    * configure retry = { count: 60, interval: 1000 }

  Scenario: every Kafka message lands, while searchable records and structural failures are classified honestly
    * def occurredAt = '2026-08-27T00:00:00Z'
    * def traceId = '1234567890abcdef1234567890abcdef'
    * def fullValidId = marker + '-VALID'
    * def minimumOnlyId = marker + '-MINIMUM'
    * def unknownTypeId = marker + '-UNKNOWN'
    * def unknownVersionId = marker + '-UNKNOWN-VERSION'
    * def invalidTraceId = marker + '-INVALID-TRACE'
    * def missingFieldId = marker + '-MISSING'
    * def nonObjectPayloadId = marker + '-NON-OBJECT'
    * def malformedId = marker + '-MALFORMED'
    # Resolve embedded variables while constructing the Karate objects, before serializing them to Kafka bytes.
    * def fullValidObject = { eventId: '#(fullValidId)', eventType: 'OrderCreated', eventVersion: 1, occurredAt: '#(occurredAt)', producer: 'poc-e2e', correlationId: '#(marker)', causationId: null, traceId: '#(traceId)', aggregateType: 'Order', aggregateId: '#(marker)', sequence: 1, payload: { orderId: '#(marker)', customerId: 'CUSTOMER-POC', totalAmount: 100, currency: 'TWD' } }
    * def minimumOnlyObject = { eventId: '#(minimumOnlyId)', eventType: 'OrderCreated', eventVersion: 1, occurredAt: '#(occurredAt)', producer: 'poc-e2e', correlationId: '#(marker)', causationId: null, traceId: null, aggregateType: 'Order', aggregateId: '#(marker)', sequence: 2, payload: {} }
    * def unknownTypeObject = { eventId: '#(unknownTypeId)', eventType: 'FutureOrderEvent', eventVersion: 1, occurredAt: '#(occurredAt)', producer: 'poc-e2e', correlationId: '#(marker)', causationId: null, traceId: '#(traceId)', aggregateType: 'Order', aggregateId: '#(marker)', sequence: 3, payload: {} }
    * def unknownVersionObject = { eventId: '#(unknownVersionId)', eventType: 'OrderCreated', eventVersion: 2, occurredAt: '#(occurredAt)', producer: 'poc-e2e', correlationId: '#(marker)', causationId: null, traceId: '#(traceId)', aggregateType: 'Order', aggregateId: '#(marker)', sequence: 4, payload: { orderId: '#(marker)', customerId: 'CUSTOMER-POC', totalAmount: 100, currency: 'TWD' } }
    * def invalidTraceObject = { eventId: '#(invalidTraceId)', eventType: 'OrderCreated', eventVersion: 1, occurredAt: '#(occurredAt)', producer: 'poc-e2e', correlationId: '#(marker)', causationId: null, traceId: '00000000000000000000000000000000', aggregateType: 'Order', aggregateId: '#(marker)', sequence: 5, payload: { orderId: '#(marker)', customerId: 'CUSTOMER-POC', totalAmount: 100, currency: 'TWD' } }
    * def missingFieldObject = { eventId: '#(missingFieldId)', eventType: 'OrderCreated', eventVersion: 1, occurredAt: '#(occurredAt)', producer: 'poc-e2e', causationId: null, traceId: '#(traceId)', aggregateType: 'Order', aggregateId: '#(marker)', sequence: 6, payload: {} }
    * def nonObjectPayloadObject = { eventId: '#(nonObjectPayloadId)', eventType: 'OrderCreated', eventVersion: 1, occurredAt: '#(occurredAt)', producer: 'poc-e2e', correlationId: '#(marker)', causationId: null, traceId: '#(traceId)', aggregateType: 'Order', aggregateId: '#(marker)', sequence: 7, payload: [] }
    * def fullValid = karate.toString(fullValidObject)
    * def minimumOnly = karate.toString(minimumOnlyObject)
    * def unknownType = karate.toString(unknownTypeObject)
    * def unknownVersion = karate.toString(unknownVersionObject)
    * def invalidTrace = karate.toString(invalidTraceObject)
    * def missingField = karate.toString(missingFieldObject)
    * def nonObjectPayload = karate.toString(nonObjectPayloadObject)
    * def malformed = '{"eventId":"' + malformedId + '",'

    Given url redpandaHttpProxyUrl
    And path 'topics', 'event-hunter.poc.events'
    And header Content-Type = 'application/vnd.kafka.binary.v2+json'
    And header Accept = 'application/vnd.kafka.v2+json'
    And request { records: [ { value: '#(toBase64(fullValid))' }, { value: '#(toBase64(minimumOnly))' }, { value: '#(toBase64(unknownType))' }, { value: '#(toBase64(unknownVersion))' }, { value: '#(toBase64(invalidTrace))' }, { value: '#(toBase64(missingField))' }, { value: '#(toBase64(nonObjectPayload))' }, { value: '#(toBase64(malformed))' } ] }
    When method post
    Then status 200
    # Binary HTTP Proxy groups the acknowledgement by assigned partition; the raw landing count below
    # is the end-to-end proof that all eight records were consumed.
    And assert response.offsets.length >= 1
    And match each response.offsets contains { partition: '#number', offset: '#number' }

    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And param output_format_json_quote_64bit_integers = 0
    And param query = "SELECT count() AS count FROM event_hunter_poc.poc_event_landing_raw WHERE position(raw_payload, '" + marker + "') > 0 FORMAT JSON"
    And retry until responseStatus == 200 && response.data[0].count == 8
    When method get
    Then status 200
    And match response.data[0].count == 8

    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And param output_format_json_quote_64bit_integers = 0
    And param query = "SELECT count() AS count FROM event_hunter.poc_forensics_events WHERE startsWith(event_id, '" + marker + "') FORMAT JSON"
    And retry until responseStatus == 200 && response.data[0].count == 4
    When method get
    Then status 200
    And match response.data[0].count == 4

    # Promoted payload must remain a JSON object compatible with the canonical
    # forensics_events contract; JSON_QUERY would incorrectly produce [{...}].
    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And param output_format_json_quote_64bit_integers = 0
    And param query = "SELECT count() AS count FROM event_hunter.poc_forensics_events WHERE startsWith(event_id, '" + marker + "') AND JSONType(payload) = 'Object' FORMAT JSON"
    When method get
    Then status 200
    And match response.data[0].count == 4

    # SEARCHABLE means the minimum envelope is structurally usable. Unknown contract metadata
    # remains visible for investigation but is explicitly marked with quality warnings.
    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And param output_format_json_quote_64bit_integers = 0
    And param query = "SELECT admission_status, count() AS count FROM event_hunter.poc_forensics_events WHERE startsWith(event_id, '" + marker + "') GROUP BY admission_status ORDER BY admission_status FORMAT JSON"
    When method get
    Then status 200
    And match response.data ==
      """
      [
        { "admission_status": "SEARCHABLE", "count": 1 },
        { "admission_status": "SEARCHABLE_WITH_WARNINGS", "count": 3 }
      ]
      """

    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And param query = "SELECT event_id, quality_flags FROM event_hunter.poc_forensics_events WHERE event_id IN ('" + unknownTypeId + "','" + unknownVersionId + "','" + invalidTraceId + "') ORDER BY event_id FORMAT JSON"
    When method get
    Then status 200
    And match response.data contains deep { event_id: '#(unknownTypeId)', quality_flags: ['UNKNOWN_EVENT_TYPE'] }
    And match response.data contains deep { event_id: '#(unknownVersionId)', quality_flags: ['UNKNOWN_EVENT_VERSION'] }
    And match response.data contains deep { event_id: '#(invalidTraceId)', quality_flags: ['INVALID_TRACE_ID'] }

    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And param output_format_json_quote_64bit_integers = 0
    And param query = "SELECT error_code, count() AS count FROM event_hunter.poc_event_admission_failures WHERE payload_sha256 IN (SELECT payload_sha256 FROM event_hunter_poc.poc_event_landing_raw WHERE position(raw_payload, '" + marker + "') > 0) GROUP BY error_code ORDER BY error_code FORMAT JSON"
    And retry until responseStatus == 200 && response.rows == 4
    When method get
    Then status 200
    And match response.data ==
      """
      [
        { "error_code": "INVALID_JSON", "count": 1 },
        { "error_code": "MISSING_OR_INVALID_REQUIRED_FIELD", "count": 1 },
        { "error_code": "PAYLOAD_NOT_OBJECT", "count": 1 },
        { "error_code": "SCHEMA_VIOLATION", "count": 1 }
      ]
      """

    # Known event types must include their contract-required payload keys even in the ClickHouse-first profile.
    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And param output_format_json_quote_64bit_integers = 0
    And param query = "SELECT count() AS count FROM event_hunter.poc_event_admission_failures WHERE event_id = '" + minimumOnlyId + "' AND error_code = 'SCHEMA_VIOLATION' FORMAT JSON"
    When method get
    Then status 200
    And match response.data[0].count == 1

  Scenario: an actual sink poison record is projected from technical DLQ without sensitive details
    * assert pocTechnicalSourcePartition >= 0
    * assert pocTechnicalSourceOffset >= 0

    Given url technicalDlqProjectorBaseUrl
    And path 'health', 'ready'
    When method get
    Then status 200
    And match response.status == 'ready'

    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And param output_format_json_quote_64bit_integers = 0
    And param query = "SELECT failure_id,dlq_topic,source_topic,source_partition,source_offset,connector_name,connector_task,failure_stage,exception_class,payload_sha256 FROM event_hunter.ingestion_technical_failures WHERE source_topic = 'event-hunter.poc.events' AND source_partition = " + pocTechnicalSourcePartition + " AND source_offset = " + pocTechnicalSourceOffset + " FORMAT JSON"
    And retry until responseStatus == 200 && response.rows == 1
    When method get
    Then status 200
    And match response.data == '#[1]'
    And match response.data[0] contains { failure_id: '#regex [a-f0-9]{64}', dlq_topic: 'event-hunter.poc-clickhouse-sink.dlq', source_topic: 'event-hunter.poc.events', source_partition: '#(pocTechnicalSourcePartition)', source_offset: '#(pocTechnicalSourceOffset)', connector_name: 'event-hunter-poc-raw-landing', connector_task: 0, failure_stage: 'TASK_PUT', exception_class: 'java.lang.RuntimeException', payload_sha256: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855' }
    * def safeProjectionJSON = karate.toString(response.data[0])
    * assert !safeProjectionJSON.includes('raw_payload')
    * assert !safeProjectionJSON.includes('exception_message')
    * assert !safeProjectionJSON.includes('stacktrace')
