package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	stripeaym "stripe-aym/pkg/stripe-aym" // Replace with your actual module path

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func init() {
	// 2. Load the .env file
	// If test_request_api.go is in a subdirectory, use godotenv.Load("../.env") instead
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Warning: No .env file found, reading from system env instead")
	}
}

func main() {
	// 1. Load Environment Configuration
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	stripeWebhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if stripeWebhookSecret == "" {
		log.Fatal("CRITICAL: STRIPE_WEBHOOK_SECRET environment variable is missing")
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:51968" // Default fallback for local testing
	}

	// 2. Initialize Redis Client
	opt, err := redis.ParseURL("redis://" + redisURL)
	if err != nil {
		// Fallback to direct address if URL parsing isn't used
		opt = &redis.Options{
			Addr: redisURL,
		}
	}
	rdb := redis.NewClient(opt)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("CRITICAL: Failed to connect to Redis: %v", err)
	}
	log.Println("Successfully connected to Redis instance.")

	// 3. Initialize your Stripe Webhook Handler
	webhookHandler := stripeaym.NewStripeWebhookHandler(stripeWebhookSecret, rdb)

	// 4. Setup Router / Multiplexer
	mux := http.NewServeMux()

	// Register the webhook endpoint route
	mux.Handle("/webhook/stripe", webhookHandler)

	// Add a lightweight health-check endpoint for load balancers (AWS ALB, K8s probes)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// 5. Build Production-Hardened HTTP Server
	server := &http.Server{
		Addr:           ":" + port,
		Handler:        mux,
		ReadTimeout:    10 * time.Second, // Protects against slow clients hanging open connections
		WriteTimeout:   10 * time.Second, // Protects against slow writes
		IdleTimeout:    60 * time.Second, // Keep-alive timeout
		MaxHeaderBytes: 1 << 20,          // 1 MB max header size
	}

	// 6. Graceful Shutdown Channel Listener
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Printf("Starting production web server on port %s...", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server execution failed: %v", err)
		}
	}()

	// Block execution until interruption signal is received
	sig := <-shutdownChan
	log.Printf("Received system signal (%v). Initiating graceful shutdown...", sig)

	// Create a context with a 15-second timeout for active requests to finish
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server forced shutdown due to error: %v", err)
	}

	log.Println("Server gracefully stopped. Exiting process.")
}
