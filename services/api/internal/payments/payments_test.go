package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"proguidegh/api/internal/bookings"
)

func TestAllocateExactSums(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                           string
		gross, feePct, levyPct         int64
		wantFee, wantLevy, wantPayable int64
	}{
		// Spec §9.1 example: GHS 450.00 at 15% / 3%.
		{"spec example", 45000, 1500, 300, 6750, 1350, 36900},
		// GHS 250.00 city tour at 15% / 3%.
		{"city tour", 25000, 1500, 300, 3750, 750, 20500},
		// Odd pesewas: 100.01 GHS. 15% of 10001 = 1500.15 -> 1500;
		// 3% = 300.03 -> 300; remainder absorbs the rounding.
		{"odd pesewas", 10001, 1500, 300, 1500, 300, 8201},
		// Half-pesewa tie: 15% of 10 pesewas = 1.5 -> 2 (half away from zero).
		{"half tie", 10, 1500, 0, 2, 0, 8},
		// Zero percentages: everything goes to the guide.
		{"zero pcts", 9999, 0, 0, 0, 0, 9999},
		// Single pesewa with rounding against the guide.
		{"single pesewa", 1, 1500, 300, 0, 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := Allocate(tc.gross, tc.feePct, tc.levyPct)
			if a.PlatformFee != tc.wantFee || a.TourismLevy != tc.wantLevy || a.GuidePayable != tc.wantPayable {
				t.Fatalf("Allocate(%d,%d,%d) = %+v, want fee %d levy %d payable %d",
					tc.gross, tc.feePct, tc.levyPct, a, tc.wantFee, tc.wantLevy, tc.wantPayable)
			}
			// Launch-blocking invariant: shares ALWAYS sum exactly to gross.
			if a.PlatformFee+a.TourismLevy+a.GuidePayable != a.Gross {
				t.Fatalf("allocation does not sum to gross: %+v", a)
			}
		})
	}
}

// TestAllocateMatchesQuote pins the allocation to the Phase 3 quote math:
// the ledger must agree with ComputeBreakdown pesewa-for-pesewa over a sweep
// of odd amounts.
func TestAllocateMatchesQuote(t *testing.T) {
	t.Parallel()
	for gross := int64(1); gross < 20000; gross += 997 {
		a := Allocate(gross, 1500, 300)
		b, err := bookings.ComputeBreakdown(bookings.FormatMinor(gross), "GHS", "15", "3")
		if err != nil {
			t.Fatalf("breakdown: %v", err)
		}
		if b.PlatformFee != bookings.FormatMinor(a.PlatformFee) ||
			b.TourismLevy != bookings.FormatMinor(a.TourismLevy) ||
			b.GuidePayableEstimate != bookings.FormatMinor(a.GuidePayable) {
			t.Fatalf("gross %d: allocation %+v disagrees with quote %+v", gross, a, b)
		}
	}
}

func TestMockWebhookSignature(t *testing.T) {
	t.Parallel()
	m := NewMockProvider("test-secret")
	body, sig := m.SignWebhookPayload("ggpay_abc123")

	headers := http.Header{MockSignatureHeader: {sig}}
	if err := m.VerifyWebhook(headers, body); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}

	// Tampered body must fail even with the original signature.
	if err := m.VerifyWebhook(headers, append(body, ' ')); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("tampered body: got %v, want ErrBadSignature", err)
	}
	// Wrong secret.
	if err := NewMockProvider("other-secret").VerifyWebhook(headers, body); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("wrong secret: got %v, want ErrBadSignature", err)
	}
	// Missing header.
	if err := m.VerifyWebhook(http.Header{}, body); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("missing header: got %v, want ErrBadSignature", err)
	}
	// VerifyPayment agrees after completion.
	v, err := m.VerifyPayment(context.Background(), "ggpay_abc123")
	if err != nil || v.Status != "success" {
		t.Fatalf("VerifyPayment = %+v, %v; want success", v, err)
	}
	v, _ = m.VerifyPayment(context.Background(), "ggpay_never")
	if v.Status != "pending" {
		t.Fatalf("uncompleted ref = %q, want pending", v.Status)
	}
}

func TestPaystackWebhookSignature(t *testing.T) {
	t.Parallel()
	secret := "paystack-test-secret"
	p := NewPaystackProvider("sk_test_x", secret)
	body := []byte(`{"event":"charge.success","data":{"reference":"ref-1","status":"success"}}`)

	h := hmac.New(sha512.New, []byte(secret))
	h.Write(body)
	good := hex.EncodeToString(h.Sum(nil))

	if err := p.VerifyWebhook(http.Header{PaystackSignatureHeader: {good}}, body); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := p.VerifyWebhook(http.Header{PaystackSignatureHeader: {good}}, append(body, '\n')); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("tampered body: got %v, want ErrBadSignature", err)
	}
	if err := p.VerifyWebhook(http.Header{PaystackSignatureHeader: {"deadbeef"}}, body); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("bad signature: got %v, want ErrBadSignature", err)
	}
	// Falls back to the secret key when no webhook secret is configured.
	p2 := NewPaystackProvider("sk_test_x", "")
	h2 := hmac.New(sha512.New, []byte("sk_test_x"))
	h2.Write(body)
	if err := p2.VerifyWebhook(http.Header{PaystackSignatureHeader: {hex.EncodeToString(h2.Sum(nil))}}, body); err != nil {
		t.Fatalf("key-fallback signature rejected: %v", err)
	}
}

// TestPaystackProviderAgainstMockServer drives initialize/verify/refund/
// transfer against an httptest server shaped like api.paystack.co.
func TestPaystackProviderAgainstMockServer(t *testing.T) {
	t.Parallel()
	var gotPath, gotAuth string
	var gotInitBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.Method+" "+r.URL.Path, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/transaction/initialize":
			_ = json.NewDecoder(r.Body).Decode(&gotInitBody)
			_, _ = w.Write([]byte(`{"status":true,"message":"ok","data":{"authorization_url":"https://checkout.paystack.com/xyz","reference":"ggpay_t1"}}`))
		case "/transaction/verify/ggpay_t1":
			_, _ = w.Write([]byte(`{"status":true,"message":"ok","data":{"reference":"ggpay_t1","status":"success","amount":45000,"currency":"GHS"}}`))
		case "/refund":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["transaction"] == "fail" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"status":false,"message":"Transaction not found"}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":true,"message":"ok","data":{"reference":"rf_1","status":"processed"}}`))
		case "/transfer":
			_, _ = w.Write([]byte(`{"status":true,"message":"ok","data":{"reference":"tr_1","status":"pending"}}`))
		case "/fail":
			_, _ = w.Write([]byte(`{"status":false,"message":"Invalid key"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"status":false,"message":"not found"}`))
		}
	}))
	defer srv.Close()

	p := NewPaystackProvider("sk_test_secret", "")
	p.setBaseURL(srv.URL)
	ctx := context.Background()

	init, err := p.InitializePayment(ctx, InitializePaymentRequest{
		Email: "tourist@example.com", AmountMinor: 45000, Currency: "GHS", Reference: "ggpay_t1",
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if init.AuthorizationURL != "https://checkout.paystack.com/xyz" || init.Reference != "ggpay_t1" {
		t.Fatalf("initialize = %+v", init)
	}
	if gotAuth != "Bearer sk_test_secret" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	// Amount must travel as integer minor units.
	if amt, ok := gotInitBody["amount"].(float64); !ok || int64(amt) != 45000 {
		t.Fatalf("amount in request = %v, want 45000", gotInitBody["amount"])
	}

	v, err := p.VerifyPayment(ctx, "ggpay_t1")
	if err != nil || v.Status != "success" || v.AmountMinor != 45000 || v.Currency != "GHS" {
		t.Fatalf("verify = %+v, %v", v, err)
	}

	rf, err := p.Refund(ctx, RefundRequest{ProviderReference: "ggpay_t1", AmountMinor: 45000, Reason: "cancelled"})
	if err != nil || rf.Reference != "rf_1" {
		t.Fatalf("refund = %+v, %v", rf, err)
	}

	tr, err := p.CreateTransfer(ctx, TransferRequest{AmountMinor: 36900, Currency: "GHS", RecipientCode: "RCP_x", Reason: "payout"})
	if err != nil || tr.Reference != "tr_1" {
		t.Fatalf("transfer = %+v, %v", tr, err)
	}

	if _, err := p.Refund(ctx, RefundRequest{ProviderReference: "fail"}); err == nil {
		t.Fatal("provider error response must surface as an error")
	}
	_ = gotPath
}

func TestParseWebhookEvent(t *testing.T) {
	t.Parallel()
	ev, err := ParseWebhookEvent([]byte(`{"event":"charge.success","data":{"reference":"r1","status":"success"}}`))
	if err != nil || ev.Reference != "r1" || ev.Type != "charge.success" || ev.Status != "success" {
		t.Fatalf("parse = %+v, %v", ev, err)
	}
	if _, err := ParseWebhookEvent([]byte(`{"event":"charge.success","data":{}}`)); err == nil {
		t.Fatal("missing reference must fail")
	}
	if _, err := ParseWebhookEvent([]byte(`not json`)); err == nil {
		t.Fatal("malformed JSON must fail")
	}
}
