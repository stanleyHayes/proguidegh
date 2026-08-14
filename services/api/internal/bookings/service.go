package bookings

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"proguidegh/api/internal/availability"
	"proguidegh/api/internal/catalog"
	"proguidegh/api/internal/certification"
	"proguidegh/api/internal/guides"
)

// Service is the bookings domain service: all booking validation and
// server-authoritative pricing flows through it (spec §14).
type Service struct {
	repo    *Repository
	catalog *catalog.Repository
	guides  *guides.Repository
	cert    *certification.Service
	avail   *availability.Repository
	now     func() time.Time // injectable for tests
}

// NewService builds the service.
func NewService(repo *Repository, cat *catalog.Repository, g *guides.Repository,
	cert *certification.Service, avail *availability.Repository) *Service {
	return &Service{repo: repo, catalog: cat, guides: g, cert: cert, avail: avail, now: time.Now}
}

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrValidation — malformed or inconsistent request payload (400).
	ErrValidation = errors.New("bookings: invalid request")
	// ErrPackageNotFound — unknown package id (404).
	ErrPackageNotFound = errors.New("bookings: package not found")
	// ErrPackageInactive — package exists but is not bookable (422).
	ErrPackageInactive = errors.New("bookings: package is not active")
	// ErrGuideNotEligible — the guide fails the §10.2 gate (422); returned
	// for unknown, uncertified, suspended or under-documented guides alike.
	ErrGuideNotEligible = errors.New("bookings: guide is not eligible for booking")
	// ErrGuideUnavailable — schedule/time-off does not cover the requested
	// interval (422).
	ErrGuideUnavailable = errors.New("bookings: guide is not available at the requested time")
)

// PackageSummary is the package view embedded in quotes and create responses.
type PackageSummary struct {
	ID              string `json:"id"`
	Code            string `json:"code"`
	Name            string `json:"name"`
	DurationMinutes int    `json:"duration_minutes"`
	Currency        string `json:"currency"`
}

// Quote is the server-authoritative quote response (spec §13.3, §14): the
// package, the computed interval and the price breakdown. Clients never
// submit totals; creation recomputes them.
type Quote struct {
	Package  PackageSummary `json:"package"`
	StartsAt time.Time      `json:"starts_at"`
	EndsAt   time.Time      `json:"ends_at"`
	Guests   int            `json:"guests"`
	Price    PriceBreakdown `json:"price"`
}

// QuoteParams is one quote request.
type QuoteParams struct {
	PackageID string
	StartsAt  time.Time
	Guests    int
}

// Quote computes the server-authoritative price for a package at a start
// time. No guide is needed: region-scoped pricing applies at booking time
// (Create re-quotes with the guide's region).
func (s *Service) Quote(ctx context.Context, p QuoteParams) (Quote, error) {
	pkg, rules, err := s.loadActivePackage(ctx, p.PackageID)
	if err != nil {
		return Quote{}, err
	}
	if p.StartsAt.IsZero() {
		return Quote{}, fmt.Errorf("%w: starts_at is required", ErrValidation)
	}
	guests := p.Guests
	if guests == 0 {
		guests = 1
	}
	if guests < 0 {
		return Quote{}, fmt.Errorf("%w: guests must be positive", ErrValidation)
	}

	endsAt := p.StartsAt.Add(time.Duration(pkg.DurationMinutes) * time.Minute)
	breakdown, err := s.breakdownFor(ctx, rules, nil, pkg, p.StartsAt)
	if err != nil {
		return Quote{}, err
	}
	return Quote{
		Package:  summarize(pkg),
		StartsAt: p.StartsAt,
		EndsAt:   endsAt,
		Guests:   guests,
		Price:    breakdown,
	}, nil
}

// CreateParams is one validated booking creation request.
type CreateParams struct {
	TouristID    string
	PackageID    string
	GuideID      string
	StartsAt     time.Time
	MeetingPoint *string
	MeetingLat   *string
	MeetingLng   *string
	Guests       int
	Notes        *string

	// IdempotencyKey is the required Idempotency-Key header (spec §14).
	IdempotencyKey string
}

// Create validates the request end to end and creates the booking in
// PAYMENT_PENDING (payment confirmation lands in Phase 4):
//
//  1. Package exists and is active; ends_at = starts_at + duration.
//  2. Direct flow (guide_id set): the guide passes the §10.2 eligibility
//     gate (ACTIVE certification, unsuspended account/profile, valid
//     mandatory documents) and their weekly schedule covers the interval
//     with no intersecting time off. Marketplace flow (guide_id empty):
//     these checks are deferred to dispatch (Phase 5), which only offers
//     the booking to eligible, available guides after confirmation.
//  3. Price is recomputed server-side (EffectivePrice + system_settings
//     percentages) and snapshotted onto the booking — client totals are
//     never trusted (spec §14). Marketplace bookings price from the
//     package's default rule (no guide region is known yet).
//  4. Repository.Create claims the idempotency key and enforces the
//     overlap guard transactionally (direct flow only — a guideless
//     booking holds no calendar slot).
func (s *Service) Create(ctx context.Context, p CreateParams) (CreateResult, Quote, error) {
	p.IdempotencyKey = strings.TrimSpace(p.IdempotencyKey)
	if p.IdempotencyKey == "" || len(p.IdempotencyKey) > 200 {
		return CreateResult{}, Quote{}, fmt.Errorf("%w: Idempotency-Key header is required (max 200 chars)", ErrValidation)
	}
	if p.StartsAt.IsZero() {
		return CreateResult{}, Quote{}, fmt.Errorf("%w: starts_at is required", ErrValidation)
	}
	if p.StartsAt.Before(s.now().Add(-5 * time.Minute)) {
		return CreateResult{}, Quote{}, fmt.Errorf("%w: starts_at must be in the future", ErrValidation)
	}
	guests := p.Guests
	if guests == 0 {
		guests = 1
	}
	if guests < 0 || guests > 50 {
		return CreateResult{}, Quote{}, fmt.Errorf("%w: guests must be between 1 and 50", ErrValidation)
	}
	if (p.MeetingLat == nil) != (p.MeetingLng == nil) {
		return CreateResult{}, Quote{}, fmt.Errorf("%w: meeting_lat and meeting_lng must be provided together", ErrValidation)
	}

	pkg, rules, err := s.loadActivePackage(ctx, p.PackageID)
	if err != nil {
		return CreateResult{}, Quote{}, err
	}
	endsAt := p.StartsAt.Add(time.Duration(pkg.DurationMinutes) * time.Minute)

	// Marketplace flow (spec §10): no guide chosen — dispatch assigns one
	// after payment confirmation (Phase 5). Skip the guide gates and price
	// from the package's default (region-less) rule.
	if p.GuideID == "" {
		breakdown, err := s.breakdownFor(ctx, rules, nil, pkg, p.StartsAt)
		if err != nil {
			return CreateResult{}, Quote{}, err
		}
		res, err := s.repo.Create(ctx, CreateInput{
			TouristID:      p.TouristID,
			GuideID:        "",
			PackageID:      pkg.ID,
			StartsAt:       p.StartsAt,
			EndsAt:         endsAt,
			MeetingPoint:   p.MeetingPoint,
			MeetingLat:     p.MeetingLat,
			MeetingLng:     p.MeetingLng,
			NumGuests:      guests,
			Notes:          p.Notes,
			Amount:         breakdown.Amount,
			Currency:       breakdown.Currency,
			IdempotencyKey: p.IdempotencyKey,
			IdemScope:      "booking.create:" + p.TouristID,
			PayloadHash:    payloadHash(p, endsAt),
		})
		if err != nil {
			return CreateResult{}, Quote{}, err
		}
		return res, Quote{
			Package:  summarize(pkg),
			StartsAt: p.StartsAt,
			EndsAt:   endsAt,
			Guests:   guests,
			Price:    breakdown,
		}, nil
	}

	// §10.2 eligibility gate — same signals as the public visibility gate.
	view, err := s.guides.GetPublicView(ctx, p.GuideID)
	if errors.Is(err, guides.ErrNotFound) {
		return CreateResult{}, Quote{}, ErrGuideNotEligible
	}
	if err != nil {
		return CreateResult{}, Quote{}, fmt.Errorf("bookings: load guide: %w", err)
	}
	docsValid, _, err := s.cert.DocumentsValid(ctx, p.GuideID)
	if err != nil {
		return CreateResult{}, Quote{}, fmt.Errorf("bookings: validate documents: %w", err)
	}
	caseStatus := ""
	if view.CaseStatus != nil {
		caseStatus = *view.CaseStatus
	}
	if !guides.PubliclyVisible(guides.VisibilityInput{
		CaseStatus:     caseStatus,
		UserStatus:     view.UserStatus,
		GuideStatus:    view.GuideStatus,
		DocumentsValid: docsValid,
	}) {
		return CreateResult{}, Quote{}, ErrGuideNotEligible
	}

	// Availability: weekly schedule coverage + no intersecting time off.
	ok, err := s.avail.AvailableAt(ctx, p.GuideID, p.StartsAt, endsAt)
	if err != nil {
		return CreateResult{}, Quote{}, err
	}
	if !ok {
		return CreateResult{}, Quote{}, ErrGuideUnavailable
	}

	breakdown, err := s.breakdownFor(ctx, rules, view.RegionID, pkg, p.StartsAt)
	if err != nil {
		return CreateResult{}, Quote{}, err
	}

	res, err := s.repo.Create(ctx, CreateInput{
		TouristID:      p.TouristID,
		GuideID:        p.GuideID,
		PackageID:      pkg.ID,
		StartsAt:       p.StartsAt,
		EndsAt:         endsAt,
		MeetingPoint:   p.MeetingPoint,
		MeetingLat:     p.MeetingLat,
		MeetingLng:     p.MeetingLng,
		NumGuests:      guests,
		Notes:          p.Notes,
		Amount:         breakdown.Amount,
		Currency:       breakdown.Currency,
		IdempotencyKey: p.IdempotencyKey,
		IdemScope:      "booking.create:" + p.TouristID,
		PayloadHash:    payloadHash(p, endsAt),
	})
	if err != nil {
		return CreateResult{}, Quote{}, err
	}
	return res, Quote{
		Package:  summarize(pkg),
		StartsAt: p.StartsAt,
		EndsAt:   endsAt,
		Guests:   guests,
		Price:    breakdown,
	}, nil
}

// loadActivePackage resolves the package or returns
// ErrPackageNotFound/ErrPackageInactive.
func (s *Service) loadActivePackage(ctx context.Context, id string) (catalog.Package, []catalog.PricingRule, error) {
	pkg, rules, err := s.catalog.GetPackage(ctx, id)
	if errors.Is(err, catalog.ErrNotFound) {
		return catalog.Package{}, nil, ErrPackageNotFound
	}
	if err != nil {
		return catalog.Package{}, nil, fmt.Errorf("bookings: load package: %w", err)
	}
	if !pkg.Active {
		return catalog.Package{}, nil, ErrPackageInactive
	}
	return pkg, rules, nil
}

// breakdownFor computes the server-authoritative price: the effective-dated
// rule in force at startsAt (region-scoped when the guide has a region,
// falling back to the package base price), split by the configured
// percentages (spec §27).
func (s *Service) breakdownFor(ctx context.Context, rules []catalog.PricingRule, regionID *string, pkg catalog.Package, startsAt time.Time) (PriceBreakdown, error) {
	amount := pkg.Price
	if rule, ok := catalog.EffectivePrice(rules, regionID, startsAt); ok {
		amount = rule.Amount
	}
	feePct, err := s.repo.SettingText(ctx, "platform_fee_pct")
	if err != nil {
		return PriceBreakdown{}, err
	}
	levyPct, err := s.repo.SettingText(ctx, "tourism_levy_pct")
	if err != nil {
		return PriceBreakdown{}, err
	}
	return ComputeBreakdown(amount, pkg.Currency, feePct, levyPct)
}

func summarize(pkg catalog.Package) PackageSummary {
	return PackageSummary{
		ID:              pkg.ID,
		Code:            pkg.Code,
		Name:            pkg.Name,
		DurationMinutes: pkg.DurationMinutes,
		Currency:        pkg.Currency,
	}
}

// payloadHash fingerprints the canonical creation payload so a reused
// Idempotency-Key with a different body is a conflict (spec §14).
func payloadHash(p CreateParams, endsAt time.Time) string {
	parts := []string{
		p.TouristID, p.PackageID, p.GuideID,
		p.StartsAt.UTC().Format(time.RFC3339Nano), endsAt.UTC().Format(time.RFC3339Nano),
		deref(p.MeetingPoint), deref(p.MeetingLat), deref(p.MeetingLng),
		fmt.Sprintf("%d", p.Guests), deref(p.Notes),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
