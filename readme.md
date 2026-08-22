STEP RUN LOCAL
1. **SETUP FORWARD LOCAL PORT FOR STRIPE WEBHOOK**
   1. Install Stripe CLI
   2. Login Stripe via Stripe CLI
      1. `stripe login`
         1. login into stripe with stripe account: username, password
      2. `stripe listen --forward-to localhost:4242/webhook`
         1. copy the secret key: `whsec_`
         2. add into `.env` file
2. **STEPS RUN COMMAND**
   1. run server api to create session payment intent
      1. `go run server-pi.go`
   2. run webhook (webhook port `8080`)
      1. `go run server-webhook.go`
   3. run forward stripe webhook event
      1. `stripe listen --forward-to localhost:8080/webhook/stripe`
   4. run workers to process order event
      1. `go run workers-order.go`