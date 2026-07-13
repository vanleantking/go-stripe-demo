package stripeaym

import "github.com/stripe/stripe-go/v86"

type COSessionI interface {
	MakeCheckoutSessions(
		li *LineItemsCheckoutSessionParams,
	) (string, error)
}

type PII interface {
	MakePaymentIntents(siParams *PaymentIntentParams) (*stripe.PaymentIntent, error)
}
