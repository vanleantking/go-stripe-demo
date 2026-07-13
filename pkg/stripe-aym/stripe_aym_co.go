package stripeaym

import (
	"context"

	"github.com/stripe/stripe-go/v86"
)

type stripeCheckout struct {
	stripeClient *stripe.Client
}

func NewStripeCO(apiKey string) *stripeCheckout {
	sc := stripe.NewClient(apiKey)
	return &stripeCheckout{
		stripeClient: sc,
	}
}

func (sc *stripeCheckout) MakeCheckoutSessions(
	li *LineItemsCheckoutSessionParams,
) (string, error) {
	params, er := li.MappingLICheckoutSessionParams()
	if er != nil {
		return "", er
	}

	s, err := sc.stripeClient.V1CheckoutSessions.Create(
		context.TODO(),
		params,
	)
	if err != nil {
		return "", err
	}

	return s.URL, nil
}
