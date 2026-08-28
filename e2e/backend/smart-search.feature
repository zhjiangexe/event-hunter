Feature: REQ-EH-011 Smart Search 確定性識別

  Background:
    * url apiBaseUrl
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'VIEWER' }
    When method post
    Then status 200

  Scenario: 明確前綴會回傳唯一 identifier 類型
    Given path 'api', 'v1', 'search', 'identify'
    And request { input: 'trace:0123456789abcdef0123456789abcdef' }
    When method post
    Then status 200
    And match response.status == 'IDENTIFIED'
    And match response.normalized_input == '0123456789abcdef0123456789abcdef'
    And match response.candidates == [{ identifier_type: 'TRACE_ID', query_parameter: 'trace_id', certainty: 'EXACT', reason: 'EXPLICIT_PREFIX' }]

  Scenario: Opaque identifier 回傳候選而不自動猜測
    Given path 'api', 'v1', 'search', 'identify'
    And request { input: 'ORDER-2001' }
    When method post
    Then status 200
    And match response.status == 'AMBIGUOUS'
    And match response.message == 'SELECT_IDENTIFIER_TYPE'
    And match response.candidates[*].identifier_type contains ['CORRELATION_ID', 'AGGREGATE_ID', 'EVENT_ID']

  Scenario Outline: 失效輸入不會進入事件查詢
    Given path 'api', 'v1', 'search', 'identify'
    And request { input: '<input>' }
    When method post
    Then status 200
    And match response.status == 'INVALID'
    And match response.candidates == []

    Examples:
      | input                                  |
      |                                        |
      | x                                      |
      | contains spaces                        |
      | trace:00000000000000000000000000000000 |
