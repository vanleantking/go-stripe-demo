package stripeaym

import (
	"context"
	"fmt"

	"github.com/stripe/stripe-go/v86"
)

func (sc *stripeCheckout) IssueRefund(ctx context.Context, refundParams *PaymentRefundParams) (*stripe.Refund, error) {
	params, er := refundParams.MappingRefundParams(ctx)
	if er != nil {
		return nil, er
	}

	ref, err := sc.stripeClient.V1Refunds.Create(ctx, params)
	if err != nil {
		if stripeErr, ok := err.(*stripe.Error); ok {
			// Handle specific Stripe API errors (e.g., charge already fully refunded)
			return nil, fmt.Errorf("stripe error [%s]: %s", stripeErr.Code, stripeErr.Msg)
		}
		return nil, fmt.Errorf("failed to issue refund: %w", err)
	}

	return ref, nil
}
