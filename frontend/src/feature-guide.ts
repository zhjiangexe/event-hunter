export type FeatureGuideKey =
  "getting-started" | "timeline" | "journey" | "investigations" | "patterns";

export interface IntegrationTierDefinition {
  label: string;
  summary: string;
  requirements: string[];
  outcomes: string[];
}

export interface IntegrationPathDefinition {
  label: string;
  flow: string;
  note: string;
}

export interface IntegrationDataStoreDefinition {
  label: string;
  role: string;
  note: string;
}

export interface IntegrationAdmissionGateDefinition {
  label: string;
  requirement: string;
  failure: string;
}

export interface IntegrationRunbookStepDefinition {
  id: string;
  title: string;
  goal: string;
  actions: string[];
  sourceFiles: string[];
  verification: string[];
  doneWhen: string;
}

export interface IntegrationFailureModeDefinition {
  label: string;
  signal: string;
  lookAt: string;
  interpretation: string;
}

export interface IntegrationCommandDefinition {
  risk: "SAFE" | "CONTROLLED";
  label: string;
  command: string;
  expected: string;
}

export interface IntegrationDecisionDefinition {
  question: string;
  yes: string;
  no: string;
}

export interface IntegrationWalkthroughDefinition {
  label: string;
  example: string;
  meaning: string;
}

export interface IntegrationChangeCaseDefinition {
  situation: string;
  adapter: string;
  changes: string;
  firstProof: string;
}

export interface IntegrationGlossaryDefinition {
  term: string;
  plainMeaning: string;
  reason: string;
}

export interface JourneyInterpretationDefinition {
  principles: string[];
  exampleEvents: string[];
  exampleResults: Array<{
    label: string;
    state: string;
    reason: string;
  }>;
}

export interface IntegrationGuideDefinition {
  decisions: IntegrationDecisionDefinition[];
  walkthrough: IntegrationWalkthroughDefinition[];
  changeCases: IntegrationChangeCaseDefinition[];
  glossary: IntegrationGlossaryDefinition[];
  noDataChecks: string[];
  tiers: IntegrationTierDefinition[];
  paths: IntegrationPathDefinition[];
  dataStores: IntegrationDataStoreDefinition[];
  admissionGates: IntegrationAdmissionGateDefinition[];
  runbookSteps: IntegrationRunbookStepDefinition[];
  failureModes: IntegrationFailureModeDefinition[];
  commands: IntegrationCommandDefinition[];
  requiredFields: string[];
  fieldNotes: string[];
  checklist: string[];
  boundaries: string[];
}

export interface FeatureGuideDefinition {
  id: FeatureGuideKey;
  label: string;
  layer: string;
  route: string;
  actionLabel?: string;
  purpose: string;
  question: string;
  inputs: string[];
  outputs: string[];
  steps: string[];
  capabilities: string[];
  gaps: string[];
  integration?: IntegrationGuideDefinition;
  journeyInterpretation?: JourneyInterpretationDefinition;
}

export const featureGuides: FeatureGuideDefinition[] = [
  {
    id: "getting-started",
    label: "Getting Started & Integration",
    layer: "平台使用與接入",
    route: "/timeline",
    actionLabel: "從 Business Timeline 開始 →",
    purpose:
      "先用一條可重現的調查路徑理解 Event Hunter，再依需求選擇事件、觀測資料與自動建案的接入深度。",
    question:
      "我要如何用 Event Hunter 完成一次調查？其他系統要提供哪些事件與觀測資訊才能接入？",
    inputs: [
      "已知的業務識別碼或異常發生時間",
      "符合 Canonical Event Envelope 的 Kafka 事件，或可轉換的來源事件",
      "選配的 processing attempt、OpenTelemetry、Journey、Pattern 與 Grafana Alert",
    ],
    outputs: [
      "可重現的 Timeline → Journey／Pattern → Case 調查路徑",
      "依 Minimum、Recommended、Full 分級的接入決策",
      "事件契約、資料管線、觀測連結與驗收清單",
    ],
    steps: [
      "從 Overview／Smart Search 或已知識別碼進入 Business Timeline，先確認事實與查詢窗口。",
      "用 Business Journey 對照預期里程碑，再以 Pattern 判斷是否命中已知異常。",
      "需要協作時建立 Investigation Case，保存 Event、Trace、Alert 與 Pattern Evidence。",
      "外部系統先選擇直接送 canonical event，或以獨立 Normalization Adapter 轉成 canonical event。",
      "依需要補上 OTel、processing attempt、Journey profile、Pattern 與 Grafana 自動建案。",
      "用 contract validation、fixture 與端到端測試確認 Timeline、Journey、Case、Tempo 與 Loki 的資料一致。",
    ],
    capabilities: [
      "已具備 Kafka → ClickHouse → Timeline 的 canonical event ingestion",
      "已具備 OTel → Tempo／Loki／Grafana deep links 與真實跨服務 trace 範例",
      "已具備 YAML Journey／Pattern 與簽署 Grafana webhook 自動建案範例",
    ],
    gaps: [
      "目前沒有輸入 topic 與 schema 後自動完成接入的 self-service onboarding UI",
      "新增 topic、ACL、mapping、Journey 或 Pattern 仍需修改契約／YAML、部署並驗收",
      "Event Hunter 不直接輪詢業務資料庫，也不提供 Event Catalog／Topic Registry 管理 UI",
    ],
    integration: {
      decisions: [
        {
          question: "來源事件已符合 Canonical Event Envelope 嗎？",
          yes: "不用 Adapter；直接發布到已核准的 canonical topic。",
          no: "需要 Normalization Adapter，把來源格式翻譯後再發布。",
        },
        {
          question: "只需要用 Timeline 搜尋事件嗎？",
          yes: "先做 Minimum：事件契約、Kafka topic 與 ClickHouse ingestion 即可。",
          no: "要查重試、log、trace 時再做 Recommended；要 Journey／Pattern／自動建案時做 Full。",
        },
        {
          question: "這是既有 event type／topic，還是全新的？",
          yes: "先送一筆測試事件，確認現有 schema、訂閱與 mapping 都可沿用。",
          no: "必須登錄 schema／allowlist；若 topic 也新建，還要補訂閱、ACL、owner 與 retention。",
        },
      ],
      walkthrough: [
        {
          label: "業務系統產生事件",
          example: "PaymentCompleted · correlationId=ORDER-1001",
          meaning: "付款服務說明 ORDER-1001 已完成付款。這是要保存的業務事實。",
        },
        {
          label: "必要時翻譯格式",
          example: "legacy payment JSON → Canonical Event Envelope",
          meaning:
            "只有來源格式不符合契約時才需要 Adapter；它像翻譯員，不是資料庫 sink。",
        },
        {
          label: "發布到核准 Kafka topic",
          example: "payment.events · key=ORDER-1001",
          meaning:
            "Kafka 負責可靠傳遞；Event Hunter 的 ingestion 使用自己的 consumer group 讀取。",
        },
        {
          label: "驗證並寫入 ClickHouse",
          example: "ClickHouse Sink → raw landing → Materialized Views",
          meaning:
            "Sink 先忠實保存 Kafka record；Materialized View 再把可搜尋資料與 admission failure 分流。",
        },
        {
          label: "在 Timeline 查詢",
          example: "Correlation ID = ORDER-1001",
          meaning:
            "Business Timeline 查 ClickHouse 的歷史事件，不會直接掃描 Kafka。",
        },
      ],
      changeCases: [
        {
          situation: "既有 canonical topic ＋既有 event type",
          adapter: "不需要",
          changes:
            "通常不用改設定；先確認 ID、時間與 payload 符合既有 schema。",
          firstProof: "送一筆新事件，以 Correlation ID 在 Timeline 查到它。",
        },
        {
          situation: "既有 canonical topic ＋新 event type",
          adapter: "通常不需要",
          changes: "新增 event schema，並更新 AsyncAPI／event type allowlist。",
          firstProof:
            "合法 fixture 可落庫；未知 version 的 fixture 進 failure route。",
        },
        {
          situation: "全新 canonical topic",
          adapter: "格式已 canonical 就不需要",
          changes:
            "新增 topic、owner、retention、ACL、bootstrap 與 ingestion subscription，再登錄 schema。",
          firstProof:
            "專用 consumer group 有 member、lag 回到 0，事件可在 Timeline 查到。",
        },
        {
          situation: "既有 Kafka 事件不是 canonical 格式",
          adapter: "需要",
          changes:
            "建立 consumer + transformer + producer；讀 source topic，發布到核准 canonical topic。",
          firstProof:
            "同一來源訊息重送仍產生相同 eventId，且 Adapter 不直接寫 ClickHouse。",
        },
      ],
      glossary: [
        {
          term: "Topic",
          plainMeaning: "事件信箱／頻道",
          reason:
            "決定 producer 把事件送去哪裡，以及 Event Hunter 要訂閱哪裡。",
        },
        {
          term: "Partition key",
          plainMeaning: "決定事件排隊位置的分組鍵",
          reason:
            "使用 correlationId，可讓同一旅程在同一 topic 內維持可理解的順序。",
        },
        {
          term: "Owner",
          plainMeaning: "發生問題時真正負責的團隊",
          reason: "避免 topic、schema 或 producer 壞掉時沒有人能處理。",
        },
        {
          term: "Retention",
          plainMeaning: "Kafka 要保留訊息多久",
          reason: "它是重送緩衝，不等於 Timeline 的歷史保存期限。",
        },
        {
          term: "ACL",
          plainMeaning: "誰可以讀、誰可以寫的權限",
          reason: "Event Hunter 只需讀取權；不應拿到不必要的 producer 權限。",
        },
        {
          term: "Consumer group",
          plainMeaning: "一組獨立讀者的身分",
          reason:
            "Event Hunter 用自己的 group，不會搶走其他 group 已有的訊息。",
        },
        {
          term: "Normalization Adapter",
          plainMeaning: "事件格式翻譯員",
          reason:
            "把 legacy event 轉成 canonical event，再送回 Kafka；不是 ClickHouse sink。",
        },
        {
          term: "Schema／allowlist／mapping",
          plainMeaning: "可接受的形狀／種類／落庫欄位",
          reason:
            "三者共同決定事件是否被接受，以及欄位如何安全寫進 ClickHouse。",
        },
      ],
      noDataChecks: [
        "先確認 Timeline 的時間範圍涵蓋事件 occurredAt，而且輸入的是正確 ID 類型。",
        "確認 producer 真的把訊息送到已訂閱 topic，而不是名稱相近的其他 topic。",
        "確認 event type／version 已登錄；若未通過契約，改查 ingestion failure，而不是正常 Timeline。",
        "確認 ingestion consumer group 有 member、lag 可下降，而且 ClickHouse insert 沒有失敗。",
        "最後直接用 eventId 或 correlationId 查 ClickHouse，區分『未落庫』與『UI 查詢條件不符』。",
      ],
      tiers: [
        {
          label: "Minimum",
          summary: "先讓事件可搜尋與保存案件。",
          requirements: [
            "Canonical Event Envelope",
            "已核准 Kafka topic、schema 與 ingestion mapping",
            "穩定的 Correlation／Aggregate ID 語意",
          ],
          outcomes: [
            "Business Timeline",
            "基本搜尋",
            "手動 Investigation Case",
          ],
        },
        {
          label: "Recommended",
          summary: "再把事件與實際處理行為連起來。",
          requirements: [
            "Minimum 全部項目",
            "processing-attempt events",
            "OpenTelemetry trace／log 與 W3C Trace Context",
          ],
          outcomes: [
            "處理重試／DLQ 判讀",
            "Tempo／Loki deep links",
            "跨服務 trace",
          ],
        },
        {
          label: "Full",
          summary: "補上領域語意與主動偵測。",
          requirements: [
            "Recommended 全部項目",
            "Journey profile 與 deterministic Pattern YAML",
            "Grafana alert rule 與簽署 webhook",
          ],
          outcomes: [
            "Business Journey",
            "Pattern Analysis",
            "符合資格時自動建案",
          ],
        },
      ],
      paths: [
        {
          label: "Transactional Outbox",
          flow: "Business DB Outbox → Debezium → Kafka domain topic → ClickHouse Sink → raw landing／Materialized Views → Event Hunter",
          note: "服務與 outbox 共用交易，Debezium 是唯一發布者；Event Hunter 不直接監聽 production DB。",
        },
        {
          label: "Existing Kafka Events",
          flow: "Source topic → Normalization Adapter → approved canonical topic → ClickHouse Sink → raw landing／Materialized Views",
          note: "Adapter 使用自己的 consumer group，不會搶走其他 group 的訊息；它是 consumer + producer 的事件處理器，不是 ClickHouse sink。",
        },
        {
          label: "Live Observability",
          flow: "Services → OTel Collector → Tempo／Loki／Prometheus → Grafana deep links",
          note: "保留 trace context 與 correlation attributes，Event Hunter 保存查詢參照，不複製完整 logs 或 traces。",
        },
        {
          label: "Automatic Case Intake",
          flow: "Grafana Alerting → HMAC webhook → Event Hunter → Investigation Case",
          note: "只有帶有效 Correlation ID 且符合資格的 firing alert 建案；resolved 只追加 Evidence，不自動結案。",
        },
      ],
      dataStores: [
        {
          label: "Kafka",
          role: "事件傳輸與 7 天緩衝",
          note: "Producer／Adapter 將事件送到 topic；Business Timeline 不直接掃描 Kafka。",
        },
        {
          label: "ClickHouse",
          role: "合法事件與處理紀錄的 90 天查詢庫",
          note: "官方 Sink 保存 raw record，Materialized View 完成 admission 與 mapping 後，Business Timeline 才能查到。",
        },
        {
          label: "PostgreSQL",
          role: "案件與協作 control plane",
          note: "保存 Investigation Case、Evidence reference、Notes、Findings 與 Audit，不保存完整技術 telemetry。",
        },
        {
          label: "Tempo／Loki／Prometheus",
          role: "Trace／Log／Metric 原始觀測資料",
          note: "Event Hunter 保存可信 deep-link context；Grafana 負責實際查詢與呈現。",
        },
      ],
      admissionGates: [
        {
          label: "Topic subscription",
          requirement:
            "事件必須發布到 ClickHouse Sink connector 已訂閱的 Kafka topic。",
          failure:
            "未訂閱的 topic 不會被 ingestion 看見，也不會自動出現在 failure table。",
        },
        {
          label: "Canonical envelope",
          requirement:
            "JSON 必須具備完整 envelope keys，欄位型別與格式符合 schema。",
          failure:
            "Invalid JSON／schema violation 進 restricted DLQ 與 event_ingestion_failures。",
        },
        {
          label: "Type and version allowlist",
          requirement:
            "eventType、eventVersion 必須已登錄在 AsyncAPI 與對應 event schema。",
          failure: "未知 type／version 會被拒絕，不進正常 forensics_events。",
        },
        {
          label: "Sink acknowledgement",
          requirement:
            "Mapping 與 ClickHouse insert 成功後才提交來源 Kafka offset。",
          failure:
            "ClickHouse 不可用時保留未提交 offset，恢復後重送；不能把 HTTP ready 當成已落庫。",
        },
        {
          label: "Timeline query scope",
          requirement:
            "查詢識別碼、occurredAt 時間窗、retention 與結果上限必須涵蓋該事件。",
          failure:
            "事件可能已在 ClickHouse，但因時間窗或條件不符而顯示查無資料。",
        },
      ],
      runbookSteps: [
        {
          id: "scope",
          title: "決定接入層級與責任人",
          goal: "先確認要解決的是 Timeline、技術診斷，還是 Journey／Pattern／自動建案。",
          actions: [
            "選擇 Minimum、Recommended 或 Full，不要求每個系統一次完成所有能力。",
            "指定 event producer owner、topic owner、schema reviewer 與 Event Hunter ingestion owner。",
            "寫清楚 Correlation、Aggregate、Event、Causation、Trace ID 由誰產生及其生命週期。",
          ],
          sourceFiles: [
            "requirements/project-scope.yaml",
            "contracts/platform/topic-topology.yaml",
            "contracts/platform/identity-time-policy.yaml",
          ],
          verification: [
            "每個 ID 有唯一語意，不使用相同欄位混放 order、customer、request 等不同概念。",
            "已決定是否需要 processing attempts、OTel、Journey、Pattern 與 Grafana alert intake。",
          ],
          doneWhen: "接入範圍、owner、識別碼語意與驗收責任都有人承接。",
        },
        {
          id: "transport",
          title: "登錄 Kafka transport",
          goal: "讓事件進入一條有 owner、順序、保留與權限邊界的可管理管線。",
          actions: [
            "在 topic topology 與 AsyncAPI 登錄 topic、accepted event types 與 owner。",
            "目前建議使用 correlationId 作 partition key；同一 correlation 在單一 topic partition 內依 offset 有序。",
            "設定 partitions、retention、classification、production READ ACL，並讓 Event Hunter 使用獨立 consumer group。",
            "將新 topic 加入 bootstrap 與 ClickHouse Sink connector topics／topic2TableMap；不能只建立 topic 而不更新訂閱設定。",
          ],
          sourceFiles: [
            "contracts/platform/topic-topology.yaml",
            "contracts/asyncapi.yaml",
            "scripts/bootstrap-topics.sh",
            "infra/kafka-connect-clickhouse/connectors/poc-raw-landing.json",
          ],
          verification: [
            "Topic 存在且 producer 能寫入。",
            "connect-event-hunter-poc-raw-landing 為 Stable、有 member 且 lag 可回到 0。",
            "其他 consumer groups 仍各自收到訊息；獨立 group 不會搶走它們的工作。",
          ],
          doneWhen:
            "已核准 topic 可被專用 ingestion group 穩定消費，且 owner／retention／ACL 有紀錄。",
        },
        {
          id: "normalize",
          title: "產生 Canonical Event",
          goal: "將來源語意轉成 Event Hunter 可驗證、可追溯、可冪等重播的 envelope。",
          actions: [
            "來源已是 canonical 時直接發布；來源格式不同才建立 Normalization Adapter。",
            "Adapter 以自己的 consumer group 讀 source topic，轉換後發布到 approved canonical topic。",
            "保留真實 occurredAt、correlationId 與 trace context；eventId 必須 deterministic，重送不可每次換新 ID。",
            "Adapter 不直接寫 ClickHouse，避免繞過 schema、DLQ、Kafka metadata 與 sink recovery。",
          ],
          sourceFiles: [
            "contracts/events/canonical-envelope.schema.json",
            "contracts/events/*.schema.json",
            "contracts/platform/outbox-routing.yaml",
          ],
          verification: [
            "同一來源訊息重送會產生同一 eventId。",
            "causationId／traceId 無資料時明確為 null，不填入假的關聯值。",
            "禁止欄位在 outbox／adapter 發布前就被拒絕。",
          ],
          doneWhen:
            "代表性成功、失敗與重送 fixture 都能產生相同契約下的 canonical event。",
        },
        {
          id: "governance",
          title: "更新 Schema、Mapping 與資料治理",
          goal: "只讓已知事件進入查詢庫，並確保敏感資料不因接入而外洩。",
          actions: [
            "為每個 eventType／version 建立 JSON Schema，並在 AsyncAPI 與 topic allowlist 登錄。",
            "確認 generic envelope mapping 能寫入 forensics_events；新增 envelope 欄位時才修改欄位 mapping／migration。",
            "設定 payload classification、masking、prohibited fields 與 evidence policy。",
            "定義未知 type／version、schema violation 的 restricted DLQ 與 checksum-only ClickHouse metadata。",
          ],
          sourceFiles: [
            "contracts/asyncapi.yaml",
            "contracts/platform/ingestion-mapping.yaml",
            "contracts/platform/data-classification.yaml",
            "contracts/platform/event-versioning-policy.yaml",
          ],
          verification: [
            "合法 fixture 通過 schema 並出現在 forensics_events。",
            "錯誤 fixture 只出現在 restricted DLQ／event_ingestion_failures，不進正常 Timeline。",
            "Viewer／Investigator 看不到 payload；允許的 Admin 查詢仍套用 masking 與 audit。",
          ],
          doneWhen:
            "合法、非法與敏感資料案例都有 deterministic contract test 與明確去向。",
        },
        {
          id: "observability",
          title: "接入 Processing 與 OpenTelemetry",
          goal: "區分事件格式問題、Consumer 技術失敗與真正的業務失敗。",
          actions: [
            "Consumer 發布 processing-attempt event，包含 event、consumer group、partition／offset、status 與 retry reason。",
            "服務經 OTel Collector 發送 trace、log、metric，並在 Kafka headers 注入／還原 W3C trace context。",
            "Log 使用相同 event.id、event.type、correlation.id、trace_id 與 Kafka position。",
            "保留 live 與 synthetic telemetry namespace，不用 fixture 冒充正式服務證據。",
          ],
          sourceFiles: [
            "contracts/telemetry/event-processing-attempt.schema.json",
            "infra/otel-collector/config.yaml",
            "compose.yaml",
            "scripts/test-live-observability.sh",
          ],
          verification: [
            "同一 live order 的 domain events、Tempo trace、Loki logs 與 ClickHouse trace_id 一致。",
            "Consumer retry／DLQ 能在 processing attempts 顯示，而原始合法 event 仍保留在 Timeline。",
          ],
          doneWhen:
            "可以由一個 Correlation ID 從 Timeline 追到 processing、Tempo 與 Loki。",
        },
        {
          id: "domain",
          title: "加入領域判讀與自動案件（選配）",
          goal: "在事件已可信後，再定義『流程應該怎麼走』與『哪些異常需要調查』。",
          actions: [
            "以 versioned Journey YAML 定義 milestones、state 與 anomaly rules。",
            "以 deterministic Pattern YAML、schema 與 fixtures 定義可重現的已知異常。",
            "只有帶有效 Correlation ID 且符合 eligibility 的 Grafana firing alert 才透過 HMAC webhook 建案。",
            "resolved alert 追加 Evidence，不自動把案件結案。",
          ],
          sourceFiles: [
            "contracts/journeys/*.yaml",
            "contracts/patterns/*.yaml",
            "contracts/integrations/grafana-alert-webhook.schema.json",
            "infra/grafana/provisioning/",
          ],
          verification: [
            "Journey／Pattern generator drift check 通過。",
            "Completed、failed、missing event、no match、match 與 alert dedup 都有 E2E。",
          ],
          doneWhen:
            "流程判讀、Pattern finding 與自動建案都可由固定輸入重現，且不依賴人工猜測。",
        },
        {
          id: "release",
          title: "執行接入驗收與交接",
          goal: "證明接入不是只在 producer 端成功，而是能查、能診斷、能重啟後保存。",
          actions: [
            "先跑 contract validation，再確認 Debezium、兩個 ClickHouse Sink connectors 與 ClickHouse readiness。",
            "以新的 Correlation ID 發布代表性事件，驗證 Timeline、failure route 與 processing attempts。",
            "Recommended／Full 再驗證 Tempo、Loki、Journey、Pattern 與 Case deep links。",
            "更新 owner、故障處理、retention、known gaps 與 rollback／停用方式。",
          ],
          sourceFiles: [
            "requirements/operations-runbook.md",
            "requirements/backend-e2e-test-plan.md",
            "scripts/verify-event-pipeline-readiness.sh",
            "scripts/test-phase-1-exit.sh",
          ],
          verification: [
            "重啟後同一 event／case／trace reference 仍可查詢。",
            "無效事件不汙染正常 Timeline，且 failure metadata 可定位來源 topic／partition／offset。",
            "交接者能只靠 repo source of truth 重建設定，不需進 container 手改。",
          ],
          doneWhen:
            "Contract、pipeline、browser E2E、restart persistence 與交接文件全部通過。",
        },
      ],
      failureModes: [
        {
          label: "Ingestion／契約錯誤",
          signal: "INVALID_JSON、SCHEMA_VIOLATION、UNKNOWN_EVENT_TYPE／VERSION",
          lookAt:
            "Ingestion Issues、admission failure table、technical DLQ projector 與 Kafka Connect logs",
          interpretation:
            "事件無法被安全理解；可能是 producer／Adapter bug，也可能是 schema rollout 未同步。",
        },
        {
          label: "Consumer 技術處理失敗",
          signal:
            "原始 event 存在，但 processing attempt 為 FAILED／RETRYING／DLQ",
          lookAt:
            "event_processing_attempts、Tempo error span、Loki service logs",
          interpretation:
            "事件格式合法，是下游處理 timeout、dependency 或程式執行失敗。",
        },
        {
          label: "合法的業務失敗",
          signal: "PaymentFailed、ShipmentDispatchFailed 等已登錄 domain event",
          lookAt: "Business Timeline、Business Journey 與案件 Evidence",
          interpretation: "這是可觀測的業務結果，不必然代表服務程式有 bug。",
        },
        {
          label: "格式正確但語意錯誤",
          signal: "Schema 全部通過，但事件內容或流程判斷與真實業務不符",
          lookAt:
            "Journey anomalies、Pattern、Domain invariants、Trace／Log 與人工 Investigation",
          interpretation:
            "Ingestion 無法單獨識別，需要領域規則與證據交叉驗證。",
        },
      ],
      commands: [
        {
          risk: "SAFE",
          label: "驗證契約與 Registry drift",
          command: "python3 scripts/validate-contracts.py",
          expected:
            "所有 YAML／JSON、references、fixtures、Journey／Pattern registry 都通過。",
        },
        {
          risk: "SAFE",
          label: "確認 Event Pipeline readiness",
          command: "bash scripts/verify-event-pipeline-readiness.sh",
          expected:
            "Debezium tasks RUNNING；兩個 ingestion groups Stable、有 member 且 lag=0。",
        },
        {
          risk: "CONTROLLED",
          label: "驗證合法／非法 ingestion 與 DLQ",
          command: "bash scripts/test-ingestion-pipeline.sh",
          expected:
            "合法資料可落庫，非法資料只留下 restricted DLQ 與 checksum metadata。",
        },
        {
          risk: "CONTROLLED",
          label: "驗證 live telemetry chain",
          command: "bash scripts/test-live-observability.sh --skip-restart",
          expected:
            "新訂單的 ClickHouse events、Tempo trace 與 Loki logs 使用同一 trace ID。",
        },
        {
          risk: "CONTROLLED",
          label: "執行非破壞 release gate",
          command:
            "bash scripts/test-phase-1-exit.sh --no-start --skip-disruptive",
          expected:
            "Contract、backend／frontend E2E、observability 與品質報告通過。",
        },
      ],
      requiredFields: [
        "eventId",
        "eventType",
        "eventVersion",
        "occurredAt",
        "producer",
        "correlationId",
        "causationId",
        "traceId",
        "aggregateType",
        "aggregateId",
        "sequence",
        "payload",
      ],
      fieldNotes: [
        "eventId 必須全域穩定，用於 transport deduplication 與 Evidence reference。",
        "correlationId 串起同一段業務旅程；一個 Correlation ID 可以包含多個 Trace ID。",
        "aggregateType + aggregateId 標識事件所屬聚合，sequence 協助判斷亂序與遺漏候選。",
        "causationId 與 traceId 欄位必須存在但可為 null；有 live telemetry 時應填入真實值。",
      ],
      checklist: [
        "定義 topic、partition key、owner、retention、讀取 ACL 與獨立 consumer group。",
        "以 AsyncAPI／JSON Schema 鎖定 envelope、event type、version 與 payload 契約。",
        "確認 Correlation、Aggregate、Event、Causation 與 Trace ID 的產生者及生命週期。",
        "若來源不是 canonical，建立可冪等的 Normalization Adapter 並發布到核准 topic。",
        "更新 ingestion mapping、schema allowlist 與資料分類／遮罩規則。",
        "接入 OTel Collector，驗證 Kafka header 能注入與還原 W3C trace context。",
        "需要流程判讀時新增 Journey YAML；需要異常判讀時新增 Pattern YAML 與 fixtures。",
        "以 contract、backend E2E 與 browser E2E 驗證 Timeline、Journey、Case、Tempo、Loki 與重啟保存。",
      ],
      boundaries: [
        "不能只發布到任意 Kafka topic；現行 ingestion topic、schema 與 mapping 都是 allowlisted 設定。",
        "Event Hunter API 不是事件 ingestion endpoint；正式事件入口是已配置的 Kafka pipeline。",
        "Normalization Adapter 是外部接入選項，現階段不是 Event Hunter 內建的自助式服務。",
        "Grafana／Tempo／Loki 保存技術觀測資料；Event Hunter 保存事件、案件與可信 deep-link reference。",
      ],
    },
  },
  {
    id: "timeline",
    label: "Business Timeline",
    layer: "事實層",
    route: "/timeline",
    purpose:
      "依事件時間還原真正發生過的業務與技術事件，是開始調查時最直接的入口。",
    question:
      "這筆訂單實際發生了什麼？哪個事件、服務或 Kafka 處理步驟出現異常？",
    inputs: [
      "Correlation、Trace、Event 或 Aggregate ID",
      "最長 7 天的事件時間範圍",
      "Event type、Producer、Kafka metadata 等進階條件",
    ],
    outputs: [
      "依 occurred_at 排序的 canonical event 序列",
      "Event metadata、processing attempts 與遮罩後 payload",
      "Grafana、Tempo、Loki deep links 與案件 Evidence 操作",
    ],
    steps: [
      "選擇識別碼類型並輸入值。",
      "確認時間範圍涵蓋事件實際發生時間。",
      "搜尋並展開事件，檢查因果鏈、處理狀態與觀測連結。",
      "用「查詢捷徑」保存固定時間或每次重算的相對時間查詢。",
      "需要追蹤時建立案件，或把事件加入既有案件。",
    ],
    capabilities: [
      "支援四種基本識別碼與 allowlist 進階條件",
      "同一 Correlation ID 可回傳帶有不同 Trace ID 的事件",
      "事件日期、時間、技術 metadata 與 processing summary 可追溯",
      "內建 Preset 與個人 Saved Search 集中在頁面右側查詢捷徑",
      "空白頁使用最近 72 小時，已提交查詢可由 URL 分享並在重新整理或返回後還原",
    ],
    gaps: [
      "尚未將同一 Correlation ID 下的多條 Trace 分段呈現",
      "大量結果目前以查詢上限保護，尚無事件列表分頁",
    ],
  },
  {
    id: "journey",
    label: "Business Journey",
    layer: "流程層",
    route: "/journey",
    purpose:
      "把實際事件與版本化 Journey YAML 的預期里程碑比較，快速判斷流程完成、失敗或停滯的位置。",
    question: "這筆業務走到哪一步？下一個預期事件是什麼？目前缺少哪個里程碑？",
    inputs: ["Correlation ID", "事件時間範圍", "後端已啟用的 Journey profile"],
    outputs: [
      "Journey 狀態與完成比例",
      "各里程碑的實際事件、時間與耗時",
      "缺少事件、失敗或補償狀態",
    ],
    steps: [
      "輸入 Correlation ID 與事件時間範圍。",
      "查看整體 Journey 狀態與目前里程碑。",
      "展開里程碑，回到實際 Event 或技術觀測資料。",
      "需要反覆追蹤時，以「查詢捷徑」保存固定或相對時間範圍。",
    ],
    capabilities: [
      "預期流程由 YAML 明確定義，不用畫面硬編碼猜測",
      "能區分沒有事件、進行中、完成、失敗與補償",
      "里程碑與 Timeline canonical events 使用同一事實來源",
    ],
    journeyInterpretation: {
      principles: [
        "整體與里程碑狀態，是 Journey Profile 用同一 Correlation ID 的完整事件集合推導出來的。",
        "里程碑卡片的『實際事件』只列該里程碑 expected_event_types 內的事件，不會重複列前一階段事件。",
        "因此前置事件可以讓下一個里程碑進入 IN_PROGRESS，但下一個里程碑仍可能尚無自己的實際事件。",
        "NOT_APPLICABLE 表示選配或尚未觸發的支線目前不適用，不代表事件遺失或系統故障。",
      ],
      exampleEvents: ["OrderCreated", "PaymentCompleted", "ShipmentCreated"],
      exampleResults: [
        {
          label: "整體 Journey",
          state: "進行中",
          reason:
            "已有流程事件，但尚未出現 ShipmentDelivered，也沒有符合失敗或補償規則。",
        },
        {
          label: "Delivery",
          state: "進行中",
          reason:
            "ShipmentCreated 是 Delivery 的啟動條件；正在等待自己的預期事件 ShipmentDelivered。",
        },
        {
          label: "Return",
          state: "尚未適用",
          reason:
            "退貨是選配支線；尚未出現 ReturnRequested 或 ReturnReceived，所以沒有被觸發。",
        },
      ],
    },
    gaps: [
      "目前主要提供物流 Order profile",
      "已有唯讀 Profile Registry；尚無畫面編輯、版本選擇與發布 UI",
      "跨多個 Correlation ID 的複合旅程尚未產品化",
    ],
  },
  {
    id: "investigations",
    label: "Investigation Cases",
    layer: "協作層",
    route: "/investigations",
    purpose:
      "把一次查詢升級為可指派、可追蹤、可保存證據與稽核紀錄的正式調查案件。",
    question: "這個問題由誰處理、判斷依據是什麼、最後如何解決？",
    inputs: [
      "案件標題、Severity、Priority 與 Correlation ID",
      "Timeline event、Pattern finding、Grafana alert 等 Evidence reference",
      "Assignee、Tags、Notes、Root cause 與 Resolution",
    ],
    outputs: [
      "具序號、狀態與分頁的案件清單",
      "右側 drawer 的 Summary、Timeline、Patterns、Evidence、Audit",
      "完整狀態異動與協作稽核紀錄",
    ],
    steps: [
      "由 Timeline 手動建立，或由符合規則的 Grafana alert 自動建立。",
      "設定負責人、優先級與標籤，加入調查 Notes 與 Evidence。",
      "執行 Pattern Analysis，記錄 root cause 與 resolution 後結案。",
    ],
    capabilities: [
      "Viewer 唯讀，Investigator／Admin 可協作與結案",
      "支援 optimistic locking、Evidence manifest 與 Audit trail",
      "Grafana firing webhook 可依 Correlation ID 去重並自動建案",
    ],
    gaps: [
      "自動建案目前集中在已配置的 Grafana 規則，尚非通用事件規則引擎",
      "尚未提供案件通知、升級與值班系統整合",
      "跨案件合併、批次指派與相似案件建議尚未提供",
    ],
  },
  {
    id: "patterns",
    label: "Pattern Library",
    layer: "規則層",
    route: "/patterns",
    purpose:
      "展示由程式碼與 YAML 管理的確定性異常規則，以及規則來源、版本、測試覆蓋與歷史成效。",
    question: "這是否符合已知故障模式？規則為何命中，而且能否被重現與稽核？",
    inputs: [
      "版本化 Pattern YAML 與 JSON Schema",
      "Correlation ID 的 event-time window",
      "Fixture regression 與案件 finding feedback",
    ],
    outputs: [
      "Pattern ID、版本、Severity、條件與建議查詢",
      "來源路徑、SHA-256 checksum 與 fixture coverage",
      "Finding 命中、案件數與 Confirmed／Dismissed 回饋",
    ],
    steps: [
      "先在 Library 理解規則條件與資料窗口。",
      "於案件執行 Pattern Analysis，取得可解釋 finding。",
      "調查員確認或排除 finding，讓成效統計反映真實品質。",
    ],
    capabilities: [
      "Runtime 規則唯讀，避免 UI 即時修改造成不可追溯漂移",
      "Generator drift、Schema 與 fixture regression 可驗證",
      "NO_EVENTS 與 NO_MATCH 明確區分，不把缺資料誤判為正常",
    ],
    gaps: [
      "尚無視覺化 Pattern 編輯、審核與發布流程",
      "目前 Pattern 數量與領域覆蓋仍有限",
      "成效統計需要足夠的案件回饋才能具有代表性",
    ],
  },
];

export const featureGuideWorkflow = [
  "Overview／Smart Search 找到調查入口",
  "Timeline 還原事件事實",
  "Journey 對照預期流程",
  "Pattern 辨識已知異常",
  "Case 保存證據並協作處理",
  "Query Shortcut 重複使用有效查詢",
];

export function featureGuideByID(value: string | null) {
  return featureGuides.find((guide) => guide.id === value) ?? featureGuides[0];
}
