package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	stripeaym "stripe-aym/pkg/stripe-aym" // Replace with your actual module path

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func init() {
	// Load the .env file if present
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: No .env file found, reading from system env instead")
	}
}

func main() {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:51698" // Default fallback for local testing
	}

	// Format Redis URL properly for parsing
	fullRedisURL := redisURL
	if len(redisURL) < 8 || redisURL[:8] != "redis://" {
		fullRedisURL = "redis://" + redisURL
	}

	// Initialize Redis Client
	opt, err := redis.ParseURL(fullRedisURL)
	if err != nil {
		// Fallback to direct address configuration if parsing fails
		opt = &redis.Options{
			Addr: redisURL,
		}
	}
	rdb := redis.NewClient(opt)

	// Test Redis connectivity with a timeout
	ctxPing, cancelPing := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelPing()

	if err := rdb.Ping(ctxPing).Err(); err != nil {
		log.Fatalf("CRITICAL: Worker failed to connect to Redis: %v", err)
	}
	log.Println("Successfully connected worker to Redis instance.")

	// Create a cancellable context for graceful shutdown handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize and start the Order Event Consumer
	orderConsumer := stripeaym.NewOrderEventConsumer(rdb, 0)

	// Run the consumer in a separate goroutine so we can listen for termination signals below
	go func() {
		if err := orderConsumer.Start(ctx); err != nil && err != context.Canceled {
			log.Printf("Consumer error: %v", err)
		}
	}()

	log.Println("👷 Order Worker Process is running and waiting for events...")

	// Graceful Shutdown Listener
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	// Block main execution until a termination signal is caught
	sig := <-shutdownChan
	log.Printf("Received system signal (%v). Shutting down worker gracefully...", sig)

	// Cancel context to stop subscriber routines
	cancel()

	// Give active event handlers a brief window to wrap up execution
	time.Sleep(1 * time.Second)
	log.Println("Worker process gracefully stopped. Exiting.")
}
