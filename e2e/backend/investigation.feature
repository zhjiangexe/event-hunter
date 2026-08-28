@requires-fixtures
Feature: REQ-EH-002 REQ-EH-004 REQ-EH-005 REQ-EH-014 REQ-EH-015 案件生命週期、協作與整合 Read Model

  Background:
    # 此檔案需要 ORDER-2001 fixture，並以可修改案件的 Investigator 身分執行。
    * url apiBaseUrl
    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200

  @p1-1-02-03
  Scenario: 案件更新使用 ETag 樂觀鎖且清單支援後端複合篩選
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] 樂觀鎖驗證案件', severity: 'HIGH', correlation_id: 'ORDER-2001' }
    When method post
    Then status 201
    * def investigationId = response.id
    * def originalEtag = responseHeaders['Etag'][0]

    Given path 'api', 'v1', 'investigations', investigationId
    And header If-Match = originalEtag
    And request { status: 'INVESTIGATING', assignee: 'shipping-oncall', priority: 'P0', tags: ['shipping', 'urgent'], related_correlation_ids: ['SHIPMENT-2001'] }
    When method patch
    Then status 200
    And match response.lock_version == 1
    And match response.assignee == 'shipping-oncall'
    And match response.priority == 'P0'
    And match response.tags == ['shipping', 'urgent']
    And match response.related_correlation_ids == ['SHIPMENT-2001']
    And match response.sla_status == '#string'
    * def updatedEtag = responseHeaders['Etag'][0]

    Given path 'api', 'v1', 'investigations', investigationId, 'notes'
    And header If-Match = updatedEtag
    And request { body: '第一筆 append-only 協作筆記。' }
    When method post
    Then status 201
    And match response.investigation.lock_version == 2
    And match response.note.body == '第一筆 append-only 協作筆記。'
    And match response.note.author_role == 'INVESTIGATOR'
    * def noteEtag = responseHeaders['Etag'][0]

    # 故意重用追加筆記前的 ETag，證明後送達的請求不會覆蓋先前協作資料。
    Given path 'api', 'v1', 'investigations', investigationId, 'notes'
    And header If-Match = updatedEtag
    And request { body: '過期版本不可追加案件筆記。' }
    When method post
    Then status 409
    And match response.code == 'OPTIMISTIC_LOCK_CONFLICT'
    And match response.current_lock_version == 2

    Given path 'api', 'v1', 'investigations', investigationId
    When method get
    Then status 200
    And match response.collaboration_notes == '#[1]'
    And match response.collaboration_notes[0].body == '第一筆 append-only 協作筆記。'
    And match response.last_updated_by == 'demo-investigator'

    Given path 'api', 'v1', 'investigations'
    And param status = 'INVESTIGATING'
    And param severity = 'HIGH'
    And param priority = 'P0'
    And param tag = 'urgent'
    And param assignee = 'shipping-oncall'
    And param correlation_id = 'ORDER-2001'
    And param sort_by = 'updated_at'
    And param sort_order = 'desc'
    When method get
    Then status 200
    And match response.items[*].id contains investigationId

    Given path 'api', 'v1', 'investigations', investigationId, 'close'
    And header If-Match = noteEtag
    And request { root_cause: '測試根因', resolution_summary: '測試結案摘要' }
    When method post
    Then status 200
    And match response.status == 'CLOSED'

    Given path 'api', 'v1', 'investigations', investigationId, 'summary'
    And param from = '2026-08-20T11:00:00Z'
    And param to = '2026-08-20T11:06:00Z'
    When method get
    Then status 200
    And match response.audit_entries[*].action contains 'UPDATE_INVESTIGATION'
    And match response.audit_entries[*].action contains 'ADD_INVESTIGATION_NOTE'
    And match response.audit_entries[*].action contains 'CLOSE_INVESTIGATION'

  Scenario: Summary 會組合案件、時間線、Finding 與 Evidence reference
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] 整合摘要案件', severity: 'HIGH', correlation_id: 'ORDER-2001' }
    When method post
    Then status 201
    * def investigationId = response.id

    Given path 'api', 'v1', 'investigations', investigationId, 'analyze'
    And request { execution_mode: 'SYNC' }
    When method post
    Then status 200

    # Summary 是有界的唯讀組合查詢，不會啟動 Replay 或修改正式業務資料。
    Given path 'api', 'v1', 'investigations', investigationId, 'summary'
    And param from = '2026-08-20T11:00:00Z'
    And param to = '2026-08-20T11:06:00Z'
    When method get
    Then status 200
    And match response.investigation_id == investigationId
    And match response.case.correlation_id == 'ORDER-2001'
    And match response.timeline.event_count == 2
    And match response.pattern_findings[0].pattern_id == 'payment-completed-without-shipment'
    And match response.audit_entries[*].action contains 'CREATE_INVESTIGATION'
    And match response.audit_entries[*].action contains 'ANALYZE_INVESTIGATION'
    And match response.partial == '#boolean'
    And match response.source_status.postgres == 'OK'
    And match response.source_status.clickhouse == 'OK'

  @eh-p1-1-014
  Scenario: Timeline 建案保存不可變 Incident Window 並作為 Summary 與 Evidence 預設窗口
    * def incidentFrom = '2026-08-20T11:00:00Z'
    * def incidentTo = '2026-08-20T11:06:00Z'
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] Incident Window 案件', severity: 'HIGH', correlation_id: 'ORDER-2001', incident_from: '#(incidentFrom)', incident_to: '#(incidentTo)' }
    When method post
    Then status 201
    And match response.incident_from == incidentFrom
    And match response.incident_to == incidentTo
    And match response.incident_window_source == 'TIMELINE_SEARCH'
    * def investigationId = response.id

    # 未帶查詢參數時必須重用建立當下保存的 baseline，而不是 rolling now。
    Given path 'api', 'v1', 'investigations', investigationId, 'summary'
    When method get
    Then status 200
    And match response.query_window == { from: '#(incidentFrom)', to: '#(incidentTo)' }
    And match response.timeline.event_count == 2
    And match response.event_retention_boundary == '#string'
    And match response.source_last_success_at.postgres == '#string'
    And match response.source_last_success_at.clickhouse == '#string'
    And match response.timeline.truncated == false

    Given path 'api', 'v1', 'investigations', investigationId, 'evidence-bundle'
    When method get
    Then status 200
    And match response.query_window == { from: '#(incidentFrom)', to: '#(incidentTo)' }

    # 臨時檢視其他窗口不會改寫案件基準。
    Given path 'api', 'v1', 'investigations', investigationId, 'summary'
    And param from = '2026-08-20T11:00:00Z'
    And param to = '2026-08-20T11:03:00Z'
    When method get
    Then status 200
    And match response.query_window.to == '2026-08-20T11:03:00Z'

    Given path 'api', 'v1', 'investigations', investigationId
    When method get
    Then status 200
    And match response.incident_from == incidentFrom
    And match response.incident_to == incidentTo
    And match response.incident_window_source == 'TIMELINE_SEARCH'

  @eh-p1-1-014
  Scenario: Incident Window 必須成對且不可超過七天
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] 不完整窗口', severity: 'HIGH', correlation_id: 'ORDER-2001', incident_from: '2026-08-20T11:00:00Z' }
    When method post
    Then status 422
    And match response.code == 'INVALID_INCIDENT_WINDOW'

    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] 過大窗口', severity: 'HIGH', correlation_id: 'ORDER-2001', incident_from: '2026-08-01T00:00:00Z', incident_to: '2026-08-20T00:00:00Z' }
    When method post
    Then status 422
    And match response.code == 'INVALID_INCIDENT_WINDOW'

  Scenario: Evidence Bundle 是只含參照與 SHA-256 的 JSON manifest
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] 證據包驗證案件', severity: 'HIGH', correlation_id: 'ORDER-2001' }
    When method post
    Then status 201
    * def investigationId = response.id

    Given path 'api', 'v1', 'investigations', investigationId, 'analyze'
    And request { execution_mode: 'SYNC' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'investigations', investigationId, 'evidence-bundle'
    And param from = '2026-08-20T11:00:00Z'
    And param to = '2026-08-20T11:06:00Z'
    When method get
    Then status 200
    And match response.schema_version == 1
    And match response.checksum_algorithm == 'SHA-256'
    And match response.manifest_sha256 == '#regex [0-9a-f]{64}'
    And match response.partial == false
    And match response.warnings == []
    And match response.source_status == { postgres: 'OK', clickhouse: 'NOT_REQUESTED', technical_observability: 'NOT_REQUESTED' }
    And match response.items == '#[3]'
    And match response.items[*].evidence_type contains ['EVENT', 'TRACE', 'PATTERN_FINDING']
    And match each response.items contains { id: '#uuid', reference: '#string', source: '#string', open_action: '#string', collected_at: '#string', checksum: '#regex [0-9a-f]{64}' }
    # MVP 不把完整 Logs／Traces 複製進證據包，避免形成第二套 Observability 儲存。
    And match response !contains { raw_logs: '#present' }
    And match response !contains { raw_traces: '#present' }

  Scenario: Timeline Event 可用 bounded source lookup 加入既有案件且重送不重複
    * def uniqueId = java.util.UUID.randomUUID().toString()
    * def primaryCorrelationId = 'CASE-ATTACH-' + uniqueId
    * def from = '2026-08-20T11:00:00Z'
    * def to = '2026-08-20T11:06:00Z'

    Given path 'api', 'v1', 'events', 'search'
    And param from = from
    And param to = to
    And param correlation_id = 'ORDER-2001'
    When method get
    Then status 200
    And assert response.count > 0
    * def eventId = response.events[0].event_id

    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] Timeline evidence attachment', severity: 'HIGH', correlation_id: '#(primaryCorrelationId)' }
    When method post
    Then status 201
    * def investigationId = response.id
    * def originalEtag = responseHeaders['Etag'][0]

    Given path 'api', 'v1', 'investigations', investigationId, 'evidence', 'events'
    And header If-Match = originalEtag
    And request { event_id: '#(eventId)', from: '#(from)', to: '#(to)' }
    When method post
    Then status 200
    And match response.attached == true
    And match response.investigation.lock_version == 1
    And match response.investigation.related_correlation_ids contains 'ORDER-2001'
    And match response.evidence == { id: '#uuid', evidence_type: 'EVENT', reference: '#(eventId)', source: 'CLICKHOUSE', open_action: 'GRAFANA_EVENT', collected_at: '#string', checksum: '#regex [0-9a-f]{64}' }
    * def attachedEtag = responseHeaders['Etag'][0]

    # 使用目前 ETag 重送同一個 Event，必須回傳既有 reference 且不增加版本。
    Given path 'api', 'v1', 'investigations', investigationId, 'evidence', 'events'
    And header If-Match = attachedEtag
    And request { event_id: '#(eventId)', from: '#(from)', to: '#(to)' }
    When method post
    Then status 200
    And match response.attached == false
    And match response.investigation.lock_version == 1
    And match response.evidence.reference == eventId

    # 過期 ETag 不得因 idempotency 偷渡成功。
    Given path 'api', 'v1', 'investigations', investigationId, 'evidence', 'events'
    And header If-Match = originalEtag
    And request { event_id: '#(eventId)', from: '#(from)', to: '#(to)' }
    When method post
    Then status 409
    And match response.code == 'OPTIMISTIC_LOCK_CONFLICT'
    And match response.current_lock_version == 1

    Given path 'api', 'v1', 'investigations', investigationId, 'evidence-bundle'
    And param from = from
    And param to = to
    When method get
    Then status 200
    * def attachedItems = karate.filter(response.items, function(x){ return x.reference == eventId })
    And match attachedItems == '#[1]'
    And match attachedItems[0].evidence_type == 'EVENT'
    And match attachedItems[0].checksum == '#regex [0-9a-f]{64}'

    Given path 'api', 'v1', 'investigations', investigationId, 'summary'
    And param from = from
    And param to = to
    When method get
    Then status 200
    And match response.audit_entries[*].action contains 'ATTACH_INVESTIGATION_EVENT'

    Given path 'api', 'v1', 'investigations', investigationId, 'evidence', 'events'
    And header If-Match = attachedEtag
    And request { event_id: 'event-does-not-exist', from: '#(from)', to: '#(to)' }
    When method post
    Then status 404
    And match response.code == 'EVENT_NOT_FOUND'

    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'VIEWER' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'investigations', investigationId, 'evidence', 'events'
    And header If-Match = attachedEtag
    And request { event_id: '#(eventId)', from: '#(from)', to: '#(to)' }
    When method post
    Then status 403
    And match response.code == 'FORBIDDEN'

  Scenario: Investigation Summary 的 payload expansion 同樣只允許 ADMIN
    Given path 'api', 'v1', 'investigations'
    And request { title: '[E2E] Summary payload authorization', severity: 'MEDIUM', correlation_id: 'ORDER-1001' }
    When method post
    Then status 201
    * def investigationId = response.id

    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'VIEWER' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'investigations', investigationId, 'summary'
    And param from = '2026-08-20T10:00:00Z'
    And param to = '2026-08-20T10:06:00Z'
    And param include_payload = true
    When method get
    Then status 403
    And match response.code == 'FORBIDDEN'

    Given path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'ADMIN' }
    When method post
    Then status 200

    Given path 'api', 'v1', 'investigations', investigationId, 'summary'
    And param from = '2026-08-20T10:00:00Z'
    And param to = '2026-08-20T10:06:00Z'
    And param include_payload = true
    When method get
    Then status 200
    And match response.timeline.events[0].payload.totalAmount == '[REDACTED_AMOUNT]'
