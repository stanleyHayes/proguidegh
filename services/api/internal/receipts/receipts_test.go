package receipts

import (
	"bytes"
	"strings"
	"testing"
)

func TestWritePDFShape(t *testing.T) {
	t.Parallel()
	pdf := WritePDF([]Line{
		{Text: "ProGuideGH", Size: 22, Bold: true},
		{Text: "Payment Receipt (PGH-ABCDE)", Size: 14, Bold: true},
		{Text: "Gross amount: 450.00 GHS"},
		{Text: `Escaped \ parens \(ok\)`},
		{Text: "Unicode: Ɛε → replaced"},
	})

	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatal("PDF must start with %PDF header")
	}
	if !bytes.Contains(pdf, []byte("%%EOF")) {
		t.Fatal("PDF missing EOF trailer")
	}
	if !bytes.Contains(pdf, []byte("xref")) || !bytes.Contains(pdf, []byte("startxref")) {
		t.Fatal("PDF missing xref table")
	}
	// Parens/backslash escaping keeps the literal string parseable.
	if !strings.Contains(string(pdf), `\(PGH-ABCDE\)`) {
		t.Fatal("parens were not escaped")
	}
	// Non-ASCII bytes are replaced so the Type1 font encoding stays valid.
	for _, b := range pdf {
		if b > 127 {
			t.Fatalf("non-ASCII byte %d in output", b)
		}
	}
}

func TestPdfLinesInsuranceIndicator(t *testing.T) {
	t.Parallel()
	c := Content{
		PackageName: "Heritage Tour", TouristName: "Ama", GuideName: "Kofi",
		GrossAmount: "450.00", Currency: "GHS", PaymentMethod: "mock",
		ProviderReference: "ref-1", PlatformFee: "67.50", TourismLevy: "13.50",
		GuidePayable: "369.00",
	}
	without := string(WritePDF(pdfLines(c)))
	if strings.Contains(without, "Insurance Covered") {
		t.Fatal("insurance indicator must be omitted when coverage is not active (spec §17)")
	}
	c.InsuranceActive = true
	with := string(WritePDF(pdfLines(c)))
	if !strings.Contains(with, "Insurance Covered") {
		t.Fatal("insurance indicator missing when coverage is active")
	}
}

func TestNewReceiptNumberFormat(t *testing.T) {
	t.Parallel()
	for i := 0; i < 100; i++ {
		n, err := newReceiptNumber()
		if err != nil {
			t.Fatalf("entropy: %v", err)
		}
		if len(n) != 9 || !strings.HasPrefix(n, "PGH-") {
			t.Fatalf("bad receipt number %q", n)
		}
		for _, c := range n[4:] {
			if c < '0' || c > '9' {
				t.Fatalf("non-digit in receipt number %q", n)
			}
		}
	}
}
