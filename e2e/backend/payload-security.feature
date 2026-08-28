@infrastructure
Feature: EH-P1.1-009 Payload 授權與防禦性遞迴遮罩

  Background:
    * def clickhouseBasicAuth = 'Basic ' + java.util.Base64.getEncoder().encodeToString((clickhouseUser + ':' + clickhousePassword).getBytes())

  Scenario: ADMIN 取得的 payload 仍會遮罩巢狀 object 與 array 內的 credentials 和 PII
    # 直接寫入 read model 是刻意的防禦性測試：正式 ingestion 會先拒絕 prohibited fields，
    # 但 API 仍不可假設歷史資料或其他 adapter 永遠乾淨。
    * def nestedPayload = { orderId: 'ORDER-MASK-9001', customer: { customerId: 'CUSTOMER-MASK-9001', credentials: { accessToken: 'live-token', client_secret: 'live-secret' } }, recipients: [{ email: 'buyer@example.com', phoneNumber: '+886900000000' }], lines: [{ amount: 1280 }] }
    * def row = { event_id: 'evt-mask-security-9001', event_type: 'MaskingPolicyProbe', event_version: 1, occurred_at: '2026-08-23T12:00:00Z', producer: 'security-e2e', correlation_id: 'ORDER-MASK-9001', causation_id: null, trace_id: '90019001900190019001900190019001', aggregate_type: 'SecurityProbe', aggregate_id: 'ORDER-MASK-9001', sequence: 1, kafka_topic: 'event-hunter.security-fixture', kafka_partition: 99, kafka_offset: 9001, service_version: 'security-e2e', payload: '#(karate.toString(nestedPayload))', ingested_at: '2026-08-23T12:00:01Z' }
    * def insertStatement = 'INSERT INTO event_hunter.forensics_events FORMAT JSONEachRow\n' + karate.toString(row)

    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And header Content-Type = 'application/json'
    And request insertStatement
    When method post
    Then status 200

    # Probe 同時鏡像到 candidate，確保相同 API 安全測試不綁死目前 active source。
    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And header Content-Type = 'application/json'
    And request "INSERT INTO event_hunter.poc_forensics_events (event_id,event_type,event_version,occurred_at,producer,correlation_id,causation_id,trace_id,aggregate_type,aggregate_id,sequence,kafka_topic,kafka_partition,kafka_offset,payload,payload_sha256,admission_profile,admission_status,quality_flags,ingested_at) SELECT event_id,event_type,event_version,occurred_at,producer,correlation_id,causation_id,trace_id,aggregate_type,aggregate_id,sequence,kafka_topic,kafka_partition,kafka_offset,payload,lower(hex(SHA256(payload))),'synthetic-security-probe-v1','SEARCHABLE',CAST([],'Array(String)'),ingested_at FROM event_hunter.forensics_events WHERE event_id = 'evt-mask-security-9001'"
    When method post
    Then status 200

    Given url apiBaseUrl
    And path 'api', 'v1', 'auth', 'demo-session'
    And request { role: 'ADMIN' }
    When method post
    Then status 200
    And match response.permissions contains 'payload:read_sensitive'

    Given path 'api', 'v1', 'events', 'search'
    And param from = '2026-08-23T11:59:00Z'
    And param to = '2026-08-23T12:01:00Z'
    And param event_id = 'evt-mask-security-9001'
    And param include_payload = true
    When method get
    Then status 200
    And match response.count == 1
    And match response.events[0].payload.orderId == 'ORDER-MASK-9001'
    And match response.events[0].payload.customer.customerId == '#regex CUSTOMER-\\*\\*\\*-[0-9a-f]{8}'
    And match response.events[0].payload.customer.credentials.accessToken == '[REDACTED]'
    And match response.events[0].payload.customer.credentials.client_secret == '[REDACTED]'
    And match response.events[0].payload.recipients[0].email == '[REDACTED_PII]'
    And match response.events[0].payload.recipients[0].phoneNumber == '[REDACTED_PII]'
    And match response.events[0].payload.lines[0].amount == '[REDACTED_AMOUNT]'

    # 不讓防禦性 probe 汙染產品總覽或後續測試；固定 event_id 可同時清掉先前中斷留下的重複 probe。
    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And header Content-Type = 'application/json'
    And request "ALTER TABLE event_hunter.forensics_events DELETE WHERE event_id = 'evt-mask-security-9001' SETTINGS mutations_sync = 1"
    When method post
    Then status 200
    Given url clickhouseHttpUrl
    And header Authorization = clickhouseBasicAuth
    And header Content-Type = 'application/json'
    And request "ALTER TABLE event_hunter.poc_forensics_events DELETE WHERE event_id = 'evt-mask-security-9001' SETTINGS mutations_sync = 1"
    When method post
    Then status 200
