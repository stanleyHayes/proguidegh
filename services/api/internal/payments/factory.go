package payments

import "proguidegh/api/internal/platform/config"

// NewProvider selects the active payment adapter from configuration
// (spec §16.1, §31.29):
//
//   - PAYMENT_PROVIDER=paystack AND PAYSTACK_SECRET_KEY set → PaystackProvider
//     (production path; PAYSTACK_WEBHOOK_SECRET verifies webhook signatures).
//   - anything else (PAYMENT_PROVIDER=mock, or no Paystack secret) →
//     MockProvider, the deterministic sandbox. The mock is the default in
//     every environment without Paystack credentials, so development and CI
//     never require external accounts.
func NewProvider(cfg config.Config) PaymentProvider {
	if cfg.PaymentProvider == "paystack" && cfg.PaystackSecretKey != "" {
		return NewPaystackProvider(cfg.PaystackSecretKey, cfg.PaystackWebhookSecret)
	}
	return NewMockProvider(cfg.MockWebhookSecret)
}
