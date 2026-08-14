package catalog

import (
	"net/http"

	"proguidegh/api/internal/platform/httpx"
)

// Handler serves the public catalog endpoints (spec §13.2). No
// authentication: this is tourist-facing reference data.
type Handler struct {
	repo *Repository
}

// NewHandler builds the handler.
func NewHandler(repo *Repository) *Handler { return &Handler{repo: repo} }

// Regions handles GET /api/v1/regions.
func (h *Handler) Regions(w http.ResponseWriter, r *http.Request) {
	regions, err := h.repo.ListRegions(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list regions", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"regions": regions})
}

// Specialties handles GET /api/v1/specialties.
func (h *Handler) Specialties(w http.ResponseWriter, r *http.Request) {
	specialties, err := h.repo.ListSpecialties(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list specialties", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"specialties": specialties})
}

// TourPackages handles GET /api/v1/tour-packages: active packages with the
// current effective price from pricing_rules (server-authoritative, spec §14).
func (h *Handler) TourPackages(w http.ResponseWriter, r *http.Request) {
	packages, err := h.repo.ListPackages(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not list tour packages", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"packages": packages})
}
