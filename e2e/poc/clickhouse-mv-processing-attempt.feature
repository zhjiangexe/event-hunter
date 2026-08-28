@poc @infrastructure @processing-attempts
Feature: ClickHouse-first processing-attempt admission

  Background:
    * def marker = 'E2E-ATTEMPT-' + pocRunToken
    * def clickhouseBasicAuth = 'Basic ' + java.util.Base64.getEncoder().encodeToString((clickhouseUser + ':' + clickhousePassword).getBytes())
    * def toBase64 = function(value){ return java.util.Base64.getEncoder().encodeToString(value.getBytes()) }
    * configure retry = { count: 60, interval: 1000 }

  Scenario: valid attempts are promoted while contract violations are quarantined and redelivery is logically deduplicated
    * def eventId = marker + '-EVENT'
    * def traceId = 'abcdefabcdefabcdefabcdefabcdefab'
    * def startedAt = '2026-08-27T12:00:00Z'
    * def completedAt = '2026-08-27T12:00:00.100Z'
    * def base = { eventId: '#(eventId)', eventType: 'OrderCreated', correlationId: '#(marker)', traceId: '#(traceId)', consumerGroupId: 'payment-service-v1', consumerService: 'payment-service', attempt: 1, retryReason: null, retryTopic: null, kafkaTopic: 'order.events', kafkaPartition: 0, kafkaOffset: 9001, startedAt: '#(startedAt)', completedAt: null, observedAt: '#(startedAt)' }
    * def validStarted = karate.merge(base, { attemptId: '#(marker + "-STARTED")', processingStatus: 'STARTED' })
    * def validSucceeded = karate.merge(base, { attemptId: '#(marker + "-SUCCEEDED")', processingStatus: 'SUCCEEDED', completedAt: '#(completedAt)', observedAt: '#(completedAt)' })
    * def validFailed = karate.merge(base, { attemptId: '#(marker + "-FAILED")', processingStatus: 'FAILED', retryReason: 'gateway timeout', completedAt: '#(completedAt)', observedAt: '#(completedAt)' })
    * def invalidEnum = karate.merge(base, { attemptId: '#(marker + "-ENUM")', processingStatus: 'BROKEN' })
    * def invalidReason = karate.merge(base, { attemptId: '#(marker + "-REASON")', processingStatus: 'DLQ', completedAt: '#(completedAt)' })
    * def missingService = karate.merge(base, { attemptId: '#(marker + "-MISSING")', processingStatus: 'STARTED' })
    * remove missingService.consumerService
    * def invalidTrace = karate.merge(base, { attemptId: '#(marker + "-TRACE")', processingStatus: 'STARTED', traceId: '00000000000000000000000000000000' })
    * def invalidTime = karate.merge(base, { attemptId: '#(marker + "-TIME")', processingStatus: 'STARTED', startedAt: 'not-a-time' })
    * def values = [ '#(validStarted)', '#(validSucceeded)', '#(validSucceeded)', '#(validFailed)', '#(invalidEnum)', '#(invalidReason)', '#(missingService)', '#(invalidTrace)', '#(invalidTime)' ]
    * def records = karate.map(values, function(value){ return { value: toBase64(karate.toString(value)) } })

    Given url redpandaHttpProxyUrl
    And path 'topics', 'event-hunter.processing-attempts'
    And header Content-Type = 'application/vnd.kafka.binary.v2+json'
    And header Accept = 'application/vnd.kafka.v2+json'
    And request { records: '#(records)' }
    When method post
    Then status 200
    And assert response.offsets.length >= 1

    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And param output_format_json_quote_64bit_integers = 0
    And param query = "SELECT count() AS count FROM event_hunter_poc.poc_processing_attempt_landing_raw WHERE position(raw_payload, '" + marker + "') > 0 FORMAT JSON"
    And retry until responseStatus == 200 && response.data[0].count == 9
    When method get
    Then status 200
    And match response.data[0].count == 9

    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And param output_format_json_quote_64bit_integers = 0
    And param query = "SELECT count() AS count FROM event_hunter.poc_event_processing_attempts FINAL WHERE startsWith(attempt_id, '" + marker + "') FORMAT JSON"
    And retry until responseStatus == 200 && response.data[0].count == 3
    When method get
    Then status 200
    And match response.data[0].count == 3

    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And param output_format_json_quote_64bit_integers = 0
    And param query = "SELECT processing_status, count() AS count FROM event_hunter.poc_event_processing_attempts FINAL WHERE startsWith(attempt_id, '" + marker + "') GROUP BY processing_status ORDER BY processing_status FORMAT JSON"
    When method get
    Then status 200
    And match response.data contains deep { processing_status: 'STARTED', count: 1 }
    And match response.data contains deep { processing_status: 'FAILED', count: 1 }
    And match response.data contains deep { processing_status: 'SUCCEEDED', count: 1 }

    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And param output_format_json_quote_64bit_integers = 0
    And param query = "SELECT error_code, count() AS count FROM event_hunter.poc_processing_attempt_admission_failures WHERE startsWith(attempt_id, '" + marker + "') GROUP BY error_code ORDER BY error_code FORMAT JSON"
    And retry until responseStatus == 200 && response.rows == 2
    When method get
    Then status 200
    And match response.data ==
      """
      [
        { "error_code": "MISSING_OR_INVALID_REQUIRED_FIELD", "count": 2 },
        { "error_code": "SCHEMA_VIOLATION", "count": 3 }
      ]
      """
