// Package catalog implements the public catalog endpoints (spec §13.2):
// regions, specialties and active tour packages with their current
// effective-dated prices (spec §27).
package catalog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Region is a regions reference row (Ghana's 16 regions).
type Region struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// Specialty is a specialties reference row (spec Appendix C).
type Specialty struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// Package is an active tour_packages row with its current effective price.
type Package struct {
	ID              string `json:"id"`
	Code            string `json:"code"`
	Name            string `json:"name"`
	DurationMinutes int    `json:"duration_minutes"`
	Currency        string `json:"currency"`
	Active          bool   `json:"active"`
	// Price is the server-authoritative current price (spec §14), selected
	// from pricing_rules by EffectivePrice; base_price is the fallback.
	Price string `json:"price"`
}

// ErrNotFound is returned when a catalog row (package, ...) does not exist.
var ErrNotFound = errors.New("catalog: not found")

// PricingRule is a pricing_rules row: an effective-dated price, optionally
// scoped to one region (region_id NULL = all regions).
type PricingRule struct {
	PackageID     string
	RegionID      *string
	Amount        string
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
}

// EffectivePrice selects the rule in force at the given time: the rule with
// the latest effective_from not after `at`, still in force (effective_to NULL
// or after `at`), preferring a region-specific rule over the global one.
// Returns false when no rule applies.
func EffectivePrice(rules []PricingRule, regionID *string, at time.Time) (PricingRule, bool) {
	var best *PricingRule
	for i := range rules {
		r := &rules[i]
		if r.EffectiveFrom.After(at) {
			continue // not yet in force
		}
		if r.EffectiveTo != nil && !r.EffectiveTo.After(at) {
			continue // expired
		}
		if r.RegionID != nil && (regionID == nil || *r.RegionID != *regionID) {
			continue // scoped to another region
		}
		if best == nil {
			best = r
			continue
		}
		// Region-specific beats global at equal or later effective_from.
		switch {
		case r.RegionID != nil && best.RegionID == nil && !r.EffectiveFrom.Before(best.EffectiveFrom):
			best = r
		case r.RegionID == nil && best.RegionID != nil && r.EffectiveFrom.After(best.EffectiveFrom):
			best = r
		case (r.RegionID == nil) == (best.RegionID == nil) && r.EffectiveFrom.After(best.EffectiveFrom):
			best = r
		}
	}
	if best == nil {
		return PricingRule{}, false
	}
	return *best, true
}

// Repository owns catalog persistence (explicit SQL, spec §7.2).
type Repository struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, now: time.Now}
}

// ListRegions returns all regions ordered by name.
func (r *Repository) ListRegions(ctx context.Context) ([]Region, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, code, name FROM regions ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("catalog: list regions: %w", err)
	}
	defer rows.Close()

	out := []Region{}
	for rows.Next() {
		var reg Region
		if err := rows.Scan(&reg.ID, &reg.Code, &reg.Name); err != nil {
			return nil, fmt.Errorf("catalog: scan region: %w", err)
		}
		out = append(out, reg)
	}
	return out, rows.Err()
}

// ListSpecialties returns all specialties ordered by name.
func (r *Repository) ListSpecialties(ctx context.Context) ([]Specialty, error) {
	rows, err := r.pool.Query(ctx, `SELECT id, code, name FROM specialties ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("catalog: list specialties: %w", err)
	}
	defer rows.Close()

	out := []Specialty{}
	for rows.Next() {
		var s Specialty
		if err := rows.Scan(&s.ID, &s.Code, &s.Name); err != nil {
			return nil, fmt.Errorf("catalog: scan specialty: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListPackages returns active packages with the current effective price
// (global rule scope; region overrides arrive with region-scoped booking in
// Phase 3+). Falls back to base_price when no rule is in force yet.
func (r *Repository) ListPackages(ctx context.Context) ([]Package, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, code, name, duration_minutes, base_price::text, currency, active
		FROM tour_packages
		WHERE active
		ORDER BY duration_minutes`)
	if err != nil {
		return nil, fmt.Errorf("catalog: list packages: %w", err)
	}
	defer rows.Close()

	out := []Package{}
	for rows.Next() {
		var p Package
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.DurationMinutes, &p.Price, &p.Currency, &p.Active); err != nil {
			return nil, fmt.Errorf("catalog: scan package: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	at := r.now()
	for i := range out {
		rules, err := r.rulesForPackage(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		if rule, ok := EffectivePrice(rules, nil, at); ok {
			out[i].Price = rule.Amount
		}
	}
	return out, nil
}

// GetPackage returns one package (any active flag) plus all its pricing
// rules for caller-side EffectivePrice selection. Price is base_price; apply
// EffectivePrice before quoting (bookings does this per request).
func (r *Repository) GetPackage(ctx context.Context, id string) (Package, []PricingRule, error) {
	var p Package
	err := r.pool.QueryRow(ctx, `
		SELECT id, code, name, duration_minutes, base_price::text, currency, active
		FROM tour_packages WHERE id = $1`, id).
		Scan(&p.ID, &p.Code, &p.Name, &p.DurationMinutes, &p.Price, &p.Currency, &p.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return Package{}, nil, ErrNotFound
	}
	if err != nil {
		return Package{}, nil, fmt.Errorf("catalog: get package: %w", err)
	}
	rules, err := r.rulesForPackage(ctx, id)
	if err != nil {
		return Package{}, nil, err
	}
	return p, rules, nil
}

// rulesForPackage loads every pricing rule for a package; selection happens
// in Go via EffectivePrice so the effective-date logic is unit-testable.
func (r *Repository) rulesForPackage(ctx context.Context, packageID string) ([]PricingRule, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT package_id, region_id, amount::text, effective_from, effective_to
		FROM pricing_rules
		WHERE package_id = $1`, packageID)
	if err != nil {
		return nil, fmt.Errorf("catalog: list pricing rules: %w", err)
	}
	defer rows.Close()

	out := []PricingRule{}
	for rows.Next() {
		var pr PricingRule
		if err := rows.Scan(&pr.PackageID, &pr.RegionID, &pr.Amount, &pr.EffectiveFrom, &pr.EffectiveTo); err != nil {
			return nil, fmt.Errorf("catalog: scan pricing rule: %w", err)
		}
		out = append(out, pr)
	}
	return out, rows.Err()
}
