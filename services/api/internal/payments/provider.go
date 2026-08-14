// Package payments implements the payments domain (spec §4.5, §8.3, §9,
// §13.3, §14, §16.1): provider adapters behind the PaymentProvider
// interface, payment initiation, the signed-webhook confirmation flow (which
// confirms the booking, posts the balanced ledger allocation, issues the
// receipt and queues notifications in ONE database transaction), and the
// privileged refund skeleton with ledger reversal.
package payments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

// PaymentProvider is the payment adapter interface (spec §16.1). Two
// implementations exist: MockProvider (deterministic sandbox, the default
// whenever PAYSTACK_SECRET_KEY is unset or PAYMENT_PROVIDER=mock) and
// PaystackProvider (production, config-gated).
type PaymentProvider interface {
	// Name is the provider code used in webhook routing and payments rows
	// ("mock", "paystack").
	Name() string
	// InitializePayment starts a provider-hosted payment and returns the
	// reference and hosted authorization URL the client is sent to.
	InitializePayment(ctx context.Context, req InitializePaymentRequest) (InitializePaymentResult, error)
	// VerifyPayment re-reads a payment's status from the provider
	// (reconciliation path; webhooks stay authoritative, spec §4.5).
	VerifyPayment(ctx context.Context, providerRef string) (VerifyResult, error)
	// Refund issues a (possibly partial) refund for a succeeded payment.
	Refund(ctx context.Context, req RefundRequest) (RefundResult, error)
	// CreateTransfer pays out to a tokenized recipient (payout phase).
	CreateTransfer(ctx context.Context, req TransferRequest) (TransferResult, error)
	// VerifyWebhook authenticates a raw webhook delivery (constant-time
	// signature comparison). It MUST be called on the exact bytes received,
	// before any JSON parsing.
	VerifyWebhook(headers http.Header, rawBody []byte) error
}

// InitializePaymentRequest is one payment initiation. AmountMinor is integer
// pesewas (Paystack's API takes minor units directly).
type InitializePaymentRequest struct {
	Email       string
	AmountMinor int64
	Currency    string
	Reference   string
	CallbackURL string
	Metadata    map[string]string
}

// InitializePaymentResult carries the provider reference and the hosted
// authorization URL.
type InitializePaymentResult struct {
	Reference        string
	AuthorizationURL string
}

// VerifyResult is the provider-reported payment state.
type VerifyResult struct {
	Reference   string
	Status      string // "success" | "pending" | "failed"
	AmountMinor int64
	Currency    string
}

// RefundRequest is one refund call.
type RefundRequest struct {
	ProviderReference string
	AmountMinor       int64
	Reason            string
}

// RefundResult carries the provider's refund reference.
type RefundResult struct {
	Reference string
	Status    string
}

// TransferRequest is one payout transfer (payout phase; present per §16.1).
type TransferRequest struct {
	AmountMinor   int64
	Currency      string
	RecipientCode string
	Reason        string
}

// TransferResult carries the provider's transfer reference.
type TransferResult struct {
	Reference string
	Status    string
}

// ErrBadSignature — webhook signature verification failed (mapped to 401).
var ErrBadSignature = errors.New("payments: invalid webhook signature")

// WebhookEvent is the parsed provider callback. Both adapters use the
// Paystack envelope shape ({"event","data":{"reference","status"}}) so one
// parser serves both — the mock deliberately mirrors production.
type WebhookEvent struct {
	Type      string // e.g. "charge.success"
	Reference string // data.reference — the dedupe event_reference
	Status    string // data.status, e.g. "success"
}

// ParseWebhookEvent extracts the event reference and status from a raw
// webhook body. Called only AFTER VerifyWebhook has authenticated the bytes.
func ParseWebhookEvent(rawBody []byte) (WebhookEvent, error) {
	var payload struct {
		Event string `json:"event"`
		Data  struct {
			Reference string `json:"reference"`
			Status    string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return WebhookEvent{}, fmt.Errorf("payments: parse webhook: %w", err)
	}
	if payload.Data.Reference == "" {
		return WebhookEvent{}, errors.New("payments: webhook carries no data.reference")
	}
	return WebhookEvent{
		Type:      payload.Event,
		Reference: payload.Data.Reference,
		Status:    payload.Data.Status,
	}, nil
}
