// Package dispatch implements the §10.3 dispatch algorithm: candidate
// filtering over the §10.2 eligibility gate, weighted scoring, batched
// offers with Redis-cached TTLs, atomic first-wins acceptance, decline and
// expiry (sweeper + lazy). Scoring features and outcomes persist on every
// offer row for offline model evaluation.
package dispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"proguidegh/api/internal/availability"
	"proguidegh/api/internal/bookings"
	"proguidegh/api/internal/realtime"
)

// Config is the dispatch policy read from system_settings (migration 0006
// seeds the defaults). Redis offer TTL keys are a cache of expires_at; the
// database deadline is authoritative.
type Config struct {
	Weights        Weights
	BatchSize      int           // dispatch_batch_size (default 3)
	OfferTTL       time.Duration // dispatch_offer_ttl_seconds (default 30)
	RadiusKm       float64       // dispatch_radius_km (default 50)
	PresenceWindow time.Duration // dispatch_presence_window_minutes (default 120)
}

// Service is the dispatch domain service (spec §10.3): candidate filtering,
// scoring, batch offers, atomic acceptance, decline and expiry.
type Service struct {
	pool     *pgxpool.Pool
	repo     *Repository
	bookings *bookings.Repository
	presence *availability.Presence
	rdb      *goredis.Client
	hub      *realtime.Hub
	now      func() time.Time // injectable for tests
}

// NewService builds the service.
func NewService(pool *pgxpool.Pool, repo *Repository, bookingsRepo *bookings.Repository,
	presence *availability.Presence, rdb *goredis.Client, hub *realtime.Hub) *Service {
	return &Service{pool: pool, repo: repo, bookings: bookingsRepo,
		presence: presence, rdb: rdb, hub: hub, now: time.Now}
}

// LoadConfig reads the dispatch policy from system_settings, falling back to
// the documented defaults for missing/malformed rows.
func (s *Service) LoadConfig(ctx context.Context) (Config, error) {
	cfg := Config{
		Weights:        DefaultWeights(),
		BatchSize:      3,
		OfferTTL:       30 * time.Second,
		RadiusKm:       50,
		PresenceWindow: 120 * time.Minute,
	}
	raw, err := s.repo.settingText(ctx, "dispatch_weights")
	if err != nil {
		return Config{}, err
	}
	cfg.Weights = ParseWeights(raw)

	if v, err := s.repo.settingText(ctx, "dispatch_batch_size"); err != nil {
		return Config{}, err
	} else if n, err := strconv.Atoi(v); err == nil && n > 0 {
		cfg.BatchSize = n
	}
	if v, err := s.repo.settingText(ctx, "dispatch_offer_ttl_seconds"); err != nil {
		return Config{}, err
	} else if n, err := strconv.Atoi(v); err == nil && n > 0 {
		cfg.OfferTTL = time.Duration(n) * time.Second
	}
	if v, err := s.repo.settingText(ctx, "dispatch_radius_km"); err != nil {
		return Config{}, err
	} else if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
		cfg.RadiusKm = f
	}
	if v, err := s.repo.settingText(ctx, "dispatch_presence_window_minutes"); err != nil {
		return Config{}, err
	} else if n, err := strconv.Atoi(v); err == nil && n > 0 {
		cfg.PresenceWindow = time.Duration(n) * time.Minute
	}
	return cfg, nil
}

// Result summarizes one Dispatch run.
type Result struct {
	BookingID  string  `json:"booking_id"`
	BatchSeq   int     `json:"batch_seq"`
	Offers     []Offer `json:"offers"`
	Candidates int     `json:"candidates"`  // eligible guides before top-N cut
	ReusedLive bool    `json:"reused_live"` // a live batch already existed (idempotent re-dispatch)
}

// Dispatch runs one offer batch for a booking awaiting a guide (§10.3):
//
//  1. The booking must be CONFIRMED with no guide assigned — direct bookings
//     (guide chosen at creation) never enter dispatch.
//  2. A still-live batch makes the call a no-op returning that batch, so the
//     payment-confirmation hook and the admin endpoint cannot double-offer.
//  3. Candidates: §10.2 gate + window availability + schedule coverage, and
//     Redis presence when the booking starts within the presence window
//     ("available now"); guides who DECLINED an earlier batch are excluded.
//  4. Score every candidate (pure Score), take the top BatchSize, insert the
//     offer rows + Redis TTL keys, and push each offer to the guide's WS
//     channel. Zero candidates pushes a dispatch.unmatched event to the
//     operations feed (§30.2: why a booking has not been matched).
func (s *Service) Dispatch(ctx context.Context, bookingID, trigger string) (Result, error) {
	// Lazy expiry keeps the live-batch check truthful without waiting for
	// the sweeper.
	if _, err := s.repo.ExpireStale(ctx); err != nil {
		return Result{}, err
	}

	b, err := s.bookings.GetByID(ctx, bookingID)
	if errors.Is(err, bookings.ErrNotFound) {
		return Result{}, err
	}
	if err != nil {
		return Result{}, fmt.Errorf("dispatch: load booking: %w", err)
	}
	if b.Status != bookings.StatusConfirmed || b.GuideID != nil {
		return Result{}, fmt.Errorf("%w: booking is %s (guide assigned: %v)",
			ErrNotDispatchable, b.Status, b.GuideID != nil)
	}
	if b.EndsAt == nil {
		return Result{}, fmt.Errorf("dispatch: booking %s has no ends_at", bookingID)
	}

	if live, err := s.repo.LiveOffers(ctx, bookingID); err != nil {
		return Result{}, err
	} else if len(live) > 0 {
		seq := live[0].BatchSeq
		return Result{BookingID: bookingID, BatchSeq: seq, Offers: live, Candidates: len(live), ReusedLive: true}, nil
	}

	cfg, err := s.LoadConfig(ctx)
	if err != nil {
		return Result{}, err
	}
	batchSeq, err := s.repo.NextBatchSeq(ctx, bookingID)
	if err != nil {
		return Result{}, err
	}

	cands, err := s.repo.Candidates(ctx, b, cfg.RadiusKm)
	if err != nil {
		return Result{}, err
	}
	if cands, err = s.repo.FilterBySchedule(ctx, cands, b.StartsAt, *b.EndsAt); err != nil {
		return Result{}, err
	}

	// "Available now": bookings starting within the presence window only go
	// to guides holding a live Redis presence marker (spec §10.2/§11).
	if b.StartsAt.Before(s.now().Add(cfg.PresenceWindow)) {
		ids := make([]string, 0, len(cands))
		for _, c := range cands {
			ids = append(ids, c.UserID)
		}
		online, err := s.presence.OnlineIDs(ctx, ids)
		if err != nil {
			return Result{}, fmt.Errorf("dispatch: presence check: %w", err)
		}
		filtered := cands[:0]
		for _, c := range cands {
			if online[c.UserID] {
				filtered = append(filtered, c)
			}
		}
		cands = filtered
	}

	res := Result{BookingID: bookingID, BatchSeq: batchSeq, Candidates: len(cands), Offers: []Offer{}}
	if len(cands) == 0 {
		s.broadcast(realtime.ChannelAdminOperations, "dispatch.unmatched", map[string]any{
			"booking_id": bookingID, "batch_seq": batchSeq, "trigger": trigger,
			"reason": "no eligible available candidates",
		})
		return res, nil
	}

	lang, err := s.repo.touristLanguage(ctx, b.TouristID)
	if err != nil {
		return Result{}, err
	}
	pkgCode, err := s.repo.packageCode(ctx, b.PackageID)
	if err != nil {
		return Result{}, err
	}

	type scored struct {
		cand     Candidate
		features Features
		score    float64
	}
	ranked := make([]scored, 0, len(cands))
	for _, c := range cands {
		f := Features{
			DistanceKm:            distanceFor(c, b),
			RatingAvg:             c.RatingAvg,
			RatingCount:           c.RatingCount,
			SpecialtyMatch:        specialtyMatch(pkgCode, c.Specialties),
			LanguageMatch:         languageMatch(lang, c.Languages),
			RecentWorkload:        c.Workload,
			AcceptanceReliability: Reliability(c.OffersSeen, c.AcceptsSeen),
			OffersSeen:            c.OffersSeen,
			AcceptsSeen:           c.AcceptsSeen,
		}
		ranked = append(ranked, scored{cand: c, features: f, score: Score(cfg.Weights, f)})
	}
	// Deterministic rank: score desc, rating desc, user_id (§10.1 rank rule).
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		if ranked[i].cand.RatingAvg != ranked[j].cand.RatingAvg {
			return ranked[i].cand.RatingAvg > ranked[j].cand.RatingAvg
		}
		return ranked[i].cand.UserID < ranked[j].cand.UserID
	})
	if len(ranked) > cfg.BatchSize {
		ranked = ranked[:cfg.BatchSize]
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("dispatch: begin offers: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	expiresAt := s.now().Add(cfg.OfferTTL)
	views := make([]OfferView, 0, len(ranked))
	for _, r := range ranked {
		o, err := s.repo.InsertOffer(ctx, tx, Offer{
			BookingID: bookingID, GuideID: r.cand.UserID,
			BatchSeq: batchSeq, Score: strconv.FormatFloat(r.score, 'f', 5, 64),
			ExpiresAt: expiresAt,
		}, r.features)
		if err != nil {
			return Result{}, err
		}
		res.Offers = append(res.Offers, o)
		views = append(views, OfferView{
			Offer: o, BookingReference: b.Reference, StartsAt: b.StartsAt, EndsAt: b.EndsAt,
			MeetingPoint: b.MeetingPointText, NumGuests: b.NumGuests,
		})
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, fmt.Errorf("dispatch: commit offers: %w", err)
	}

	// Post-commit fan-out: Redis TTL keys (cache of expires_at) and WS push.
	for i, o := range res.Offers {
		if err := s.rdb.Set(ctx, offerKey(o.ID), bookingID, cfg.OfferTTL).Err(); err != nil {
			slog.Warn("dispatch: offer redis key failed", "offer_id", o.ID, "error", err)
		}
		s.broadcast(realtime.ChannelGuide(o.GuideID), "dispatch.offer", views[i])
	}
	s.broadcast(realtime.ChannelAdminOperations, "dispatch.batch", map[string]any{
		"booking_id": bookingID, "batch_seq": batchSeq, "trigger": trigger,
		"offers": len(res.Offers), "candidates": res.Candidates, "expires_at": expiresAt,
	})
	return res, nil
}

// AcceptResult pairs the winning offer with the assigned booking.
type AcceptResult struct {
	Offer   Offer            `json:"offer"`
	Booking bookings.Booking `json:"booking"`
}

// Accept atomically assigns the booking to the accepting guide (§10.3 step
// 4: first valid acceptance wins). One transaction: lock the offer, verify
// it is OFFERED and unexpired (the DB expires_at is authoritative — the
// Redis key is only a cache), lock the booking, verify it is still
// CONFIRMED and guideless, assign the guide, mark this offer ACCEPTED and
// every sibling SUPERSEDED, and record the immutable assignment event. A
// concurrent second accept fails the re-checks and gets 409; a schedule
// collision fails on the overlap exclusion constraint and also gets 409.
func (s *Service) Accept(ctx context.Context, offerID, guideID string) (AcceptResult, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return AcceptResult{}, fmt.Errorf("dispatch: begin accept: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	offer, err := s.repo.GetOfferForUpdate(ctx, tx, offerID)
	if err != nil {
		return AcceptResult{}, err
	}
	if offer.GuideID != guideID {
		return AcceptResult{}, ErrOfferNotFound // never leak another guide's offer
	}
	if offer.Status == OfferExpired || (offer.Status == OfferOffered && offer.IsExpired(s.now())) {
		// Lazy expiry: persist the terminal state the sweeper would have set.
		if offer.Status == OfferOffered {
			if err := s.repo.SetOfferStatus(ctx, tx, offer.ID, offer.GuideID, OfferExpired); err != nil {
				return AcceptResult{}, err
			}
			if err := tx.Commit(ctx); err != nil {
				return AcceptResult{}, fmt.Errorf("dispatch: commit lazy expiry: %w", err)
			}
		}
		return AcceptResult{}, ErrOfferExpired
	}
	if offer.Status != OfferOffered {
		return AcceptResult{}, fmt.Errorf("%w: offer is %s", ErrOfferClosed, offer.Status)
	}

	b, err := s.repo.BookingForUpdate(ctx, tx, offer.BookingID)
	if err != nil {
		return AcceptResult{}, err
	}
	if b.Status != bookings.StatusConfirmed || b.GuideID != nil {
		return AcceptResult{}, fmt.Errorf("%w: booking already assigned or not dispatchable", ErrOfferClosed)
	}

	if err := s.repo.AssignGuide(ctx, tx, b.ID, guideID); err != nil {
		if errors.Is(err, bookings.ErrOverlap) {
			return AcceptResult{}, fmt.Errorf("%w: guide has an overlapping active booking", bookings.ErrOverlap)
		}
		return AcceptResult{}, err
	}
	if err := s.repo.SetOfferStatus(ctx, tx, offer.ID, guideID, OfferAccepted); err != nil {
		return AcceptResult{}, err
	}
	losers, err := s.repo.SupersedeOthers(ctx, tx, b.ID, offer.ID)
	if err != nil {
		return AcceptResult{}, err
	}
	meta, _ := json.Marshal(map[string]string{
		"action": "dispatch.guide_assigned", "offer_id": offer.ID, "guide_id": guideID,
	})
	if _, err := tx.Exec(ctx, `
		INSERT INTO booking_status_events (booking_id, from_status, to_status, actor_id, metadata)
		VALUES ($1, $2, $2, $3, $4)`,
		b.ID, bookings.StatusConfirmed, guideID, json.RawMessage(meta)); err != nil {
		return AcceptResult{}, fmt.Errorf("dispatch: assignment event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return AcceptResult{}, fmt.Errorf("dispatch: commit accept: %w", err)
	}

	// Post-commit: drop the Redis offer keys and notify every channel.
	b.Status = bookings.StatusConfirmed
	b.GuideID = &guideID
	for _, o := range append(losers, offer) {
		s.rdb.Del(ctx, offerKey(o.ID)) //nolint:errcheck // cache only
	}
	for _, o := range losers {
		s.broadcast(realtime.ChannelGuide(o.GuideID), "dispatch.offer_resolved",
			map[string]string{"offer_id": o.ID, "status": OfferSuperseded})
	}
	s.broadcast(realtime.ChannelGuide(guideID), "dispatch.offer_resolved",
		map[string]string{"offer_id": offer.ID, "status": OfferAccepted})
	s.broadcast(realtime.ChannelBooking(b.ID), "booking.updated", b)
	s.broadcast(realtime.ChannelAdminOperations, "booking.updated", b)

	offer.Status = OfferAccepted
	return AcceptResult{Offer: offer, Booking: b}, nil
}

// Decline marks one of the guide's live offers DECLINED. When the decline
// empties the booking's live offer set and the booking is still unmatched,
// the next batch is dispatched immediately (excluding decliners).
func (s *Service) Decline(ctx context.Context, offerID, guideID string) (Offer, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Offer{}, fmt.Errorf("dispatch: begin decline: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit

	offer, err := s.repo.GetOfferForUpdate(ctx, tx, offerID)
	if err != nil {
		return Offer{}, err
	}
	if offer.GuideID != guideID {
		return Offer{}, ErrOfferNotFound
	}
	if offer.Status == OfferExpired || (offer.Status == OfferOffered && offer.IsExpired(s.now())) {
		return Offer{}, ErrOfferExpired
	}
	if offer.Status != OfferOffered {
		return Offer{}, fmt.Errorf("%w: offer is %s", ErrOfferClosed, offer.Status)
	}
	if err := s.repo.SetOfferStatus(ctx, tx, offer.ID, guideID, OfferDeclined); err != nil {
		return Offer{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Offer{}, fmt.Errorf("dispatch: commit decline: %w", err)
	}

	s.rdb.Del(ctx, offerKey(offer.ID)) //nolint:errcheck // cache only
	offer.Status = OfferDeclined

	// Next batch when nothing live remains and the booking is still waiting.
	if live, err := s.repo.LiveOffers(ctx, offer.BookingID); err == nil && len(live) == 0 {
		if _, err := s.Dispatch(ctx, offer.BookingID, "offer.declined"); err != nil &&
			!errors.Is(err, ErrNotDispatchable) && !errors.Is(err, bookings.ErrNotFound) {
			slog.Error("dispatch: re-dispatch after decline", "booking_id", offer.BookingID, "error", err)
		}
	}
	return offer, nil
}

// ExpireOffers is the sweeper entry point: marks every live offer past its
// DB deadline EXPIRED (lazy reads do the same per-offer) and notifies the
// operations feed about bookings whose batch fully expired without an
// acceptance. The API runs it on a ticker; the worker can call the same
// exported method when it takes over scheduling (spec §21).
func (s *Service) ExpireOffers(ctx context.Context) (int, error) {
	expired, err := s.repo.ExpireStale(ctx)
	if err != nil {
		return 0, err
	}
	for _, o := range expired {
		s.rdb.Del(ctx, offerKey(o.ID)) //nolint:errcheck // cache only
		s.broadcast(realtime.ChannelGuide(o.GuideID), "dispatch.offer_resolved",
			map[string]string{"offer_id": o.ID, "status": OfferExpired})
	}
	// One admin event per booking whose batch just fully expired.
	seen := map[string]bool{}
	for _, o := range expired {
		if seen[o.BookingID] {
			continue
		}
		seen[o.BookingID] = true
		live, err := s.repo.LiveOffers(ctx, o.BookingID)
		if err != nil || len(live) > 0 {
			continue
		}
		b, err := s.bookings.GetByID(ctx, o.BookingID)
		if err != nil || b.Status != bookings.StatusConfirmed || b.GuideID != nil {
			continue
		}
		s.broadcast(realtime.ChannelAdminOperations, "dispatch.batch_expired", map[string]any{
			"booking_id": o.BookingID, "batch_seq": o.BatchSeq,
			"reason": "batch expired without acceptance",
		})
	}
	return len(expired), nil
}

// ListMine returns the guide's current (OFFERED, unexpired) offers for the
// REST inbox — the realtime catch-up path (§31.27).
func (s *Service) ListMine(ctx context.Context, guideID string) ([]OfferView, error) {
	if _, err := s.repo.ExpireStale(ctx); err != nil {
		return nil, err
	}
	return s.repo.ListForGuide(ctx, guideID, true, 50)
}

// SnapshotMessages renders the guide's live offers as WS catch-up messages
// (pushed on /ws/guide connect).
func (s *Service) SnapshotMessages(ctx context.Context, guideID string) []realtime.Message {
	offers, err := s.ListMine(ctx, guideID)
	if err != nil {
		slog.Error("dispatch: guide snapshot", "guide_id", guideID, "error", err)
		return nil
	}
	msgs := make([]realtime.Message, 0, len(offers))
	for _, o := range offers {
		msgs = append(msgs, realtime.NewMessage("dispatch.offer", o))
	}
	return msgs
}

// BookingOffers is the operations "why unmatched" view (§30.2).
func (s *Service) BookingOffers(ctx context.Context, bookingID string) ([]Offer, error) {
	if _, err := s.repo.ExpireStale(ctx); err != nil {
		return nil, err
	}
	return s.repo.ListForBooking(ctx, bookingID)
}

func (s *Service) broadcast(channel, msgType string, data any) {
	if s.hub == nil {
		return
	}
	s.hub.Broadcast(channel, realtime.NewMessage(msgType, data))
}

func offerKey(offerID string) string { return "offer:" + offerID }

// distanceFor computes the guide→meeting-point distance when both ends have
// coordinates (nil otherwise — neutral distance sub-score).
func distanceFor(c Candidate, b bookings.Booking) *float64 {
	if c.Latitude == nil || c.Longitude == nil || b.MeetingLatitude == nil || b.MeetingLongitude == nil {
		return nil
	}
	lat, err1 := strconv.ParseFloat(*b.MeetingLatitude, 64)
	lng, err2 := strconv.ParseFloat(*b.MeetingLongitude, 64)
	if err1 != nil || err2 != nil {
		return nil
	}
	d := haversineKm(*c.Latitude, *c.Longitude, lat, lng)
	return &d
}

// haversineKm mirrors guides.haversineKm (duplicated: dispatch must not
// depend on the search slice's internals).
func haversineKm(lat1, lng1, lat2, lng2 float64) float64 {
	const earthKm = 6371.0
	rad := func(deg float64) float64 { return deg * math.Pi / 180 }
	dLat := rad(lat2 - lat1)
	dLng := rad(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthKm * 2 * math.Asin(math.Sqrt(a))
}

// specialtyMatch scores package↔specialty alignment via the V1 static table.
func specialtyMatch(packageCode string, guideSpecialties []string) float64 {
	want, ok := PackageSpecialty[packageCode]
	if !ok {
		return 0.5 // package carries no specialty signal
	}
	for _, s := range guideSpecialties {
		if s == want {
			return 1
		}
	}
	return 0
}

// languageMatch scores tourist↔guide language alignment.
func languageMatch(touristLang string, guideLangs []string) float64 {
	if touristLang == "" {
		return 0.5 // tourist set no preference
	}
	for _, l := range guideLangs {
		if l == touristLang {
			return 1
		}
	}
	return 0
}
