@infrastructure
Feature: Scenario Lab 固定劇本由真實後端執行並回查 actual 結果

  Background:
    * url eventLabBaseUrl
    * configure retry = { count: 80, interval: 500 }

  Scenario: Catalog 固定提供 S1 到 S14 並清楚標示 Hybrid 執行模式
    Given path 'api', 'v1', 'scenarios'
    When method get
    Then status 200
    And match response.items[*].id == ['S1','S2','S3','S4','S5','S6','S7','S8','S9','S10','S11','S12','S13','S14']
    And match response.items[0] contains { execution_mode: 'LIVE_SERVICES', synthetic: false }
    And match each response.items.slice(1, 11) contains { execution_mode: 'LAB_INJECTION', synthetic: true }
    And match each response.items.slice(11) contains { execution_mode: 'LIVE_SERVICES', synthetic: false }

  Scenario Outline: 每個劇本都必須以實際 Kafka 與 ClickHouse 結果通過檢查
    Given path 'api', 'v1', 'scenario-runs'
    And request { scenario_id: '<scenarioId>' }
    When method post
    Then status 202
    * def runId = response.run_id
    * def correlationId = response.correlation_id

    Given path 'api', 'v1', 'scenario-runs', runId
    And retry until responseStatus == 200 && (response.status == 'PASSED' || response.status == 'FAILED' || response.status == 'TIMED_OUT')
    When method get
    Then status 200
    And match response.status == 'PASSED'
    And match response.scenario.id == '<scenarioId>'
    And match response.correlation_id == correlationId
    And match response.execution_mode == '<executionMode>'
    And match response.synthetic == <synthetic>
    And match response.actual.event_count == <eventCount>
    * def actualCheck = response.checks.find(x => x.id == '<checkId>')
    And assert actualCheck != null
    And match actualCheck.passed == true
    And match response.links contains { timeline: '#regex http://localhost:28334/timeline\\?correlation_id=.+', grafana: '#string', loki: '#string' }
    And match response.trace_id == '#regex [0-9a-f]{32}'
    And match response.error == null

    Examples:
      | scenarioId | executionMode | synthetic | eventCount | checkId             |
      | S1         | LIVE_SERVICES | false     | 3          | event-sequence       |
      | S2         | LAB_INJECTION | true      | 2          | shipment-missing     |
      | S3         | LAB_INJECTION | true      | 4          | duplicate-event      |
      | S4         | LAB_INJECTION | true      | 3          | out-of-order         |
      | S5         | LAB_INJECTION | true      | 2          | processing-dlq       |
      | S6         | LAB_INJECTION | true      | 0          | schema-violation-dlq |
      | S7         | LAB_INJECTION | true      | 3          | event-delay          |
      | S8         | LAB_INJECTION | true      | 3          | event-sequence       |
      | S9         | LAB_INJECTION | true      | 6          | event-sequence       |
      | S10        | LAB_INJECTION | true      | 6          | event-sequence       |
      | S11        | LAB_INJECTION | true      | 8          | event-sequence       |
      | S12        | LIVE_SERVICES | false     | 3          | event-sequence       |
      | S13        | LIVE_SERVICES | false     | 6          | event-sequence       |
      | S14        | LIVE_SERVICES | false     | 8          | event-sequence       |

  Scenario: LAB_INJECTION 的 envelope、Kafka OTel、Tempo 與 Loki 共用 trace ID
    * def startedMs = java.lang.System.currentTimeMillis() - 60000
    * def startNs = '' + startedMs + '000000'
    Given url eventLabBaseUrl
    And path 'api', 'v1', 'scenario-runs'
    And request { scenario_id: 'S8' }
    When method post
    Then status 202
    * def runId = response.run_id
    * def correlationId = response.correlation_id

    Given path 'api', 'v1', 'scenario-runs', runId
    And retry until responseStatus == 200 && response.status == 'PASSED'
    When method get
    Then status 200
    * def traceId = response.trace_id
    And match response.actual.trace_id == traceId

    Given url tempoBaseUrl
    And path 'api', 'traces', traceId
    And retry until responseStatus == 200 && response.batches.length > 0
    When method get
    Then status 200
    And assert response.batches.length > 0

    * def endMs = java.lang.System.currentTimeMillis() + 60000
    * def endNs = '' + endMs + '000000'
    Given url lokiBaseUrl
    And path 'loki', 'api', 'v1', 'query_range'
    And param query = '{service_name="event-lab"} | correlation_id="' + correlationId + '"'
    And param start = startNs
    And param end = endNs
    And param limit = 20
    And retry until responseStatus == 200 && response.data.result.length > 0
    When method get
    Then status 200
    * def traceIds = response.data.result.map(x => x.stream.trace_id)
    * def eventTypes = response.data.result.map(x => x.stream.event_type).filter(x => x != null)
    And match traceIds contains traceId
    And match eventTypes contains 'PaymentFailed'
    And match eventTypes contains 'OrderCancelled'
