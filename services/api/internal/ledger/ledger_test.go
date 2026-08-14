package ledger

import (
	"errors"
	"testing"
)

func TestValidateRejectsUnbalanced(t *testing.T) {
	t.Parallel()
	txn := Transaction{
		Reference: "PAY:test",
		Type:      "PAYMENT",
		Entries: []Entry{
			{AccountID: "a", Direction: Debit, AmountMinor: 10000},
			{AccountID: "b", Direction: Credit, AmountMinor: 9999}, // one pesewa short
		},
	}
	if err := Validate(txn); !errors.Is(err, ErrUnbalanced) {
		t.Fatalf("got %v, want ErrUnbalanced", err)
	}
}

func TestValidateRejectsBadShapes(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		txn  Transaction
		want error
	}{
		"no reference": {
			txn:  Transaction{Type: "PAYMENT", Entries: []Entry{{AccountID: "a", Direction: Debit, AmountMinor: 1}, {AccountID: "b", Direction: Credit, AmountMinor: 1}}},
			want: ErrNoReference,
		},
		"no entries": {
			txn:  Transaction{Reference: "r", Type: "PAYMENT"},
			want: ErrEmpty,
		},
		"zero amount": {
			txn: Transaction{Reference: "r", Type: "PAYMENT", Entries: []Entry{
				{AccountID: "a", Direction: Debit, AmountMinor: 0},
				{AccountID: "b", Direction: Credit, AmountMinor: 100},
			}},
			want: ErrBadAmount,
		},
		"negative amount": {
			txn: Transaction{Reference: "r", Type: "PAYMENT", Entries: []Entry{
				{AccountID: "a", Direction: Debit, AmountMinor: -50},
				{AccountID: "b", Direction: Credit, AmountMinor: 100},
			}},
			want: ErrBadAmount,
		},
		"bad direction": {
			txn: Transaction{Reference: "r", Type: "PAYMENT", Entries: []Entry{
				{AccountID: "a", Direction: "sideways", AmountMinor: 100},
			}},
			want: ErrBadDirection,
		},
		"all debits": {
			txn: Transaction{Reference: "r", Type: "PAYMENT", Entries: []Entry{
				{AccountID: "a", Direction: Debit, AmountMinor: 100},
			}},
			want: ErrUnbalanced,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate(tc.txn); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

func TestValidateAcceptsSpecExample(t *testing.T) {
	t.Parallel()
	// Spec §9.1: GHS 450 allocation — debit clearing 450.00, credits
	// 67.50 + 13.50 + 369.00. Exact integer sum.
	txn := Transaction{
		Reference: "PAY:ref",
		Type:      "PAYMENT",
		Entries: []Entry{
			{AccountID: "clearing", Direction: Debit, AmountMinor: 45000},
			{AccountID: "revenue", Direction: Credit, AmountMinor: 6750},
			{AccountID: "levy", Direction: Credit, AmountMinor: 1350},
			{AccountID: "payable", Direction: Credit, AmountMinor: 36900},
		},
	}
	if err := Validate(txn); err != nil {
		t.Fatalf("spec example must validate: %v", err)
	}
}

func TestReversedEntriesFlipsDirections(t *testing.T) {
	t.Parallel()
	orig := []Entry{
		{AccountID: "a", Direction: Debit, AmountMinor: 45000},
		{AccountID: "b", Direction: Credit, AmountMinor: 6750},
		{AccountID: "c", Direction: Credit, AmountMinor: 38250},
	}
	rev := reversedEntries(orig)

	// Originals untouched (reversal never mutates — spec §9.2).
	if orig[0].Direction != Debit || orig[1].Direction != Credit {
		t.Fatalf("reversedEntries mutated its input: %+v", orig)
	}
	want := []Entry{
		{AccountID: "a", Direction: Credit, AmountMinor: 45000},
		{AccountID: "b", Direction: Debit, AmountMinor: 6750},
		{AccountID: "c", Direction: Debit, AmountMinor: 38250},
	}
	for i := range want {
		if rev[i] != want[i] {
			t.Fatalf("entry %d = %+v, want %+v", i, rev[i], want[i])
		}
	}

	// A reversal of a valid transaction is itself valid: same sums flipped.
	rtx := Transaction{Reference: "REV:ref", Type: "PAYMENT_REVERSAL", Entries: rev}
	if err := Validate(rtx); err != nil {
		t.Fatalf("reversal must validate: %v", err)
	}
}
