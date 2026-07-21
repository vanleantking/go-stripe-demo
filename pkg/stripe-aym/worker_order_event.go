package stripeaym

import (
	"context"
	"encoding/json"
	"log"

	"github.com/redis/go-redis/v9"
)

type OrderEventConsumer struct {
	redisClient *redis.Client
	// db *sql.DB // Your database handle for performing transaction locks
}

func NewOrderEventConsumer(rdb *redis.Client) *OrderEventConsumer {
	return &OrderEventConsumer{redisClient: rdb}
}

// StartListening starts the consumer loop in a background goroutine
func (c *OrderEventConsumer) StartListening(ctx context.Context) {
	pubsub := c.redisClient.Subscribe(ctx, RedisOrderChannel)
	defer pubsub.Close()

	ch := pubsub.Channel()
	log.Printf("🚀 Order event consumer listening on channel: %s", RedisOrderChannel)

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			var payload OrderEventPayload
			if err := json.Unmarshal([]byte(msg.Payload), &payload); err != nil {
				log.Printf("Failed to unmarshal order event payload: %v", err)
				continue
			}

			c.processEvent(ctx, payload)
		}
	}
}

func (c *OrderEventConsumer) processEvent(ctx context.Context, payload OrderEventPayload) {
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
	}
}
