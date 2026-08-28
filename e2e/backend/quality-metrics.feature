@infrastructure @requires-quality-fixture
Feature: REQ-EH-008 固定窗口的事件品質聚合

  Background:
    # 執行前由 production fixture loader 載入 quality-window.json，再執行一次 quality-worker aggregate。
    * url clickhouseHttpUrl
    * def clickhouseBasicAuth = 'Basic ' + java.util.Base64.getEncoder().encodeToString((clickhouseUser + ':' + clickhousePassword).getBytes())
    * header Authorization = clickhouseBasicAuth

  Scenario: 一分鐘窗口會區分業務 duplicate 與 sink redelivery
    Given param output_format_json_quote_64bit_integers = 0
    # Partition 99 is reserved for this synthetic fixture so product demo events in the same window cannot alter the aggregate.
    And def qualityQuery = "SELECT topic_name, kafka_partition, consumer_group_id, event_count, duplicate_count, schema_violation_count, out_of_order_count, dlq_count, max_event_delay_ms, consumer_lag_messages, max_processing_latency_ms, source FROM event_hunter.event_quality_metrics WHERE window_start = parseDateTime64BestEffort('2026-08-20T15:00:00Z') AND window_end = parseDateTime64BestEffort('2026-08-20T15:01:00Z') AND topic_name = 'payment.events' AND kafka_partition = 99 AND consumer_group_id = 'shipping-service-v1' ORDER BY calculated_at DESC LIMIT 1 FORMAT JSON"
    And param query = qualityQuery
    When method get
    Then status 200
    And match response.data[0].topic_name == 'payment.events'
    And match response.data[0].event_count == 4
    And match response.data[0].duplicate_count == 1
    And match response.data[0].schema_violation_count == 1
    And match response.data[0].out_of_order_count == 1
    And match response.data[0].dlq_count == 2
    And match response.data[0].max_event_delay_ms == 400
    And match response.data[0].consumer_lag_messages == 120
    And match response.data[0].max_processing_latency_ms == 200
    And match response.data[0].source == 'quality-worker-v1'

  Scenario: 只有 ingestion failure 的窗口仍會產生告警用品質列
    Given param output_format_json_quote_64bit_integers = 0
    And def failureOnlyQuery = "SELECT topic_name, kafka_partition, consumer_group_id, event_count, schema_violation_count, dlq_count, source FROM event_hunter.event_quality_metrics WHERE window_start = parseDateTime64BestEffort('2026-08-20T16:00:00Z') AND window_end = parseDateTime64BestEffort('2026-08-20T16:01:00Z') AND topic_name = 'order.events' AND kafka_partition = 1 AND consumer_group_id = '' ORDER BY calculated_at DESC LIMIT 1 FORMAT JSON"
    And param query = failureOnlyQuery
    When method get
    Then status 200
    And match response.data[0].event_count == 0
    And match response.data[0].schema_violation_count == 1
    And match response.data[0].dlq_count == 1
    And match response.data[0].source == 'quality-worker-v1'
