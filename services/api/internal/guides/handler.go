package guides

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"proguidegh/api/internal/availability"
	"proguidegh/api/internal/catalog"
	"proguidegh/api/internal/certification"
	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
	"proguidegh/api/internal/platform/storage"
)

// documentTypes are the certification evidence kinds accepted in V1
// (spec §5); stored as TEXT so the dictionary can grow without migrations.
var documentTypes = map[string]bool{
	"national_id":      true,
	"passport":         true,
	"drivers_license":  true,
	"background_check": true,
	"certification":    true,
	"insurance":        true,
	"other":            true,
}

// Handler serves the guide endpoints.
type Handler struct {
	repo     *Repository
	search   *SearchRepository
	avail    *availability.Repository
	presence *availability.Presence
	catalog  *catalog.Repository
	store    storage.Store
	cert     *certification.Service
	onApply  func(userID string) // rbac cache invalidation hook
}

// NewHandler builds the handler. cert drives the certification pipeline
// (spec §5): Apply opens a case through it. onApply is invoked after a new
// application so the permission cache can be invalidated; it may be nil.
// search/avail/presence/catalog power GET /guides/search and the
// /me/guide/availability endpoints (Phase 3).
func NewHandler(repo *Repository, search *SearchRepository, avail *availability.Repository,
	presence *availability.Presence, cat *catalog.Repository,
	store storage.Store, cert *certification.Service, onApply func(userID string)) *Handler {
	return &Handler{repo: repo, search: search, avail: avail, presence: presence,
		catalog: cat, store: store, cert: cert, onApply: onApply}
}

type applyRequest struct {
	PublicName string  `json:"public_name"`
	Bio        *string `json:"bio"`
	RegionID   *string `json:"region_id"`
}

// Apply handles POST /api/v1/guides/apply (auth required). Creates the
// guide_profiles shell and ensures the caller holds the guide_applicant
// role. Idempotent per user: repeats return the existing profile.
func (h *Handler) Apply(w http.ResponseWriter, r *http.Request) {
	var req applyRequest
	if !decode(w, r, &req) {
		return
	}
	req.PublicName = strings.TrimSpace(req.PublicName)
	if req.PublicName == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "public_name is required", nil)
		return
	}

	id, _ := rbac.FromContext(r.Context())
	p, err := h.repo.Apply(r.Context(), id.UserID, req.PublicName, req.Bio, req.RegionID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not create application", nil)
		return
	}
	if err := h.repo.AddRoleIfMissing(r.Context(), id.UserID, "guide_applicant"); err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not update roles", nil)
		return
	}
	// The application opens the certification pipeline (spec §5): a case in
	// APPLIED with its opening event. Idempotent like the profile.
	c, err := h.cert.OpenCase(r.Context(), id.UserID, id.UserID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not open certification case", nil)
		return
	}
	if h.onApply != nil {
		h.onApply(id.UserID)
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"guide_profile":      p,
		"certification_case": c,
	})
}

type documentRequest struct {
	Type        string     `json:"type"`
	ContentType string     `json:"content_type"`
	ExpiresAt   *time.Time `json:"expires_at"`
}

// RegisterDocument handles POST /api/v1/guides/documents (auth required,
// guide profile must exist). Registers the document metadata and returns a
// short-lived signed upload URL (spec §16.4 — private storage, signed URLs
// only).
func (h *Handler) RegisterDocument(w http.ResponseWriter, r *http.Request) {
	var req documentRequest
	if !decode(w, r, &req) {
		return
	}
	if !documentTypes[req.Type] {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "unsupported document type", nil)
		return
	}
	if req.ContentType == "" {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "content_type is required", nil)
		return
	}

	id, _ := rbac.FromContext(r.Context())
	if _, err := h.repo.GetByUser(r.Context(), id.UserID); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpx.WriteError(w, r, http.StatusConflict, "NO_GUIDE_PROFILE",
				"submit an application first (POST /api/v1/guides/apply)", nil)
			return
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load guide profile", nil)
		return
	}

	key, uploadURL, err := h.store.PresignUpload(r.Context(), "guide-documents/"+id.UserID, req.ContentType)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not create upload URL", nil)
		return
	}
	doc, err := h.repo.CreateDocument(r.Context(), id.UserID, req.Type, key, req.ExpiresAt)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not register document", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"document":   doc,
		"upload_url": uploadURL,
	})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
		return false
	}
	return true
}

// Me handles GET /api/v1/me/guide (auth required, own record): the guide
// dashboard aggregate — profile, current certification case, outstanding
// requirements and document list (spec §4.2).
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	id, _ := rbac.FromContext(r.Context())
	p, err := h.repo.GetByUser(r.Context(), id.UserID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NO_GUIDE_PROFILE",
			"submit an application first (POST /api/v1/guides/apply)", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load guide profile", nil)
		return
	}

	var certCase *certification.Case
	outstanding := []string{}
	c, err := h.cert.CurrentCase(r.Context(), id.UserID)
	switch {
	case err == nil:
		certCase = &c
		outstanding, err = h.cert.Outstanding(r.Context(), id.UserID, c.Status)
		if err != nil {
			httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not compute requirements", nil)
			return
		}
	case errors.Is(err, certification.ErrCaseNotFound):
		// Profile without a case is a pre-Phase-2 leftover; report it as null.
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load certification case", nil)
		return
	}

	docs, err := h.cert.Documents(r.Context(), id.UserID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load documents", nil)
		return
	}
	languages, err := h.repo.ListLanguages(r.Context(), id.UserID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load languages", nil)
		return
	}
	specialties, err := h.repo.ListSpecialties(r.Context(), id.UserID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load specialties", nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"profile":                  p,
		"certification":            certCase,
		"outstanding_requirements": outstanding,
		"documents":                docs,
		"languages":                languages,
		"specialties":              specialties,
	})
}

// MeCertification handles GET /api/v1/me/guide/certification (auth required,
// own record): the pipeline detail — current case plus its immutable event
// history.
func (h *Handler) MeCertification(w http.ResponseWriter, r *http.Request) {
	id, _ := rbac.FromContext(r.Context())
	c, err := h.cert.CurrentCase(r.Context(), id.UserID)
	if errors.Is(err, certification.ErrCaseNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NO_CERTIFICATION_CASE",
			"no certification case; apply first (POST /api/v1/guides/apply)", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load certification case", nil)
		return
	}
	events, err := h.cert.Events(r.Context(), c.ID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load events", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"case": c, "events": events})
}

type patchProfileRequest struct {
	PublicName *string `json:"public_name"`
	Bio        *string `json:"bio"`
	RegionID   *string `json:"region_id"`
	Latitude   *string `json:"latitude"`
	Longitude  *string `json:"longitude"`
	Languages  []struct {
		Code        string `json:"code"`
		Proficiency string `json:"proficiency"`
	} `json:"languages"`
	SpecialtyIDs []string `json:"specialty_ids"`
}

// PatchProfile handles PATCH /api/v1/me/guide/profile (auth required, own
// record). Scalar fields update in place; languages/specialty_ids replace
// the guide_languages/guide_specialties rows atomically.
func (h *Handler) PatchProfile(w http.ResponseWriter, r *http.Request) {
	var req patchProfileRequest
	if !decode(w, r, &req) {
		return
	}

	patch := ProfilePatch{
		Bio:          req.Bio,
		RegionID:     req.RegionID,
		SpecialtyIDs: req.SpecialtyIDs,
	}
	// Operating-base coordinates (radius search, §10.1): must move together
	// and stay within valid ranges.
	if (req.Latitude == nil) != (req.Longitude == nil) {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION",
			"latitude and longitude must be provided together", nil)
		return
	}
	if req.Latitude != nil {
		lat, err1 := strconv.ParseFloat(*req.Latitude, 64)
		lng, err2 := strconv.ParseFloat(*req.Longitude, 64)
		if err1 != nil || err2 != nil || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION",
				"latitude/longitude out of range", nil)
			return
		}
		patch.Latitude, patch.Longitude = req.Latitude, req.Longitude
	}
	if req.PublicName != nil {
		name := strings.TrimSpace(*req.PublicName)
		if name == "" {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "public_name must not be empty", nil)
			return
		}
		patch.PublicName = &name
	}
	if req.Languages != nil {
		patch.Languages = make([]Language, 0, len(req.Languages))
		for _, l := range req.Languages {
			code := strings.ToLower(strings.TrimSpace(l.Code))
			if code == "" {
				httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "language code is required", nil)
				return
			}
			patch.Languages = append(patch.Languages, Language{Code: code, Proficiency: l.Proficiency})
		}
	}

	id, _ := rbac.FromContext(r.Context())
	err := h.repo.UpdateProfile(r.Context(), id.UserID, patch)
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "NO_GUIDE_PROFILE",
			"submit an application first (POST /api/v1/guides/apply)", nil)
		return
	case errors.Is(err, ErrUnknownRegion), errors.Is(err, ErrUnknownLanguage),
		errors.Is(err, ErrUnknownSpecialty), errors.Is(err, ErrInvalidProficiency):
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	case err != nil:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not update profile", nil)
		return
	}

	p, err := h.repo.GetByUser(r.Context(), id.UserID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not reload profile", nil)
		return
	}
	languages, err := h.repo.ListLanguages(r.Context(), id.UserID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not reload languages", nil)
		return
	}
	specialties, err := h.repo.ListSpecialties(r.Context(), id.UserID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not reload specialties", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"profile":     p,
		"languages":   languages,
		"specialties": specialties,
	})
}

// PublicDetail handles GET /api/v1/guides/{id} (unauthenticated): the public
// guide detail, gated by the §10.2 availability rules. Guides that fail the
// gate are indistinguishable from unknown ids (404) so certification state
// never leaks.
func (h *Handler) PublicDetail(w http.ResponseWriter, r *http.Request) {
	guideID := r.PathValue("id")
	v, err := h.repo.GetPublicView(r.Context(), guideID)
	if errors.Is(err, ErrNotFound) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "guide not found", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load guide", nil)
		return
	}

	docsValid, _, err := h.cert.DocumentsValid(r.Context(), guideID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not validate documents", nil)
		return
	}
	caseStatus := ""
	if v.CaseStatus != nil {
		caseStatus = *v.CaseStatus
	}
	if !PubliclyVisible(VisibilityInput{
		CaseStatus:     caseStatus,
		UserStatus:     v.UserStatus,
		GuideStatus:    v.GuideStatus,
		DocumentsValid: docsValid,
	}) {
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "guide not found", nil)
		return
	}

	languages, err := h.repo.ListLanguages(r.Context(), guideID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load languages", nil)
		return
	}
	specialties, err := h.repo.ListSpecialties(r.Context(), guideID)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load specialties", nil)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"guide": map[string]any{
			"user_id":      v.UserID,
			"public_name":  v.PublicName,
			"bio":          v.Bio,
			"rating_avg":   v.RatingAvg,
			"rating_count": v.RatingCount,
			"elite_status": v.EliteStatus,
			"region_id":    v.RegionID,
			"region_name":  v.RegionName,
		},
		"languages":   languages,
		"specialties": specialties,
	})
}
