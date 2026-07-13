package stripeaym

import (
	"fmt"

	"github.com/stripe/stripe-go/v86"
)

type PaymentIntentParams struct {
	Items           []CartItem       `json:"items"`
	Subtotal        int64            `json:"sub_total"`
	ShippingCost    *int64           `json:"shipping_cost"`
	TotalCost       *int64           `json:"total_cost"`
	TaxCost         *int64           `json:"tax_cost"`
	Mode            string           `json:"mode"`
	SuccessURL      string           `json:"success_url"`
	ReturnURL       string           `json:"return_url"`
	CancelURL       string           `json:"cancel_url"`
	Currency        string           `json:"currency"`
	OrderID         string           `json:"order_id"`
	ExpiresAt       *int64           `json:"expires_at"`
	ShippingAddress *ShippingAddress `json:"shipping_address"`
	Carrier         string           `json:"carrier"`
}

type ShippingAddress struct {
	Name       string `json:"name"`
	Line1      string `json:"line1"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"` // ISO 3166-1 alpha-2 code (e.g., "VN")
}

func (pi *PaymentIntentParams) MappingPaymentIntentParams() (*stripe.PaymentIntentCreateParams, error) {
	if pi.OrderID == "" {
		return nil, fmt.Errorf(
			"billing error, order id is required, %s",
			pi.OrderID,
		)
	}
	if pi.TotalCost == nil || *pi.TotalCost == 0 {
		return nil, fmt.Errorf(
			"billing error, total cost is required, %s",
			pi.OrderID,
		)
	}
	if len(pi.Items) == 0 {
		return nil, fmt.Errorf("cart items is empty, %s", pi.OrderID)
	}
	calculatedSubtotal := pi.calculatedSubTotal(pi.Items)

	// Initialize tracking variables for absolute metadata safety
	var shippingVal int64
	var taxVal int64

	if pi.ShippingCost != nil {
		shippingVal = *pi.ShippingCost
	}

	// 3. Safely unpack tax cost
	if pi.TaxCost != nil {
		taxVal = *pi.TaxCost
	}

	expectedTotal := calculatedSubtotal + shippingVal + taxVal
	if *pi.TotalCost != expectedTotal {
		return nil, fmt.Errorf(
			"billing discrepancy: incoming total %d does not match calculated total %d",
			*pi.TotalCost,
			expectedTotal,
		)
	}

	// 3. Create Stripe PaymentIntent
	params := &stripe.PaymentIntentCreateParams{
		Amount:   stripe.Int64(*pi.TotalCost),
		Currency: stripe.String(string(pi.Currency)), // Or USD, etc.
		// Attach your internal Order ID so webhooks can map it back
		Metadata: map[string]string{
			"order_id":     pi.OrderID,
			"raw_subtotal": fmt.Sprintf("%d", calculatedSubtotal),
			"raw_tax":      fmt.Sprintf("%d", taxVal),
			"raw_shipping": fmt.Sprintf("%d", shippingVal),
		},
		// Automatic payment methods enable Apple Pay, Google Pay, etc., automatically
		AutomaticPaymentMethods: &stripe.PaymentIntentCreateAutomaticPaymentMethodsParams{
			Enabled: stripe.Bool(true),
		},
	}

	if pi.ShippingAddress != nil {
		params.Shipping = &stripe.ShippingDetailsParams{
			Name: stripe.String(pi.ShippingAddress.Name),
			Address: &stripe.AddressParams{
				Line1:      stripe.String(pi.ShippingAddress.Line1),
				City:       stripe.String(pi.ShippingAddress.City),
				State:      stripe.String(pi.ShippingAddress.State),
				PostalCode: stripe.String(pi.ShippingAddress.PostalCode),
				Country:    stripe.String(pi.ShippingAddress.Country),
			},
			Carrier: stripe.String(pi.Carrier),
		}
	}

	// Idempotency: Prevent double charges if the network drops and client retries
	params.IdempotencyKey = stripe.String(fmt.Sprintf("pi_create_%s", pi.OrderID))
	return params, nil
}

func (pi *PaymentIntentParams) calculatedSubTotal(items []CartItem) int64 {
	var subTotal = int64(0)
	fmt.Println(len(items))
	for _, item := range items {
		subTotal += item.Quantity * item.UnitAmount
	}
	return subTotal
}
