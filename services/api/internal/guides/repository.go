// Package guides implements the guide slices (spec §13.4): application,
// document registration, the self-service dashboard/profile endpoints and
// the §10.2-gated public guide detail.
package guides

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Profile is a guide_profiles row. Latitude/Longitude are the guide's
// operating base for radius search (§10.1); they appear only on own-record
// responses, never on public ones.
type Profile struct {
	UserID      string    `json:"user_id"`
	PublicName  string    `json:"public_name"`
	Bio         *string   `json:"bio"`
	Status      string    `json:"status"`
	RatingAvg   string    `json:"rating_avg"`
	RatingCount int       `json:"rating_count"`
	EliteStatus bool      `json:"elite_status"`
	RegionID    *string   `json:"region_id"`
	Latitude    *string   `json:"latitude"`
	Longitude   *string   `json:"longitude"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Document is a guide_documents row (metadata only; the bytes live in
// private object storage behind signed URLs).
type Document struct {
	ID        string     `json:"id"`
	GuideID   string     `json:"guide_id"`
	Type      string     `json:"type"`
	ObjectKey string     `json:"object_key"`
	Status    string     `json:"status"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedAt time.Time  `json:"created_at"`
}

// ErrNotFound is returned when the caller has no guide profile.
var ErrNotFound = errors.New("guides: profile not found")

// Repository owns guide persistence (explicit SQL, spec §7.2).
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const profileColumns = `user_id, public_name, bio, status, rating_avg::text, rating_count, elite_status, region_id, latitude::text, longitude::text, created_at, updated_at`

// Apply creates the guide profile shell, idempotent per user: an existing
// profile is returned unchanged so retried applications never duplicate.
func (r *Repository) Apply(ctx context.Context, userID, publicName string, bio, regionID *string) (Profile, error) {
	p, err := r.insertShell(ctx, userID, publicName, bio, regionID)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, err
	}
	// Conflict: profile already exists — return it as-is (idempotent).
	return r.GetByUser(ctx, userID)
}

func (r *Repository) insertShell(ctx context.Context, userID, publicName string, bio, regionID *string) (Profile, error) {
	var p Profile
	err := r.pool.QueryRow(ctx, `
		INSERT INTO guide_profiles (user_id, public_name, bio, region_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id) DO NOTHING
		RETURNING `+profileColumns, userID, publicName, bio, regionID).
		Scan(&p.UserID, &p.PublicName, &p.Bio, &p.Status, &p.RatingAvg, &p.RatingCount,
			&p.EliteStatus, &p.RegionID, &p.Latitude, &p.Longitude, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return Profile{}, fmt.Errorf("guides: apply: %w", err)
	}
	return p, nil
}

// GetByUser returns the guide profile for a user.
func (r *Repository) GetByUser(ctx context.Context, userID string) (Profile, error) {
	var p Profile
	err := r.pool.QueryRow(ctx, `SELECT `+profileColumns+`
		FROM guide_profiles WHERE user_id = $1`, userID).
		Scan(&p.UserID, &p.PublicName, &p.Bio, &p.Status, &p.RatingAvg, &p.RatingCount,
			&p.EliteStatus, &p.RegionID, &p.Latitude, &p.Longitude, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("guides: get profile: %w", err)
	}
	return p, nil
}

// AddRoleIfMissing assigns a role to a user (no-op when already assigned).
func (r *Repository) AddRoleIfMissing(ctx context.Context, userID, roleCode string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_id)
		SELECT $1, id FROM roles WHERE code = $2
		ON CONFLICT DO NOTHING`, userID, roleCode)
	if err != nil {
		return fmt.Errorf("guides: add role %s: %w", roleCode, err)
	}
	return nil
}

// CreateDocument registers document metadata against the guide profile.
func (r *Repository) CreateDocument(ctx context.Context, guideID, docType, objectKey string, expiresAt *time.Time) (Document, error) {
	var d Document
	err := r.pool.QueryRow(ctx, `
		INSERT INTO guide_documents (guide_id, type, object_key, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id, guide_id, type, object_key, status, expires_at, created_at`,
		guideID, docType, objectKey, expiresAt).
		Scan(&d.ID, &d.GuideID, &d.Type, &d.ObjectKey, &d.Status, &d.ExpiresAt, &d.CreatedAt)
	if err != nil {
		return Document{}, fmt.Errorf("guides: create document: %w", err)
	}
	return d, nil
}

// Language is one guide_languages row.
type Language struct {
	Code        string `json:"code"`
	Proficiency string `json:"proficiency"`
}

// SpecialtyRef is one specialty attached to a guide, resolved to code/name.
type SpecialtyRef struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// PublicView is the raw material for the §10.2 visibility gate and the
// public guide detail response.
type PublicView struct {
	UserID      string  `json:"user_id"`
	PublicName  string  `json:"public_name"`
	Bio         *string `json:"bio"`
	GuideStatus string  `json:"-"`
	UserStatus  string  `json:"-"`
	CaseStatus  *string `json:"-"`
	RatingAvg   string  `json:"rating_avg"`
	RatingCount int     `json:"rating_count"`
	EliteStatus bool    `json:"elite_status"`
	RegionID    *string `json:"region_id"`
	RegionName  *string `json:"region_name"`
}

// ProfilePatch is the validated field set for PATCH /me/guide/profile.
// Nil slices/pointers mean "leave unchanged". Latitude/Longitude move
// together (the handler validates pairing); both nil clears neither.
type ProfilePatch struct {
	PublicName   *string
	Bio          *string
	RegionID     *string
	Latitude     *string
	Longitude    *string
	Languages    []Language
	SpecialtyIDs []string
}

// Validation error sentinels (mapped to 400 by the handler).
var (
	ErrUnknownRegion      = errors.New("guides: unknown region")
	ErrUnknownLanguage    = errors.New("guides: unknown language code")
	ErrUnknownSpecialty   = errors.New("guides: unknown specialty")
	ErrInvalidProficiency = errors.New("guides: invalid proficiency")
)

// GetPublicView loads the guide with the signals the §10.2 gate needs:
// account status, guide profile status and the latest case status.
func (r *Repository) GetPublicView(ctx context.Context, guideID string) (PublicView, error) {
	var v PublicView
	err := r.pool.QueryRow(ctx, `
		SELECT gp.user_id, gp.public_name, gp.bio, gp.status, u.status,
		       (SELECT cc.status FROM certification_cases cc
		        WHERE cc.guide_id = gp.user_id
		        ORDER BY cc.opened_at DESC LIMIT 1),
		       gp.rating_avg::text, gp.rating_count, gp.elite_status,
		       gp.region_id, rg.name
		FROM guide_profiles gp
		JOIN users u ON u.id = gp.user_id
		LEFT JOIN regions rg ON rg.id = gp.region_id
		WHERE gp.user_id = $1`, guideID).
		Scan(&v.UserID, &v.PublicName, &v.Bio, &v.GuideStatus, &v.UserStatus, &v.CaseStatus,
			&v.RatingAvg, &v.RatingCount, &v.EliteStatus, &v.RegionID, &v.RegionName)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicView{}, ErrNotFound
	}
	if err != nil {
		return PublicView{}, fmt.Errorf("guides: public view: %w", err)
	}
	return v, nil
}

// ListLanguages returns the guide's language skills.
func (r *Repository) ListLanguages(ctx context.Context, guideID string) ([]Language, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT language_code, proficiency FROM guide_languages
		WHERE guide_id = $1 ORDER BY language_code`, guideID)
	if err != nil {
		return nil, fmt.Errorf("guides: list languages: %w", err)
	}
	defer rows.Close()

	out := []Language{}
	for rows.Next() {
		var l Language
		if err := rows.Scan(&l.Code, &l.Proficiency); err != nil {
			return nil, fmt.Errorf("guides: scan language: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ListSpecialties returns the guide's specialties resolved to code/name.
func (r *Repository) ListSpecialties(ctx context.Context, guideID string) ([]SpecialtyRef, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT s.id, s.code, s.name
		FROM guide_specialties gs
		JOIN specialties s ON s.id = gs.specialty_id
		WHERE gs.guide_id = $1
		ORDER BY s.name`, guideID)
	if err != nil {
		return nil, fmt.Errorf("guides: list specialties: %w", err)
	}
	defer rows.Close()

	out := []SpecialtyRef{}
	for rows.Next() {
		var s SpecialtyRef
		if err := rows.Scan(&s.ID, &s.Code, &s.Name); err != nil {
			return nil, fmt.Errorf("guides: scan specialty: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// validProficiencies mirrors the guide_languages CHECK constraint.
var validProficiencies = map[string]bool{
	"basic": true, "conversational": true, "fluent": true, "native": true,
}

// UpdateProfile applies a profile patch atomically: scalar fields are
// updated in place; when languages or specialty_ids are supplied, the
// guide_languages / guide_specialties rows are replaced wholesale in the
// same transaction (spec §4.2).
func (r *Repository) UpdateProfile(ctx context.Context, userID string, p ProfilePatch) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("guides: begin update profile: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM guide_profiles WHERE user_id = $1)`, userID).Scan(&exists); err != nil {
		return fmt.Errorf("guides: check profile: %w", err)
	}
	if !exists {
		return ErrNotFound
	}

	if p.PublicName != nil || p.Bio != nil || p.RegionID != nil || p.Latitude != nil {
		if p.RegionID != nil {
			var regionOK bool
			if err := tx.QueryRow(ctx,
				`SELECT EXISTS(SELECT 1 FROM regions WHERE id = $1)`, *p.RegionID).Scan(&regionOK); err != nil {
				return fmt.Errorf("guides: check region: %w", err)
			}
			if !regionOK {
				return ErrUnknownRegion
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE guide_profiles SET
			    public_name = COALESCE($2, public_name),
			    bio         = COALESCE($3, bio),
			    region_id   = COALESCE($4, region_id),
			    latitude    = COALESCE($5, latitude),
			    longitude   = COALESCE($6, longitude),
			    updated_at  = now()
			WHERE user_id = $1`, userID, p.PublicName, p.Bio, p.RegionID, p.Latitude, p.Longitude); err != nil {
			return fmt.Errorf("guides: update profile: %w", err)
		}
	}

	if p.Languages != nil {
		for _, l := range p.Languages {
			if !validProficiencies[l.Proficiency] {
				return ErrInvalidProficiency
			}
		}
		var unknown int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM unnest($1::text[]) AS c
			WHERE c NOT IN (SELECT code FROM languages)`, languageCodes(p.Languages)).Scan(&unknown); err != nil {
			return fmt.Errorf("guides: check languages: %w", err)
		}
		if unknown > 0 {
			return ErrUnknownLanguage
		}
		if _, err := tx.Exec(ctx, `DELETE FROM guide_languages WHERE guide_id = $1`, userID); err != nil {
			return fmt.Errorf("guides: clear languages: %w", err)
		}
		for _, l := range p.Languages {
			if _, err := tx.Exec(ctx, `
				INSERT INTO guide_languages (guide_id, language_code, proficiency)
				VALUES ($1, $2, $3)
				ON CONFLICT (guide_id, language_code) DO UPDATE SET proficiency = EXCLUDED.proficiency`,
				userID, l.Code, l.Proficiency); err != nil {
				return fmt.Errorf("guides: insert language: %w", err)
			}
		}
	}

	if p.SpecialtyIDs != nil {
		var unknown int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM unnest($1::uuid[]) AS s
			WHERE s NOT IN (SELECT id FROM specialties)`, p.SpecialtyIDs).Scan(&unknown); err != nil {
			return fmt.Errorf("guides: check specialties: %w", err)
		}
		if unknown > 0 {
			return ErrUnknownSpecialty
		}
		if _, err := tx.Exec(ctx, `DELETE FROM guide_specialties WHERE guide_id = $1`, userID); err != nil {
			return fmt.Errorf("guides: clear specialties: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO guide_specialties (guide_id, specialty_id)
			SELECT $1, s FROM unnest($2::uuid[]) AS s
			ON CONFLICT DO NOTHING`, userID, p.SpecialtyIDs); err != nil {
			return fmt.Errorf("guides: insert specialties: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("guides: commit update profile: %w", err)
	}
	return nil
}

func languageCodes(ls []Language) []string {
	out := make([]string, 0, len(ls))
	for _, l := range ls {
		out = append(out, l.Code)
	}
	return out
}
