package stripeaym

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/redis/go-redis/v9"
	"github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/webhook"
)

const (
	WEBHOOK_PAYMENT_INTENT_SUCCESS     = "payment_intent.succeeded"
	WEBHOOK_PAYMENT_INTENT_FAILED      = "payment_intent.payment_failed"
	WEBHOOK_CHECKOUT_SESSION_COMPLETED = "checkout.session.completed"

	RedisOrderChannel = "channel:order:events"
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
	const MaxBodyBytes = int64(65536)
	req.Body = http.MaxBytesReader(w, req.Body, MaxBodyBytes)
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
	ctx := req.Context()

	// 2. Handle the Event Type
	switch event.Type {
	case WEBHOOK_PAYMENT_INTENT_SUCCESS:
		var pi stripe.PaymentIntent
		err := json.Unmarshal(event.Data.Raw, &pi)
		if err != nil {
			log.Printf("Error parsing webhook JSON: %v\n", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// h.handlePaymentSuccess(pi)
		h.publishPaymentSuccess(ctx, pi.Metadata["order_id"], pi.ID, pi.Amount, "payment_intent")

	case WEBHOOK_PAYMENT_INTENT_FAILED:
		var pi stripe.PaymentIntent
		json.Unmarshal(event.Data.Raw, &pi)
		// h.handlePaymentFailure(pi)
		h.publishPaymentFailure(ctx, pi.Metadata["order_id"], pi.ID, "payment_intent")

	case WEBHOOK_CHECKOUT_SESSION_COMPLETED:
		var session stripe.CheckoutSession
		err := json.Unmarshal(event.Data.Raw, &session)
		if err != nil {
			log.Printf("Error parsing webhook JSON: %v\n", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		// Access the metadata directly on the session!
		orderID := session.Metadata["order_id"]
		if orderID == "" {
			log.Println("Critical: checkout.session.completed missing order_id metadata")
			return
		}

		// h.handleCOSuccess(session)
		h.publishPaymentSuccess(ctx, orderID, session.ID, session.AmountTotal, "checkout_session")

	default:
		// Unhandled event type
		log.Printf("Unhandled event type: %s\n", event.Type)
	}

	// 3. Acknowledge Receipt
	w.WriteHeader(http.StatusOK)
}

func (h *StripeWebhookHandler) publishPaymentSuccess(ctx context.Context, orderID, txID string, amount int64, source string) {
	if orderID == "" {
		log.Printf("Critical: %s event succeeded but missing order_id metadata", source)
		return
	}

	payload := OrderEventPayload{
		EventType:     EventOrderPaid,
		OrderID:       orderID,
		TransactionID: txID,
		Amount:        amount,
		Source:        source,
	}

	if err := h.redisClient.Publish(ctx, RedisOrderChannel, payload).Err(); err != nil {
		log.Printf("Failed to publish ORDER_PAID event to Redis for order %s: %v", orderID, err)
		return
	}

	log.Printf("Published ORDER_PAID event to Redis for Order: %s", orderID)
}

func (h *StripeWebhookHandler) publishPaymentFailure(ctx context.Context, orderID, txID, source string) {
	if orderID == "" {
		return
	}

	payload := OrderEventPayload{
		EventType:     EventOrderFailed,
		OrderID:       orderID,
		TransactionID: txID,
		Source:        source,
	}

	if err := h.redisClient.Publish(ctx, RedisOrderChannel, payload).Err(); err != nil {
		log.Printf("Failed to publish ORDER_FAILED event to Redis for order %s: %v", orderID, err)
		return
	}

	log.Printf("Published ORDER_FAILED event to Redis for Order: %s", orderID)
}

func (h *StripeWebhookHandler) handlePaymentSuccess(pi stripe.PaymentIntent) {
	orderID := pi.Metadata["order_id"]
	if orderID == "" {
		log.Printf("Critcal: PaymentIntent %s succeeded but missing order_id metadata", pi.ID)
		return
	}

	log.Printf("Payment succeeded for Order: %s. Amount: %d", orderID, pi.Amount)

	// TODO:
	// 1. Begin Database Transaction
	// 2. Select Order by ID FOR UPDATE (Lock the row)
	// 3. If Order is already PAID, return early (Idempotency)
	// 4. Update Order Status to PAID
	// 5. Commit Transaction
	// 6. Publish Event: Produce message to Kafka topic `order.events` (e.g., {"type": "order_paid", "order_id": orderID})
}

func (h *StripeWebhookHandler) handlePaymentFailure(pi stripe.PaymentIntent) {
	orderID := pi.Metadata["order_id"]
	// TODO: Update local database order status to FAILED, release held inventory
	log.Printf("Payment failed for Order: %s", orderID)
}

func (h *StripeWebhookHandler) handleCOSuccess(pi stripe.CheckoutSession) {

	orderID := pi.Metadata["order_id"]

	// Now you know the payment is secured by Stripe
	// Fulfill the order in your DB
	log.Printf("Fulfilling order %s via checkout session %s", orderID, pi.ID)

	// 1. Start Transaction
	// 2. SELECT ... FOR UPDATE on Order table by order_id
	// 3. Verify status != 'PAID' (Idempotency)
	// 4. UPDATE order status to PAID
	// 5. Commit Transaction
}
