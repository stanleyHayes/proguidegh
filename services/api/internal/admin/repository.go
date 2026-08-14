// Package admin implements the Phase 1 admin slice (spec §13.6): user
// listing, role management (audited) and the guide queue. All endpoints
// sit behind explicit permission checks (spec §3 RBAC rule).
package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// User is one row of the admin user list.
type User struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	PhoneE164   *string    `json:"phone_e164"`
	Status      string     `json:"status"`
	Roles       []string   `json:"roles"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Guide is one row of the admin guide queue.
type Guide struct {
	UserID      string    `json:"user_id"`
	Email       string    `json:"email"`
	PublicName  string    `json:"public_name"`
	Status      string    `json:"status"`
	RatingAvg   string    `json:"rating_avg"`
	RatingCount int       `json:"rating_count"`
	EliteStatus bool      `json:"elite_status"`
	CreatedAt   time.Time `json:"created_at"`
}

// BookingRef is the guide/tourist identity embedded in an operations
// bookings row (nil guide when the booking is still unassigned).
type BookingRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AdminBooking is one row of the operations bookings board
// (GET /api/v1/admin/bookings).
type AdminBooking struct {
	ID          string      `json:"id"`
	Reference   string      `json:"reference"`
	Status      string      `json:"status"`
	Guide       *BookingRef `json:"guide"`
	Tourist     BookingRef  `json:"tourist"`
	PackageName string      `json:"package_name"`
	StartsAt    time.Time   `json:"starts_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
	LastEventAt time.Time   `json:"last_event_at"`
}

// ErrUserNotFound is returned when the target user does not exist.
var ErrUserNotFound = errors.New("admin: user not found")

// ErrUnknownRole is returned when a role code does not exist.
var ErrUnknownRole = errors.New("admin: unknown role code")

var (
	ErrInvitationExists  = errors.New("admin: pending invitation already exists")
	ErrInvitationInvalid = errors.New("admin: invitation is invalid or expired")
	ErrEmailInUse        = errors.New("admin: email already belongs to an account")
)

type Invitation struct {
	ID         string     `json:"id"`
	Email      string     `json:"email"`
	Roles      []string   `json:"roles"`
	InvitedBy  *string    `json:"invited_by"`
	ExpiresAt  time.Time  `json:"expires_at"`
	AcceptedAt *time.Time `json:"accepted_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Repository owns admin persistence (explicit SQL, spec §7.2).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) CreateInvitation(ctx context.Context, email string, roles []string, tokenHash, invitedBy string, expiresAt time.Time) (Invitation, error) {
	var existing bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE lower(email) = lower($1))`, email).Scan(&existing); err != nil {
		return Invitation{}, fmt.Errorf("admin: check invitation email: %w", err)
	}
	if existing {
		return Invitation{}, ErrEmailInUse
	}
	var unknown int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM unnest($1::text[]) c WHERE c NOT IN (SELECT code FROM roles)`, roles).Scan(&unknown); err != nil {
		return Invitation{}, fmt.Errorf("admin: validate invitation roles: %w", err)
	}
	if unknown > 0 {
		return Invitation{}, ErrUnknownRole
	}
	var inv Invitation
	err := r.pool.QueryRow(ctx, `INSERT INTO admin_invitations (email, roles, token_hash, invited_by, expires_at) VALUES (lower($1), $2, $3, $4, $5) RETURNING id, email, roles, invited_by, expires_at, accepted_at, revoked_at, created_at`, email, roles, tokenHash, invitedBy, expiresAt).Scan(&inv.ID, &inv.Email, &inv.Roles, &inv.InvitedBy, &inv.ExpiresAt, &inv.AcceptedAt, &inv.RevokedAt, &inv.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "idx_admin_invitations_one_pending" {
			return Invitation{}, ErrInvitationExists
		}
		return Invitation{}, fmt.Errorf("admin: create invitation: %w", err)
	}
	return inv, nil
}

func (r *Repository) DeleteInvitation(ctx context.Context, invitationID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM admin_invitations WHERE id = $1 AND accepted_at IS NULL`, invitationID)
	if err != nil {
		return fmt.Errorf("admin: delete invitation: %w", err)
	}
	return nil
}

func (r *Repository) ListInvitations(ctx context.Context) ([]Invitation, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, email, roles, invited_by, expires_at, accepted_at, revoked_at, created_at FROM admin_invitations ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, fmt.Errorf("admin: list invitations: %w", err)
	}
	defer rows.Close()
	out := []Invitation{}
	for rows.Next() {
		var inv Invitation
		if err := rows.Scan(&inv.ID, &inv.Email, &inv.Roles, &inv.InvitedBy, &inv.ExpiresAt, &inv.AcceptedAt, &inv.RevokedAt, &inv.CreatedAt); err != nil {
			return nil, fmt.Errorf("admin: scan invitation: %w", err)
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

func (r *Repository) AcceptInvitation(ctx context.Context, tokenHash, passwordHash string) (User, []string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, nil, fmt.Errorf("admin: begin invitation acceptance: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	var inv Invitation
	err = tx.QueryRow(ctx, `SELECT id, email, roles, invited_by, expires_at, accepted_at, revoked_at, created_at FROM admin_invitations WHERE token_hash = $1 FOR UPDATE`, tokenHash).Scan(&inv.ID, &inv.Email, &inv.Roles, &inv.InvitedBy, &inv.ExpiresAt, &inv.AcceptedAt, &inv.RevokedAt, &inv.CreatedAt)
	if err != nil || inv.AcceptedAt != nil || inv.RevokedAt != nil || !inv.ExpiresAt.After(time.Now()) {
		return User{}, nil, ErrInvitationInvalid
	}
	var u User
	err = tx.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ($1, $2) RETURNING id, email, phone_e164, status, last_login_at, created_at`, inv.Email, passwordHash).Scan(&u.ID, &u.Email, &u.PhoneE164, &u.Status, &u.LastLoginAt, &u.CreatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.ConstraintName == "users_email_key" {
			return User{}, nil, ErrEmailInUse
		}
		return User{}, nil, fmt.Errorf("admin: create invited user: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE code = ANY($2)`, u.ID, inv.Roles); err != nil {
		return User{}, nil, fmt.Errorf("admin: assign invited roles: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE admin_invitations SET accepted_at = now() WHERE id = $1`, inv.ID); err != nil {
		return User{}, nil, fmt.Errorf("admin: consume invitation: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, nil, fmt.Errorf("admin: commit invitation acceptance: %w", err)
	}
	u.Roles = inv.Roles
	return u, inv.Roles, nil
}

// ListUsers returns one page of users with their roles (offset pagination —
// low-volume admin list, spec §14).
func (r *Repository) ListUsers(ctx context.Context, limit, offset int) ([]User, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("admin: count users: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT u.id, u.email, u.phone_e164, u.status, u.last_login_at, u.created_at,
		       COALESCE(array_agg(r.code ORDER BY r.code) FILTER (WHERE r.code IS NOT NULL), '{}') AS roles
		FROM users u
		LEFT JOIN user_roles ur ON ur.user_id = u.id
		LEFT JOIN roles r ON r.id = ur.role_id
		GROUP BY u.id
		ORDER BY u.created_at DESC
		LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("admin: list users: %w", err)
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Email, &u.PhoneE164, &u.Status, &u.LastLoginAt, &u.CreatedAt, &u.Roles); err != nil {
			return nil, 0, fmt.Errorf("admin: scan user: %w", err)
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

// GetRoles returns the role codes for a user (audit "before" snapshot).
func (r *Repository) GetRoles(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT r.code FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1
		ORDER BY r.code`, userID)
	if err != nil {
		return nil, fmt.Errorf("admin: get roles: %w", err)
	}
	defer rows.Close()
	roles := []string{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("admin: scan role: %w", err)
		}
		roles = append(roles, c)
	}
	return roles, rows.Err()
}

// SetRoles replaces the user's role set atomically and returns the new set.
// Returns ErrUserNotFound / ErrUnknownRole on bad input.
func (r *Repository) SetRoles(ctx context.Context, userID string, roleCodes []string) ([]string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("admin: begin set roles: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, userID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("admin: check user: %w", err)
	}
	if !exists {
		return nil, ErrUserNotFound
	}
	var unknown int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM unnest($1::text[]) AS c
		WHERE c NOT IN (SELECT code FROM roles)`, roleCodes).Scan(&unknown); err != nil {
		return nil, fmt.Errorf("admin: check roles: %w", err)
	}
	if unknown > 0 {
		return nil, ErrUnknownRole
	}

	if _, err := tx.Exec(ctx, `DELETE FROM user_roles WHERE user_id = $1`, userID); err != nil {
		return nil, fmt.Errorf("admin: clear roles: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE code = ANY($2)
		ON CONFLICT DO NOTHING`, userID, roleCodes); err != nil {
		return nil, fmt.Errorf("admin: assign roles: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("admin: commit set roles: %w", err)
	}
	return r.GetRoles(ctx, userID)
}

// ListGuides returns one page of guide profiles with the owner's email.
func (r *Repository) ListGuides(ctx context.Context, status string, limit, offset int) ([]Guide, int, error) {
	where := ""
	countWhere := ""
	args := []any{limit, offset}
	countArgs := []any{}
	if status != "" {
		where = "WHERE gp.status = $3"
		countWhere = "WHERE gp.status = $1"
		args = append(args, status)
		countArgs = append(countArgs, status)
	}

	countQ := `SELECT count(*) FROM guide_profiles gp ` + countWhere
	var total int
	if err := r.pool.QueryRow(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("admin: count guides: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT gp.user_id, u.email, gp.public_name, gp.status, gp.rating_avg::text,
		       gp.rating_count, gp.elite_status, gp.created_at
		FROM guide_profiles gp
		JOIN users u ON u.id = gp.user_id
		`+where+`
		ORDER BY gp.created_at DESC
		LIMIT $1 OFFSET $2`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("admin: list guides: %w", err)
	}
	defer rows.Close()

	guides := []Guide{}
	for rows.Next() {
		var g Guide
		if err := rows.Scan(&g.UserID, &g.Email, &g.PublicName, &g.Status, &g.RatingAvg,
			&g.RatingCount, &g.EliteStatus, &g.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("admin: scan guide: %w", err)
		}
		guides = append(guides, g)
	}
	return guides, total, rows.Err()
}

// ListBookings returns one page of bookings for the operations board
// (offset pagination like the other admin lists), most-recently-updated
// first. An empty statuses slice means no status filter. last_event_at is
// the latest immutable status-event time (bookings always have their opening
// events, so it is never zero).
func (r *Repository) ListBookings(ctx context.Context, statuses []string, limit, offset int) ([]AdminBooking, int, error) {
	where := ""
	countArgs := []any{}
	args := []any{limit, offset}
	if len(statuses) > 0 {
		where = `WHERE b.status = ANY($3::text[])`
		args = append(args, statuses)
		countArgs = append(countArgs, statuses)
	}

	countQ := `SELECT count(*) FROM bookings b`
	if len(statuses) > 0 {
		countQ += ` WHERE b.status = ANY($1::text[])`
	}
	var total int
	if err := r.pool.QueryRow(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("admin: count bookings: %w", err)
	}

	rows, err := r.pool.Query(ctx, `
		SELECT b.id, b.reference, b.status,
		       b.guide_id, gp.public_name,
		       b.tourist_id, COALESCE(tp.full_name, ''),
		       p.name, b.starts_at, b.updated_at,
		       COALESCE((SELECT max(e.created_at) FROM booking_status_events e
		                 WHERE e.booking_id = b.id), b.updated_at)
		FROM bookings b
		JOIN tour_packages p ON p.id = b.package_id
		LEFT JOIN guide_profiles gp ON gp.user_id = b.guide_id
		LEFT JOIN tourist_profiles tp ON tp.user_id = b.tourist_id
		`+where+`
		ORDER BY b.updated_at DESC, b.id
		LIMIT $1 OFFSET $2`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("admin: list bookings: %w", err)
	}
	defer rows.Close()

	bookings := []AdminBooking{}
	for rows.Next() {
		var b AdminBooking
		var guideID, guideName *string
		if err := rows.Scan(&b.ID, &b.Reference, &b.Status,
			&guideID, &guideName,
			&b.Tourist.ID, &b.Tourist.Name,
			&b.PackageName, &b.StartsAt, &b.UpdatedAt, &b.LastEventAt); err != nil {
			return nil, 0, fmt.Errorf("admin: scan booking: %w", err)
		}
		if guideID != nil {
			name := ""
			if guideName != nil {
				name = *guideName
			}
			b.Guide = &BookingRef{ID: *guideID, Name: name}
		}
		bookings = append(bookings, b)
	}
	return bookings, total, rows.Err()
}

// RevokeSessionsForUser revokes every active refresh session for a user
// (spec §15.2: suspend/revoke sessions on role removal).
func (r *Repository) RevokeSessionsForUser(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE refresh_sessions SET revoked_at = now()
		WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	if err != nil {
		return fmt.Errorf("admin: revoke sessions: %w", err)
	}
	return nil
}
