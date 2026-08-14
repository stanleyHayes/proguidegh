package guides

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"proguidegh/api/internal/availability"
	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// requireGuideProfile resolves the caller's guide profile or writes the
// error response. Returns the guide's user id.
func (h *Handler) requireGuideProfile(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, _ := rbac.FromContext(r.Context())
	if _, err := h.repo.GetByUser(r.Context(), id.UserID); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.WriteError(w, r, http.StatusNotFound, "NO_GUIDE_PROFILE",
				"submit an application first (POST /api/v1/guides/apply)", nil)
			return "", false
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load guide profile", nil)
		return "", false
	}
	return id.UserID, true
}

type availabilityRequest struct {
	Online *bool `json:"online"`
}

// SetAvailability handles POST /api/v1/me/guide/availability (auth required,
// guide profile must exist): go online/offline (spec §13.4). Online state is
// ephemeral — a Redis marker with a TTL; the client must heartbeat (re-POST)
// within ttl_seconds to stay discoverable as "available now".
func (h *Handler) SetAvailability(w http.ResponseWriter, r *http.Request) {
	var req availabilityRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Online == nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "online is required", nil)
		return
	}
	guideID, ok := h.requireGuideProfile(w, r)
	if !ok {
		return
	}
	if err := h.presence.SetOnline(r.Context(), guideID, *req.Online); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not update presence", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"guide_id":    guideID,
		"online":      *req.Online,
		"ttl_seconds": int(availability.PresenceTTL / time.Second),
	})
}

type scheduleWindowRequest struct {
	Weekday   int    `json:"weekday"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Timezone  string `json:"timezone"`
}

type scheduleRequest struct {
	Windows []scheduleWindowRequest `json:"windows"`
}

// PutSchedule handles PUT /api/v1/me/guide/availability/schedule (auth
// required): atomically replaces the guide's recurring weekly schedule
// (spec §10.1). An empty windows array clears the schedule, which makes the
// guide unavailable for dated searches and new bookings.
func (h *Handler) PutSchedule(w http.ResponseWriter, r *http.Request) {
	var req scheduleRequest
	if !decode(w, r, &req) {
		return
	}
	if req.Windows == nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "windows is required (may be empty)", nil)
		return
	}

	windows := make([]availability.WindowInput, 0, len(req.Windows))
	for i, wn := range req.Windows {
		if wn.Weekday < 0 || wn.Weekday > 6 {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION",
				"weekday must be 0 (Sunday) to 6 (Saturday)", map[string]int{"window": i})
			return
		}
		startMin, err := availability.ParseClock(wn.StartTime)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), map[string]int{"window": i})
			return
		}
		endMin, err := availability.ParseClock(wn.EndTime)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), map[string]int{"window": i})
			return
		}
		if endMin <= startMin {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION",
				"end_time must be after start_time (overnight windows are not supported; split at midnight)",
				map[string]int{"window": i})
			return
		}
		tz := strings.TrimSpace(wn.Timezone)
		if tz == "" {
			tz = "Africa/Accra"
		}
		if _, err := time.LoadLocation(tz); err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION",
				"unknown timezone (IANA name, e.g. Africa/Accra)", map[string]int{"window": i})
			return
		}
		windows = append(windows, availability.WindowInput{
			Weekday: wn.Weekday, StartMin: startMin, EndMin: endMin, Timezone: tz,
		})
	}

	guideID, ok := h.requireGuideProfile(w, r)
	if !ok {
		return
	}
	if err := h.avail.ReplaceSchedule(r.Context(), guideID, windows); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not replace schedule", nil)
		return
	}
	schedule, err := h.avail.ListWindowsJSON(r.Context(), guideID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not reload schedule", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"schedule": schedule})
}

type timeOffRequest struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
	Reason   *string   `json:"reason"`
}

// AddTimeOff handles POST /api/v1/me/guide/availability/time-off (auth
// required): record a one-off unavailability block. Time off wins over the
// weekly schedule and blocks dated search results and new bookings.
func (h *Handler) AddTimeOff(w http.ResponseWriter, r *http.Request) {
	var req timeOffRequest
	if !decode(w, r, &req) {
		return
	}
	if req.StartsAt.IsZero() || req.EndsAt.IsZero() || !req.EndsAt.After(req.StartsAt) {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION",
			"starts_at and ends_at are required and ends_at must be after starts_at", nil)
		return
	}
	guideID, ok := h.requireGuideProfile(w, r)
	if !ok {
		return
	}
	t, err := h.avail.AddTimeOff(r.Context(), guideID, req.StartsAt, req.EndsAt, req.Reason)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not add time off", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"time_off": t})
}

// DeleteTimeOff handles DELETE /api/v1/me/guide/availability/time-off/{id}
// (auth required): remove one of the guide's own time-off rows.
func (h *Handler) DeleteTimeOff(w http.ResponseWriter, r *http.Request) {
	guideID, ok := h.requireGuideProfile(w, r)
	if !ok {
		return
	}
	err := h.avail.DeleteTimeOff(r.Context(), guideID, r.PathValue("id"))
	switch {
	case errors.Is(err, availability.ErrTimeOffNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "time off not found", nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not delete time off", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusNoContent, nil)
}
