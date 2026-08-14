package guides

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"proguidegh/api/internal/catalog"
	"proguidegh/api/internal/platform/httpx"
)

// Search handles GET /api/v1/guides/search (unauthenticated): the §10.1
// filter set over §10.2-eligible guides, deterministically ranked
// (rating_avg desc, rating_count desc) with offset pagination — see
// SearchRepository.Candidates for the pagination rationale (spec §14).
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	f, err := parseSearchFilters(r)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}

	// Package eligibility (§10.1): the package must be active; with
	// starts_at its duration supplies the availability window's end.
	if pkgID := strings.TrimSpace(r.URL.Query().Get("package_id")); pkgID != "" {
		dur, err := packageDuration(r, h.catalog, pkgID)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
			return
		}
		if !f.HasWindow {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION",
				"package_id requires starts_at", nil)
			return
		}
		if !f.EndsAt.IsZero() {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION",
				"pass either ends_at or package_id, not both", nil)
			return
		}
		f.EndsAt = f.StartsAt.Add(dur)
	}

	rows, err := h.search.Candidates(r.Context(), *f)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not search guides", nil)
		return
	}

	// Date/time availability: weekly-schedule coverage (batched, pure-Go
	// check); time-off and active-booking exclusion ran in SQL already.
	if f.HasWindow {
		rows, err = h.search.FilterBySchedule(r.Context(), rows, f.StartsAt, f.EndsAt)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not check availability", nil)
			return
		}
	}

	// "Available now" reads Redis presence (online marker with TTL); the
	// schedule reads Postgres (spec §10.2 split).
	if f.AvailableNow {
		ids := make([]string, 0, len(rows))
		for _, sr := range rows {
			ids = append(ids, sr.UserID)
		}
		online, err := h.presence.OnlineIDs(r.Context(), ids)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not read presence", nil)
			return
		}
		kept := rows[:0]
		for _, sr := range rows {
			if online[sr.UserID] {
				kept = append(kept, sr)
			}
		}
		rows = kept
	} else {
		ids := make([]string, 0, len(rows))
		for _, sr := range rows {
			ids = append(ids, sr.UserID)
		}
		if online, err := h.presence.OnlineIDs(r.Context(), ids); err == nil {
			for i := range rows {
				rows[i].Online = online[rows[i].UserID]
			}
		}
	}

	total := len(rows)
	start := f.Offset
	if start > total {
		start = total
	}
	end := start + f.Limit
	if end > total {
		end = total
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"guides": rows[start:end],
		"total":  total,
		"limit":  f.Limit,
		"offset": f.Offset,
	})
}

// parseSearchFilters validates the §10.1 query parameters.
func parseSearchFilters(r *http.Request) (*SearchFilters, error) {
	q := r.URL.Query()
	f := &SearchFilters{RadiusKm: 10, Limit: 20}

	if v := strings.TrimSpace(q.Get("region_id")); v != "" {
		f.RegionID = &v
	}
	if v := q.Get("lat"); v != "" {
		lat, err := strconv.ParseFloat(v, 64)
		if err != nil || lat < -90 || lat > 90 {
			return nil, errors.New("lat must be a number between -90 and 90")
		}
		f.Lat = &lat
	}
	if v := q.Get("lng"); v != "" {
		lng, err := strconv.ParseFloat(v, 64)
		if err != nil || lng < -180 || lng > 180 {
			return nil, errors.New("lng must be a number between -180 and 180")
		}
		f.Lng = &lng
	}
	if (f.Lat == nil) != (f.Lng == nil) {
		return nil, errors.New("lat and lng must be provided together")
	}
	if v := q.Get("radius_km"); v != "" {
		radius, err := strconv.ParseFloat(v, 64)
		if err != nil || radius <= 0 || radius > 500 {
			return nil, errors.New("radius_km must be a number between 0 and 500")
		}
		f.RadiusKm = radius
	}
	if v := strings.ToLower(strings.TrimSpace(q.Get("language"))); v != "" {
		f.Language = v
	}
	if v := strings.ToLower(strings.TrimSpace(q.Get("min_proficiency"))); v != "" {
		ord := ProficiencyOrdinal(v)
		if ord == 0 {
			return nil, errors.New("min_proficiency must be one of basic, conversational, fluent, native")
		}
		f.MinProficiency = ord
	}
	if v := strings.TrimSpace(q.Get("specialty_id")); v != "" {
		f.SpecialtyID = &v
	}
	if v := q.Get("min_rating"); v != "" {
		rating, err := strconv.ParseFloat(v, 64)
		if err != nil || rating < 0 || rating > 5 {
			return nil, errors.New("min_rating must be a number between 0 and 5")
		}
		f.MinRating = &rating
	}
	f.EliteOnly = q.Get("elite") == "true"
	f.AvailableNow = q.Get("available_now") == "true"

	// Date/time availability window: starts_at plus ends_at, or a package
	// whose duration supplies ends_at (package eligibility, §10.1).
	if v := q.Get("starts_at"); v != "" {
		startsAt, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, errors.New("starts_at must be RFC 3339")
		}
		f.StartsAt = startsAt
		f.HasWindow = true
	}
	if v := q.Get("ends_at"); v != "" {
		endsAt, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return nil, errors.New("ends_at must be RFC 3339")
		}
		f.EndsAt = endsAt
		if !f.HasWindow {
			return nil, errors.New("ends_at requires starts_at")
		}
	}
	if f.HasWindow && f.EndsAt.IsZero() {
		return nil, errors.New("ends_at is required with starts_at (or pass package_id on /bookings/quote for the duration)")
	}
	if f.HasWindow && !f.StartsAt.Before(f.EndsAt) {
		return nil, errors.New("ends_at must be after starts_at")
	}

	if v := q.Get("limit"); v != "" {
		limit, err := strconv.Atoi(v)
		if err != nil || limit < 1 || limit > 50 {
			return nil, errors.New("limit must be between 1 and 50")
		}
		f.Limit = limit
	}
	if v := q.Get("offset"); v != "" {
		offset, err := strconv.Atoi(v)
		if err != nil || offset < 0 {
			return nil, errors.New("offset must be >= 0")
		}
		f.Offset = offset
	}
	return f, nil
}

// packageDuration resolves the ends_at for a package-scoped search: the
// package must exist and be active ("package eligibility", spec §10.1).
func packageDuration(r *http.Request, cat *catalog.Repository, packageID string) (time.Duration, error) {
	pkg, _, err := cat.GetPackage(r.Context(), packageID)
	if errors.Is(err, catalog.ErrNotFound) {
		return 0, errors.New("unknown package_id")
	}
	if err != nil {
		return 0, err
	}
	if !pkg.Active {
		return 0, errors.New("package_id is not active")
	}
	return time.Duration(pkg.DurationMinutes) * time.Minute, nil
}
