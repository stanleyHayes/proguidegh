package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PaystackBaseURL is the production API root; overridable for tests.
const PaystackBaseURL = "https://api.paystack.co"

// PaystackSignatureHeader carries the HMAC-SHA512 webhook signature.
const PaystackSignatureHeader = "X-Paystack-Signature"

// PaystackProvider is the production adapter (spec §16.1). It is selected
// only when PAYMENT_PROVIDER=paystack AND PAYSTACK_SECRET_KEY is set; it is
// fully implemented but never active without credentials. Amounts are
// integer minor units end to end, matching Paystack's API.
type PaystackProvider struct {
	secretKey     string
	webhookSecret string
	baseURL       string
	client        *http.Client
}

// NewPaystackProvider builds the adapter. webhookSecret falls back to the
// secret key when unset (Paystack signs with the account's secret key unless
// a dedicated webhook secret is configured).
func NewPaystackProvider(secretKey, webhookSecret string) *PaystackProvider {
	if webhookSecret == "" {
		webhookSecret = secretKey
	}
	return &PaystackProvider{
		secretKey:     secretKey,
		webhookSecret: webhookSecret,
		baseURL:       PaystackBaseURL,
		client:        &http.Client{Timeout: 15 * time.Second},
	}
}

// Name implements PaymentProvider.
func (p *PaystackProvider) Name() string { return "paystack" }

// envelope is the standard Paystack API response wrapper.
type envelope struct {
	Status  bool            `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// call performs one authenticated JSON request and decodes the envelope.
func (p *PaystackProvider) call(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("payments: paystack marshal: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("payments: paystack request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.secretKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("payments: paystack %s %s: %w", method, path, err)
	}
	defer resp.Body.Close() //nolint:errcheck
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("payments: paystack read: %w", err)
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("payments: paystack decode (%d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode >= 400 || !env.Status {
		return fmt.Errorf("payments: paystack %s %s failed (%d): %s", method, path, resp.StatusCode, env.Message)
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("payments: paystack decode data: %w", err)
		}
	}
	return nil
}

// InitializePayment starts a hosted payment (POST /transaction/initialize).
func (p *PaystackProvider) InitializePayment(ctx context.Context, req InitializePaymentRequest) (InitializePaymentResult, error) {
	payload := map[string]any{
		"email":     req.Email,
		"amount":    req.AmountMinor,
		"currency":  req.Currency,
		"reference": req.Reference,
	}
	if req.CallbackURL != "" {
		payload["callback_url"] = req.CallbackURL
	}
	if len(req.Metadata) > 0 {
		payload["metadata"] = req.Metadata
	}
	var data struct {
		AuthorizationURL string `json:"authorization_url"`
		Reference        string `json:"reference"`
	}
	if err := p.call(ctx, http.MethodPost, "/transaction/initialize", payload, &data); err != nil {
		return InitializePaymentResult{}, err
	}
	return InitializePaymentResult{Reference: data.Reference, AuthorizationURL: data.AuthorizationURL}, nil
}

// VerifyPayment re-reads the payment status (GET /transaction/verify/{ref}).
func (p *PaystackProvider) VerifyPayment(ctx context.Context, providerRef string) (VerifyResult, error) {
	var data struct {
		Reference string `json:"reference"`
		Status    string `json:"status"`
		Amount    int64  `json:"amount"`
		Currency  string `json:"currency"`
	}
	if err := p.call(ctx, http.MethodGet, "/transaction/verify/"+providerRef, nil, &data); err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{
		Reference:   data.Reference,
		Status:      data.Status,
		AmountMinor: data.Amount,
		Currency:    data.Currency,
	}, nil
}

// Refund issues a refund (POST /refund); amount in minor units.
func (p *PaystackProvider) Refund(ctx context.Context, req RefundRequest) (RefundResult, error) {
	payload := map[string]any{"transaction": req.ProviderReference}
	if req.AmountMinor > 0 {
		payload["amount"] = req.AmountMinor
	}
	if req.Reason != "" {
		payload["merchant_note"] = req.Reason
	}
	var data struct {
		Reference string `json:"reference"`
		Status    string `json:"status"`
	}
	if err := p.call(ctx, http.MethodPost, "/refund", payload, &data); err != nil {
		return RefundResult{}, err
	}
	return RefundResult{Reference: data.Reference, Status: data.Status}, nil
}

// CreateTransfer pays out to a tokenized recipient (POST /transfer).
func (p *PaystackProvider) CreateTransfer(ctx context.Context, req TransferRequest) (TransferResult, error) {
	payload := map[string]any{
		"source":    "balance",
		"amount":    req.AmountMinor,
		"recipient": req.RecipientCode,
		"reason":    req.Reason,
	}
	var data struct {
		Reference string `json:"reference"`
		Status    string `json:"status"`
	}
	if err := p.call(ctx, http.MethodPost, "/transfer", payload, &data); err != nil {
		return TransferResult{}, err
	}
	return TransferResult{Reference: data.Reference, Status: data.Status}, nil
}

// VerifyWebhook authenticates the delivery per Paystack docs:
// X-Paystack-Signature must equal hex(HMAC-SHA512(rawBody, secret)),
// compared in constant time over the exact bytes received.
func (p *PaystackProvider) VerifyWebhook(headers http.Header, rawBody []byte) error {
	got := headers.Get(PaystackSignatureHeader)
	if got == "" {
		return ErrBadSignature
	}
	h := hmac.New(sha512.New, []byte(p.webhookSecret))
	h.Write(rawBody)
	want := hex.EncodeToString(h.Sum(nil))
	if !hmac.Equal([]byte(got), []byte(want)) {
		return ErrBadSignature
	}
	return nil
}

// setBaseURL overrides the API root (tests point this at httptest servers).
func (p *PaystackProvider) setBaseURL(u string) { p.baseURL = u }
