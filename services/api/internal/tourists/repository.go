// Package tourists implements the tourist profile slice (spec §13.2):
// GET/PATCH /api/v1/me/tourist-profile.
package tourists

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Profile is a tourist_profiles row.
type Profile struct {
	UserID                    string    `json:"user_id"`
	FullName                  string    `json:"full_name"`
	Nationality               *string   `json:"nationality"`
	PreferredLanguage         *string   `json:"preferred_language"`
	EmergencyContactName      *string   `json:"emergency_contact_name"`
	EmergencyContactPhoneE164 *string   `json:"emergency_contact_phone_e164"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

// ErrNotFound is returned when the user has no tourist profile.
var ErrNotFound = errors.New("tourists: profile not found")

// Repository owns tourist profile persistence (explicit SQL, spec §7.2).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// Get returns the caller's tourist profile.
func (r *Repository) Get(ctx context.Context, userID string) (Profile, error) {
	var p Profile
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, full_name, nationality, preferred_language,
		       emergency_contact_name, emergency_contact_phone_e164, created_at, updated_at
		FROM tourist_profiles WHERE user_id = $1`, userID).
		Scan(&p.UserID, &p.FullName, &p.Nationality, &p.PreferredLanguage,
			&p.EmergencyContactName, &p.EmergencyContactPhoneE164, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("tourists: get profile: %w", err)
	}
	return p, nil
}

// PatchInput carries the optional fields of a profile update; nil means
// "leave unchanged".
type PatchInput struct {
	FullName                  *string
	Nationality               *string
	PreferredLanguage         *string
	EmergencyContactName      *string
	EmergencyContactPhoneE164 *string
}

// Patch applies a partial update to the caller's profile. Only non-nil
// fields are changed.
func (r *Repository) Patch(ctx context.Context, userID string, in PatchInput) (Profile, error) {
	var out Profile
	err := r.pool.QueryRow(ctx, `
		UPDATE tourist_profiles SET
			full_name                    = COALESCE($2, full_name),
			nationality                  = COALESCE($3, nationality),
			preferred_language           = COALESCE($4, preferred_language),
			emergency_contact_name       = COALESCE($5, emergency_contact_name),
			emergency_contact_phone_e164 = COALESCE($6, emergency_contact_phone_e164),
			updated_at                   = now()
		WHERE user_id = $1
		RETURNING user_id, full_name, nationality, preferred_language,
		          emergency_contact_name, emergency_contact_phone_e164, created_at, updated_at`,
		userID, in.FullName, in.Nationality, in.PreferredLanguage,
		in.EmergencyContactName, in.EmergencyContactPhoneE164).
		Scan(&out.UserID, &out.FullName, &out.Nationality, &out.PreferredLanguage,
			&out.EmergencyContactName, &out.EmergencyContactPhoneE164, &out.CreatedAt, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("tourists: patch profile: %w", err)
	}
	return out, nil
}
