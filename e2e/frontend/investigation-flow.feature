@ui @requires-fixtures
Feature: Event Hunter 調查員主要操作流程

  Background:
    # UI E2E 只驗證跨頁核心流程；元件與 hooks 的細節由 Vitest／RTL 負責。
    * configure driver = { type: 'chrome', showDriverLog: true }

  Scenario: Investigator 搜尋時間線、建立案件並查看 Evidence
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/timeline')

    Given driver webBaseUrl + '/timeline?from=2026-08-20T11%3A00%3A00Z&to=2026-08-20T11%3A06%3A00Z'
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

  Scenario: Investigator 可把 Timeline Event 加入既有案件並保存為 Evidence reference
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
    Then waitForUrl(webBaseUrl + '/timeline')
    Given driver webBaseUrl + '/timeline?from=2026-08-20T11%3A00%3A00Z&to=2026-08-20T11%3A06%3A00Z'
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
    Then waitForUrl(webBaseUrl + '/timeline')
    And match exists("[data-testid='create-investigation']") == false
    Given driver webBaseUrl + '/timeline?from=2026-08-20T11%3A00%3A00Z&to=2026-08-20T11%3A06%3A00Z'
    When input("[data-testid='timeline-correlation-id']", 'ORDER-2001')
    And click("[data-testid='timeline-search-submit']")
    Then waitFor("[data-testid='timeline-event-0-type']")
    When click("[data-testid='timeline-event-0-type']")
    Then waitFor("[data-testid='timeline-event-0-detail']")
    And match exists("[data-testid='timeline-event-0-attach']") == false

  @eh-p1-1-013
  Scenario: Timeline 與 Journey 空白查詢預設最近 72 小時
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/timeline')
    And waitFor("[data-testid='timeline-from']")
    And match script("new Date(document.querySelector('[data-testid=timeline-to]').value).getTime() - new Date(document.querySelector('[data-testid=timeline-from]').value).getTime()") == 72 * 60 * 60 * 1000

    Given driver webBaseUrl + '/journey'
    Then waitFor("[data-testid='journey-from']")
    And match script("new Date(document.querySelector('[data-testid=journey-to]').value).getTime() - new Date(document.querySelector('[data-testid=journey-from]').value).getTime()") == 72 * 60 * 60 * 1000

  @eh-p1-1-013
  Scenario: Timeline 查詢 URL 在 reload、back 與 forward 後完整還原
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/timeline')

    Given driver webBaseUrl + '/timeline?from=2026-08-20T11%3A00%3A00Z&to=2026-08-20T11%3A06%3A00Z'
    When input("[data-testid='timeline-correlation-id']", 'ORDER-2001')
    And click("[data-testid='timeline-search-submit']")
    Then waitUntil("new URLSearchParams(window.location.search).get('correlation_id') == 'ORDER-2001'")
    And waitUntil("new URLSearchParams(window.location.search).get('include_processing_attempts') == 'true'")
    And waitFor("[data-testid='timeline-results']")
    And match text("[data-testid='timeline-event-count']") == '2'
    And match text("[data-testid='timeline-query-window']") contains '查詢窗口'

    When script("window.location.reload()")
    Then waitUntil("new URLSearchParams(window.location.search).get('correlation_id') == 'ORDER-2001'")
    And waitFor("[data-testid='timeline-results']")
    And match script("document.querySelector('[data-testid=timeline-correlation-id]').value") == 'ORDER-2001'
    And match text("[data-testid='timeline-event-count']") == '2'

    When input("[data-testid='timeline-correlation-id']", 'ORDER-4002')
    And click("[data-testid='timeline-search-submit']")
    Then waitUntil("new URLSearchParams(window.location.search).get('correlation_id') == 'ORDER-4002'")
    And waitFor("[data-testid='timeline-results']")

    When script("window.history.back()")
    Then waitUntil("new URLSearchParams(window.location.search).get('correlation_id') == 'ORDER-2001'")
    And waitFor("[data-testid='timeline-results']")
    And match script("document.querySelector('[data-testid=timeline-correlation-id]').value") == 'ORDER-2001'
    And match text("[data-testid='timeline-event-count']") == '2'

    When script("window.history.forward()")
    Then waitUntil("new URLSearchParams(window.location.search).get('correlation_id') == 'ORDER-4002'")
    And waitFor("[data-testid='timeline-results']")
    And match script("document.querySelector('[data-testid=timeline-correlation-id]').value") == 'ORDER-4002'

  @feature-guide
  Scenario: Viewer 可從 Event Hunter Guide 了解調查與接入方式並前往對應頁面
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/timeline')

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
    And match text("[data-testid='integration-data-plane']") contains 'Business Timeline 不直接從 Kafka'
    And match text("[data-testid='integration-admission-gates']") contains '五關都通過'
    And match text("[data-testid='integration-failure-modes']") contains '格式正確但語意錯誤'
    And match text("[data-testid='integration-commands']") contains 'verify-event-pipeline-readiness.sh'

    When click("[data-testid='integration-runbook-step-normalize'] summary")
    Then match script("document.querySelector('[data-testid=integration-runbook-step-normalize]').open") == true
    And match text("[data-testid='integration-runbook-step-normalize']") contains 'Adapter 不直接寫 ClickHouse'

    When select("[data-testid='feature-guide-select']", 'journey')
    Then waitUntil("new URLSearchParams(window.location.search).get('feature') == 'journey'")
    And match text("[data-testid='journey-interpretation-guide']") contains '為什麼「進行中」卻沒有該里程碑事件'
    And match text("[data-testid='journey-interpretation-guide']") contains 'ShipmentCreated 是 Delivery 的啟動條件'
    And match text("[data-testid='journey-interpretation-guide']") contains '退貨是選配支線'

    When select("[data-testid='feature-guide-select']", 'investigations')
    Then waitUntil("new URLSearchParams(window.location.search).get('feature') == 'investigations'")
    And match text("[data-testid='feature-guide-title']") == 'Investigation Cases'
    And match text("[data-testid='feature-guide-question']") contains '這個問題由誰處理'

    When click("[data-testid='feature-guide-open']")
    Then waitForUrl(webBaseUrl + '/investigations')

  Scenario: Viewer 可在 Timeline 查詢捷徑儲存相對時間搜尋並刪除
    * def savedSearchName = 'UI payment timeline ' + java.util.UUID.randomUUID().toString()
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/timeline')

    Given driver webBaseUrl + '/timeline?from=2026-08-20T11%3A00%3A00Z&to=2026-08-20T11%3A06%3A00Z'
    When input("[data-testid='timeline-correlation-id']", 'ORDER-2001')
    And click("[data-testid='timeline-search-submit']")
    Then waitFor("[data-testid='timeline-results']")
    When click("[data-testid='query-shortcuts-open']")
    Then waitFor("[data-testid='query-shortcuts-drawer']")
    And input("[data-testid='saved-search-name']", savedSearchName)
    And select("[data-testid='saved-search-time-mode']", 'RELATIVE')
    And select("[data-testid='saved-search-relative-window']", '86400')
    And click("[data-testid='save-search-submit']")
    Then waitFor("[data-testid='saved-search-success']")
    And match text("[data-testid='saved-search-success']") contains savedSearchName

    And waitFor("[data-testid='saved-search-row-0']")
    And match text("[data-testid='saved-search-row-0']") contains savedSearchName
    And match exists("[data-testid='saved-search-open-0'][href^='/timeline?']") == true

    When click("[data-testid='saved-search-delete-0']")
    Then waitUntil("![...document.querySelectorAll('[data-testid^=saved-search-row-]')].some(node => node.textContent.includes('" + savedSearchName + "'))")

  Scenario: 舊 Saved Searches 路徑導向 Timeline 查詢捷徑
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/timeline')

    Given driver webBaseUrl + '/saved-searches'
    Then waitUntil("window.location.pathname == '/timeline' && new URLSearchParams(window.location.search).get('panel') == 'query-shortcuts'")
    And waitFor("[data-testid='query-shortcuts-drawer']")

  Scenario: Investigator 可從 Overview 查看真實聚合數字與資料來源狀態
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/timeline')

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

  Scenario: Smart Search 對 opaque ID 先要求選擇類型再以 correlation 進入 bounded Journey
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/timeline')

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
    Then waitUntil("window.location.pathname == '/journey' && new URLSearchParams(window.location.search).get('correlation_id') == 'ORDER-2001'")
    And waitFor("[data-testid='journey-results']")

  @journey-profile-registry
  Scenario: Viewer 可從導覽查看 API 實際載入的 Journey Profile Registry
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/timeline')

    When click("[data-testid='nav-journey-profiles']")
    Then waitForUrl(webBaseUrl + '/journey-profiles')
    And waitFor("[data-testid='journey-profile-registry']")
    And match text("[data-testid='profile-count']") == '1'
    And match text("[data-testid='journey-profile-order-fulfillment']") contains 'Order Fulfillment'
    And match text("[data-testid='journey-profile-boundary']") contains '尚未提供'

    When click("[data-testid='journey-profile-order-fulfillment']")
    Then waitUntil("new URLSearchParams(window.location.search).get('profile') == 'order-fulfillment@v1'")
    And waitFor("[data-testid='journey-profile-detail']")
    And match text("[data-testid='journey-profile-detail-order-fulfillment']") contains 'MISSING_SHIPMENT_AFTER_PAYMENT'
    And match text("[data-testid='journey-profile-detail-order-fulfillment']") contains 'contracts/journeys/order-fulfillment.yaml'

    When click("[data-testid='journey-profile-detail-close']")
    Then waitUntil("!new URLSearchParams(window.location.search).has('profile')")
    And match exists("[data-testid='journey-profile-detail']") == false

    When click("[data-testid='profiles-open-journey']")
    Then waitForUrl(webBaseUrl + '/journey')

  Scenario: Viewer 可用已送達 fixture 查看完整 Business Journey
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/timeline')

    Given driver webBaseUrl + '/journey?correlation_id=ORDER-4002&from=2026-08-20T16%3A00%3A00Z&to=2026-08-20T16%3A21%3A00Z'
    Then waitFor("[data-testid='journey-results']")
    And match text("[data-testid='journey-status']") == 'COMPLETED'
    And match text("[data-testid='journey-event-count']") == '6'
    And match text("[data-testid='journey-milestone-order']") contains 'OrderCreated'
    And match text("[data-testid='journey-milestone-payment']") contains 'PaymentCompleted'
    And match text("[data-testid='journey-milestone-shipping']") contains 'ShipmentCreated'
    And match text("[data-testid='journey-milestone-delivery']") contains 'ShipmentDelivered'

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
    Then waitForUrl(webBaseUrl + '/timeline')

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
    And match exists("[data-testid='case-evidence'] a[href^='/patterns?pattern_id=']") == true
    When click("[data-testid='case-tab-audit']")
    Then waitFor("[data-testid='case-audit']")
    When click("[data-testid='case-tab-evidence']")
    Then waitFor("[data-testid='case-evidence'] [data-testid='evidence-manifest']")
    When click("[data-testid='case-evidence'] a[href^='/patterns?pattern_id=']")
    Then waitFor("[data-testid='pattern-library']")
    And match exists("[data-testid='pattern-row-0'][data-selected='true']") == true

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
    Then waitForUrl(webBaseUrl + '/timeline')
    Given driver detailUrl
    Then waitFor("[data-testid='case-incident-window']")
    And match text("[data-testid='case-incident-window']") contains 'TIMELINE_SEARCH'
    And match attribute("[data-testid='case-open-baseline-timeline']", 'href') contains 'correlation_id=ORDER-2001'
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
    Then waitForUrl(webBaseUrl + '/timeline')
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
  Scenario: Viewer 可查看由程式碼管理的唯讀 Pattern Library 與後端成效
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-viewer']")
    When click("[data-testid='role-viewer']")
    Then waitForUrl(webBaseUrl + '/timeline')

    When click("[data-testid='nav-patterns']")
    Then waitForUrl(webBaseUrl + '/patterns')
    And waitFor("[data-testid='pattern-library']")
    And match text("[data-testid='pattern-active-count']") contains '1 active'
    And match text("[data-testid='pattern-0-id']") == 'payment-completed-without-shipment'
    And match text("[data-testid='pattern-0-status']") == 'ACTIVE'
    And waitFor("[data-testid='pattern-effectiveness-window']")
    And match text("[data-testid='pattern-0-hit-count']") != '不可用'
    And match text("[data-testid='pattern-0-case-count']") != '不可用'
    And match text("[data-testid='pattern-0-last-hit']") != '不可用'
    And match text("[data-testid='pattern-0-fixture-coverage']") contains '1 match / 2 non-match'
    And match text("[data-testid='pattern-row-0']") contains 'contracts/patterns/payment-completed-without-shipment.yaml'

  Scenario: Investigator 可從 Scenario Lab 執行劇本並查看實際結果與 deep links
    Given driver webBaseUrl + '/login'
    And waitFor("[data-testid='role-investigator']")
    When click("[data-testid='role-investigator']")
    Then waitForUrl(webBaseUrl + '/timeline')

    When click("[data-testid='nav-scenario-lab']")
    Then waitForUrl(webBaseUrl + '/scenario-lab')
    And waitFor("[data-testid='run-scenario-s8']")
    When click("[data-testid='run-scenario-s8']")
    Then waitFor("[data-testid='scenario-result']")
    And waitUntil("document.querySelector('[data-testid=scenario-run-status]').textContent.includes('PASSED')")
    And match text("[data-testid='scenario-run-status']") == 'PASSED'
    And match text("[data-testid='scenario-actual-events']") contains 'PaymentFailed'
    And match exists("[data-testid='scenario-link-timeline'][href^='http://localhost:28334/timeline?correlation_id=']") == true
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
    Then waitForUrl(webBaseUrl + '/timeline')
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
    Then waitForUrl(webBaseUrl + '/timeline')

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
    Then waitForUrl(webBaseUrl + '/timeline')
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
    Then waitForUrl(webBaseUrl + '/timeline')
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
    Then waitForUrl(webBaseUrl + '/timeline')

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
    Then waitForUrl(webBaseUrl + '/timeline')
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
    Then waitForUrl(webBaseUrl + '/timeline')
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
    Then waitForUrl(webBaseUrl + '/timeline')
    And match script("document.documentElement.scrollWidth <= document.documentElement.clientWidth") == true

    When click("[data-testid='nav-dashboard']")
    Then waitFor("[data-testid='overview-dashboard']")
    And match script("document.documentElement.scrollWidth <= document.documentElement.clientWidth") == true

    When click("[data-testid='nav-feature-guide']")
    Then waitFor("[data-testid='feature-guide']")
    And match script("document.documentElement.scrollWidth <= document.documentElement.clientWidth") == true

    When click("[data-testid='nav-journey-profiles']")
    Then waitFor("[data-testid='journey-profile-registry']")
    And match script("document.documentElement.scrollWidth <= document.documentElement.clientWidth") == true

    When click("[data-testid='nav-investigations']")
    Then waitForUrl(webBaseUrl + '/investigations')
    And match script("document.documentElement.scrollWidth <= document.documentElement.clientWidth") == true

    When click("[data-testid='nav-patterns']")
    Then waitFor("[data-testid='pattern-library']")
    And match script("document.documentElement.scrollWidth <= document.documentElement.clientWidth") == true

    When click("[data-testid='nav-scenario-lab']")
    Then waitFor("[data-testid='scenario-lab']")
    And match script("document.documentElement.scrollWidth <= document.documentElement.clientWidth") == true
