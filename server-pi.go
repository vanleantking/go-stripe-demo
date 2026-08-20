package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	stripeaym "stripe-aym/pkg/stripe-aym"

	"github.com/joho/godotenv"
	"github.com/stripe/stripe-go/v86"
)

type RefundRequest struct {
	PaymentIntentID string `json:"payment_intent_id"`
	OrderID         string `json:"order_id"`
	Reason          string `json:"reason"`
	Amount          int64  `json:"amount"` // Optional: pass amount dynamically if needed
}

var sc *stripe.Client

func init() {
	// 2. Load the .env file
	// If test_request_api.go is in a subdirectory, use godotenv.Load("../.env") instead
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Warning: No .env file found, reading from system env instead")
	}
}

func main() {
	secretKey := os.Getenv("STRIPE_SECRET_KEY")
	if secretKey == "" {
		log.Fatal("STRIPE_SECRET_KEY is empty! Check your .env file setup.")
	}
	// This is your test secret API key.
	// Don't put any keys in code. See https://docs.stripe.com/keys-best-practices.
	sc = stripe.NewClient(secretKey)
	scService := stripeaym.NewStripeCO(secretKey)

	fs := http.FileServer(http.Dir("public-pi"))
	http.Handle("/", fs)
	http.HandleFunc("/create-payment-intent", func(w http.ResponseWriter, r *http.Request) {
		handleCreatePaymentIntent(scService, w, r)
	})

	// Example trigger for a partial refund
	http.HandleFunc("/api/refund", func(w http.ResponseWriter, r *http.Request) {
		makeRefundRequest(scService, w, r)
	})

	addr := "localhost:4242"
	log.Printf("Listening on %s ...", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

type item struct {
	Id     string
	Amount int64
}

func calculateOrderAmount(items []item) int64 {
	// Calculate the order total on the server to prevent
	// people from directly manipulating the amount on the client
	total := int64(0)
	for _, item := range items {
		total += item.Amount
	}
	return total
}

func handleCreatePaymentIntent(service stripeaym.PII, w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Items []item `json:"items"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("json.NewDecoder.Decode: %v", err)
		return
	}

	// Create a PaymentIntent with amount and
	params := stripeaym.PaymentIntentParams{
		Items: []stripeaym.CartItem{
			stripeaym.CartItem{
				SKU:        req.Items[0].Id,
				UnitAmount: req.Items[0].Amount,
				Quantity:   1,
			},
		},
		Currency:  string(stripe.CurrencyUSD),
		Subtotal:  calculateOrderAmount(req.Items),
		TotalCost: stripe.Int64(calculateOrderAmount(req.Items)),
		ShippingAddress: &stripeaym.ShippingAddress{
			Country:    "vietnam",
			City:       "phu yen, daklak",
			PostalCode: "70000",
		},
		OrderID: "sunway-777",
	}
	pi, err := service.MakePaymentIntents(&params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("pi.Create: %v", err)
		return
	}
	log.Printf("pi.Create: %v", pi.ClientSecret)

	writeJSON(w, struct {
		ClientSecret string `json:"clientSecret"`
	}{
		ClientSecret: pi.ClientSecret,
	})
}

func handleCreatePaymentIntent2(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Items []item `json:"items"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("json.NewDecoder.Decode: %v", err)
		return
	}

	// Create a PaymentIntent with amount and currency
	params := &stripe.PaymentIntentCreateParams{
		Amount:   stripe.Int64(calculateOrderAmount(req.Items)),
		Currency: stripe.String(string(stripe.CurrencyUSD)),
		// In the latest version of the API, specifying the `automatic_payment_methods` parameter is optional because Stripe enables its functionality by default.
		AutomaticPaymentMethods: &stripe.PaymentIntentCreateAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
	}

	pi, err := sc.V1PaymentIntents.Create(context.TODO(), params)
	log.Printf("pi.Create: %v", pi.ClientSecret)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("pi.Create: %v", err)
		return
	}

	writeJSON(w, struct {
		ClientSecret string `json:"clientSecret"`
	}{
		ClientSecret: pi.ClientSecret,
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("json.NewEncoder.Encode: %v", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := io.Copy(w, &buf); err != nil {
		log.Printf("io.Copy: %v", err)
		return
	}
}

func makeRefundRequest(service stripeaym.PII, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req RefundRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Validate required fields
	if req.PaymentIntentID == "" || req.OrderID == "" {
		http.Error(w, "payment_intent_id and order_id are required", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Refunding $5.00 (500 cents) partially on a PaymentIntent
	paymentIntentID := req.PaymentIntentID
	pirf := &stripeaym.PaymentRefundParams{
		PaymentIntentId: paymentIntentID,
		Amount:          500,
		Reason:          req.Reason,
		OrderID:         req.OrderID,
	}

	ref, err := service.IssueRefund(ctx, pirf)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ref)
}
