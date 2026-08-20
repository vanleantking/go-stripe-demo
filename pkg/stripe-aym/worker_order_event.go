package stripeaym

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stripe/stripe-go/v86"
)

type OrderEventConsumer struct {
	redisClient *redis.Client
	concurrency int
	// db *sql.DB // Your database handle for performing transaction locks
}

func NewOrderEventConsumer(
	rdb *redis.Client,
	concurrency int,
) *OrderEventConsumer {
	return &OrderEventConsumer{
		redisClient: rdb,
		concurrency: concurrency,
	}
}

func (c *OrderEventConsumer) Start(ctx context.Context) error {
	if c.concurrency > 0 {
		return c.StartwWorkers(ctx)
	}

	return c.StartListening(ctx)
}

func (c *OrderEventConsumer) StartwWorkers(ctx context.Context) error {
	pubsub := c.redisClient.Subscribe(ctx, RedisOrderEventsTopic)
	defer pubsub.Close()

	ch := pubsub.Channel()
	log.Printf("🚀 Order event worker pool initialized (%d workers) listening on: %s", c.concurrency, RedisOrderEventsTopic)

	jobQueue := make(chan *OrderEventPayload, 100)
	var wg sync.WaitGroup

	// Spin up worker pool
	for i := 1; i <= c.concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for evt := range jobQueue {
				workerCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := c.processEvent(workerCtx, evt); err != nil {
					log.Printf("❌ [Worker %d] Failed processing event %s for Order %s: %v", workerID, evt.EventID, evt.OrderID, err)
				}
				cancel()
			}
		}(i)
	}

	// Dispatcher loop
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(jobQueue)
				return
			case msg, ok := <-ch:
				if !ok {
					close(jobQueue)
					return
				}
				var evt OrderEventPayload
				if err := json.Unmarshal([]byte(msg.Payload), &evt); err != nil {
					log.Printf("⚠️ Worker received malformed JSON payload: %v", err)
					continue
				}
				jobQueue <- &evt
			}
		}
	}()

	<-ctx.Done()
	log.Println("🛑 Shutting down order event consumers, waiting for running jobs to finish...")
	wg.Wait()
	log.Println("✅ All order workers shut down cleanly.")
	return nil
}

// StartListening starts the consumer loop in a background goroutine
func (c *OrderEventConsumer) StartListening(ctx context.Context) error {
	pubsub := c.redisClient.Subscribe(ctx, RedisOrderEventsTopic)
	defer pubsub.Close()

	ch := pubsub.Channel()
	log.Printf("🚀 Order event consumer listening on channel: %s", RedisOrderChannel)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				log.Println("Redis pubsub channel closed")
				return nil
			}
			var payload OrderEventPayload
			if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
				log.Printf("Failed to unmarshal order event payload: %v", err)
				continue
			}

			// Execute without returning to keep the loop active
			if err := c.processEvent(ctx, &payload); err != nil {
				log.Printf("❌ Failed to process order event %s: %v", payload.EventID, err)
			}
		}
	}
}

func (c *OrderEventConsumer) processEvent(ctx context.Context, payload *OrderEventPayload) error {
	fmt.Println("enter here on order event processEvent, ", *payload)
	switch payload.EventType {
	case EventOrderPaid:
		log.Printf("[Worker] Processing payment success for Order ID: %s", payload.OrderID)
		// TODO: Execute Database Transaction
		// 1. BEGIN TX
		// 2. SELECT * FROM orders WHERE id = ? FOR UPDATE
		// 3. IF status == 'PAID' -> COMMIT (Idempotency check)
		// 4. UPDATE orders SET status = 'PAID' WHERE id = ?
		// 5. COMMIT TX

	case EventOrderFailed:
		log.Printf("[Worker] Processing payment failure for Order ID: %s", payload.OrderID)
		// TODO: Execute Database Transaction to mark order as FAILED and restore inventory
	case EventOrderRefundSuccess:
		log.Printf("[Worker] Refund process success for Order ID: %s", payload.OrderID)
	case EventOrderRefundUpdated:
		log.Printf("[Worker] Refund issued update for Order ID: %s", payload.OrderID)
	}
	return nil
}

// func (c *OrderEventConsumer) processEventWithTx(ctx context.Context, evt *OrderEventPayload) error {
// 	tx, err := c.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
// 	if err != nil {
// 		return fmt.Errorf("begin transaction failed: %w", err)
// 	}
// 	defer tx.Rollback()

// 	// 1. Idempotency Check: Prevent re-processing already completed Stripe event IDs
// 	if evt.EventID != "" {
// 		var existing string
// 		err := tx.QueryRowContext(ctx, "SELECT event_id FROM processed_webhook_events WHERE event_id = $1", evt.EventID).Scan(&existing)
// 		if err == nil {
// 			log.Printf("⏩ Duplicate event %s already processed. Skipping.", evt.EventID)
// 			return nil
// 		} else if !errors.Is(err, sql.ErrNoRows) {
// 			return fmt.Errorf("failed to query idempotency store: %w", err)
// 		}
// 	}
// 	// 2. Process based on EventType
// 	switch evt.EventType {
// 	case EventOrderPaid:
// 		if err := c.handleOrderPaid(ctx, tx, evt); err != nil {
// 			return err
// 		}
// 	case EventOrderFailed:
// 		if err := c.handleOrderFailed(ctx, tx, evt); err != nil {
// 			return err
// 		}
// 	case EventOrderRefundSuccess:
// 		if err := c.handleOrderRefunded(ctx, tx, evt); err != nil {
// 			return err
// 		}
// 	case EventOrderRefundUpdated:
// 		if err := c.handleRefundUpdated(ctx, tx, evt); err != nil {
// 			return err
// 		}
// 	default:
// 		log.Printf("Unhandled internal event type: %s", evt.EventType)
// 	}

// 	// 3. Mark event as processed
// 	if evt.EventID != "" {
// 		_, err = tx.ExecContext(ctx,
// 			"INSERT INTO processed_webhook_events (event_id, event_type, processed_at) VALUES ($1, $2, NOW())",
// 			evt.EventID, string(evt.EventType),
// 		)
// 		if err != nil {
// 			return fmt.Errorf("failed recording processed event %s: %w", evt.EventID, err)
// 		}
// 	}

// 	return tx.Commit()
// }

func (c *OrderEventConsumer) handleOrderPaid(ctx context.Context, tx *sql.Tx, evt *OrderEventPayload) error {
	var currentStatus string
	query := `SELECT status FROM orders WHERE id = $1 FOR UPDATE`
	err := tx.QueryRowContext(ctx, query, evt.OrderID).Scan(&currentStatus)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) && evt.PaymentIntentID != "" {
			// Fallback by payment_intent_id
			err = tx.QueryRowContext(ctx, `SELECT status FROM orders WHERE payment_intent_id = $1 FOR UPDATE`, evt.PaymentIntentID).Scan(&currentStatus)
		}
		if err != nil {
			return fmt.Errorf("failed locking order %s: %w", evt.OrderID, err)
		}
	}

	// Idempotency: Already paid or completed
	if currentStatus == "paid" || currentStatus == "completed" {
		log.Printf("Order %s already in '%s' status. Skipping.", evt.OrderID, currentStatus)
		return nil
	}

	updateQuery := `
		UPDATE orders 
		SET status = 'paid', payment_intent_id = COALESCE(NULLIF($1, ''), payment_intent_id), updated_at = NOW() 
		WHERE id = $2`
	if _, err := tx.ExecContext(ctx, updateQuery, evt.PaymentIntentID, evt.OrderID); err != nil {
		return fmt.Errorf("failed updating order %s to paid: %w", evt.OrderID, err)
	}

	log.Printf("💰 [TX SUCCESS] Order %s marked as PAID.", evt.OrderID)
	return nil
}

func (c *OrderEventConsumer) handleOrderFailed(ctx context.Context, tx *sql.Tx, evt *OrderEventPayload) error {
	query := `UPDATE orders SET status = 'failed', updated_at = NOW() WHERE id = $1 AND status != 'paid'`
	_, err := tx.ExecContext(ctx, query, evt.OrderID)
	if err != nil {
		return fmt.Errorf("failed marking order %s as failed: %w", evt.OrderID, err)
	}
	log.Printf("❌ [TX SUCCESS] Order %s marked as FAILED.", evt.OrderID)
	return nil
}

func (c *OrderEventConsumer) handleOrderRefunded(ctx context.Context, tx *sql.Tx, evt *OrderEventPayload) error {
	var orderID string
	var totalAmount int64
	var currentStatus string

	query := `SELECT id, total_amount, status FROM orders WHERE id = $1 OR payment_intent_id = $2 FOR UPDATE`
	err := tx.QueryRowContext(ctx, query, evt.OrderID, evt.PaymentIntentID).Scan(&orderID, &totalAmount, &currentStatus)
	if err != nil {
		return fmt.Errorf("order lookup failed for refund (order: %s, pi: %s): %w", evt.OrderID, evt.PaymentIntentID, err)
	}

	newStatus := "partially_refunded"
	if !evt.IsPartialRefund || evt.AmountRefunded >= totalAmount {
		newStatus = "refunded"
	}

	updateQuery := `UPDATE orders SET refunded_amount = $1, status = $2, updated_at = NOW() WHERE id = $3`
	if _, err := tx.ExecContext(ctx, updateQuery, evt.AmountRefunded, newStatus, orderID); err != nil {
		return fmt.Errorf("failed updating order %s refund: %w", orderID, err)
	}

	log.Printf("🔄 [TX SUCCESS] Order %s refund updated: status=%s, refunded_amount=%d", orderID, newStatus, evt.AmountRefunded)
	return nil
}

func (c *OrderEventConsumer) handleRefundUpdated(ctx context.Context, tx *sql.Tx, evt *OrderEventPayload) error {
	var orderID = evt.OrderID
	if orderID == "" && evt.PaymentIntentID != "" {
		_ = tx.QueryRowContext(ctx, `SELECT id FROM orders WHERE payment_intent_id = $1`, evt.PaymentIntentID).Scan(&orderID)
	}
	if orderID == "" {
		log.Printf("⚠️ Refund %s has no matching local order", evt.RefundID)
		return nil
	}

	upsertQuery := `
		INSERT INTO refunds (
			id, order_id, payment_intent_id, charge_id, 
			amount, currency, status, failure_reason, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		ON CONFLICT (id) DO UPDATE SET 
			status = EXCLUDED.status,
			failure_reason = EXCLUDED.failure_reason,
			updated_at = NOW()`

	_, err := tx.ExecContext(ctx, upsertQuery,
		evt.RefundID, orderID, evt.PaymentIntentID, evt.ChargeID,
		evt.Amount, evt.Currency, evt.Status, evt.FailureReason,
	)
	if err != nil {
		return fmt.Errorf("failed upserting refund %s: %w", evt.RefundID, err)
	}

	if evt.Status == string(stripe.RefundStatusFailed) {
		log.Printf("🚨 ALERT: Refund %s failed for Order %s. Reason: %s", evt.RefundID, orderID, evt.FailureReason)
	}
	return nil
}

// onChargeRefunded updates the aggregate order refunded amount and status using row locking.
// func (s *StripeWebhookHandler) onChargeRefunded(ctx context.Context, tx *sql.Tx, charge *stripe.Charge) error {
// 	var paymentIntentID string
// 	if charge.PaymentIntent != nil {
// 		paymentIntentID = charge.PaymentIntent.ID
// 	}

// 	// 1. Lock the order row to prevent concurrent race updates
// 	var orderID string
// 	var totalAmount, currentRefundedAmount int64
// 	var currentStatus string

// 	query := `
// 		SELECT id, total_amount, refunded_amount, status
// 		FROM orders
// 		WHERE payment_intent_id = $1
// 		FOR UPDATE`

// 	err := tx.QueryRowContext(ctx, query, paymentIntentID).Scan(
// 		&orderID, &totalAmount, &currentRefundedAmount, &currentStatus,
// 	)
// 	if err != nil {
// 		if errors.Is(err, sql.ErrNoRows) {
// 			// Fallback: check metadata if payment_intent_id isn't mapped yet
// 			if metaOrderID, ok := charge.Metadata["order_id"]; ok {
// 				orderID = metaOrderID
// 			} else {
// 				return fmt.Errorf("no order found for payment_intent: %s", paymentIntentID)
// 			}
// 		} else {
// 			return fmt.Errorf("failed to lock order: %w", err)
// 		}
// 	}

// 	// 2. Determine updated order status
// 	newStatus := "partially_refunded"
// 	if charge.Refunded || charge.AmountRefunded >= totalAmount {
// 		newStatus = "refunded"
// 	}

// 	// 3. Update order state
// 	updateQuery := `
// 		UPDATE orders
// 		SET refunded_amount = $1, status = $2, updated_at = NOW()
// 		WHERE id = $3`

// 	_, err = tx.ExecContext(ctx, updateQuery, charge.AmountRefunded, newStatus, orderID)
// 	if err != nil {
// 		return fmt.Errorf("failed to update order %s refund status: %w", orderID, err)
// 	}

// 	log.Printf("[DB UPDATED] Order %s status -> %s | Refunded: %d / %d",
// 		orderID, newStatus, charge.AmountRefunded, totalAmount)

// 	return nil
// }

// // onRefundUpdated performs idempotent upserts on individual refund audit records.
// func (s *StripeWebhookHandler) onRefundUpdated(ctx context.Context, tx *sql.Tx, ref *stripe.Refund) error {
// 	var paymentIntentID, chargeID string
// 	if ref.PaymentIntent != nil {
// 		paymentIntentID = ref.PaymentIntent.ID
// 	}
// 	if ref.Charge != nil {
// 		chargeID = ref.Charge.ID
// 	}

// 	// 1. Resolve order_id from metadata or foreign key lookups
// 	orderID := ref.Metadata["order_id"]
// 	if orderID == "" && paymentIntentID != "" {
// 		err := tx.QueryRowContext(ctx, "SELECT id FROM orders WHERE payment_intent_id = $1", paymentIntentID).Scan(&orderID)
// 		if err != nil && !errors.Is(err, sql.ErrNoRows) {
// 			return fmt.Errorf("failed to look up order for refund %s: %w", ref.ID, err)
// 		}
// 	}

// 	if orderID == "" {
// 		log.Printf("⚠️ Warning: Refund %s cannot be matched to a local order", ref.ID)
// 		return nil
// 	}

// 	failureReason := ""
// 	if ref.FailureReason != "" {
// 		failureReason = string(ref.FailureReason)
// 	}

// 	// 2. Upsert refund tracking record
// 	upsertQuery := `
// 		INSERT INTO refunds (
// 			id, order_id, payment_intent_id, charge_id,
// 			amount, currency, status, failure_reason, updated_at
// 		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
// 		ON CONFLICT (id) DO UPDATE SET
// 			status = EXCLUDED.status,
// 			failure_reason = EXCLUDED.failure_reason,
// 			updated_at = NOW()`

// 	_, err := tx.ExecContext(ctx, upsertQuery,
// 		ref.ID,
// 		orderID,
// 		paymentIntentID,
// 		chargeID,
// 		ref.Amount,
// 		string(ref.Currency),
// 		string(ref.Status),
// 		failureReason,
// 	)
// 	if err != nil {
// 		return fmt.Errorf("failed to upsert refund record %s: %w", ref.ID, err)
// 	}

// 	// 3. Status-specific business logic
// 	switch ref.Status {
// 	case stripe.RefundStatusSucceeded:
// 		log.Printf("[REFUND SUCCEEDED] Refund %s for Order %s settled.", ref.ID, orderID)
// 		// e.g., tx.ExecContext(ctx, "UPDATE inventory SET ...")

// 	case stripe.RefundStatusFailed:
// 		log.Printf("❌ [REFUND FAILED] Refund %s for Order %s failed. Reason: %s",
// 			ref.ID, orderID, ref.FailureReason)
// 		// e.g., tx.ExecContext(ctx, "INSERT INTO support_alerts ...")

// 	case stripe.RefundStatusPending:
// 		log.Printf("[REFUND PENDING] Refund %s awaiting rail settlement", ref.ID)
// 	}

// 	return nil
// }

// func (h *StripeWebhookHandler) handlePaymentSuccess(pi stripe.PaymentIntent) {
// 	orderID := pi.Metadata["order_id"]
// 	if orderID == "" {
// 		log.Printf("Critcal: PaymentIntent %s succeeded but missing order_id metadata", pi.ID)
// 		return
// 	}

// 	log.Printf("Payment succeeded for Order: %s. Amount: %d", orderID, pi.Amount)

// 	// TODO:
// 	// 1. Begin Database Transaction
// 	// 2. Select Order by ID FOR UPDATE (Lock the row)
// 	// 3. If Order is already PAID, return early (Idempotency)
// 	// 4. Update Order Status to PAID
// 	// 5. Commit Transaction
// 	// 6. Publish Event: Produce message to Kafka topic `order.events` (e.g., {"type": "order_paid", "order_id": orderID})
// }

// func (h *StripeWebhookHandler) handlePaymentFailure(pi stripe.PaymentIntent) {
// 	orderID := pi.Metadata["order_id"]
// 	// TODO: Update local database order status to FAILED, release held inventory
// 	log.Printf("Payment failed for Order: %s", orderID)
// }

// func (h *StripeWebhookHandler) handleCOSuccess(pi stripe.CheckoutSession) {

// 	orderID := pi.Metadata["order_id"]

// 	// Now you know the payment is secured by Stripe
// 	// Fulfill the order in your DB
// 	log.Printf("Fulfilling order %s via checkout session %s", orderID, pi.ID)

// 	// 1. Start Transaction
// 	// 2. SELECT ... FOR UPDATE on Order table by order_id
// 	// 3. Verify status != 'PAID' (Idempotency)
// 	// 4. UPDATE order status to PAID
// 	// 5. Commit Transaction
// }
