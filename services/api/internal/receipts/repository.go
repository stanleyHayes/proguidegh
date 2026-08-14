package receipts

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"proguidegh/api/internal/platform/storage"
)

// ErrNotFound — no receipt has been issued for the booking.
var ErrNotFound = errors.New("receipts: not found")

// Receipt is a receipts row (immutable once issued, spec §17).
type Receipt struct {
	ID            string    `json:"id"`
	BookingID     string    `json:"booking_id"`
	ReceiptNumber string    `json:"receipt_number"`
	ObjectKey     string    `json:"-"` // internal storage pointer; never exposed
	IssuedAt      time.Time `json:"issued_at"`
}

// Repository owns receipt persistence (explicit SQL, spec §7.2).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// Insert writes the receipt inside the caller's transaction, retrying the
// random receipt number on the (astronomically unlikely) unique collision.
// A UNIQUE booking_id violation means the receipt already exists — the caller
// treats that as the idempotent-replay case.
func (r *Repository) Insert(ctx context.Context, tx pgx.Tx, bookingID, objectKey string) (Receipt, error) {
	for attempt := 0; attempt < 5; attempt++ {
		number, err := newReceiptNumber()
		if err != nil {
			return Receipt{}, err
		}
		var rec Receipt
		err = tx.QueryRow(ctx, `
			INSERT INTO receipts (booking_id, receipt_number, object_key)
			VALUES ($1, $2, $3)
			RETURNING id, booking_id, receipt_number, object_key, issued_at`,
			bookingID, number, objectKey).
			Scan(&rec.ID, &rec.BookingID, &rec.ReceiptNumber, &rec.ObjectKey, &rec.IssuedAt)
		if isReceiptNumberCollision(err) {
			continue
		}
		if err != nil {
			return Receipt{}, fmt.Errorf("receipts: insert: %w", err)
		}
		return rec, nil
	}
	return Receipt{}, fmt.Errorf("receipts: could not allocate a unique receipt number")
}

// GetByBooking returns the receipt issued for a booking.
func (r *Repository) GetByBooking(ctx context.Context, bookingID string) (Receipt, error) {
	var rec Receipt
	err := r.pool.QueryRow(ctx, `
		SELECT id, booking_id, receipt_number, object_key, issued_at
		FROM receipts WHERE booking_id = $1`, bookingID).
		Scan(&rec.ID, &rec.BookingID, &rec.ReceiptNumber, &rec.ObjectKey, &rec.IssuedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Receipt{}, ErrNotFound
	}
	if err != nil {
		return Receipt{}, fmt.Errorf("receipts: get by booking: %w", err)
	}
	return rec, nil
}

// Service issues receipts (PDF + private storage + row) and resolves
// signed download URLs.
type Service struct {
	repo  *Repository
	store storage.Store
}

// NewService builds the service.
func NewService(repo *Repository, store storage.Store) *Service {
	return &Service{repo: repo, store: store}
}

// Content is the data rendered onto the receipt PDF (spec §17). Amounts are
// pre-formatted NUMERIC(14,2) strings.
type Content struct {
	BookingReference  string
	PackageName       string
	StartsAt          time.Time
	TouristName       string
	GuideName         string
	GrossAmount       string
	Currency          string
	PaymentMethod     string
	ProviderReference string
	PlatformFee       string
	TourismLevy       string
	GuidePayable      string
	InsuranceActive   bool // "Insurance Covered" only when actually active (§17)
}

// Issue renders the PDF, stores it privately and inserts the receipt row
// inside tx, so the receipt commits atomically with the payment confirmation.
// The object write is idempotent (same deterministic key per booking).
func (s *Service) Issue(ctx context.Context, tx pgx.Tx, bookingID string, c Content) (Receipt, error) {
	key := "receipts/" + bookingID + ".pdf"
	if err := s.store.Put(ctx, key, "application/pdf", WritePDF(pdfLines(c))); err != nil {
		return Receipt{}, fmt.Errorf("receipts: store pdf: %w", err)
	}
	return s.repo.Insert(ctx, tx, bookingID, key)
}

// Download returns the receipt plus a short-lived signed download URL.
func (s *Service) Download(ctx context.Context, bookingID string) (Receipt, string, error) {
	rec, err := s.repo.GetByBooking(ctx, bookingID)
	if err != nil {
		return Receipt{}, "", err
	}
	u, err := s.store.PresignGet(ctx, rec.ObjectKey)
	if err != nil {
		return Receipt{}, "", fmt.Errorf("receipts: presign download: %w", err)
	}
	return rec, u, nil
}

// pdfLines lays out the receipt body per spec §17.
func pdfLines(c Content) []Line {
	lines := []Line{
		{Text: "ProGuideGH", Size: 22, Bold: true, Gap: 4},
		{Text: "Payment Receipt", Size: 14, Bold: true, Gap: 10},
		{Text: "Tour:     " + c.PackageName},
		{Text: "Date:     " + c.StartsAt.UTC().Format("2006-01-02 15:04 UTC")},
		{Text: "Booking:  " + c.BookingReference, Gap: 6},
		{Text: "Tourist:  " + c.TouristName},
		{Text: "Guide:    " + c.GuideName, Gap: 6},
		{Text: "Gross amount: " + c.GrossAmount + " " + c.Currency, Bold: true},
		{Text: "Payment method: " + c.PaymentMethod},
		{Text: "Transaction reference: " + c.ProviderReference, Gap: 6},
		{Text: "Platform fee:  " + c.PlatformFee + " " + c.Currency},
		{Text: "Tourism levy:  " + c.TourismLevy + " " + c.Currency},
		{Text: "Guide payable: " + c.GuidePayable + " " + c.Currency, Gap: 6},
	}
	if c.InsuranceActive {
		lines = append(lines, Line{Text: "Insurance Covered", Bold: true, Gap: 6})
	}
	return append(lines,
		Line{Text: "Issued: " + time.Now().UTC().Format("2006-01-02 15:04:05 UTC"), Gap: 6},
		Line{Text: "Support: support@guideghana.example — quote your booking reference.", Size: 9},
	)
}

// newReceiptNumber generates a human-readable receipt reference ("PGH-88291"
// style, spec §4.5): five digits from crypto/rand. Uniqueness is enforced by
// the receipts_receipt_number_key constraint; Insert retries on collision.
func newReceiptNumber() (string, error) {
	buf := make([]byte, 5)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("receipts: number entropy: %w", err)
	}
	out := make([]byte, 5)
	for i, b := range buf {
		out[i] = '0' + b%10
	}
	return "PGH-" + string(out), nil
}

func isReceiptNumberCollision(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		pgErr.ConstraintName == "receipts_receipt_number_key"
}
