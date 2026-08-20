package stripeaym

import (
	"encoding/json"
	"time"
)

type OrderEventType string
type OrderEventSource string

const (
	EventOrderPaid          OrderEventType = "ORDER_PAID"
	EventOrderFailed        OrderEventType = "ORDER_FAILED"
	EventOrderRefundSuccess OrderEventType = "ORDER_REFUND_SUCCESS"
	EventOrderRefundUpdated OrderEventType = "ORDER_REFUND_UPDATED"
)

const (
	OrderEventSourcePaymentIntent   OrderEventSource = "PAYMENT_INTENT"
	OrderEventSourceCheckoutSession OrderEventSource = "CHECKOUT_SESSION"
	OrderEventSourceOrderRefund     OrderEventSource = "ORDER_REFUND"
)

const (
	EventRefundStatusSuccess = "success"
	EventRefundStatusFailed  = "failed"
	EventRefundStatusPending = "pending"
)

const (
	// Stripe Webhook Types
	WEBHOOK_PAYMENT_INTENT_SUCCESS     = "payment_intent.succeeded"
	WEBHOOK_PAYMENT_INTENT_FAILED      = "payment_intent.payment_failed"
	WEBHOOK_CHECKOUT_SESSION_COMPLETED = "checkout.session.completed"
	WEBHOOK_CHARGE_REFUND              = "charge.refunded"
	WEBHOOK_REFUND_CREATE              = "refund.created"
	WEBHOOK_REFUND_UPDATED             = "refund.updated"

	// webhook stripe config max body bytes
	MAX_BODY_BYTES_WEBHOOK = 65536

	RedisOrderChannel              = "channel:order:events"
	RedisOrderRefundSuccessChannel = "channel:order:refund:success"
	RedisOrderRefundUpdateChannel  = "channel:order:refund:update"

	// Unified Redis Event Topic
	RedisOrderEventsTopic = "topic:order:events"
)

type OrderEventPayload struct {
	EventID         string            `json:"event_id"`
	EventType       OrderEventType    `json:"event_type"`
	OrderID         string            `json:"order_id"`
	PaymentIntentID string            `json:"payment_intent_id,omitempty"`
	ChargeID        string            `json:"charge_id,omitempty"`
	RefundID        string            `json:"refund_id,omitempty"`
	Amount          int64             `json:"amount,omitempty"`
	AmountRefunded  int64             `json:"amount_refunded,omitempty"`
	Currency        string            `json:"currency,omitempty"`
	Status          string            `json:"status,omitempty"`
	FailureReason   string            `json:"failure_reason,omitempty"`
	IsPartialRefund bool              `json:"is_partial_refund,omitempty"`
	Source          OrderEventSource  `json:"source,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	Timestamp       time.Time         `json:"timestamp"`
}

func (p OrderEventPayload) MarshalBinary() ([]byte, error) {
	return json.Marshal(p)
}

func (e *OrderEventPayload) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, e)
}

type RefundEventPayload struct {
	PaymentIntentId *string
	OrderId         string
	AmountRefunded  int64
	IsPartialRefund bool
}

type RefundUpdateEventPayload struct {
	PaymentIntentId *string
	OrderId         string
	ChargeId        string
	FailureReason   string
	Amount          int64
	Status          string
	Currency        string
}
