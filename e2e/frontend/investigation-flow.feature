@ui @requires-fixtures
Feature: Event Hunter 調查員主要操作流程

  Background:
    # UI E2E 只驗證跨頁核心流程；元件與 hooks 的細節由 Vitest／RTL 負責。
    * configure driver = { type: 'chrome', showDriverLog: true }

  Scenario: Investigator 搜尋時間線、建立案件並查看 Evidence
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/event-check')

    # 帶 broad explorer filter，明確驗證仍保留的 Legacy Event Explorer 能力。
    Given driver webBaseUrl + '/timeline?from=2026-08-20T11%3A00%3A00Z&to=2026-08-20T11%3A06%3A00Z&event_version=1'
    Then match script("document.querySelector('[data-testid=timeline-from]').step") == '1'
    And match script("document.querySelector('[data-testid=timeline-to]').step") == '1'
    When input("[data-testid='timeline-correlation-id']", 'ORDER-2001')
    And click("[data-testid='timeline-search-submit']")
    Then waitFor("[data-testid='timeline-results']")
    And match text("[data-testid='timeline-event-count']") == '2'
    And match text("[data-testid='timeline-event-0-type']") == 'OrderCreated'
    And match text("[data-testid='timeline-event-1-type']") == 'PaymentCompleted'
    And match text("[data-testid='timeline-event-0-occurred-at']") contains '2026/08/20'
    And match attribute("[data-testid='timeline-event-0-occurred-at']", 'datetime') == '2026-08-20T11:00:00Z'

    # Event detail 應可展開，並提供受信任 Grafana Explore／Tempo／Loki 連結。
    When click("[data-testid='timeline-event-0-type']")
    Then waitFor("[data-testid='timeline-event-0-detail']")
    And match exists("[data-testid='timeline-event-0-link-event'][href^='http://localhost:28332/explore?']") == true
    And match script("decodeURIComponent(document.querySelector('[data-testid=timeline-event-0-link-event]').href)") contains 'canonical_forensics_events'
    And match exists("[data-testid='timeline-event-0-link-logs'][href^='http://localhost:28332/explore?']") == true
    And match exists("[data-testid='timeline-event-0-link-trace'][href^='http://localhost:28332/explore?']") == true

    # 由時間線直接建立案件，correlation ID 應沿用目前查詢的 ORDER-2001。
    When click("[data-testid='create-investigation']")
    Then waitFor("[data-testid='investigation-create-modal']")
    And input("[data-testid='investigation-title']", '[E2E] 付款完成但未建立出貨')
    And select("[data-testid='investigation-severity']", 'HIGH')
    And click("[data-testid='investigation-create-submit']")
    Then waitFor("[data-testid='investigation-detail']")

    # Pattern／Evidence 統一在 routeable 案件 workspace 操作，不保留 Timeline 專用副本。
    When click("[data-testid='open-created-investigation']")
    Then waitFor("[data-testid='case-detail']")
    When click("[data-testid='case-tab-patterns']")
    And click("[data-testid='run-case-pattern-analysis']")
    Then waitFor("[data-testid='case-pattern-finding-payment-completed-without-shipment']")
    And match text("[data-testid='case-pattern-analysis-status']") contains '已命中'
    And match text("[data-testid='case-pattern-effective-window']") contains 'source events'

    When click("[data-testid='case-tab-evidence']")
    Then waitFor("[data-testid='case-evidence'] [data-testid='evidence-manifest']")
    And match text("[data-testid='evidence-checksum-algorithm']") == 'SHA-256'
    And match text("[data-testid='evidence-manifest-state']") == 'COMPLETE'
    And match exists("[data-testid='evidence-manifest'] a[href^='http://localhost:28332/explore?']") == true

    # Reload 後 mutation local state 已消失；最近一次分析必須由 durable Audit 還原。
    When script("window.location.reload()")
    Then waitFor("[data-testid='case-detail']")
    When click("[data-testid='case-tab-patterns']")
    Then waitFor("[data-testid='case-pattern-analysis-result']")
    And match text("[data-testid='case-pattern-analysis-status']") contains '已命中'
    And match text("[data-testid='case-pattern-analysis-result']") contains 'payment-completed-without-shipment'

  Scenario: Investigator 可把 Timeline Event 加入案件並保存為 Evidence reference
    * def uniqueId = java.util.UUID.randomUUID().toString()
    * def caseTitle = '[E2E] UI Timeline evidence ' + uniqueId
    * def from = '2026-08-20T11:00:00Z'
    * def to = '2026-08-20T11:06:00Z'
    Given url apiBaseUrl + '/api/v1/auth/demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200
    Given url apiBaseUrl + '/api/v1/investigations'
    And request { title: '#(caseTitle)', severity: 'HIGH', correlation_id: 'ORDER-2001' }
    When method post
    Then status 201
    * def investigationId = response.id

    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/event-check')
    Given driver webBaseUrl + '/timeline?from=2026-08-20T11%3A00%3A00Z&to=2026-08-20T11%3A06%3A00Z&event_version=1'
    When input("[data-testid='timeline-correlation-id']", 'ORDER-2001')
    And click("[data-testid='timeline-search-submit']")
    Then waitFor("[data-testid='timeline-event-0-type']")
    When click("[data-testid='timeline-event-0-type']")
    Then waitFor("[data-testid='timeline-event-0-attach']")
    * def eventId = script("document.querySelector('[data-testid=timeline-event-0-detail] dd').textContent")
    When click("[data-testid='timeline-event-0-attach']")
    Then waitFor("[data-testid='event-attachment-modal']")
    And waitFor("[data-testid='event-attachment-case-0']")
    And match text("[data-testid='event-attachment-case-0']") contains caseTitle
    When click("[data-testid='event-attachment-case-0']")
    Then waitFor("[data-testid='event-attachment-success']")
    And match text("[data-testid='event-attachment-success']") contains 'Event evidence 已加入'

    Given url apiBaseUrl + '/api/v1/investigations/' + investigationId + '/evidence-bundle'
    And param from = from
    And param to = to
    When method get
    Then status 200
    * def attachedItems = karate.filter(response.items, function(x){ return x.reference == eventId })
    And match attachedItems == '#[1]'
    And match attachedItems[0].source == 'CLICKHOUSE'
    And match attachedItems[0].checksum == '#regex [0-9a-f]{64}'

    Given url apiBaseUrl + '/api/v1/investigations/' + investigationId + '/summary'
    And param from = from
    And param to = to
    When method get
    Then status 200
    And match response.audit_entries[*].action contains 'ATTACH_INVESTIGATION_EVENT'

  Scenario: Viewer 可以查看但不能修改調查案件
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/event-check')
    And match exists("[data-testid='create-investigation']") == false
    Given driver webBaseUrl + '/timeline?from=2026-08-20T11%3A00%3A00Z&to=2026-08-20T11%3A06%3A00Z&event_version=1'
    When input("[data-testid='timeline-correlation-id']", 'ORDER-2001')
    And click("[data-testid='timeline-search-submit']")
    Then waitFor("[data-testid='timeline-event-0-type']")
    When click("[data-testid='timeline-event-0-type']")
    Then waitFor("[data-testid='timeline-event-0-detail']")
    And match exists("[data-testid='timeline-event-0-attach']") == false

  @eh-p1-1-013
  Scenario: Event Check 與舊 Journey 入口都使用最近 72 小時的 bounded request
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/event-check')
    And waitFor("[data-testid='event-check-from']")
    And match script("new Date(document.querySelector('[data-testid=event-check-to]').value).getTime() - new Date(document.querySelector('[data-testid=event-check-from]').value).getTime()") == 72 * 60 * 60 * 1000

    Given driver webBaseUrl + '/journey'
    Then waitUntil("window.location.pathname == '/event-check' && new URLSearchParams(window.location.search).get('tab') == 'flow'")
    And waitFor("[data-testid='event-check-from']")
    And match script("new Date(document.querySelector('[data-testid=event-check-to]').value).getTime() - new Date(document.querySelector('[data-testid=event-check-from]').value).getTime()") == 72 * 60 * 60 * 1000

  @eh-p1-1-013
  Scenario: Event Check bounded request 在 reload、back 與 forward 後完整還原
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/event-check')

    Given driver webBaseUrl + '/event-check?identifier_type=CORRELATION_ID&identifier=ORDER-2001&from=2026-08-20T11%3A00%3A00Z&to=2026-08-20T11%3A06%3A00Z&tab=timeline'
    Then waitFor("[data-testid='event-check-results']")
    And match text("[data-testid='event-check-status']") == 'DEVIATED'
    And match text("[data-testid='event-check-event-0-type']") == 'OrderCreated'
    And match text("[data-testid='event-check-event-1-type']") == 'PaymentCompleted'
    And match exists("[data-testid='event-check-event-0-link-event']") == true
    And match exists("[data-testid='event-check-event-0-link-logs']") == true
    And match exists("[data-testid='event-check-event-0-link-trace']") == true
    And match attribute("[data-testid='event-check-event-0-link-event']", 'href') contains '/explore?'

    When script("window.location.reload()")
    Then waitUntil("new URLSearchParams(window.location.search).get('identifier') == 'ORDER-2001'")
    And waitFor("[data-testid='event-check-results']")
    And match script("document.querySelector('[data-testid=event-check-identifier]').value") == 'ORDER-2001'
    And match text("[data-testid='event-check-event-0-type']") == 'OrderCreated'

    When input("[data-testid='event-check-identifier']", 'ORDER-DOES-NOT-EXIST')
    And click("[data-testid='event-check-submit']")
    Then waitUntil("new URLSearchParams(window.location.search).get('identifier') == 'ORDER-DOES-NOT-EXIST'")
    And waitFor("[data-testid='event-check-results']")
    And match text("[data-testid='event-check-status']") == 'NO_DATA'

    When script("window.history.back()")
    Then waitUntil("new URLSearchParams(window.location.search).get('identifier') == 'ORDER-2001'")
    And waitFor("[data-testid='event-check-results']")
    And match script("document.querySelector('[data-testid=event-check-identifier]').value") == 'ORDER-2001'
    And match text("[data-testid='event-check-status']") == 'DEVIATED'

    When script("window.history.forward()")
    Then waitUntil("new URLSearchParams(window.location.search).get('identifier') == 'ORDER-DOES-NOT-EXIST'")
    And waitFor("[data-testid='event-check-results']")
    And match script("document.querySelector('[data-testid=event-check-identifier]').value") == 'ORDER-DOES-NOT-EXIST'
    And match text("[data-testid='event-check-status']") == 'NO_DATA'

  @feature-guide
  Scenario: Viewer 可從 Event Hunter Guide 了解調查與接入方式並前往對應頁面
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/event-check')

    When click("[data-testid='nav-feature-guide']")
    Then waitForUrl(webBaseUrl + '/guide')
    And waitFor("[data-testid='feature-guide']")
    And match text("[data-testid='nav-feature-guide']") == 'Event Hunter Guide'
    And match script("document.querySelector('[data-testid=nav-scenario-lab]').nextElementSibling.getAttribute('data-testid')") == 'nav-feature-guide'
    And match text("[data-testid='feature-guide-title']") == 'Getting Started & Integration'
    And match text("[data-testid='integration-guide']") contains 'Normalization Adapter'
    And match text("[data-testid='integration-guide']") contains 'eventId'
    And match text("[data-testid='integration-guide']") contains '不是 ClickHouse sink'
    And match text("[data-testid='integration-quick-start']") contains '先回答三個問題'
    And match text("[data-testid='integration-change-cases']") contains '既有 canonical topic ＋新 event type'
    And match text("[data-testid='integration-glossary']") contains '事件信箱／頻道'
    And match text("[data-testid='integration-no-data']") contains '照這個順序查'
    And match text("[data-testid='integration-data-plane']") contains 'Event Check 不直接從 Kafka'
    And match text("[data-testid='integration-admission-gates']") contains '五關都通過'
    And match text("[data-testid='integration-failure-modes']") contains '格式正確但語意錯誤'
    And match text("[data-testid='integration-commands']") contains 'verify-event-pipeline-readiness.sh'

    When click("[data-testid='integration-runbook-step-normalize'] summary")
    Then match script("document.querySelector('[data-testid=integration-runbook-step-normalize]').open") == true
    And match text("[data-testid='integration-runbook-step-normalize']") contains 'Adapter 不直接寫 ClickHouse'

    When select("[data-testid='feature-guide-select']", 'event-check')
    Then waitUntil("new URLSearchParams(window.location.search).get('feature') == 'event-check'")
    And match text("[data-testid='feature-guide-title']") == 'Event Check'
    And match text("[data-testid='feature-guide-question']") contains '這個 ID 關聯到哪些事件'
    And match text("[data-testid='feature-guide']") contains 'Snapshot 固定 Model checksum'

    When select("[data-testid='feature-guide-select']", 'investigations')
    Then waitUntil("new URLSearchParams(window.location.search).get('feature') == 'investigations'")
    And match text("[data-testid='feature-guide-title']") == 'Investigation Cases'
    And match text("[data-testid='feature-guide-question']") contains '這個問題由誰處理'

    When click("[data-testid='feature-guide-open']")
    Then waitForUrl(webBaseUrl + '/investigations')

  Scenario: Viewer 可在 Event Check 查詢捷徑儲存相對時間搜尋並刪除
    * def savedSearchName = 'UI event check ' + java.util.UUID.randomUUID().toString()
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/event-check')

    Given driver webBaseUrl + '/event-check?identifier_type=CORRELATION_ID&identifier=ORDER-2001&from=2026-08-20T11%3A00%3A00Z&to=2026-08-20T11%3A06%3A00Z&tab=timeline'
    Then waitFor("[data-testid='event-check-results']")
    When click("[data-testid='event-check-shortcuts-open']")
    Then waitFor("[data-testid='event-check-shortcuts']")
    And input("[data-testid='event-check-shortcut-name']", savedSearchName)
    And select("[data-testid='event-check-shortcut-time-mode']", 'RELATIVE')
    And select("[data-testid='event-check-shortcut-relative-window']", '86400')
    And click("[data-testid='event-check-shortcut-save']")
    Then waitUntil("[...document.querySelectorAll('[data-testid^=event-check-shortcut-row-]')].some(node => node.textContent.includes('" + savedSearchName + "'))")
    And match exists("[data-testid^='event-check-shortcut-open-'][href^='/event-check?']") == true

    When click("[data-testid='event-check-shortcut-delete-0']")
    Then waitUntil("![...document.querySelectorAll('[data-testid^=event-check-shortcut-row-]')].some(node => node.textContent.includes('" + savedSearchName + "'))")

  Scenario: 舊 Saved Searches 路徑導向 Event Check 查詢捷徑
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/event-check')

    Given driver webBaseUrl + '/saved-searches'
    Then waitUntil("window.location.pathname == '/event-check' && new URLSearchParams(window.location.search).get('panel') == 'query-shortcuts'")
    And waitFor("[data-testid='event-check-shortcuts']")

  Scenario: Investigator 可從 Overview 查看真實聚合數字與資料來源狀態
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/event-check')

    When click("[data-testid='nav-dashboard']")
    Then waitForUrl(webBaseUrl + '/dashboard')
    And waitFor("[data-testid='overview-dashboard']")
    And waitFor("[data-testid='overview-open-cases']")
    And match text("[data-testid='overview-open-cases']") contains 'Open cases'
    And match exists("[data-testid='source-postgresql']") == true
    And match exists("[data-testid='source-clickhouse']") == true
    And match exists("[data-testid='source-tempo']") == true
    And match exists("[data-testid='source-loki']") == true
    And match exists("[data-testid='source-grafana']") == true

    When click("[data-testid='overview-open-cases']")
    Then waitForUrl(webBaseUrl + '/investigations?status=OPEN')
    And waitFor("[data-testid='case-row-0']")

  Scenario: Smart Search 對 opaque ID 先要求選擇類型再進入 bounded Event Check
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/event-check')

    When click("[data-testid='nav-dashboard']")
    Then waitForUrl(webBaseUrl + '/dashboard')
    And waitFor("[data-testid='smart-search-input']")
    When input("[data-testid='smart-search-input']", 'ORDER-2001')
    And click("[data-testid='smart-search-identify']")
    Then waitFor("[data-testid='smart-search-candidates']")
    And match exists("[data-testid='smart-search-candidate-correlation_id']") == true
    And match exists("[data-testid='smart-search-candidate-aggregate_id']") == true
    And match exists("[data-testid='smart-search-candidate-event_id']") == true

    When click("[data-testid='smart-search-candidate-correlation_id']")
    Then waitUntil("window.location.pathname == '/event-check' && new URLSearchParams(window.location.search).get('identifier_type') == 'CORRELATION_ID' && new URLSearchParams(window.location.search).get('identifier') == 'ORDER-2001'")
    And waitFor("[data-testid='event-check-results']")

  @check-model-registry
  Scenario: Viewer 可從導覽查看 API 實際載入的 immutable Check Model Registry
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/event-check')

    When click("[data-testid='nav-check-models']")
    Then waitForUrl(webBaseUrl + '/check-models')
    And waitFor("[data-testid='check-models-page']")
    And match text("[data-testid='check-model-row-order-fulfillment-2']") contains 'Order Fulfillment'
    And match exists("[data-testid='check-model-detail']") == false
    When click("[data-testid='check-model-row-order-fulfillment-2']")
    Then waitFor("[data-testid='check-model-detail']")
    And match text("[data-testid='check-model-detail']") contains 'order-fulfillment@2'
    And match text("[data-testid='check-model-detail']") contains 'contracts/check-models/order-fulfillment.yaml'
    And match text("[data-testid='check-model-detail']") contains 'MISSING_SHIPMENT_AFTER_PAYMENT'

    When click("[data-testid='check-model-panel-scenarios']")
    Then waitUntil("new URLSearchParams(window.location.search).get('panel') == 'scenarios'")
    And match text("[data-testid='check-model-detail']") contains 'success-happy-path'
    And match text("[data-testid='check-model-detail']") contains 'cross-correlation-child-flow'

    When click("[data-testid='check-model-panel-source']")
    Then waitUntil("new URLSearchParams(window.location.search).get('panel') == 'source'")
    And waitFor("[data-testid='check-model-source-yaml']")
    And match text("[data-testid='check-model-source-yaml']") contains 'model_id: order-fulfillment'
    And match text("[data-testid='check-model-source-yaml']") contains 'MISSING_SHIPMENT_AFTER_PAYMENT'

    Given driver webBaseUrl + '/journey-profiles'
    Then waitUntil("window.location.pathname == '/check-models' && new URLSearchParams(window.location.search).get('kind') == 'FLOW'")
    And waitFor("[data-testid='check-models-page']")

  Scenario: Viewer 可用已送達 fixture 在 Event Check 查看完整 Flow 判讀
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/event-check')

    Given driver webBaseUrl + '/event-check?identifier_type=CORRELATION_ID&identifier=ORDER-4002&from=2026-08-20T16%3A00%3A00Z&to=2026-08-20T16%3A21%3A00Z&tab=flow'
    Then waitFor("[data-testid='event-check-results']")
    And match text("[data-testid='event-check-status']") == 'CONFORMANT'
    And match text("[data-testid='event-check-flow-order-fulfillment-status']") == 'CONFORMANT'
    And match text("[data-testid='event-check-flow-order-fulfillment']") contains 'HAPPY_PATH'
    And match text("[data-testid='event-check-expectation-PAYMENT_REQUIRES_ORDER']") contains 'SATISFIED'
    And match text("[data-testid='event-check-expectation-SHIPMENT_REQUIRES_PAYMENT']") contains 'SATISFIED'
    And match text("[data-testid='event-check-expectation-PAYMENT_REQUIRES_SHIPMENT']") contains 'SATISFIED'

    When click("[data-testid='event-check-tab-timeline']")
    Then waitFor("[data-testid='event-check-event-list']")
    And match script("document.querySelectorAll('[data-testid^=event-check-event-][data-testid$=-type]').length") == 6
    And match text("[data-testid='event-check-event-0-type']") == 'OrderCreated'
    And match text("[data-testid='event-check-event-5-type']") == 'ShipmentDelivered'

  Scenario: Investigator 可保存 Event Check 並分別建立案件或加入案件
    * def uniqueId = java.util.UUID.randomUUID().toString()
    * def existingTitle = '[E2E] Saved Result existing case ' + uniqueId
    Given url apiBaseUrl + '/api/v1/auth/demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200
    Given url apiBaseUrl + '/api/v1/investigations'
    And request { title: '#(existingTitle)', severity: 'MEDIUM', correlation_id: 'ORDER-2001', incident_from: '2026-08-20T11:00:00Z', incident_to: '2026-08-20T11:06:00Z' }
    When method post
    Then status 201
    * def existingCaseId = response.id

    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/event-check')
    Given driver webBaseUrl + '/event-check?identifier_type=CORRELATION_ID&identifier=ORDER-2001&from=2026-08-20T11%3A00%3A00Z&to=2026-08-20T11%3A06%3A00Z&tab=summary'
    Then waitFor("[data-testid='event-check-results']")
    When click("[data-testid='event-check-save']")
    Then waitUntil("new URLSearchParams(window.location.search).has('snapshot_id')")
    * def snapshotId = script("new URLSearchParams(window.location.search).get('snapshot_id')")

    When click("[data-testid='event-check-saved-results-open']")
    Then waitForUrl(webBaseUrl + '/event-check/saved-results')
    And waitFor("[data-testid='saved-check-results-page']")
    * def savedRow = "[data-testid='saved-result-" + snapshotId + "']"
    * def savedJoin = "[data-testid='saved-result-" + snapshotId + "-join-case']"
    * def savedCreate = "[data-testid='saved-result-" + snapshotId + "-create-case']"
    And waitFor(savedRow)
    And match text(savedRow) contains 'ORDER-2001'
    And match text(savedRow) contains 'DEVIATED'

    When click(savedJoin)
    Then waitFor("[role='dialog'][aria-labelledby='check-case-title']")
    * def existingJoin = "[data-testid='event-check-join-case-" + existingCaseId + "']"
    When click(existingJoin)
    Then waitUntil("document.querySelector('.attachment-success') && document.querySelector('.attachment-success').textContent.includes('Snapshot 已連結案件')")
    When click("button[aria-label='關閉案件選擇']")

    When click(savedCreate)
    Then waitFor("[data-testid='event-check-create-case-title']")
    When input("[data-testid='event-check-create-case-title']", '[E2E] Saved Result new case ' + uniqueId)
    And click("[data-testid='event-check-create-case-confirm']")
    Then waitFor("[data-testid='event-check-created-case-open']")
    * def createdCasePath = attribute("[data-testid='event-check-created-case-open']", 'href')
    * def createdCaseId = createdCasePath.substring(createdCasePath.lastIndexOf('/') + 1)

    Given url apiBaseUrl + '/api/v1/investigations/' + existingCaseId + '/check-snapshots'
    When method get
    Then status 200
    And match response[*].snapshot_id contains snapshotId
    Given url apiBaseUrl + '/api/v1/investigations/' + createdCaseId + '/check-snapshots'
    When method get
    Then status 200
    And match response[*].snapshot_id contains snapshotId

    Given url apiBaseUrl + '/api/v1/investigations/' + existingCaseId
    When method get
    Then status 200
    * def existingCloseEtag = responseHeaders['Etag'][0]
    Given url apiBaseUrl + '/api/v1/investigations/' + existingCaseId + '/close'
    And header If-Match = existingCloseEtag
    And request { root_cause: 'E2E cleanup', resolution_summary: 'Saved Result join verified' }
    When method post
    Then status 200
    Given url apiBaseUrl + '/api/v1/investigations/' + createdCaseId
    When method get
    Then status 200
    * def createdCloseEtag = responseHeaders['Etag'][0]
    Given url apiBaseUrl + '/api/v1/investigations/' + createdCaseId + '/close'
    And header If-Match = createdCloseEtag
    And request { root_cause: 'E2E cleanup', resolution_summary: 'Saved Result create verified' }
    When method post
    Then status 200

  @p1-1-04-02
  Scenario: Investigator 可在案件抽屜切換完整調查資料頁籤並判定 Pattern finding
    * def uniqueId = java.util.UUID.randomUUID().toString()
    * def caseTitle = '[E2E] UI drawer tabs ' + uniqueId
    * def incidentFrom = '2026-08-20T11:00:00Z'
    * def incidentTo = '2026-08-20T11:06:00Z'
    Given url apiBaseUrl + '/api/v1/auth/demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200
    Given url apiBaseUrl + '/api/v1/investigations'
    And request { title: '#(caseTitle)', severity: 'HIGH', correlation_id: 'ORDER-2001', incident_from: '#(incidentFrom)', incident_to: '#(incidentTo)' }
    When method post
    Then status 201
    * def investigationId = response.id
    Given url apiBaseUrl + '/api/v1/investigations/' + investigationId + '/analyze'
    And request { execution_mode: 'SYNC' }
    When method post
    Then status 200

    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/event-check')

    When click("[data-testid='nav-investigations']")
    Then waitForUrl(webBaseUrl + '/investigations')
    And waitFor("[data-testid='case-row-0']")
    And match text("[data-testid='case-row-0']") contains caseTitle
    When click("[data-testid='case-row-0']")
    Then waitFor("[data-testid='case-summary']")

    When click("[data-testid='case-tab-timeline']")
    Then waitFor("[data-testid='case-timeline']")
    And match text("[data-testid='timeline-event-0-type']") == 'OrderCreated'
    When click("[data-testid='case-tab-patterns']")
    Then waitFor("[data-testid='case-patterns']")
    And match text("[data-testid='pattern-feedback-payment-completed-without-shipment']") contains 'UNREVIEWED'
    When click("button[aria-label='payment-completed-without-shipment 確認命中']")
    Then waitUntil("document.querySelector('[data-testid=pattern-feedback-payment-completed-without-shipment]').textContent.includes('CONFIRMED')")
    When click("[data-testid='case-tab-evidence']")
    Then waitFor("[data-testid='case-evidence'] [data-testid='evidence-manifest']")
    And match exists("[data-testid='case-evidence'] a[href^='/check-models?']") == true
    When click("[data-testid='case-tab-audit']")
    Then waitFor("[data-testid='case-audit']")
    When click("[data-testid='case-tab-evidence']")
    Then waitFor("[data-testid='case-evidence'] [data-testid='evidence-manifest']")
    When click("[data-testid='case-evidence'] a[href^='/check-models?']")
    Then waitFor("[data-testid='check-models-page']")
    And match text("[data-testid='check-model-detail']") contains 'order-fulfillment@2'
    And match exists("[data-testid='check-model-rule-PAYMENT_REQUIRES_SHIPMENT'].focused") == true

  @eh-p1-1-014
  Scenario: 案件抽屜揭露不可變 Incident Window 並區分目前檢視窗口
    * def uniqueId = java.util.UUID.randomUUID().toString()
    * def caseTitle = '[E2E] UI incident window ' + uniqueId
    * def incidentFrom = '2026-08-20T11:00:00Z'
    * def incidentTo = '2026-08-20T11:06:00Z'
    Given url apiBaseUrl + '/api/v1/auth/demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200
    Given url apiBaseUrl + '/api/v1/investigations'
    And request { title: '#(caseTitle)', severity: 'HIGH', correlation_id: 'ORDER-2001', incident_from: '#(incidentFrom)', incident_to: '#(incidentTo)' }
    When method post
    Then status 201
    * def investigationId = response.id
    * def detailUrl = webBaseUrl + '/investigations/' + investigationId

    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/event-check')
    Given driver detailUrl
    Then waitFor("[data-testid='case-incident-window']")
    And match text("[data-testid='case-incident-window']") contains 'TIMELINE_SEARCH'
    And match attribute("[data-testid='case-open-baseline-timeline']", 'href') contains 'identifier=ORDER-2001'
    And match attribute("[data-testid='case-open-baseline-timeline']", 'href') contains 'from=2026-08-20T11%3A00%3A00Z'
    And waitFor("[data-testid='case-summary-generated-at']")
    And match text("[data-testid='case-timeline-truncation']") == '完整（未截斷）'

    Given driver detailUrl + '?from=2026-08-20T11%3A00%3A00Z&to=2026-08-20T11%3A03%3A00Z'
    Then waitFor("[data-testid='case-current-window']")
    And match text("[data-testid='case-current-window']") contains '不會修改案件基準'
    And match text("[data-testid='case-incident-window']") contains 'TIMELINE_SEARCH'

  Scenario: Investigator 可在案件抽屜更新協作欄位並追加不可覆寫筆記
    * def uniqueId = java.util.UUID.randomUUID().toString()
    * def caseTitle = '[E2E] UI 協作案件 ' + uniqueId
    * def correlationId = 'ORDER-COLLAB-' + uniqueId
    Given url apiBaseUrl + '/api/v1/auth/demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200
    Given url apiBaseUrl + '/api/v1/investigations'
    And request { title: '#(caseTitle)', severity: 'HIGH', correlation_id: '#(correlationId)' }
    When method post
    Then status 201

    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/event-check')
    When click("[data-testid='nav-investigations']")
    Then waitForUrl(webBaseUrl + '/investigations')
    And waitFor("[data-testid='case-row-0']")
    And match text("[data-testid='case-row-0']") contains caseTitle
    When click("[data-testid='case-row-0']")
    Then waitFor("[data-testid='case-collaboration']")
    And match text("[data-testid='case-sla-status']") == 'ON_TRACK'

    When input("[data-testid='case-owner-input']", 'shipping-oncall')
    And select("[data-testid='case-priority-select']", 'P0')
    And input("[data-testid='case-tags-input']", 'shipping, urgent')
    And input("[data-testid='case-related-input']", 'SHIPMENT-UI-2001')
    And click("[data-testid='case-collaboration-save']")
    Then waitUntil("document.querySelector('[data-testid=case-tags]') && document.querySelector('[data-testid=case-tags]').textContent.includes('#urgent')")

    When input("[data-testid='case-note-input']", '已確認 shipping consumer lag')
    And click("[data-testid='case-note-submit']")
    Then waitUntil("document.querySelector('[data-testid=case-note-list]').textContent.includes('已確認 shipping consumer lag')")

  @p1-1-04-01 @p1-1-04-02
  Scenario: Viewer 可查看由程式碼管理的唯讀 Check Models 與 deterministic scenarios
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/event-check')

    When click("[data-testid='nav-check-models']")
    Then waitForUrl(webBaseUrl + '/check-models')
    And waitFor("[data-testid='check-model-row-order-fulfillment-2']")
    And match text("[data-testid='check-model-row-order-fulfillment-2']") contains 'ACTIVE'
    When click("[data-testid='check-model-row-order-fulfillment-2']")
    Then waitFor("[data-testid='check-model-detail']")
    And match text("[data-testid='check-model-detail']") contains 'contracts/check-models/order-fulfillment.yaml'
    And match text("[data-testid='check-model-rule-PAYMENT_REQUIRES_SHIPMENT']") contains 'MISSING_SHIPMENT_AFTER_PAYMENT'

    When click("[data-testid='check-model-panel-scenarios']")
    Then match text("[data-testid='check-model-detail']") contains 'violation-missing-shipment'
    And match text("[data-testid='check-model-detail']") contains 'expected-payment-failure'

    When click("[data-testid='check-model-kind-global']")
    Then waitFor("[data-testid='check-model-row-event-integrity-1']")
    When click("[data-testid='check-model-row-event-integrity-1']")
    Then waitFor("[data-testid='check-model-detail']")
    And match text("[data-testid='check-model-detail']") contains 'DUPLICATE_EVENT_ID'
    And match text("[data-testid='check-model-detail']") contains 'MISSING_TRACE_CONTEXT'

  @p1-1-ux-09
  Scenario: Investigator 以單次 start request 取得識別碼並由手動 Run History 查看實際結果
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/event-check')

    When click("[data-testid='nav-scenario-lab']")
    Then waitForUrl(webBaseUrl + '/scenario-lab')
    And waitFor("[data-testid='run-scenario-s8']")
    When click("[data-testid='run-scenario-s8']")
    Then waitFor("[data-testid='scenario-run-modal']")
    And match text("[data-testid='scenario-run-status']") == 'STARTING'
    And match text("[data-testid='scenario-run-id']") == '#regex [0-9a-f-]{36}'
    And match text("[data-testid='scenario-correlation-id']") == '#regex LAB-S8-.+'
    And match exists("[data-testid='scenario-link-timeline'][href^='http://localhost:28334/event-check?']") == true
    And match exists("[data-testid='scenario-link-tempo']") == false

    # 前端不輪詢；使用者關閉 modal 後明確按一次手動更新，再重新開啟 persisted result。
    When click("[aria-label='關閉 Scenario 執行資訊']")
    And delay(2500)
    And click("[data-testid='scenario-history-refresh']")
    Then waitUntil("document.querySelector('[data-testid^=scenario-history-run-]') && document.querySelector('[data-testid^=scenario-history-run-]').textContent.includes('PASSED')")
    When click("[data-testid^='scenario-history-run-']")
    Then match text("[data-testid='scenario-run-status']") == 'PASSED'
    And match text("[data-testid='scenario-actual-events']") contains 'PaymentFailed'
    And match exists("[data-testid='scenario-link-grafana'][href*='panes=']") == true
    And match exists("[data-testid='scenario-link-tempo'][href*='panes=']") == true
    And match exists("[data-testid='scenario-link-loki'][href*='panes=']") == true

  @eh-p1-1-014
  Scenario: Grafana alert 建立的案件可從 Evidence 開啟受信任 Alerting detail
    * def alertPayload = read('../../contracts/fixtures/grafana-alert-firing.json')
    * def uniqueId = java.util.UUID.randomUUID().toString()
    * set alertPayload.alerts[0].fingerprint = 'eh-ui-' + uniqueId
    * set alertPayload.alerts[0].labels.correlation_id = 'ORDER-UI-' + uniqueId
    * set alertPayload.commonLabels.correlation_id = 'ORDER-UI-' + uniqueId
    * def payloadText = karate.toString(alertPayload)
    * def wirePayloadText = payloadText.replace(/\//g, '\\\\/')
    * def unixTimestamp = '' + (java.lang.System.currentTimeMillis() / 1000 | 0)
    * def hmacSha256 =
      """
      function(secret, value) {
        var Mac = Java.type('javax.crypto.Mac');
        var SecretKeySpec = Java.type('javax.crypto.spec.SecretKeySpec');
        var HexFormat = Java.type('java.util.HexFormat');
        var StandardCharsets = Java.type('java.nio.charset.StandardCharsets');
        var mac = Mac.getInstance('HmacSHA256');
        mac.init(new SecretKeySpec(new java.lang.String(secret).getBytes(StandardCharsets.UTF_8), 'HmacSHA256'));
        return java.lang.String.valueOf(HexFormat.of().formatHex(mac.doFinal(new java.lang.String(value).getBytes(StandardCharsets.UTF_8))));
      }
      """
    * def signature = hmacSha256(grafanaWebhookSecret, unixTimestamp + ':' + wirePayloadText)
    Given url apiBaseUrl + '/api/v1/integrations/grafana/alerts'
    And header Content-Type = 'application/json'
    And header X-Grafana-Alerting-Timestamp = unixTimestamp
    And header X-Grafana-Alerting-Signature = signature
    And request wirePayloadText
    When method post
    Then status 202
    And match response.items[0].disposition == 'CREATED_CASE'

    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/event-check')
    When click("[data-testid='nav-investigations']")
    Then waitForUrl(webBaseUrl + '/investigations')
    And waitFor("[data-testid='case-row-0']")
    When click("[data-testid='case-row-0']")
    Then waitFor("[data-testid='case-summary']")
    And match text("[data-testid='case-incident-window']") contains 'GRAFANA_ALERT'
    When click("[data-testid='case-tab-evidence']")
    Then waitFor("[data-testid='case-evidence'] [data-testid='evidence-manifest']")
    And match exists("[data-testid='case-evidence'] a[href='http://localhost:28332/alerting/grafana/event-quality-delay/view?orgId=1'][target='_blank']") == true

  @p1-1-02-03
  Scenario: 案件複合篩選與排序可由 URL 分享並由後端一致執行
    * def uniqueId = java.util.UUID.randomUUID().toString()
    * def correlationId = 'FILTER-' + uniqueId
    * def caseTitle = '[E2E] Shareable case filters ' + uniqueId
    Given url apiBaseUrl + '/api/v1/auth/demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200

    Given url apiBaseUrl + '/api/v1/investigations'
    And request { title: '#(caseTitle)', severity: 'HIGH', correlation_id: '#(correlationId)' }
    When method post
    Then status 201
    * def investigationId = response.id
    * def openEtag = responseHeaders['Etag'][0]

    Given url apiBaseUrl + '/api/v1/investigations/' + investigationId
    And header If-Match = openEtag
    And request { status: 'INVESTIGATING', assignee: 'shipping-oncall', priority: 'P0', tags: ['urgent'] }
    When method patch
    Then status 200
    * def updatedEtag = responseHeaders['Etag'][0]

    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/event-check')

    * def filteredUrl = webBaseUrl + '/investigations?status=INVESTIGATING&severity=HIGH&priority=P0&assignee=shipping-oncall&tag=urgent&correlation_id=' + correlationId + '&sort_by=updated_at&sort_order=asc'
    Given driver filteredUrl
    Then waitFor("[data-testid='case-row-0']")
    And match text("[data-testid='case-row-0']") contains caseTitle
    And match script("document.querySelector('[name=status]').value") == 'INVESTIGATING'
    And match script("document.querySelector('[name=severity]').value") == 'HIGH'
    And match script("document.querySelector('[name=priority]').value") == 'P0'
    And match script("document.querySelector('[name=assignee]').value") == 'shipping-oncall'
    And match script("document.querySelector('[name=tag]').value") == 'urgent'
    And match script("document.querySelector('[name=correlation_id]').value") == correlationId
    And match script("document.querySelector('[name=sort_by]').value") == 'updated_at'
    And match script("document.querySelector('[name=sort_order]').value") == 'asc'

    When select("[name='sort_order']", 'desc')
    And click("[data-testid='case-list-filters'] button[type='submit']")
    Then waitUntil("new URLSearchParams(window.location.search).get('sort_order') == 'desc'")
    And waitFor("[data-testid='case-row-0']")
    And match text("[data-testid='case-row-0']") contains caseTitle

    Given url apiBaseUrl + '/api/v1/investigations/' + investigationId + '/close'
    And header If-Match = updatedEtag
    And request { root_cause: 'E2E lifecycle cleanup', resolution_summary: 'shareable filters verified' }
    When method post
    Then status 200

  @eh-p1-1-010
  Scenario: 案件列表、drawer 與 browser back forward 必須同步真實 detail route
    * def uniqueId = java.util.UUID.randomUUID().toString()
    * def caseTitle = '[E2E] UI route history ' + uniqueId
    Given url apiBaseUrl + '/api/v1/auth/demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200
    Given url apiBaseUrl + '/api/v1/investigations'
    And request { title: '#(caseTitle)', severity: 'HIGH', correlation_id: 'ORDER-2001' }
    When method post
    Then status 201
    * def investigationId = response.id
    * def detailUrl = webBaseUrl + '/investigations/' + investigationId

    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/event-check')
    When click("[data-testid='nav-investigations']")
    Then waitForUrl(webBaseUrl + '/investigations')
    And waitFor("[data-testid='case-row-0']")
    And match text("[data-testid='case-row-0']") contains caseTitle

    When click("[data-testid='case-row-0']")
    Then waitForUrl(detailUrl)
    And waitFor("[data-testid='case-detail']")
    And match text("[data-testid='case-detail']") contains caseTitle

    When script("window.history.back()")
    Then waitForUrl(webBaseUrl + '/investigations')
    And waitUntil("document.querySelector('[data-testid=case-detail]') == null")

    When script("window.history.forward()")
    Then waitForUrl(detailUrl)
    And waitFor("[data-testid='case-detail']")
    And match text("[data-testid='case-detail']") contains caseTitle

  @eh-p1-1-010
  Scenario: 直接 detail URL、reload 與 drawer close 必須保持 refresh-safe 語意
    * def uniqueId = java.util.UUID.randomUUID().toString()
    * def caseTitle = '[E2E] UI direct route ' + uniqueId
    Given url apiBaseUrl + '/api/v1/auth/demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200
    Given url apiBaseUrl + '/api/v1/investigations'
    And request { title: '#(caseTitle)', severity: 'MEDIUM', correlation_id: 'ORDER-1001' }
    When method post
    Then status 201
    * def investigationId = response.id
    * def detailUrl = webBaseUrl + '/investigations/' + investigationId

    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/event-check')
    Given driver detailUrl
    Then waitForUrl(detailUrl)
    And waitFor("[data-testid='case-detail']")
    And match text("[data-testid='case-detail']") contains caseTitle

    When script("window.location.reload()")
    Then waitForUrl(detailUrl)
    And waitFor("[data-testid='case-detail']")
    And match text("[data-testid='case-detail']") contains caseTitle

    When click("[data-testid='case-detail-close']")
    Then waitForUrl(webBaseUrl + '/investigations')
    And waitUntil("document.querySelector('[data-testid=case-detail]') == null")

  @eh-p1-1-010
  Scenario: 不存在的 detail route 必須保留案件頁並呈現明確 not-found state
    * def missingId = java.util.UUID.randomUUID().toString()
    * def missingUrl = webBaseUrl + '/investigations/' + missingId
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/event-check')

    Given driver missingUrl
    Then waitForUrl(missingUrl)
    And waitFor("[data-testid='case-detail-error']")
    And match text("[data-testid='case-detail-error']") contains '找不到案件'
    And match exists("[data-testid='timeline-results']") == false

  @eh-p1-1-010 @eh-p1-1-012
  Scenario: 新案件不得帶固定 demo resolution 且結案必須先確認
    * def uniqueId = java.util.UUID.randomUUID().toString()
    * def caseTitle = '[E2E] UI close confirmation ' + uniqueId
    Given url apiBaseUrl + '/api/v1/auth/demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200
    Given url apiBaseUrl + '/api/v1/investigations'
    And request { title: '#(caseTitle)', severity: 'HIGH', correlation_id: 'ORDER-1001' }
    When method post
    Then status 201
    * def investigationId = response.id
    * def detailUrl = webBaseUrl + '/investigations/' + investigationId

    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/event-check')
    Given driver detailUrl
    Then waitFor("[data-testid='case-detail']")
    And match exists("[data-testid='case-root-cause-input']") == false

    When click("[data-testid='case-close-start']")
    Then waitFor("[data-testid='case-close-confirmation']")
    And match attribute("[data-testid='case-close-confirmation']", 'role') == 'dialog'
    And match attribute("[data-testid='case-close-confirmation']", 'aria-modal') == 'true'
    And match script("document.querySelector('[data-testid=case-root-cause-input]').value") == ''
    And match script("document.querySelector('[data-testid=case-resolution-input]').value") == ''
    And match script("document.activeElement.dataset.testid") == 'case-root-cause-input'
    When script("document.activeElement.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))")
    Then waitUntil("document.querySelector('[data-testid=case-close-confirmation]') == null")
    And match script("document.activeElement.dataset.testid") == 'case-close-start'

    Given url apiBaseUrl + '/api/v1/investigations/' + investigationId
    When method get
    Then status 200
    And match response.status == 'OPEN'

    Given driver detailUrl
    When click("[data-testid='case-close-start']")
    Then waitFor("[data-testid='case-close-confirmation']")
    When input("[data-testid='case-root-cause-input']", '已確認 consumer 未建立出貨事件')
    And input("[data-testid='case-resolution-input']", '已由物流服務補建 ShipmentCreated')
    When click("[data-testid='case-close-confirm']")
    Then waitUntil("document.querySelector('[data-testid=case-status]').textContent.includes('CLOSED')")

    Given url apiBaseUrl + '/api/v1/investigations/' + investigationId
    When method get
    Then status 200
    And match response.status == 'CLOSED'
    And match response.root_cause == '已確認 consumer 未建立出貨事件'
    And match response.resolution_summary == '已由物流服務補建 ShipmentCreated'

  @eh-p1-1-016
  Scenario: 案件工作區以後端 allowed_transitions 完成審核、解決與重新開啟
    * def uniqueId = java.util.UUID.randomUUID().toString()
    * def caseTitle = '[E2E] UI explicit state actions ' + uniqueId
    Given url apiBaseUrl + '/api/v1/auth/demo-session'
    And request { role: 'INVESTIGATOR' }
    When method post
    Then status 200
    Given url apiBaseUrl + '/api/v1/investigations'
    And request { title: '#(caseTitle)', severity: 'HIGH', correlation_id: 'UI-STATE-#(uniqueId)' }
    When method post
    Then status 201
    And match response.allowed_transitions == ['INVESTIGATING', 'CLOSED']
    * def investigationId = response.id
    * def detailUrl = webBaseUrl + '/investigations/' + investigationId

    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/event-check')
    Given driver detailUrl
    Then waitFor("[data-testid='case-detail']")
    And waitFor("[data-testid='case-transition-investigating']")
    And match text("[data-testid='case-transition-investigating']") == '開始調查'
    And match exists("[data-testid='case-transition-waiting_approval']") == false

    When click("[data-testid='case-transition-investigating']")
    Then waitUntil("document.querySelector('[data-testid=case-status]').textContent.includes('INVESTIGATING')")
    And waitFor("[data-testid='case-transition-waiting_approval']")
    And match text("[data-testid='case-transition-waiting_approval']") == '送交審核'
    And match text("[data-testid='case-transition-resolved']") == '標記已解決'

    When click("[data-testid='case-transition-waiting_approval']")
    Then waitUntil("document.querySelector('[data-testid=case-status]').textContent.includes('WAITING_APPROVAL')")
    And match text("[data-testid='case-transition-investigating']") == '返回調查'
    When click("[data-testid='case-transition-investigating']")
    Then waitUntil("document.querySelector('[data-testid=case-status]').textContent.includes('INVESTIGATING')")

    When click("[data-testid='case-transition-resolved']")
    Then waitFor("[data-testid='case-resolution-dialog']")
    And match exists("[data-testid='case-resolve-confirm'][disabled]") == true
    When input("[data-testid='case-root-cause-input']", '審核發現 consumer mapping defect')
    And input("[data-testid='case-resolution-input']", '修正 mapping 並完成 replay 驗證')
    And click("[data-testid='case-resolve-confirm']")
    Then waitUntil("document.querySelector('[data-testid=case-status]').textContent.includes('RESOLVED')")
    And match text("[data-testid='case-transition-investigating']") == '重新開啟'
    And match text("[data-testid='case-action-success']") contains '調查結論已保存'

    When click("[data-testid='case-transition-investigating']")
    Then waitUntil("document.querySelector('[data-testid=case-status]').textContent.includes('INVESTIGATING')")

    Given url apiBaseUrl + '/api/v1/investigations/' + investigationId
    When method get
    Then status 200
    And match response.status == 'INVESTIGATING'
    * def currentEtag = responseHeaders['Etag'][0]
    Given url apiBaseUrl + '/api/v1/investigations/' + investigationId + '/close'
    And header If-Match = currentEtag
    And request { root_cause: '審核發現 consumer mapping defect', resolution_summary: 'UI state workflow verified' }
    When method post
    Then status 200
    And match response.status == 'CLOSED'
    And match response.allowed_transitions == []

  @eh-p1-1-012 @responsive
  Scenario: 390px viewport 的主要頁面不得產生 document-level 水平 overflow
    Given driver webBaseUrl + '/login'
    * driver.dimensions = { x: 0, y: 0, width: 390, height: 844 }
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/event-check')
    And match script("document.documentElement.scrollWidth <= document.documentElement.clientWidth") == true

    When click("[data-testid='nav-dashboard']")
    Then waitFor("[data-testid='overview-dashboard']")
    And match script("document.documentElement.scrollWidth <= document.documentElement.clientWidth") == true

    When click("[data-testid='nav-feature-guide']")
    Then waitFor("[data-testid='feature-guide']")
    And match script("document.documentElement.scrollWidth <= document.documentElement.clientWidth") == true

    When click("[data-testid='nav-check-models']")
    Then waitFor("[data-testid='check-models-page']")
    And match script("document.documentElement.scrollWidth <= document.documentElement.clientWidth") == true

    When click("[data-testid='nav-investigations']")
    Then waitForUrl(webBaseUrl + '/investigations')
    And match script("document.documentElement.scrollWidth <= document.documentElement.clientWidth") == true

    When click("[data-testid='nav-check-models']")
    Then waitFor("[data-testid='check-model-row-order-fulfillment-2']")
    When click("[data-testid='check-model-row-order-fulfillment-2']")
    Then waitFor("[data-testid='check-model-detail']")
    And match script("document.documentElement.scrollWidth <= document.documentElement.clientWidth") == true
    And match script("document.querySelector('[data-testid=check-model-row-order-fulfillment-2]').getBoundingClientRect().width <= document.documentElement.clientWidth") == true

    When click("[data-testid='nav-scenario-lab']")
    Then waitFor("[data-testid='scenario-lab']")
    And match script("document.documentElement.scrollWidth <= document.documentElement.clientWidth") == true
