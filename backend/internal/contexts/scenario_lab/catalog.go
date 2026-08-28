package scenariolab

import "fmt"

const (
	LiveServices = "LIVE_SERVICES"
	LabInjection = "LAB_INJECTION"
)

var catalog = []ScenarioDefinition{
	{ID: "S1", Name: "NORMAL_ORDER_FLOW", Title: "正常訂單出貨", Category: "RELIABILITY", Description: "呼叫真實 Order API，經 Outbox、Kafka、Payment 與 Shipping 完成跨服務流程。", ExecutionMode: LiveServices, Synthetic: false, ExpectedEventTypes: []string{"OrderCreated", "PaymentCompleted", "ShipmentCreated"}, ExpectedResults: []string{"Timeline 完整", "沒有 ingestion failure"}},
	{ID: "S2", Name: "PAYMENT_WITHOUT_SHIPMENT", Title: "付款後未出貨", Category: "RELIABILITY", Description: "注入已成熟的付款流程，但刻意不送 ShipmentCreated。", ExecutionMode: LabInjection, Synthetic: true, ExpectedEventTypes: []string{"OrderCreated", "PaymentCompleted"}, ExpectedResults: []string{"ShipmentCreated 缺少", "可供 missing-shipment Pattern 分析"}},
	{ID: "S3", Name: "DUPLICATE_EVENT", Title: "重複事件", Category: "RELIABILITY", Description: "相同 PaymentCompleted event ID 發送兩次。", ExecutionMode: LabInjection, Synthetic: true, ExpectedEventTypes: []string{"OrderCreated", "PaymentCompleted", "PaymentCompleted", "ShipmentCreated"}, ExpectedResults: []string{"偵測重複 event ID"}},
	{ID: "S4", Name: "OUT_OF_ORDER_EVENT", Title: "Aggregate 事件亂序", Category: "RELIABILITY", Description: "同一 Payment aggregate 先送 sequence 2，再送 sequence 1。", ExecutionMode: LabInjection, Synthetic: true, ExpectedEventTypes: []string{"OrderCreated", "PaymentCompleted", "PaymentCompleted"}, ExpectedResults: []string{"偵測 sequence 亂序"}},
	{ID: "S5", Name: "RETRY_THEN_DLQ", Title: "Retry 後進 DLQ", Category: "RELIABILITY", Description: "合法事件保留在 Timeline，另送 FAILED、RETRY_SCHEDULED、DLQ attempts。", ExecutionMode: LabInjection, Synthetic: true, ExpectedEventTypes: []string{"OrderCreated", "PaymentCompleted"}, ExpectedResults: []string{"Timeline 可見", "processing 最終狀態為 DLQ"}},
	{ID: "S6", Name: "SCHEMA_VIOLATION", Title: "Schema 違規", Category: "RELIABILITY", Description: "發送缺少必要 currency 的 PaymentCompleted。", ExecutionMode: LabInjection, Synthetic: true, ExpectedEventTypes: []string{}, ExpectedResults: []string{"正常 Timeline 無事件", "ingestion failure 類型為 SCHEMA_VIOLATION"}},
	{ID: "S7", Name: "HIGH_EVENT_DELAY", Title: "高事件延遲", Category: "RELIABILITY", Description: "事件 occurred_at 回推十分鐘後才實際送入 Kafka。", ExecutionMode: LabInjection, Synthetic: true, ExpectedEventTypes: []string{"OrderCreated", "PaymentCompleted", "ShipmentCreated"}, ExpectedResults: []string{"最大 event delay 至少十分鐘"}},
	{ID: "S8", Name: "PAYMENT_FAILED_AND_CANCELLED", Title: "付款失敗並取消", Category: "LOGISTICS", Description: "付款拒絕後由 OrderCancelled 承接 causation。", ExecutionMode: LabInjection, Synthetic: true, ExpectedEventTypes: []string{"OrderCreated", "PaymentFailed", "OrderCancelled"}, ExpectedResults: []string{"跨 Aggregate causation 完整"}},
	{ID: "S9", Name: "SHIPMENT_DELIVERED", Title: "完整配送簽收", Category: "LOGISTICS", Description: "建立出貨、派車、運送中直到簽收。", ExecutionMode: LabInjection, Synthetic: true, ExpectedEventTypes: []string{"OrderCreated", "PaymentCompleted", "ShipmentCreated", "ShipmentDispatched", "ShipmentInTransit", "ShipmentDelivered"}, ExpectedResults: []string{"完整物流生命週期"}},
	{ID: "S10", Name: "SHIPMENT_DISPATCH_RETRY", Title: "派車失敗後重試", Category: "LOGISTICS", Description: "ShipmentDispatchFailed 後再次派車並送達。", ExecutionMode: LabInjection, Synthetic: true, ExpectedEventTypes: []string{"OrderCreated", "PaymentCompleted", "ShipmentCreated", "ShipmentDispatchFailed", "ShipmentDispatched", "ShipmentDelivered"}, ExpectedResults: []string{"Shipment sequence 單調遞增"}},
	{ID: "S11", Name: "RETURN_AND_REFUND", Title: "退貨入庫後退款", Category: "LOGISTICS", Description: "簽收後申請退貨、退貨入庫並完成退款。", ExecutionMode: LabInjection, Synthetic: true, ExpectedEventTypes: []string{"OrderCreated", "PaymentCompleted", "ShipmentCreated", "ShipmentDispatched", "ShipmentDelivered", "ReturnRequested", "ReturnReceived", "PaymentRefunded"}, ExpectedResults: []string{"逆物流與付款補償順序完整"}},
	{ID: "S12", Name: "LIVE_PAYMENT_FAILED_AND_CANCELLED", Title: "Live 付款失敗並取消", Category: "RELIABILITY", Description: "呼叫真實 Order API，payment-service 寫入失敗付款，order-service 消費 PaymentFailed 後取消訂單。", ExecutionMode: LiveServices, Synthetic: false, ExpectedEventTypes: []string{"OrderCreated", "PaymentFailed", "OrderCancelled"}, ExpectedResults: []string{"三個事件均由 transactional outbox 發布", "訂單與付款狀態已保存"}},
	{ID: "S13", Name: "LIVE_SHIPMENT_DELIVERED", Title: "Live 完整配送簽收", Category: "LOGISTICS", Description: "呼叫真實 Order API，依序完成付款、建立出貨、派車、運送中與簽收。", ExecutionMode: LiveServices, Synthetic: false, ExpectedEventTypes: []string{"OrderCreated", "PaymentCompleted", "ShipmentCreated", "ShipmentDispatched", "ShipmentInTransit", "ShipmentDelivered"}, ExpectedResults: []string{"完整正向物流由三服務 outbox 發布"}},
	{ID: "S14", Name: "LIVE_RETURN_AND_REFUND", Title: "Live 退貨入庫與退款", Category: "LOGISTICS", Description: "呼叫真實 Order API，完成配送、退貨申請、退貨入庫並由 payment-service 退款。", ExecutionMode: LiveServices, Synthetic: false, ExpectedEventTypes: []string{"OrderCreated", "PaymentCompleted", "ShipmentCreated", "ShipmentDispatched", "ShipmentDelivered", "ReturnRequested", "ReturnReceived", "PaymentRefunded"}, ExpectedResults: []string{"正向與逆向物流均由真實服務 transaction/outbox 產生"}},
}

func Catalog() []ScenarioDefinition {
	result := make([]ScenarioDefinition, len(catalog))
	copy(result, catalog)
	return result
}

func Scenario(id string) (ScenarioDefinition, error) {
	for _, item := range catalog {
		if item.ID == id {
			return item, nil
		}
	}
	return ScenarioDefinition{}, fmt.Errorf("unknown scenario %s", id)
}
