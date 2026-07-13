package main

import (
	"context"
	"log"
	"net/http"
	"os"

	stripeaym "stripe-aym/pkg/stripe-aym"

	"github.com/joho/godotenv"
	"github.com/stripe/stripe-go/v86"
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
	secretKey := os.Getenv("STRIPE_SECRET_KEY")
	if secretKey == "" {
		log.Fatal("STRIPE_SECRET_KEY is empty! Check your .env file setup.")
	}

	scService := stripeaym.NewStripeCO(secretKey)
	// scService.MakeCheckoutSessions()
	// This is your test secret API key.
	// Don't put any keys in code. See https://docs.stripe.com/keys-best-practices.
	sc := stripe.NewClient(secretKey)

	http.Handle("/", http.FileServer(http.Dir("public/views")))
	http.HandleFunc("/create-checkout-session", func(w http.ResponseWriter, r *http.Request) { createCheckoutSession(sc, w, r) })
	http.HandleFunc("/create-checkout-session-v2", func(w http.ResponseWriter, r *http.Request) { createCOSessionFromService(scService, w, r) })
	addr := "localhost:4242"
	log.Printf("Listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func createCheckoutSession(sc *stripe.Client, w http.ResponseWriter, r *http.Request) {
	domain := "http://localhost:4242"
	params := &stripe.CheckoutSessionCreateParams{
		LineItems: []*stripe.CheckoutSessionCreateLineItemParams{
			&stripe.CheckoutSessionCreateLineItemParams{
				// Provide the exact Price ID (for example, price_1234) of the product you want to sell
				Price:    stripe.String("price_1To3pN2GZq7wFW9fVg4gZRcB"),
				Quantity: stripe.Int64(1),
			},
		},
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(domain + "/success.html"),
	}

	s, err := sc.V1CheckoutSessions.Create(context.TODO(), params)

	if err != nil {
		log.Printf("sc.V1CheckoutSessions.Create: %v", err)
	}

	http.Redirect(w, r, s.URL, http.StatusSeeOther)
}

func createCOSessionFromService(
	sc stripeaym.COSessionI, w http.ResponseWriter, r *http.Request,
) {
	domain := "http://localhost:4242"
	successURL := stripe.String(domain + "/success.html")
	cancelURL := stripe.String(domain + "/cancel.html")
	collectInfo := true
	totalCost := int64(1 * 35900000)
	pr := stripeaym.LineItemsCheckoutSessionParams{
		Items: []stripeaym.CartItem{
			stripeaym.CartItem{
				SKU:        "aym-ip-15-gold",
				Name:       "IPhone 17 gold",
				UnitAmount: 35900000,
				Quantity:   1,
			},
		},
		Subtotal:               1 * 35900000,
		SuccessURL:             *successURL,
		CancelURL:              *cancelURL,
		Currency:               "vnd",
		BillingAddressCollect:  &collectInfo,
		ShippingAddressCollect: &collectInfo,
		OrderID:                "123-qwae",
		TotalCost:              &totalCost,
	}
	checkoutURL, er := sc.MakeCheckoutSessions(
		&pr,
	)
	if er != nil {
		panic(er.Error())
	}

	http.Redirect(w, r, checkoutURL, http.StatusSeeOther)
}
