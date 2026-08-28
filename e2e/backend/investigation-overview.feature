@requires-fixtures
Feature: REQ-EH-010 調查總覽與資料來源健康

  Background:
    * url apiBaseUrl
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200

  Scenario: Overview 由後端聚合真實案件與事件資料
    Given path 'api', 'v1', 'investigations', 'overview'
    When method get
    Then status 200
    And match response.generated_at == '#string'
    And match response.window == { from: '#string', to: '#string' }
    And assert new Date(response.window.to).getTime() - new Date(response.window.from).getTime() == 72 * 60 * 60 * 1000
    And match response.partial == '#boolean'
    And match response.control_plane.cases contains { open: '#number', investigating: '#number', closed: '#number' }
    And match response.control_plane.severity contains { low: '#number', medium: '#number', high: '#number', critical: '#number' }
    And match response.control_plane.activity contains { cases_created: '#number', cases_closed: '#number', grafana_alerts: '#number', scenario_passed: '#number', scenario_failed: '#number', scenario_timed_out: '#number' }
    And match response.events.event_count == '#number'
    And match response.events.top_producers == '#array'
    And match response.events.top_event_types == '#array'
    And match response.sources[*].name contains ['postgresql', 'clickhouse', 'tempo', 'loki', 'grafana']

  Scenario: 建立新案件後 Overview open count 會由伺服器端增加
    Given path 'api', 'v1', 'investigations', 'overview'
    When method get
    Then status 200
    * def openBefore = response.control_plane.cases.open

    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] Overview aggregate verification', severity: 'HIGH', correlation_id: 'OVERVIEW-E2E' }
    When method post
    Then status 201
    And match response.incident_window_source == 'MANUAL_DEFAULT'
    And assert new Date(response.incident_to).getTime() - new Date(response.incident_from).getTime() == 72 * 60 * 60 * 1000

    Given path 'api', 'v1', 'investigations', 'overview'
    When method get
    Then status 200
    And match response.control_plane.cases.open == openBefore + 1

  Scenario: Source Health 使用全站一致的四段狀態
    Given path 'api', 'v1', 'source-health'
    When method get
    Then status 200
    And match response.generated_at == '#string'
    And match response.partial == '#boolean'
    And match each response.sources contains { name: '#string', state: '#regex fresh|stale|partial|unavailable', last_success_at: '##string', lag_ms: '##number', reason: '##string' }
