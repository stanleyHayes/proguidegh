// Package reporting implements executive KPIs, operational reports with
// permitted CSV exports (P8-02) and the append-only audit-log viewer
// (P8-04, spec §1.2, §13.6).
package reporting

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// KPIs is the executive dashboard payload (P8-02).
type KPIs struct {
	UsersTotal              int64   `json:"users_total"`
	GuidesCertified         int64   `json:"guides_certified"`
	Bookings30d             int64   `json:"bookings_30d"`
	BookingsActive          int64   `json:"bookings_active"`
	GMV30dMinor             int64   `json:"gmv_30d_minor"`
	PlatformRevenue30dMinor int64   `json:"platform_revenue_30d_minor"`
	SOS30d                  int64   `json:"sos_30d"`
	AverageRating           float64 `json:"average_rating"`
	ReviewsTotal            int64   `json:"reviews_total"`
	PayoutsPaid30dMinor     int64   `json:"payouts_paid_30d_minor"`
}

// StatusCount is one bookings-by-status bucket in the bookings report.
type StatusCount struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

// BookingsReport is the operational bookings report (P8-02).
type BookingsReport struct {
	From          string        `json:"from"`
	To            string        `json:"to"`
	Total         int64         `json:"total"`
	ByStatus      []StatusCount `json:"by_status"`
	GMVMinor      int64         `json:"gmv_minor"`
	RefundedMinor int64         `json:"refunded_minor"`
}

// BookingExportRow is one CSV row before rendering.
type BookingExportRow struct {
	Reference    string
	TouristEmail string
	GuideName    *string
	PackageTitle string
	StartsAt     time.Time
	Status       string
	AmountMinor  *int64
	Currency     *string
}

// AuditEntry is one audit_logs row (spec §1.2 append-only trail).
type AuditEntry struct {
	ID         string    `json:"id"`
	ActorID    *string   `json:"actor_id"`
	ActorEmail *string   `json:"actor_email"`
	Action     string    `json:"action"`
	EntityType string    `json:"entity_type"`
	EntityID   *string   `json:"entity_id"`
	Before     any       `json:"before"`
	After      any       `json:"after"`
	CreatedAt  time.Time `json:"created_at"`
}

// AuditFilter narrows the audit viewer query.
type AuditFilter struct {
	ActorID    string
	Action     string
	EntityType string
	EntityID   string
	From       *time.Time
	To         *time.Time
}

// Repository is the reporting data layer.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) scalar(ctx context.Context, q string, args ...any) (int64, error) {
	var n int64
	if err := r.pool.QueryRow(ctx, q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// KPIs computes the executive dashboard numbers.
func (r *Repository) KPIs(ctx context.Context) (KPIs, error) {
	var k KPIs
	var err error
	q := func(dst *int64, query string, args ...any) {
		if err != nil {
			return
		}
		*dst, err = r.scalar(ctx, query, args...)
	}
	q(&k.UsersTotal, `SELECT COUNT(*)::bigint FROM users`)
	q(&k.GuidesCertified, `SELECT COUNT(*)::bigint FROM guide_profiles WHERE status = 'certified'`)
	q(&k.Bookings30d, `SELECT COUNT(*)::bigint FROM bookings WHERE created_at > now() - interval '30 days'`)
	q(&k.BookingsActive, `SELECT COUNT(*)::bigint FROM bookings WHERE status IN ('CONFIRMED','GUIDE_EN_ROUTE','GUIDE_ARRIVED','IN_PROGRESS')`)
	q(&k.GMV30dMinor, `SELECT COALESCE(ROUND(SUM(amount) * 100)::bigint, 0) FROM payments
		WHERE status IN ('SUCCEEDED','PARTIALLY_REFUNDED','REFUNDED') AND paid_at > now() - interval '30 days'`)
	q(&k.PlatformRevenue30dMinor, `SELECT COALESCE(ROUND(SUM(CASE WHEN e.direction='credit' THEN e.amount ELSE -e.amount END) * 100)::bigint, 0)
		FROM ledger_entries e
		JOIN ledger_accounts a ON a.id = e.account_id
		JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE a.owner_type = 'platform' AND a.code = 'platform_revenue'
		  AND t.occurred_at > now() - interval '30 days'`)
	q(&k.SOS30d, `SELECT COUNT(*)::bigint FROM sos_events WHERE triggered_at > now() - interval '30 days'`)
	q(&k.ReviewsTotal, `SELECT COUNT(*)::bigint FROM reviews`)
	q(&k.PayoutsPaid30dMinor, `SELECT COALESCE(ROUND(SUM(amount) * 100)::bigint, 0) FROM payouts
		WHERE status = 'PAID' AND updated_at > now() - interval '30 days'`)
	if err != nil {
		return KPIs{}, fmt.Errorf("reporting: kpis: %w", err)
	}
	if err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(AVG(rating)::float8, 0) FROM reviews`).Scan(&k.AverageRating); err != nil {
		return KPIs{}, fmt.Errorf("reporting: avg rating: %w", err)
	}
	return k, nil
}

// BookingsReport aggregates bookings in [from, to) by status with GMV.
func (r *Repository) BookingsReport(ctx context.Context, from, to time.Time) (BookingsReport, error) {
	rep := BookingsReport{
		From: from.Format("2006-01-02"),
		To:   to.Format("2006-01-02"),
	}
	rows, err := r.pool.Query(ctx,
		`SELECT status, COUNT(*)::bigint FROM bookings
		 WHERE created_at >= $1 AND created_at < $2
		 GROUP BY status ORDER BY COUNT(*) DESC`, from, to)
	if err != nil {
		return BookingsReport{}, fmt.Errorf("reporting: bookings by status: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var sc StatusCount
		if err := rows.Scan(&sc.Status, &sc.Count); err != nil {
			return BookingsReport{}, fmt.Errorf("reporting: scan status: %w", err)
		}
		rep.ByStatus = append(rep.ByStatus, sc)
		rep.Total += sc.Count
	}
	if err := rows.Err(); err != nil {
		return BookingsReport{}, err
	}

	if err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(ROUND(SUM(p.amount) * 100)::bigint, 0) FROM payments p
		 JOIN bookings b ON b.id = p.booking_id
		 WHERE p.status IN ('SUCCEEDED','PARTIALLY_REFUNDED','REFUNDED')
		   AND b.created_at >= $1 AND b.created_at < $2`, from, to).Scan(&rep.GMVMinor); err != nil {
		return BookingsReport{}, fmt.Errorf("reporting: gmv: %w", err)
	}
	if err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(ROUND(SUM(r.amount) * 100)::bigint, 0) FROM refunds r
		 JOIN payments p ON p.id = r.payment_id
		 JOIN bookings b ON b.id = p.booking_id
		 WHERE b.created_at >= $1 AND b.created_at < $2`, from, to).Scan(&rep.RefundedMinor); err != nil {
		return BookingsReport{}, fmt.Errorf("reporting: refunds: %w", err)
	}
	return rep, nil
}

// BookingsExport returns the raw rows for the permitted CSV export
// (reports.export, P8-02).
func (r *Repository) BookingsExport(ctx context.Context, from, to time.Time) ([]BookingExportRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT b.reference, u.email, gp.public_name, tp.name, b.starts_at, b.status,
		        (SELECT ROUND(p.amount * 100)::bigint FROM payments p
		           WHERE p.booking_id = b.id ORDER BY p.created_at DESC LIMIT 1),
		        (SELECT p.currency FROM payments p
		           WHERE p.booking_id = b.id ORDER BY p.created_at DESC LIMIT 1)
		 FROM bookings b
		 JOIN users u ON u.id = b.tourist_id
		 LEFT JOIN guide_profiles gp ON gp.user_id = b.guide_id
		 JOIN tour_packages tp ON tp.id = b.package_id
		 WHERE b.created_at >= $1 AND b.created_at < $2
		 ORDER BY b.created_at DESC`, from, to)
	if err != nil {
		return nil, fmt.Errorf("reporting: export rows: %w", err)
	}
	defer rows.Close()
	var out []BookingExportRow
	for rows.Next() {
		var row BookingExportRow
		if err := rows.Scan(&row.Reference, &row.TouristEmail, &row.GuideName,
			&row.PackageTitle, &row.StartsAt, &row.Status, &row.AmountMinor, &row.Currency); err != nil {
			return nil, fmt.Errorf("reporting: scan export: %w", err)
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// ListAudit returns the filtered audit trail, newest first (P8-04).
func (r *Repository) ListAudit(ctx context.Context, f AuditFilter, limit, offset int) ([]AuditEntry, int, error) {
	where := "WHERE TRUE"
	args := []any{}
	add := func(clause string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(" AND %s$%d", clause, len(args))
	}
	if f.ActorID != "" {
		add("a.actor_id = ", f.ActorID)
	}
	if f.Action != "" {
		add("a.action LIKE ", f.Action+"%")
	}
	if f.EntityType != "" {
		add("a.entity_type = ", f.EntityType)
	}
	if f.EntityID != "" {
		add("a.entity_id = ", f.EntityID)
	}
	if f.From != nil {
		add("a.created_at >= ", *f.From)
	}
	if f.To != nil {
		add("a.created_at < ", *f.To)
	}

	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*)::int FROM audit_logs a `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("reporting: audit count: %w", err)
	}

	args = append(args, limit, offset)
	rows, err := r.pool.Query(ctx,
		`SELECT a.id, a.actor_id, u.email, a.action, a.entity_type, a.entity_id,
		        a.before_json, a.after_json, a.created_at
		 FROM audit_logs a LEFT JOIN users u ON u.id = a.actor_id `+where+
			fmt.Sprintf(" ORDER BY a.created_at DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("reporting: audit list: %w", err)
	}
	defer rows.Close()

	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.ActorID, &e.ActorEmail, &e.Action, &e.EntityType,
			&e.EntityID, &e.Before, &e.After, &e.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("reporting: scan audit: %w", err)
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}
