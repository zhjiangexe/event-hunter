Feature: REQ-EH-013 個人 Saved Search 與唯讀內建 Preset

  Background:
    * url apiBaseUrl
    * def runId = java.util.UUID.randomUUID().toString()
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'VIEWER' }
    When method post
    Then status 200

  Scenario: Viewer 可建立、列出、開啟並刪除自己的 bounded Timeline 搜尋
    * def searchName = 'viewer-payment-failure-' + runId
    Given path 'api', 'v1', 'saved-searches'
    And request
      """
      {
        name: '#(searchName)',
        target: 'TIMELINE',
        query: {
          from: '2026-08-20T11:00:00Z',
          to: '2026-08-20T11:06:00Z',
          event_type: 'PaymentFailed',
          severity: 'HIGH',
          include_processing_attempts: true
        }
      }
      """
    When method post
    Then status 201
    And match response.id == '#uuid'
    And match response.owner_subject == 'demo-viewer'
    And match response.name == searchName
    And match response.target == 'TIMELINE'
    And match response.open_url == '#regex /timeline\\?.*'
    And match response.open_url contains 'event_type=PaymentFailed'
    And match response.open_url !contains 'include_payload'
    * def savedSearchId = response.id

    Given path 'api', 'v1', 'saved-searches'
    When method get
    Then status 200
    And match response.items[*].id contains savedSearchId
    And match response.items[*].owner_subject contains 'demo-viewer'

    Given path 'api', 'v1', 'saved-searches', savedSearchId
    When method delete
    Then status 204

    Given path 'api', 'v1', 'saved-searches'
    When method get
    Then status 200
    And match response.items[*].id !contains savedSearchId

  Scenario: Saved Search 依 Session subject 隔離且其他帳號不可刪除
    * def searchName = 'owner-isolation-' + runId
    Given path 'api', 'v1', 'saved-searches'
    And request
      """
      {
        name: '#(searchName)',
        target: 'JOURNEY',
        query: {
          from: '2026-08-20T11:00:00Z',
          to: '2026-08-20T11:06:00Z',
          correlation_id: 'ORDER-2001',
          include_processing_attempts: false
        }
      }
      """
    When method post
    Then status 201
    * def viewerSearchId = response.id
    And match response.open_url == '#regex /journey\\?.*'

    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'saved-searches'
    When method get
    Then status 200
    And match response.items[*].id !contains viewerSearchId

    Given path 'api', 'v1', 'saved-searches', viewerSearchId
    When method delete
    Then status 404
    And match response.code == 'SAVED_SEARCH_NOT_FOUND'

    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'VIEWER' }
    When method post
    Then status 200
    Given path 'api', 'v1', 'saved-searches', viewerSearchId
    When method delete
    Then status 204

  Scenario: 不接受無界 Journey、超過七天或 payload 欄位
    Given path 'api', 'v1', 'saved-searches'
    And request { name: 'missing-correlation', target: 'JOURNEY', query: { from: '2026-08-20T11:00:00Z', to: '2026-08-20T11:06:00Z', include_processing_attempts: false } }
    When method post
    Then status 422
    And match response.code == 'INVALID_SAVED_SEARCH'

    Given path 'api', 'v1', 'saved-searches'
    And request { name: 'window-too-large', target: 'TIMELINE', query: { from: '2026-08-01T00:00:00Z', to: '2026-08-20T00:00:00Z', event_type: 'PaymentFailed', include_processing_attempts: true } }
    When method post
    Then status 422
    And match response.code == 'INVALID_SAVED_SEARCH'

    Given path 'api', 'v1', 'saved-searches'
    And request { name: 'payload-not-persisted', target: 'TIMELINE', query: { from: '2026-08-20T11:00:00Z', to: '2026-08-20T11:06:00Z', event_type: 'PaymentFailed', include_payload: true, include_processing_attempts: true } }
    When method post
    Then status 422
    And match response.code == 'INVALID_SAVED_SEARCH'

    Given path 'api', 'v1', 'saved-searches'
    And request { name: 'relative-window-missing', target: 'TIMELINE', query: { time_mode: 'RELATIVE', from: '2026-08-20T11:00:00Z', to: '2026-08-20T11:06:00Z', event_type: 'PaymentFailed' } }
    When method post
    Then status 422
    And match response.code == 'INVALID_SAVED_SEARCH'

    Given path 'api', 'v1', 'saved-searches'
    And request { name: 'absolute-with-relative-window', target: 'TIMELINE', query: { time_mode: 'ABSOLUTE', relative_window_seconds: 3600, from: '2026-08-20T11:00:00Z', to: '2026-08-20T11:06:00Z', event_type: 'PaymentFailed' } }
    When method post
    Then status 422
    And match response.code == 'INVALID_SAVED_SEARCH'

  Scenario: 同一帳號的搜尋名稱不分大小寫且不可重複
    * def searchName = 'duplicate-name-' + runId
    * def body = { name: '#(searchName)', target: 'TIMELINE', query: { from: '2026-08-20T11:00:00Z', to: '2026-08-20T11:06:00Z', event_type: 'PaymentFailed', include_processing_attempts: true } }
    Given path 'api', 'v1', 'saved-searches'
    And request body
    When method post
    Then status 201
    * def savedSearchId = response.id

    Given path 'api', 'v1', 'saved-searches'
    And request body
    When method post
    Then status 409
    And match response.code == 'SAVED_SEARCH_NAME_CONFLICT'

    Given path 'api', 'v1', 'saved-searches', savedSearchId
    When method delete
    Then status 204

  Scenario: 內建 Preset 是後端產生的唯讀相對時間查詢
    Given path 'api', 'v1', 'search-presets'
    When method get
    Then status 200
    And match response.items == '#[4]'
    And match response.items[*].id contains 'payment-failed-72h'
    And match each response.items contains { id: '#string', name: '#string', description: '#string', open_url: '#regex /timeline\\?.*' }
    And match each response.items[*].open_url == '#? _.includes("from=") && _.includes("to=") && !_.includes("include_payload")'

  Scenario: 個人相對時間搜尋每次由後端重算 open URL
    * def searchName = 'relative-payment-failure-' + runId
    Given path 'api', 'v1', 'saved-searches'
    And request
      """
      {
        name: '#(searchName)',
        target: 'TIMELINE',
        query: {
          time_mode: 'RELATIVE',
          relative_window_seconds: 3600,
          from: '2026-08-20T11:00:00Z',
          to: '2026-08-20T11:06:00Z',
          event_type: 'PaymentFailed',
          include_processing_attempts: true
        }
      }
      """
    When method post
    Then status 201
    And match response.query.time_mode == 'RELATIVE'
    And match response.query.relative_window_seconds == 3600
    And match response.open_url contains 'event_type=PaymentFailed'
    And match response.open_url !contains 'from=2026-08-20'
    * def relativeSearchId = response.id
    Given path 'api', 'v1', 'saved-searches'
    When method get
    Then status 200
    * def saved = karate.filter(response.items, x => x.id == relativeSearchId)[0]
    And match saved.query.time_mode == 'RELATIVE'
    And match saved.query.relative_window_seconds == 3600
    And match saved.open_url == '#regex /timeline\\?.*'
    And match saved.open_url contains 'event_type=PaymentFailed'

    Given path 'api', 'v1', 'saved-searches', relativeSearchId
    When method delete
    Then status 204
