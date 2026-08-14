package reviews

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"proguidegh/api/internal/bookings"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrNotFound — no such booking (also returned for bookings the caller
	// may not see, so existence never leaks).
	ErrNotFound = errors.New("reviews: booking not found")
	// ErrForbidden — the caller is not the booking's tourist.
	ErrForbidden = errors.New("reviews: only the booking's tourist may review it")
	// ErrBookingNotCompleted — reviews open only after COMPLETED (spec §4.4).
	ErrBookingNotCompleted = errors.New("reviews: booking is not completed")
	// ErrAlreadyReviewed — one verified review per booking (spec §4.4).
	ErrAlreadyReviewed = errors.New("reviews: booking already has a review")
	// ErrNoGuide — the booking was never assigned a guide.
	ErrNoGuide = errors.New("reviews: booking has no guide to review")
	// ErrValidation — malformed input.
	ErrValidation = errors.New("reviews: validation failed")
)

// AllowedTags is the Appendix B dictionary; anything outside it is rejected
// so the tag set stays curated.
var AllowedTags = map[string]bool{
	"Knowledgeable":         true,
	"Punctual":              true,
	"Friendly":              true,
	"Professional":          true,
	"Helpful":               true,
	"Great Storyteller":     true,
	"Safety Conscious":      true,
	"Good Communicator":     true,
	"Local Expert":          true,
	"Exceeded Expectations": true,
}

// Quality thresholds (spec §4.4). Defaults apply when the system_settings
// rows are absent; operators tune them via configuration, not code.
const (
	defaultLowRatingThreshold  = 4.0
	defaultMinReviewCount      = 3
	defaultEliteThreshold      = 4.8
	defaultEliteCompletedTours = 20
)

// Service is the reviews application service.
type Service struct {
	repo     *Repository
	bookings *bookings.Repository
}

// NewService builds the service.
func NewService(repo *Repository, bookingsRepo *bookings.Repository) *Service {
	return &Service{repo: repo, bookings: bookingsRepo}
}

// Create writes the verified review for a completed booking, refreshes the
// guide's cached rating aggregate and evaluates the §4.4 quality thresholds.
func (s *Service) Create(ctx context.Context, bookingID, touristID string, rating int, body *string, tags []string) (Review, error) {
	if rating < 1 || rating > 5 {
		return Review{}, fmt.Errorf("%w: rating must be between 1 and 5", ErrValidation)
	}
	seen := map[string]bool{}
	for _, tag := range tags {
		if !AllowedTags[tag] {
			return Review{}, fmt.Errorf("%w: unknown tag %q (Appendix B)", ErrValidation, tag)
		}
		if seen[tag] {
			return Review{}, fmt.Errorf("%w: duplicate tag %q", ErrValidation, tag)
		}
		seen[tag] = true
	}

	b, err := s.bookings.GetByID(ctx, bookingID)
	if errors.Is(err, bookings.ErrNotFound) {
		return Review{}, ErrNotFound
	}
	if err != nil {
		return Review{}, fmt.Errorf("reviews: load booking: %w", err)
	}
	if b.TouristID != touristID {
		return Review{}, ErrForbidden
	}
	if b.Status != "COMPLETED" {
		return Review{}, ErrBookingNotCompleted
	}
	if b.GuideID == nil {
		return Review{}, ErrNoGuide
	}

	rev, err := s.repo.Create(ctx, bookingID, touristID, *b.GuideID, rating, body, tags)
	if err != nil {
		return Review{}, err
	}

	if err := s.evaluateQuality(ctx, *b.GuideID); err != nil {
		// The review is committed; a flagging failure must not fail the
		// request. Aggregation self-heals on the next review.
		slog.ErrorContext(ctx, "reviews: quality evaluation failed after review commit",
			"guide_id", *b.GuideID, "error", err)
	}
	return rev, nil
}

// evaluateQuality recomputes the aggregate and applies the §4.4 rules:
// below the low threshold → low_rating flag (retraining queue); above the
// Elite threshold with enough completed tours → elite_review flag.
func (s *Service) evaluateQuality(ctx context.Context, guideID string) error {
	avg, count, err := s.repo.Aggregate(ctx, guideID)
	if err != nil {
		return err
	}
	if err := s.repo.RefreshGuideRating(ctx, guideID, avg, count); err != nil {
		return err
	}

	low := s.settingFloat(ctx, "quality_low_rating_threshold", defaultLowRatingThreshold)
	minCount := s.settingInt(ctx, "quality_min_review_count", defaultMinReviewCount)
	elite := s.settingFloat(ctx, "elite_rating_threshold", defaultEliteThreshold)
	eliteTours := s.settingInt(ctx, "elite_min_completed_tours", defaultEliteCompletedTours)

	if count >= minCount && avg < low {
		if _, err := s.repo.OpenQualityFlag(ctx, guideID, "low_rating", avg,
			fmt.Sprintf("rolling average %.2f across %d reviews is below the %.2f threshold", avg, count, low)); err != nil {
			return err
		}
	}
	if avg > elite {
		tours, err := s.repo.CompletedTours(ctx, guideID)
		if err != nil {
			return err
		}
		if tours >= eliteTours {
			if _, err := s.repo.OpenQualityFlag(ctx, guideID, "elite_review", avg,
				fmt.Sprintf("rolling average %.2f with %d completed tours meets the Elite review bar (%.2f, %d tours)", avg, tours, elite, eliteTours)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) settingFloat(ctx context.Context, key string, fallback float64) float64 {
	raw, err := s.repo.SettingText(ctx, key)
	if err != nil || raw == "" {
		return fallback
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return v
}

func (s *Service) settingInt(ctx context.Context, key string, fallback int) int {
	raw, err := s.repo.SettingText(ctx, key)
	if err != nil || raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

// List returns the public, paginated review view for a guide.
func (s *Service) List(ctx context.Context, guideID string, limit, offset int) ([]Review, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.ListByGuide(ctx, guideID, limit, offset)
}
