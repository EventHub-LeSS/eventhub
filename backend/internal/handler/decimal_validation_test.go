package handler

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin/binding"
	"github.com/shopspring/decimal"
)

// Decimals with a huge exponent must be rejected at once: converting one to float64 materializes
// 10^|exponent| and would stall the request (EVENTHUB-235).
func TestDecimalValidation_RejectsHugeExponentsQuickly(t *testing.T) {
	huge := []string{"1e2000000000", "1e-2000000000", "1e33", "1e-33"}
	for _, value := range huge {
		d := decimal.RequireFromString(value)

		draft := validDraftRequest()
		draft.Price = d
		payment := SimulatePaymentRequest{Outcome: "success", Amount: &d}

		for name, req := range map[string]any{"draft price": draft, "payment amount": payment} {
			done := make(chan error, 1)
			go func() { done <- binding.Validator.ValidateStruct(req) }()
			select {
			case err := <-done:
				if err == nil {
					t.Errorf("%s %s: expected a validation error", name, value)
				}
			case <-time.After(2 * time.Second):
				t.Fatalf("%s %s: validation did not finish within 2s", name, value)
			}
		}
	}

	// Ordinary values within the bounds keep validating as before.
	for _, value := range []string{"0", "15", "15.00", "1e3", "0.000000000000000000000000000001"} {
		d := decimal.RequireFromString(value)
		if err := binding.Validator.ValidateStruct(SimulatePaymentRequest{Outcome: "success", Amount: &d}); err != nil {
			t.Errorf("amount %s: expected valid, got %v", value, err)
		}
	}
}
