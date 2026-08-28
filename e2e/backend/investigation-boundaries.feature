Feature: REQ-EH-002 REQ-EH-004 REQ-EH-005 REQ-EH-014 REQ-EH-015 案件 API 邊界、狀態機與權限

  Background:
    * url apiBaseUrl
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200

  @p1-1-02-03
  Scenario: 案件清單使用後端穩定排序與綁定 query 的 cursor 分頁
    * def uniqueId = java.util.UUID.randomUUID().toString()
    * def correlationId = 'PAGE-' + uniqueId
    * def titleA = '[E2E] Cursor page case A ' + uniqueId
    * def titleB = '[E2E] Cursor page case B ' + uniqueId
    * def titleC = '[E2E] Cursor page case C ' + uniqueId

    Given path 'api', 'v1', 'investigations'
    And request { title: '#(titleA)', severity: 'LOW', correlation_id: '#(correlationId)' }
    When method post
    Then status 201
    * def firstId = response.id

    Given path 'api', 'v1', 'investigations'
    And request { title: '#(titleB)', severity: 'MEDIUM', correlation_id: '#(correlationId)' }
    When method post
    Then status 201
    * def secondId = response.id
    * def secondCaseNo = response.case_no

    Given path 'api', 'v1', 'investigations'
    And request { title: '#(titleC)', severity: 'HIGH', correlation_id: '#(correlationId)' }
    When method post
    Then status 201
    * def thirdId = response.id
    * def expectedIds = karate.append(firstId, secondId, thirdId)
    * def expectedPageOneIds = karate.append(firstId, secondId)

    # 人類使用的 case no／title 搜尋由後端執行，不只篩選目前載入的一頁。
    Given path 'api', 'v1', 'investigations'
    And param query = titleB
    And param page_size = 20
    When method get
    Then status 200
    And match response.items[*].id == ['#(secondId)']

    Given path 'api', 'v1', 'investigations'
    And param query = secondCaseNo
    And param page_size = 20
    When method get
    Then status 200
    And match response.items[*].id == ['#(secondId)']

    Given path 'api', 'v1', 'investigations'
    And param correlation_id = correlationId
    And param page_size = 2
    And param sort_by = 'updated_at'
    And param sort_order = 'asc'
    When method get
    Then status 200
    And match response.items == '#[2]'
    And match response.next_cursor == '#string'
    * def pageOneIds = karate.jsonPath(response, '$.items[*].id')
    And match pageOneIds == expectedPageOneIds
    * def nextCursor = response.next_cursor

    # Cursor 包含排序契約；換方向重用必須拒絕，避免分頁遺漏或重複。
    Given path 'api', 'v1', 'investigations'
    And param correlation_id = correlationId
    And param page_size = 2
    And param sort_by = 'updated_at'
    And param sort_order = 'desc'
    And param cursor = nextCursor
    When method get
    Then status 422
    And match response.code == 'INVALID_CURSOR'

    Given path 'api', 'v1', 'investigations'
    And param correlation_id = correlationId
    And param page_size = 2
    And param sort_by = 'updated_at'
    And param sort_order = 'asc'
    And param cursor = nextCursor
    When method get
    Then status 200
    And match response.items == '#[1]'
    And match response.next_cursor == null
    * def pageTwoIds = karate.jsonPath(response, '$.items[*].id')
    * def actualIds = karate.append(pageOneIds, pageTwoIds)
    And match actualIds contains only expectedIds

    # 透過正式 API 收斂本 scenario 建立的資料，避免重跑後增加 Overview 的 open count。
    Given path 'api', 'v1', 'investigations', firstId, 'close'
    And header If-Match = '"v0"'
    And request { root_cause: 'E2E lifecycle cleanup', resolution_summary: 'cursor contract verified' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'investigations', secondId, 'close'
    And header If-Match = '"v0"'
    And request { root_cause: 'E2E lifecycle cleanup', resolution_summary: 'cursor contract verified' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'investigations', thirdId, 'close'
    And header If-Match = '"v0"'
    And request { root_cause: 'E2E lifecycle cleanup', resolution_summary: 'cursor contract verified' }
    When method post
    Then status 200

  @p1-1-02-03
  Scenario Outline: 案件清單拒絕無效 page_size、cursor 與排序條件
    Given path 'api', 'v1', 'investigations'
    And param <parameter> = '<value>'
    When method get
    Then status 422
    And match response.code == '<errorCode>'

    Examples:
      | parameter | value         | errorCode         |
      | page_size | 0             | INVALID_PAGE_SIZE |
      | page_size | 201           | INVALID_PAGE_SIZE |
      | cursor    | not-a-cursor  | INVALID_CURSOR    |
      | sort_by   | severity      | INVALID_SORT      |
      | sort_order | sideways     | INVALID_SORT      |

  @p1-1-ux-08
  Scenario: 案件文字搜尋拒絕超過契約上限的輸入
    * def oversizedQuery = 'x'.repeat(101)
    Given path 'api', 'v1', 'investigations'
    And param query = oversizedQuery
    When method get
    Then status 422
    And match response.code == 'INVALID_QUERY'

  Scenario: 案件狀態只能沿合法路徑轉移且 CLOSED 後不可再修改
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] State machine boundary case', severity: 'HIGH', correlation_id: 'STATE-BOUNDARY' }
    When method post
    Then status 201
    And match response.allowed_transitions == ['INVESTIGATING', 'CLOSED']
    * def investigationId = response.id
    * def openEtag = responseHeaders['Etag'][0]

    Given path 'api', 'v1', 'investigations', investigationId
    And request { status: 'INVESTIGATING' }
    When method patch
    Then status 428
    And match response.code == 'IF_MATCH_REQUIRED'

    Given path 'api', 'v1', 'investigations', investigationId
    And header If-Match = openEtag
    And request { unexpected_field: true }
    When method patch
    Then status 422
    And match response.code == 'INVALID_UPDATE'

    Given path 'api', 'v1', 'investigations', investigationId
    And header If-Match = openEtag
    And request { status: 'RESOLVED', root_cause: 'too early', resolution_summary: 'invalid direct transition' }
    When method patch
    Then status 409
    And match response.code == 'INVALID_STATE_TRANSITION'

    Given path 'api', 'v1', 'investigations', investigationId
    And header If-Match = openEtag
    And request { status: 'CLOSED' }
    When method patch
    Then status 409
    And match response.code == 'CLOSE_OPERATION_REQUIRED'

    Given path 'api', 'v1', 'investigations', investigationId
    And header If-Match = openEtag
    And request { status: 'INVESTIGATING' }
    When method patch
    Then status 200
    And match response.status == 'INVESTIGATING'
    And match response.allowed_transitions == ['WAITING_APPROVAL', 'RESOLVED', 'CLOSED']
    And match response.lock_version == 1
    * def investigatingEtag = responseHeaders['Etag'][0]

    Given path 'api', 'v1', 'investigations', investigationId
    And header If-Match = investigatingEtag
    And request { status: 'RESOLVED' }
    When method patch
    Then status 422
    And match response.code == 'RESOLUTION_FIELDS_REQUIRED'

    Given path 'api', 'v1', 'investigations', investigationId
    And header If-Match = investigatingEtag
    And request { status: 'RESOLVED', root_cause: 'consumer mapping defect', resolution_summary: 'mapping corrected' }
    When method patch
    Then status 200
    And match response.status == 'RESOLVED'
    And match response.allowed_transitions == ['INVESTIGATING', 'CLOSED']
    And match response.lock_version == 2
    * def resolvedEtag = responseHeaders['Etag'][0]

    Given path 'api', 'v1', 'investigations', investigationId, 'close'
    And header If-Match = resolvedEtag
    And request { root_cause: 'consumer mapping defect', resolution_summary: '  ' }
    When method post
    Then status 422
    And match response.code == 'RESOLUTION_FIELDS_REQUIRED'

    Given path 'api', 'v1', 'investigations', investigationId, 'close'
    And header If-Match = resolvedEtag
    And request { root_cause: 'consumer mapping defect', resolution_summary: 'mapping corrected', fixed_version: '1.1.0' }
    When method post
    Then status 200
    And match response.status == 'CLOSED'
    And match response.allowed_transitions == []
    And match response.lock_version == 3
    And match response.closed_at == '#string'
    * def closedEtag = responseHeaders['Etag'][0]

    Given path 'api', 'v1', 'investigations', investigationId
    And header If-Match = closedEtag
    And request { title: 'closed cases are immutable' }
    When method patch
    Then status 409
    And match response.code == 'INVALID_STATE_TRANSITION'

    Given path 'api', 'v1', 'investigations', investigationId, 'notes'
    And header If-Match = closedEtag
    And request { body: 'closed cases reject new notes' }
    When method post
    Then status 409
    And match response.code == 'CASE_NOT_MUTABLE'

    Given path 'api', 'v1', 'investigations', investigationId, 'close'
    And header If-Match = closedEtag
    And request { root_cause: 'consumer mapping defect', resolution_summary: 'duplicate close must fail' }
    When method post
    Then status 409
    And match response.code == 'OPTIMISTIC_LOCK_CONFLICT'
    And match response.current_lock_version == 3

  @eh-p1-1-016
  Scenario: allowed_transitions 驅動完整審核、解決與重新開啟流程並留下 Audit
    * def uniqueId = java.util.UUID.randomUUID().toString()
    * def correlationId = 'STATE-ACTIONS-' + uniqueId

    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] Explicit state actions', severity: 'HIGH', correlation_id: '#(correlationId)' }
    When method post
    Then status 201
    And match response.status == 'OPEN'
    And match response.allowed_transitions == ['INVESTIGATING', 'CLOSED']
    * def investigationId = response.id
    * def currentEtag = responseHeaders['Etag'][0]

    Given path 'api', 'v1', 'investigations', investigationId
    And header If-Match = currentEtag
    And request { status: 'INVESTIGATING' }
    When method patch
    Then status 200
    And match response.allowed_transitions == ['WAITING_APPROVAL', 'RESOLVED', 'CLOSED']
    * def currentEtag = responseHeaders['Etag'][0]

    Given path 'api', 'v1', 'investigations', investigationId
    And header If-Match = currentEtag
    And request { status: 'WAITING_APPROVAL' }
    When method patch
    Then status 200
    And match response.status == 'WAITING_APPROVAL'
    And match response.allowed_transitions == ['INVESTIGATING', 'RESOLVED', 'CLOSED']
    * def currentEtag = responseHeaders['Etag'][0]

    Given path 'api', 'v1', 'investigations', investigationId
    And header If-Match = currentEtag
    And request { status: 'INVESTIGATING' }
    When method patch
    Then status 200
    And match response.status == 'INVESTIGATING'
    * def currentEtag = responseHeaders['Etag'][0]

    Given path 'api', 'v1', 'investigations', investigationId
    And header If-Match = currentEtag
    And request { status: 'RESOLVED', root_cause: 'approval revealed a mapping defect', resolution_summary: 'mapping corrected and replay verified' }
    When method patch
    Then status 200
    And match response.status == 'RESOLVED'
    And match response.allowed_transitions == ['INVESTIGATING', 'CLOSED']
    * def currentEtag = responseHeaders['Etag'][0]

    Given path 'api', 'v1', 'investigations', investigationId
    And header If-Match = currentEtag
    And request { status: 'INVESTIGATING' }
    When method patch
    Then status 200
    And match response.status == 'INVESTIGATING'
    And match response.allowed_transitions == ['WAITING_APPROVAL', 'RESOLVED', 'CLOSED']
    * def currentEtag = responseHeaders['Etag'][0]

    Given path 'api', 'v1', 'investigations', investigationId, 'close'
    And header If-Match = currentEtag
    And request { root_cause: 'approval revealed a mapping defect', resolution_summary: 'reopened verification completed' }
    When method post
    Then status 200
    And match response.status == 'CLOSED'
    And match response.allowed_transitions == []

    Given path 'api', 'v1', 'investigations', investigationId, 'summary'
    When method get
    Then status 200
    And match response.audit_entries[*].action contains 'CREATE_INVESTIGATION'
    And match response.audit_entries[*].action contains 'UPDATE_INVESTIGATION'
    And match response.audit_entries[*].action contains 'CLOSE_INVESTIGATION'
    * def transitionTargets = karate.jsonPath(response, "$.audit_entries[?(@.action == 'UPDATE_INVESTIGATION')].metadata.to_status")
    And match transitionTargets contains 'WAITING_APPROVAL'
    And match transitionTargets contains 'RESOLVED'
    And match transitionTargets contains 'INVESTIGATING'

  Scenario: Viewer 可讀案件但所有案件 mutation surface 都維持唯讀
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] Viewer authorization boundary', severity: 'MEDIUM', correlation_id: 'VIEWER-BOUNDARY' }
    When method post
    Then status 201
    * def investigationId = response.id
    * def originalEtag = responseHeaders['Etag'][0]

    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'VIEWER' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'investigations', investigationId
    When method get
    Then status 200
    And match response.lock_version == 0

    Given path 'api', 'v1', 'investigations', investigationId
    And header If-Match = originalEtag
    And request { status: 'INVESTIGATING' }
    When method patch
    Then status 403
    And match response.code == 'FORBIDDEN'

    Given path 'api', 'v1', 'investigations', investigationId, 'notes'
    And header If-Match = originalEtag
    And request { body: 'viewer cannot append notes' }
    When method post
    Then status 403
    And match response.code == 'FORBIDDEN'

    Given path 'api', 'v1', 'investigations', investigationId, 'evidence', 'events'
    And header If-Match = originalEtag
    And request { event_id: 'not-evaluated', from: '2026-08-20T11:00:00Z', to: '2026-08-20T11:06:00Z' }
    When method post
    Then status 403
    And match response.code == 'FORBIDDEN'

    Given path 'api', 'v1', 'investigations', investigationId, 'analyze'
    And request { execution_mode: 'SYNC' }
    When method post
    Then status 403
    And match response.code == 'FORBIDDEN'

    Given path 'api', 'v1', 'investigations', investigationId, 'close'
    And header If-Match = originalEtag
    And request { root_cause: 'viewer cannot close', resolution_summary: 'must remain open' }
    When method post
    Then status 403
    And match response.code == 'FORBIDDEN'

    Given path 'api', 'v1', 'investigations', investigationId
    When method get
    Then status 200
    And match response.status == 'OPEN'
    And match response.lock_version == 0
    And match response.collaboration_notes == []
    And match response.evidence == []

    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'investigations', investigationId, 'close'
    And header If-Match = originalEtag
    And request { root_cause: 'E2E lifecycle cleanup', resolution_summary: 'Viewer authorization contract verified' }
    When method post
    Then status 200

  Scenario: 不存在的案件在 detail、summary、evidence 與 mutation API 都回傳相同 404 語意
    * def missingId = java.util.UUID.randomUUID().toString()

    Given path 'api', 'v1', 'investigations', missingId
    When method get
    Then status 404
    And match response.code == 'NOT_FOUND'

    Given path 'api', 'v1', 'investigations', missingId, 'summary'
    And param from = '2026-08-20T11:00:00Z'
    And param to = '2026-08-20T11:06:00Z'
    When method get
    Then status 404
    And match response.code == 'NOT_FOUND'

    Given path 'api', 'v1', 'investigations', missingId, 'evidence-bundle'
    And param from = '2026-08-20T11:00:00Z'
    And param to = '2026-08-20T11:06:00Z'
    When method get
    Then status 404
    And match response.code == 'NOT_FOUND'

    Given path 'api', 'v1', 'investigations', missingId
    And header If-Match = '"v0"'
    And request { status: 'INVESTIGATING' }
    When method patch
    Then status 404
    And match response.code == 'NOT_FOUND'

    Given path 'api', 'v1', 'investigations', missingId, 'analyze'
    And request { execution_mode: 'SYNC' }
    When method post
    Then status 404
    And match response.code == 'NOT_FOUND'
