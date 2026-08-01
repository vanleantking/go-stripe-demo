package stripeaym

import "encoding/json"

type OrderEventType string

const (
	EventOrderPaid   OrderEventType = "ORDER_PAID"
	EventOrderFailed OrderEventType = "ORDER_FAILED"
)

type OrderEventPayload struct {
	EventType     OrderEventType `json:"event_type"`
	OrderID       string         `json:"order_id"`
	TransactionID string         `json:"transaction_id"`
	Amount        int64          `json:"amount"`
	Source        string         `json:"source"` // "payment_intent" or "checkout_session"
}

func (p OrderEventPayload) MarshalBinary() ([]byte, error) {
	return json.Marshal(p)
}
