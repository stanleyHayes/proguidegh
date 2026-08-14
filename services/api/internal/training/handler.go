package training

import (
	"encoding/json"
	"errors"
	"net/http"

	"proguidegh/api/internal/platform/httpx"
	"proguidegh/api/internal/platform/rbac"
)

// Handler serves the admin LMS endpoints and the guide self-service
// training endpoints. Permission scoping is applied at the router
// (training.manage for admin).
type Handler struct {
	svc *Service
}

// NewHandler builds the handler.
func NewHandler(svc *Service) *Handler { return &Handler{svc: svc} }

func actorID(r *http.Request) string {
	id, _ := rbac.FromContext(r.Context())
	return id.UserID
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", "malformed JSON body", nil)
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, r *http.Request, err error, fallback string) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "NOT_FOUND", "not found", nil)
	case errors.Is(err, ErrValidation):
		httpx.WriteError(w, r, http.StatusBadRequest, "VALIDATION", err.Error(), nil)
	case errors.Is(err, ErrConflict):
		httpx.WriteError(w, r, http.StatusConflict, "CONFLICT", err.Error(), nil)
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "INTERNAL", fallback, nil)
	}
}

// --- Admin (training.manage) ----------------------------------------------

type createCourseRequest struct {
	Code                     string          `json:"code"`
	Title                    string          `json:"title"`
	Description              *string         `json:"description"`
	RequiredForCertification bool            `json:"required_for_certification"`
	PassScore                int             `json:"pass_score"`
	Quiz                     []QuizQuestion  `json:"quiz"`
	Modules                  []moduleRequest `json:"modules"`
}

type moduleRequest struct {
	Title   string          `json:"title"`
	Lessons []lessonRequest `json:"lessons"`
}

type lessonRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// CreateCourse handles POST /api/v1/admin/training/courses.
func (h *Handler) CreateCourse(w http.ResponseWriter, r *http.Request) {
	var req createCourseRequest
	if !decodeBody(w, r, &req) {
		return
	}
	in := CourseInput{
		Code:                     req.Code,
		Title:                    req.Title,
		Description:              req.Description,
		RequiredForCertification: req.RequiredForCertification,
		PassScore:                req.PassScore,
		Quiz:                     req.Quiz,
	}
	for _, m := range req.Modules {
		mod := ModuleInput{Title: m.Title}
		for _, l := range m.Lessons {
			mod.Lessons = append(mod.Lessons, LessonInput{Title: l.Title, Body: l.Body})
		}
		in.Modules = append(in.Modules, mod)
	}
	c, err := h.svc.CreateCourse(r.Context(), in, actorID(r))
	if err != nil {
		writeError(w, r, err, "could not create the course")
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"course": c})
}

// ListCourses handles GET /api/v1/admin/training/courses.
func (h *Handler) ListCourses(w http.ResponseWriter, r *http.Request) {
	courses, err := h.svc.ListCourses(r.Context())
	if err != nil {
		writeError(w, r, err, "could not list courses")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"courses": courses})
}

// AdminCourseDetail handles GET /api/v1/admin/training/courses/{id} —
// includes quiz answers (admin-only surface).
func (h *Handler) AdminCourseDetail(w http.ResponseWriter, r *http.Request) {
	c, modules, quiz, err := h.svc.CourseDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, r, err, "could not load the course")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"course": c, "modules": modules, "quiz": quiz,
	})
}

type patchCourseRequest struct {
	Title                    *string `json:"title"`
	Description              *string `json:"description"`
	RequiredForCertification *bool   `json:"required_for_certification"`
	PassScore                *int    `json:"pass_score"`
	Active                   *bool   `json:"active"`
}

// PatchCourse handles PATCH /api/v1/admin/training/courses/{id}.
func (h *Handler) PatchCourse(w http.ResponseWriter, r *http.Request) {
	var req patchCourseRequest
	if !decodeBody(w, r, &req) {
		return
	}
	c, err := h.svc.UpdateCourse(r.Context(), r.PathValue("id"), CoursePatch{
		Title:                    req.Title,
		Description:              req.Description,
		RequiredForCertification: req.RequiredForCertification,
		PassScore:                req.PassScore,
		Active:                   req.Active,
	}, actorID(r))
	if err != nil {
		writeError(w, r, err, "could not update the course")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"course": c})
}

// Roster handles GET /api/v1/admin/training/courses/{id}/enrollments.
func (h *Handler) Roster(w http.ResponseWriter, r *http.Request) {
	roster, err := h.svc.Roster(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, r, err, "could not load the roster")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"enrollments": roster})
}

// --- Guide self-service ----------------------------------------------------

// MyCourses handles GET /api/v1/me/training/courses — active courses with
// the caller's enrollment/progress overlaid.
func (h *Handler) MyCourses(w http.ResponseWriter, r *http.Request) {
	courses, byCourse, err := h.svc.MyCourses(r.Context(), actorID(r))
	if err != nil {
		writeError(w, r, err, "could not load courses")
		return
	}
	type courseWithEnrollment struct {
		Course     Course      `json:"course"`
		Enrollment *Enrollment `json:"enrollment"`
	}
	out := []courseWithEnrollment{}
	for _, c := range courses {
		item := courseWithEnrollment{Course: c}
		if e, ok := byCourse[c.ID]; ok {
			eCopy := e
			item.Enrollment = &eCopy
		}
		out = append(out, item)
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"courses": out})
}

// GuideCourseDetail handles GET /api/v1/me/training/courses/{id} — quiz
// answers are stripped; completed lessons are flagged.
func (h *Handler) GuideCourseDetail(w http.ResponseWriter, r *http.Request) {
	c, modules, quiz, enrollment, done, err := h.svc.GuideCourseDetail(r.Context(), actorID(r), r.PathValue("id"))
	if err != nil {
		writeError(w, r, err, "could not load the course")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"course": c, "modules": modules, "quiz": quiz,
		"enrollment": enrollment, "completed_lessons": done,
	})
}

// Enroll handles POST /api/v1/me/training/courses/{id}/enroll.
func (h *Handler) Enroll(w http.ResponseWriter, r *http.Request) {
	e, err := h.svc.Enroll(r.Context(), actorID(r), r.PathValue("id"))
	if err != nil {
		writeError(w, r, err, "could not enroll")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"enrollment": e})
}

// CompleteLesson handles POST /api/v1/me/training/lessons/{id}/complete.
func (h *Handler) CompleteLesson(w http.ResponseWriter, r *http.Request) {
	e, err := h.svc.CompleteLesson(r.Context(), actorID(r), r.PathValue("id"))
	if err != nil {
		writeError(w, r, err, "could not complete the lesson")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"enrollment": e})
}

type quizRequest struct {
	Answers []int `json:"answers"`
}

// SubmitQuiz handles POST /api/v1/me/training/courses/{id}/quiz — scored
// server-side against the stored quiz (P8-01).
func (h *Handler) SubmitQuiz(w http.ResponseWriter, r *http.Request) {
	var req quizRequest
	if !decodeBody(w, r, &req) {
		return
	}
	e, score, passed, err := h.svc.SubmitQuiz(r.Context(), actorID(r), r.PathValue("id"), req.Answers)
	if err != nil {
		writeError(w, r, err, "could not score the quiz")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"score": score, "passed": passed, "enrollment": e,
	})
}

// Certificates handles GET /api/v1/me/training/certificates.
func (h *Handler) Certificates(w http.ResponseWriter, r *http.Request) {
	certs, err := h.svc.Certificates(r.Context(), actorID(r))
	if err != nil {
		writeError(w, r, err, "could not load certificates")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"certificates": certs})
}
