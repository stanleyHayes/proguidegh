package reporting

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"proguidegh/api/internal/platform/audit"
	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// Handler serves executive KPIs, operational reports and the audit viewer.
// Permission scoping is applied at the router (reports.read /
// reports.export / audit.read).
type Handler struct {
	repo  *Repository
	audit *audit.Recorder
}

// NewHandler builds the handler. audit may be nil in tests.
func NewHandler(repo *Repository, auditor *audit.Recorder) *Handler {
	return &Handler{repo: repo, audit: auditor}
}

func actorID(r *http.Request) string {
	id, _ := rbac.FromContext(r.Context())
	return id.UserID
}

// KPIs handles GET /api/v1/admin/reports/kpis (P8-02).
func (h *Handler) KPIs(w http.ResponseWriter, r *http.Request) {
	k, err := h.repo.KPIs(r.Context())
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not compute KPIs", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"kpis": k})
}

// parseRange reads ?from&to as YYYY-MM-DD, defaulting to the last 30 days;
// the end date is inclusive (internally exclusive +1 day).
func parseRange(r *http.Request) (time.Time, time.Time, error) {
	q := r.URL.Query()
	to := time.Now()
	from := to.AddDate(0, 0, -30)
	var err error
	if raw := strings.TrimSpace(q.Get("from")); raw != "" {
		from, err = time.Parse("2006-01-02", raw)
		if err != nil {
			return from, to, fmt.Errorf("from must be YYYY-MM-DD")
		}
	}
	if raw := strings.TrimSpace(q.Get("to")); raw != "" {
		to, err = time.Parse("2006-01-02", raw)
		if err != nil {
			return from, to, fmt.Errorf("to must be YYYY-MM-DD")
		}
	}
	return from, to.AddDate(0, 0, 1), nil
}

// Bookings handles GET /api/v1/admin/reports/bookings?from&to (P8-02).
func (h *Handler) Bookings(w http.ResponseWriter, r *http.Request) {
	from, to, err := parseRange(r)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	rep, err := h.repo.BookingsReport(r.Context(), from, to)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not build the bookings report", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"report": rep})
}

// ExportBookings handles GET /api/v1/admin/reports/bookings/export?from&to
// — the permitted CSV export (reports.export, P8-02; audited).
func (h *Handler) ExportBookings(w http.ResponseWriter, r *http.Request) {
	from, to, err := parseRange(r)
	if err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
		return
	}
	rows, err := h.repo.BookingsExport(r.Context(), from, to)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not export bookings", nil)
		return
	}

	var b strings.Builder
	b.WriteString("reference,tourist_email,guide_name,package_title,starts_at,status,amount,currency\n")
	for _, row := range rows {
		guide := ""
		if row.GuideName != nil {
			guide = *row.GuideName
		}
		amount := ""
		currency := ""
		if row.AmountMinor != nil {
			amount = fmt.Sprintf("%.2f", float64(*row.AmountMinor)/100)
		}
		if row.Currency != nil {
			currency = *row.Currency
		}
		fmt.Fprintf(&b, "%s,%s,%s,%s,%s,%s,%s,%s\n",
			row.Reference, row.TouristEmail, csvField(guide), csvField(row.PackageTitle),
			row.StartsAt.Format(time.RFC3339), row.Status, amount, currency)
	}

	if h.audit != nil {
		_ = h.audit.Record(r.Context(), audit.Entry{
			ActorID:    actorID(r),
			Action:     "reports.bookings_export",
			EntityType: "report",
			After:      map[string]any{"rows": len(rows), "from": from.Format("2006-01-02"), "to": to.Format("2006-01-02")},
		})
	}

	name := fmt.Sprintf("bookings-%s-%s.csv", from.Format("2006-01-02"), to.Format("2006-01-02"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

func csvField(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

// Audit handles GET /api/v1/admin/audit-logs with actor/entity/date
// filters, offset-paginated (audit.read, P8-04).
func (h *Handler) Audit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	f := AuditFilter{
		ActorID:    strings.TrimSpace(q.Get("actor_id")),
		Action:     strings.TrimSpace(q.Get("action")),
		EntityType: strings.TrimSpace(q.Get("entity_type")),
		EntityID:   strings.TrimSpace(q.Get("entity_id")),
	}
	if raw := strings.TrimSpace(q.Get("from")); raw != "" {
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "from must be YYYY-MM-DD", nil)
			return
		}
		f.From = &t
	}
	if raw := strings.TrimSpace(q.Get("to")); raw != "" {
		t, err := time.Parse("2006-01-02", raw)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "to must be YYYY-MM-DD", nil)
			return
		}
		t = t.AddDate(0, 0, 1)
		f.To = &t
	}

	entries, total, err := h.repo.ListAudit(r.Context(), f, limit, offset)
	if err != nil {
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", "could not load audit logs", nil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"entries": entries, "total": total})
}
