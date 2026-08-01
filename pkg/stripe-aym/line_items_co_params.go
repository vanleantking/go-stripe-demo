package stripeaym

import (
	"fmt"
	"time"

	"github.com/stripe/stripe-go/v86"
)

type LineItemsCheckoutSessionParams struct {
	Items                  []CartItem `json:"items"`
	Subtotal               int64      `json:"sub_total"`
	ShippingCost           *int64     `json:"shipping_cost"`
	TotalCost              *int64     `json:"total_cost"`
	TaxCost                *int64     `json:"tax_cost"`
	Mode                   string     `json:"mode"`
	SuccessURL             string     `json:"success_url"`
	ReturnURL              string     `json:"return_url"`
	CancelURL              string     `json:"cancel_url"`
	Currency               string     `json:"currency"`
	OrderID                string     `json:"order_id"`
	ExpiresAt              *int64     `json:"expires_at"`
	BillingAddressCollect  *bool      `json:"billing_address_collect"`
	ShippingAddressCollect *bool      `json:"shipping_address_collect"`
}

type CartItem struct {
	SKU        string `json:"sku"`
	Name       string `json:"name"`
	UnitAmount int64  `json:"unit_amount"`
	Quantity   int64  `json:"quantity"` //currency = cent = int64(USD * 100)
}

// MappingLICheckoutSessionParams constructs parameters safely and validates data integrity
func (li *LineItemsCheckoutSessionParams) MappingLICheckoutSessionParams() (*stripe.CheckoutSessionCreateParams, error) {
	if li.OrderID == "" {
		return nil, fmt.Errorf("billing error, order id is required, %s", li.OrderID)
	}
	if li.TotalCost == nil || *li.TotalCost == 0 {
		return nil, fmt.Errorf("billing error, total cost is required, %s", li.OrderID)
	}
	if len(li.Items) == 0 {
		return nil, fmt.Errorf("cart items is empty, %s", li.OrderID)
	}
	lineItems := []*stripe.CheckoutSessionCreateLineItemParams{}

	// 1. Generate items and get the calculated subtotal
	cartItems, calculatedSubtotal := li.generateCartItems(li.Items, li.Currency)
	lineItems = append(lineItems, cartItems...)

	// Initialize tracking variables for absolute metadata safety
	var shippingVal int64
	var taxVal int64

	// 2. Safely unpack shipping cost
	if li.ShippingCost != nil {
		shippingVal = *li.ShippingCost
		lineItems = append(lineItems, &stripe.CheckoutSessionCreateLineItemParams{
			PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
				Currency:   stripe.String(li.Currency),
				UnitAmount: stripe.Int64(shippingVal),
				ProductData: &stripe.CheckoutSessionCreateLineItemPriceDataProductDataParams{
					Name: stripe.String("Shipping Fee"),
				},
			},
			Quantity: stripe.Int64(1),
		})
	}

	// 3. Safely unpack tax cost
	if li.TaxCost != nil {
		taxVal = *li.TaxCost
		lineItems = append(lineItems, &stripe.CheckoutSessionCreateLineItemParams{
			PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
				Currency:   stripe.String(li.Currency),
				UnitAmount: stripe.Int64(taxVal),
				ProductData: &stripe.CheckoutSessionCreateLineItemPriceDataProductDataParams{
					Name: stripe.String("VAT"),
				},
			},
			Quantity: stripe.Int64(1),
		})
	}

	// 4. Architectural Guardrail: Validate local total matches the calculated breakdown
	expectedTotal := calculatedSubtotal + shippingVal + taxVal
	if *li.TotalCost != expectedTotal {
		return nil, fmt.Errorf(
			"billing discrepancy: incoming total %d does not match calculated total %d",
			*li.TotalCost,
			expectedTotal,
		)
	}

	// Default fallback for Checkout Mode if empty
	checkoutMode := li.Mode
	if checkoutMode == "" {
		checkoutMode = string(stripe.CheckoutSessionModePayment)
	}

	// 5. Create the Session Parameters using fields passed in the struct
	params := &stripe.CheckoutSessionCreateParams{
		LineItems:  lineItems,
		Mode:       stripe.String(checkoutMode),
		SuccessURL: stripe.String(li.SuccessURL),
		CancelURL:  stripe.String(li.CancelURL),
		Metadata: map[string]string{
			"order_id":     li.OrderID,
			"raw_subtotal": fmt.Sprintf("%d", calculatedSubtotal),
			"raw_tax":      fmt.Sprintf("%d", taxVal),
			"raw_shipping": fmt.Sprintf("%d", shippingVal),
		},
	}
	if li.ExpiresAt != nil {
		// Optional: Add validation here to ensure *li.ExpiresAt is between 1 and 24
		if *li.ExpiresAt < 1 || *li.ExpiresAt > 24 {
			return nil, fmt.Errorf("expires_at must be between 1 and 24 hours")
		}
		params.ExpiresAt = li.conExpiresAt(li.ExpiresAt)
	}

	// support save billing address
	if li.BillingAddressCollect != nil && *li.BillingAddressCollect == true {
		params.BillingAddressCollection = stripe.String(stripe.CheckoutSessionBillingAddressCollectionRequired)
	}

	// support save shipping address
	if li.ShippingAddressCollect != nil && *li.ShippingAddressCollect == true {
		params.ShippingAddressCollection = &stripe.CheckoutSessionCreateShippingAddressCollectionParams{
			AllowedCountries: []*string{
				stripe.String("US"),
				stripe.String("CA"),
			},
		}
	}

	return params, nil
}

func (li *LineItemsCheckoutSessionParams) generateCartItems(
	items []CartItem,
	currency string,
) ([]*stripe.CheckoutSessionCreateLineItemParams, int64) {
	result := make([]*stripe.CheckoutSessionCreateLineItemParams, 0, len(items))
	var subtotal int64

	for _, item := range items {
		result = append(result, &stripe.CheckoutSessionCreateLineItemParams{
			PriceData: &stripe.CheckoutSessionCreateLineItemPriceDataParams{
				Currency:   stripe.String(currency),
				UnitAmount: stripe.Int64(item.UnitAmount),
				ProductData: &stripe.CheckoutSessionCreateLineItemPriceDataProductDataParams{
					Name: stripe.String(item.Name),
				},
			},
			Quantity: stripe.Int64(item.Quantity),
		})
		subtotal += item.UnitAmount * item.Quantity
	}
	return result, subtotal
}

func (li *LineItemsCheckoutSessionParams) conExpiresAt(
	hoursToLive *int64,
) *int64 {
	expiryTime := time.Now().Add(time.Duration(*hoursToLive) * time.Hour).Unix()
	return &expiryTime
}
