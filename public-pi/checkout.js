// This is your test publishable API key.
const stripe = Stripe("pk_test_dYxARN1KXc7Tu1SMTz0IsA8a");

// The items the customer wants to buy
const items = [{ id: "xl-tshirt", amount: 1000 }];

let elements;

// Initialize the elements right away
initialize();

document
  .querySelector("#payment-form")
  .addEventListener("submit", handleSubmit);

// Fetches a payment intent and captures the client secret
async function initialize() {
  try {
    // Note: Make sure the URL matches your Go backend's route mapping
    const response = await fetch("/create-payment-intent", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ items }),
    });

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`);
    }

    const { clientSecret } = await response.json();

    const appearance = {
      theme: 'stripe',
    };
    
    // Create elements instance with the client secret
    elements = stripe.elements({ appearance, clientSecret });

    const paymentElementOptions = {
      layout: "accordion",
    };

    // Create and mount the payment element
    const paymentElement = elements.create("payment", paymentElementOptions);
    paymentElement.mount("#payment-element");
    
  } catch (error) {
    console.error("Initialization failed:", error);
    showMessage("Failed to initialize the payment system. Please try again later.");
  }
}

async function handleSubmit(e) {
  e.preventDefault();

  // GUARD: Prevent submission if elements haven't loaded yet
  // This specifically fixes the "elements should have a mounted Payment Element" error
  if (!stripe || !elements) {
    showMessage("Payment form is still loading. Please wait.");
    return;
  }

  setLoading(true);

  const { error } = await stripe.confirmPayment({
    elements,
    confirmParams: {
      // Make sure to change this to your payment completion page
      return_url: "http://localhost:4242/complete.html",
    },
  });

  // This point will only be reached if there is an immediate error when
  // confirming the payment.
  if (error.type === "card_error" || error.type === "validation_error") {
    showMessage(error.message);
  } else {
    showMessage("An unexpected error occurred.");
  }

  setLoading(false);
}

// ------- UI helpers -------
function showMessage(messageText) {
  const messageContainer = document.querySelector("#payment-message");
  messageContainer.classList.remove("hidden");
  messageContainer.textContent = messageText;
}

// Show a spinner on payment submission
function setLoading(isLoading) {
  if (isLoading) {
    // Disable the button and show a spinner
    document.querySelector("#submit").disabled = true;
    document.querySelector("#spinner").classList.remove("hidden");
    document.querySelector("#button-text").classList.add("hidden");
  } else {
    document.querySelector("#submit").disabled = false;
    document.querySelector("#spinner").classList.add("hidden");
    document.querySelector("#button-text").classList.remove("hidden");
  }
}