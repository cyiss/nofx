package kernel

import (
	"fmt"
	"nofx/mcp/payment"
	"testing"
)

func TestPaidVergexErrorsNeverTriggerFallback(t *testing.T) {
	for _, message := range []string{"invalid marketType", "invalid_request", "invalid chain", "market not found", "not_found"} {
		err := fmt.Errorf("%s: %w", message, payment.ErrPaymentOutcomeUnknown)
		if isRetryableVergexDetailError(err) {
			t.Errorf("paid failure %q allowed another charged attempt", message)
		}
		if !isRetryableVergexDetailError(fmt.Errorf("%s", message)) {
			t.Errorf("ordinary unsigned validation error %q should retain fallback", message)
		}
	}
}
