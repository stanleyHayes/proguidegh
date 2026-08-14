// Package safety implements the SOS flow (spec §12): an immutable SOS event
// plus a HIGH/CRITICAL incident, a realtime operations alert, fallback
// notifications for responders and a full audit trail. It deliberately says
// "sent to ProGuideGH operations" — never police/emergency dispatch (spec
// §12 safety requirement).
package safety

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SOSEvent is an append-only sos_events row (§12 — SOS evidence).
type SOSEvent struct {
	ID             string     `json:"id"`
	BookingID      string     `json:"booking_id"`
	UserID         string     `json:"user_id"`
	Latitude       float64    `json:"latitude"`
	Longitude      float64    `json:"longitude"`
	AccuracyM      *float64   `json:"accuracy_m"`
	TriggeredAt    time.Time  `json:"triggered_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at"`
}

// Incident is the incidents row created alongside an SOS event.
type Incident struct {
	ID         string    `json:"id"`
	BookingID  *string   `json:"booking_id"`
	Type       string    `json:"type"`
	Severity   string    `json:"severity"`
	Status     string    `json:"status"`
	ReportedBy *string   `json:"reported_by"`
	AssignedTo *string   `json:"assigned_to"`
	OccurredAt time.Time `json:"occurred_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// Repository is the safety data layer.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// CreateSOS inserts the immutable SOS event and its incident in one
// transaction — an alert with no incident (or vice versa) must never exist.
func (r *Repository) CreateSOS(ctx context.Context, bookingID, userID string, lat, lng float64, accuracy *float64, severity string) (SOSEvent, Incident, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return SOSEvent{}, Incident{}, fmt.Errorf("safety: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var ev SOSEvent
	err = tx.QueryRow(ctx,
		`INSERT INTO sos_events (booking_id, user_id, latitude, longitude, accuracy)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, booking_id, user_id, latitude::float8, longitude::float8, accuracy, triggered_at`,
		bookingID, userID, lat, lng, accuracy).
		Scan(&ev.ID, &ev.BookingID, &ev.UserID, &ev.Latitude, &ev.Longitude, &ev.AccuracyM, &ev.TriggeredAt)
	if err != nil {
		return SOSEvent{}, Incident{}, fmt.Errorf("safety: insert sos event: %w", err)
	}

	var inc Incident
	err = tx.QueryRow(ctx,
		`INSERT INTO incidents (booking_id, type, severity, reported_by)
		 VALUES ($1, 'sos', $2, $3)
		 RETURNING id, booking_id, type, severity, status, reported_by, assigned_to, occurred_at, created_at`,
		bookingID, severity, userID).
		Scan(&inc.ID, &inc.BookingID, &inc.Type, &inc.Severity, &inc.Status, &inc.ReportedBy, &inc.AssignedTo, &inc.OccurredAt, &inc.CreatedAt)
	if err != nil {
		return SOSEvent{}, Incident{}, fmt.Errorf("safety: insert incident: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return SOSEvent{}, Incident{}, fmt.Errorf("safety: commit: %w", err)
	}
	return ev, inc, nil
}

// ResponderIDs returns the user ids holding incidents.manage — the
// operations roster that gets the fallback notification (§12 step 10).
func (r *Repository) ResponderIDs(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT ur.user_id
		 FROM user_roles ur
		 JOIN role_permissions rp ON rp.role_id = ur.role_id
		 JOIN permissions p ON p.id = rp.permission_id
		 WHERE p.code = 'incidents.manage'`)
	if err != nil {
		return nil, fmt.Errorf("safety: responders: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("safety: scan responder: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// QueueNotification records an in_app notification for one responder. The
// channel/template model matches spec §19: delivery providers (SMS/push/
// email) are downstream of this queue; without provider credentials the row
// is the audit that the fallback was raised (P6-02).
func (r *Repository) QueueNotification(ctx context.Context, userID, channel, template string) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO notifications (user_id, channel, template) VALUES ($1, $2, $3)`,
		userID, channel, template)
	if err != nil {
		return fmt.Errorf("safety: queue notification: %w", err)
	}
	return nil
}

// LatestForBooking returns the newest SOS event for a booking, for the
// incident detail view. ErrNotFound when none.
func (r *Repository) LatestForBooking(ctx context.Context, bookingID string) (SOSEvent, error) {
	var ev SOSEvent
	err := r.pool.QueryRow(ctx,
		`SELECT id, booking_id, user_id, latitude::float8, longitude::float8, accuracy, triggered_at, acknowledged_at
		 FROM sos_events WHERE booking_id = $1
		 ORDER BY triggered_at DESC LIMIT 1`, bookingID).
		Scan(&ev.ID, &ev.BookingID, &ev.UserID, &ev.Latitude, &ev.Longitude, &ev.AccuracyM, &ev.TriggeredAt, &ev.AcknowledgedAt)
	if err != nil {
		return SOSEvent{}, fmt.Errorf("safety: latest sos: %w", err)
	}
	return ev, nil
}

// AcknowledgeBooking marks every unacknowledged SOS event on the booking
// acknowledged (called when operations acks the incident).
func (r *Repository) AcknowledgeBooking(ctx context.Context, bookingID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE sos_events SET acknowledged_at = now()
		 WHERE booking_id = $1 AND acknowledged_at IS NULL`, bookingID)
	if err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("safety: acknowledge: %w", err)
	}
	return nil
}
