package stripeaym

import (
	"context"

	"github.com/stripe/stripe-go/v86"
)

type COSessionI interface {
	MakeCheckoutSessions(
		li *LineItemsCheckoutSessionParams,
	) (string, error)
}

type PII interface {
	MakePaymentIntents(siParams *PaymentIntentParams) (*stripe.PaymentIntent, error)
	IssueRefund(ctx context.Context, refundParams *PaymentRefundParams) (*stripe.Refund, error)
}
