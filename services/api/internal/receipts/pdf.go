// Package receipts generates and serves payment receipts (spec §4.5, §17).
// Receipts are issued exactly once per booking inside the payment-confirmation
// transaction; the PDF is stored in private object storage and downloads are
// short-lived signed URLs only (stop condition 8).
package receipts

import (
	"fmt"
	"strings"
)

// Line is one rendered text line. Size 0 means the default body size; Bold
// switches to Helvetica-Bold; Gap adds extra points of space after the line
// (blank Text lines are spacers).
type Line struct {
	Text string
	Size int
	Bold bool
	Gap  int
}

// WritePDF renders lines as a minimal, valid single-page A4 PDF using only
// the built-in Helvetica fonts — no external dependency (Phase 4 decision:
// a hand-rolled writer is preferable to a PDF library for plain-text
// receipts). Text is sanitized to printable ASCII: the standard Type1 fonts
// cannot encode arbitrary Unicode, so non-ASCII characters become '?'.
func WritePDF(lines []Line) []byte {
	var content strings.Builder
	content.WriteString("BT\n")
	y := 800.0
	for _, l := range lines {
		size := l.Size
		if size <= 0 {
			size = 11
		}
		font := "F1"
		if l.Bold {
			font = "F2"
		}
		if l.Text != "" {
			fmt.Fprintf(&content, "/%s %d Tf 1 0 0 1 56 %.2f Tm (%s) Tj\n",
				font, size, y, pdfEscape(l.Text))
		}
		y -= float64(size)*1.4 + float64(l.Gap)
	}
	content.WriteString("ET")
	stream := content.String()

	// Objects: 1 catalog, 2 pages, 3 page, 4 Helvetica, 5 Helvetica-Bold,
	// 6 contents stream.
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] " +
			"/Resources << /Font << /F1 4 0 R /F2 5 0 R >> >> /Contents 6 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
	}

	var pdf strings.Builder
	pdf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, obj := range objects {
		offsets[i] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}

	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n", len(objects)+1)
	pdf.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		len(objects)+1, xref)
	return []byte(pdf.String())
}

// pdfEscape sanitizes text for a literal PDF string: backslash and parens
// are escaped; non-printable/non-ASCII bytes become '?'.
func pdfEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' || c == '(' || c == ')':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c < 32 || c > 126:
			b.WriteByte('?')
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
