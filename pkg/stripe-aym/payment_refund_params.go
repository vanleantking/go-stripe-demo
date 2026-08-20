package stripeaym

import (
	"context"
	"errors"
	"fmt"

	"github.com/stripe/stripe-go/v86"
	"github.com/stripe/stripe-go/v86/paymentintent"
)

type PaymentRefundParams struct {
	PaymentIntentId string `json:"payment_intent_id"`
	Amount          int64  `json:"amount"`
	Currency        string `json:"currency"`
	Reason          string `json:"reason"`
	OrderID         string `json:"order_id"`
	IdempotencyKey  string `json:"idempotency_key"`
}

func (prp *PaymentRefundParams) MappingRefundParams(
	ctx context.Context,
) (*stripe.RefundCreateParams, error) {
	if prp.OrderID == "" {
		return nil, errors.New("orderID is required")
	}

	var paymentIntentId = prp.PaymentIntentId
	var err error
	if paymentIntentId == "" {
		paymentIntentId, err = prp.findPaymentIntentByOrderID(ctx, prp.OrderID)
		if err != nil {
			return nil, err
		}
	}

	params := &stripe.RefundCreateParams{
		PaymentIntent: stripe.String(prp.PaymentIntentId),
		// Reason:        stripe.String(prp.Reason),
		Metadata: map[string]string{
			"order_id": prp.OrderID,
		},
	}
	// 1. Map to Stripe's strict enum values
	switch prp.Reason {
	case string(stripe.RefundReasonDuplicate):
		params.Reason = stripe.String(string(stripe.RefundReasonDuplicate))
	case string(stripe.RefundReasonFraudulent):
		params.Reason = stripe.String(string(stripe.RefundReasonFraudulent))
	case string(stripe.RefundReasonRequestedByCustomer), "client request", "customer_request":
		params.Reason = stripe.String(string(stripe.RefundReasonRequestedByCustomer))
	default:
		// Default standard refund reason
		params.Reason = stripe.String(string(stripe.RefundReasonRequestedByCustomer))
		// Save the raw custom reason string in Metadata
		if prp.Reason != "" {
			params.AddMetadata("custom_reason", prp.Reason)
		}
	}

	// Passing Amount creates a PARTIAL refund; omitting Amount refunds the entire remaining balance
	if prp.Amount > 0 {
		params.Amount = stripe.Int64(prp.Amount)
	}

	// Idempotency prevents duplicate refunds on network timeouts/retries
	if prp.OrderID != "" {
		params.SetIdempotencyKey(fmt.Sprintf("refund_order_%s", prp.OrderID))
	}

	// Set context for timeout cancellation
	params.Context = ctx
	return params, nil
}

func (prp *PaymentRefundParams) findPaymentIntentByOrderID(
	ctx context.Context,
	orderID string,
) (string, error) {
	searchParams := &stripe.PaymentIntentSearchParams{}
	searchParams.Query = fmt.Sprintf("metadata['order_id']:'%s'", orderID)
	searchParams.Context = ctx

	iter := paymentintent.Search(searchParams)
	for iter.Next() {
		pi := iter.PaymentIntent()
		// Return the first matching non-canceled PaymentIntent
		if pi.Status == stripe.PaymentIntentStatusSucceeded {
			return pi.ID, nil
		}
	}

	if err := iter.Err(); err != nil {
		return "", fmt.Errorf("stripe search failed: %w", err)
	}

	return "", fmt.Errorf("no succeeded payment found for order_id: %s", orderID)
}
