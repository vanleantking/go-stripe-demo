package stripeaym

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"
)

type StripeWebhookHandler struct {
	endpointSecret string // Found in Stripe Dashboard -> Developers -> Webhooks
	redisClient    *redis.Client
}

func NewStripeWebhookHandler(secret string, rdb *redis.Client) *StripeWebhookHandler {
	return &StripeWebhookHandler{
		endpointSecret: secret,
		redisClient:    rdb,
	}
}

func (h *StripeWebhookHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req.Body = http.MaxBytesReader(w, req.Body, MAX_BODY_BYTES_WEBHOOK)
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusServiceUnavailable)
		return
	}

	// 1. Verify the Webhook Signature
	signatureHeader := req.Header.Get("Stripe-Signature")
	event, err := webhook.ConstructEvent(payload, signatureHeader, h.endpointSecret)
	if err != nil {
		log.Printf("⚠️  Webhook signature verification failed: %v\n", err)
		http.Error(w, "Invalid signature", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(req.Context(), 4*time.Second)
	defer cancel()

	if err := h.routeAndPublishEvent(ctx, &event); err != nil {
		log.Printf("❌ Failed to process/publish webhook event %s (%s): %v", event.ID, event.Type, err)
		// Return 500 so Stripe automatically backs off and retries delivery
		http.Error(w, "Internal queue error", http.StatusInternalServerError)
		return
	}

	// 3. Acknowledge Receipt
	w.WriteHeader(http.StatusOK)
}

/**
 * routeAndPublishEvent: establish event order payload base on the event type webhook
 * response to send into redis channel. use unified redis event to
 * process the event order payload base on the @EventType
 * @param ctx
 * @param event: webhook event response
**/
func (h *StripeWebhookHandler) routeAndPublishEvent(
	ctx context.Context,
	event *stripe.Event,
) error {
	var orderEvt *OrderEventPayload

	switch event.Type {
	case WEBHOOK_PAYMENT_INTENT_SUCCESS:
		var pi stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
			return fmt.Errorf("unmarshal payment_intent: %w", err)
		}
		orderID := pi.Metadata["order_id"]
		if orderID == "" {
			log.Printf("⚠️ Warning: payment_intent %s missing order_id metadata", pi.ID)
			return nil
		}
		orderEvt = &OrderEventPayload{
			EventID:         event.ID,
			EventType:       EventOrderPaid,
			OrderID:         orderID,
			PaymentIntentID: pi.ID,
			Amount:          pi.Amount,
			Currency:        string(pi.Currency),
			Source:          OrderEventSourcePaymentIntent,
			Timestamp:       time.Now().UTC(),
		}

	case WEBHOOK_PAYMENT_INTENT_FAILED:
		var pi stripe.PaymentIntent
		if err := json.Unmarshal(event.Data.Raw, &pi); err != nil {
			return fmt.Errorf("unmarshal payment_intent: %w", err)
		}
		orderID := pi.Metadata["order_id"]
		if orderID == "" {
			log.Printf("⚠️ Warning: payment_intent %s missing order_id metadata", pi.ID)
			return nil
		}
		var failureReason string
		if pi.LastPaymentError != nil {
			failureReason = string(pi.LastPaymentError.Code)
		}
		orderEvt = &OrderEventPayload{
			EventID:         event.ID,
			EventType:       EventOrderFailed,
			OrderID:         orderID,
			PaymentIntentID: pi.ID,
			FailureReason:   failureReason,
			Source:          OrderEventSourcePaymentIntent,
			Timestamp:       time.Now().UTC(),
		}

	case WEBHOOK_CHECKOUT_SESSION_COMPLETED:
		var session stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &session); err != nil {
			return fmt.Errorf("unmarshal checkout_session: %w", err)
		}
		orderID := session.Metadata["order_id"]
		if orderID == "" {
			log.Printf("⚠️ Warning: checkout_session %s missing order_id metadata", session.ID)
			return nil
		}
		var paymentIntentID string
		if session.PaymentIntent != nil {
			paymentIntentID = session.PaymentIntent.ID
		}
		orderEvt = &OrderEventPayload{
			EventID:         event.ID,
			EventType:       EventOrderPaid,
			OrderID:         orderID,
			PaymentIntentID: paymentIntentID,
			Amount:          session.AmountTotal,
			Currency:        string(session.Currency),
			Source:          OrderEventSourceCheckoutSession,
			Timestamp:       time.Now().UTC(),
		}

	case WEBHOOK_CHARGE_REFUND:
		var charge stripe.Charge
		if err := json.Unmarshal(event.Data.Raw, &charge); err != nil {
			return fmt.Errorf("unmarshal charge: %w", err)
		}
		var paymentIntentID string
		if charge.PaymentIntent != nil {
			paymentIntentID = charge.PaymentIntent.ID
		}
		orderID := charge.Metadata["order_id"]
		if orderID == "" {
			log.Printf("⚠️ Warning: charge refund %s missing order_id metadata", event.ID)
			return nil
		}
		orderEvt = &OrderEventPayload{
			EventID:         event.ID,
			EventType:       EventOrderRefundSuccess,
			OrderID:         orderID,
			PaymentIntentID: paymentIntentID,
			ChargeID:        charge.ID,
			AmountRefunded:  charge.AmountRefunded,
			Currency:        string(charge.Currency),
			IsPartialRefund: !charge.Refunded,
			Timestamp:       time.Now().UTC(),
		}

	case WEBHOOK_REFUND_CREATE, WEBHOOK_REFUND_UPDATED:
		var ref stripe.Refund
		if err := json.Unmarshal(event.Data.Raw, &ref); err != nil {
			return fmt.Errorf("unmarshal refund: %w", err)
		}
		var paymentIntentID, chargeID string
		if ref.PaymentIntent != nil {
			paymentIntentID = ref.PaymentIntent.ID
		}
		if ref.Charge != nil {
			chargeID = ref.Charge.ID
		}
		orderID := ref.Metadata["order_id"]
		if orderID == "" {
			log.Printf("⚠️ Warning: charge refund upate event %s missing order_id metadata", event.ID)
			return nil
		}
		failureReason := ""
		if ref.FailureReason != "" {
			failureReason = string(ref.FailureReason)
		}
		orderEvt = &OrderEventPayload{
			EventID:         event.ID,
			EventType:       EventOrderRefundUpdated,
			OrderID:         orderID,
			RefundID:        ref.ID,
			PaymentIntentID: paymentIntentID,
			ChargeID:        chargeID,
			Amount:          ref.Amount,
			Currency:        string(ref.Currency),
			Status:          string(ref.Status),
			FailureReason:   failureReason,
			Timestamp:       time.Now().UTC(),
		}

	default:
		log.Printf("Unhandled event type: %s\n", event.Type)
		return nil
	}

	if orderEvt != nil {
		return h.publishEvent(ctx, orderEvt)
	}

	return nil
}

/**
 * publishEvent: publish event order get from webhook to redis
 * @param ctx
 * @param evt: unified event order payload send into redis channel
 *
 **/
func (h *StripeWebhookHandler) publishEvent(ctx context.Context, evt *OrderEventPayload) error {
	data, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("failed to marshal order event: %w", err)
	}

	if err := h.redisClient.Publish(ctx, RedisOrderEventsTopic, data).Err(); err != nil {
		return fmt.Errorf("redis publish failed: %w", err)
	}

	log.Printf("📢 Published [%s] for Order: %s (Stripe Event: %s)",
		evt.EventType,
		evt.OrderID,
		evt.EventID,
	)
	return nil
}
