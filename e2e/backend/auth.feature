Feature: REQ-EH-006 示範角色 Session 與後端權限控制

  Background:
    # 所有 API 場景都從外部設定讀取 base URL，避免把測試綁死在單一環境。
    * url apiBaseUrl

  Scenario Outline: 合法示範角色可以建立簽署 Session
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: '<role>' }
    When method post
    Then status 200
    And match response ==
      """
      {
        subject: '#string',
        role: '<role>',
        permissions: '#[] #string'
      }
      """
    # Cookie 必須由後端簽署且禁止 JavaScript 讀取；角色不能只保存在前端狀態。
    And match responseHeaders['Set-Cookie'][0] contains 'eh_demo_session='
    And match responseHeaders['Set-Cookie'][0] contains 'HttpOnly'
    And match responseHeaders['Ratelimit-Limit'][0] == '300'
    And match responseHeaders['Ratelimit-Remaining'][0] == '#string'

    Given path 'api', 'v1', 'auth', 'me'
    When method get
    Then status 200
    And match response.role == '<role>'

    Examples:
      | role         |
      | VIEWER       |
      | INVESTIGATOR |
      | ADMIN        |

  Scenario: 未知角色會被拒絕
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'SUPERUSER' }
    When method post
    Then status 422
    And match response.code == '#string'

  Scenario: Viewer 不可建立調查案件
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'VIEWER' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'investigations'
    And request
      """
      {
        title: 'Viewer 不可建立此案件',
        severity: 'HIGH',
        correlation_id: 'ORDER-2001'
      }
      """
    When method post
    Then status 403
    And match response.code == 'FORBIDDEN'

  Scenario: 清除 Session 後無法再取得目前 Principal
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'auth', 'demo-session'
    When method delete
    Then status 204

    Given path 'api', 'v1', 'auth', 'me'
    When method get
    Then status 401
