package stripeaym

import (
	"context"

	"github.com/stripe/stripe-go/v86"
)

func (sc *stripeCheckout) MakePaymentIntents(siParams *PaymentIntentParams) (*stripe.PaymentIntent, error) {
	params, er := siParams.MappingPaymentIntentParams()
	if er != nil {
		return nil, er
	}

	s, err := sc.stripeClient.V1PaymentIntents.Create(
		context.TODO(),
		params,
	)
	return s, err
}
