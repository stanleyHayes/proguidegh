// Package training implements the light LMS (spec §4.3, P8-01, completing
// P2-04): courses with ordered modules/lessons, guide enrollments with
// per-lesson progress, a scored quiz (answers never leave the server) and
// certificates issued when every lesson is complete and the quiz is passed.
package training

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Course is a courses row with content counts.
type Course struct {
	ID                       string    `json:"id"`
	Code                     string    `json:"code"`
	Title                    string    `json:"title"`
	Description              *string   `json:"description"`
	RequiredForCertification bool      `json:"required_for_certification"`
	PassScore                int       `json:"pass_score"`
	Active                   bool      `json:"active"`
	ModuleCount              int       `json:"module_count"`
	LessonCount              int       `json:"lesson_count"`
	QuizLength               int       `json:"quiz_length"`
	CreatedAt                time.Time `json:"created_at"`
}

// Lesson is one course_lessons row.
type Lesson struct {
	ID       string `json:"id"`
	ModuleID string `json:"module_id"`
	Position int    `json:"position"`
	Title    string `json:"title"`
	Body     string `json:"body"`
}

// Module is one course_modules row with its ordered lessons.
type Module struct {
	ID       string   `json:"id"`
	CourseID string   `json:"course_id"`
	Position int      `json:"position"`
	Title    string   `json:"title"`
	Lessons  []Lesson `json:"lessons"`
}

// QuizQuestion is one quiz item. AnswerIndex is admin-only — guide-facing
// responses carry PublicQuestion.
type QuizQuestion struct {
	Question    string   `json:"question"`
	Options     []string `json:"options"`
	AnswerIndex int      `json:"answer_index"`
}

// PublicQuestion is QuizQuestion with the answer stripped.
type PublicQuestion struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

// Enrollment is an enrollments row with derived progress.
type Enrollment struct {
	ID                string     `json:"id"`
	GuideID           string     `json:"guide_id"`
	GuideName         *string    `json:"guide_name,omitempty"`
	CourseID          string     `json:"course_id"`
	Status            string     `json:"status"`
	LessonsDone       int        `json:"lessons_done"`
	LessonsTotal      int        `json:"lessons_total"`
	QuizPassed        bool       `json:"quiz_passed"`
	BestScore         *int       `json:"best_score"`
	CertificateSerial *string    `json:"certificate_serial"`
	CompletedAt       *time.Time `json:"completed_at"`
	CreatedAt         time.Time  `json:"created_at"`
}

// Certificate is one issued certificate with its course identity.
type Certificate struct {
	ID          string    `json:"id"`
	Serial      string    `json:"serial"`
	CourseCode  string    `json:"course_code"`
	CourseTitle string    `json:"course_title"`
	IssuedAt    time.Time `json:"issued_at"`
}

// CourseInput is the admin create payload.
type CourseInput struct {
	Code                     string
	Title                    string
	Description              *string
	RequiredForCertification bool
	PassScore                int
	Quiz                     []QuizQuestion
	Modules                  []ModuleInput
}

// ModuleInput nests lessons under a module for creation.
type ModuleInput struct {
	Title   string
	Lessons []LessonInput
}

// LessonInput is one lesson in a create payload.
type LessonInput struct {
	Title string
	Body  string
}

// Sentinel errors mapped by the handler.
var (
	// ErrNotFound — no such course/enrollment/lesson.
	ErrNotFound = errors.New("training: not found")
	// ErrValidation — malformed content or attempt.
	ErrValidation = errors.New("training: validation failed")
	// ErrConflict — duplicate course code or re-enrollment.
	ErrConflict = errors.New("training: conflict")
)

// Repository is the training data layer.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository builds the repository.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// CreateCourse inserts a course with its modules and lessons atomically.
func (r *Repository) CreateCourse(ctx context.Context, in CourseInput, quizJSON []byte) (Course, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Course{}, fmt.Errorf("training: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var c Course
	err = tx.QueryRow(ctx,
		`INSERT INTO courses (code, title, description, required_for_certification, pass_score, quiz)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, code, title, description, required_for_certification, pass_score, active, created_at`,
		in.Code, in.Title, in.Description, in.RequiredForCertification, in.PassScore, quizJSON).
		Scan(&c.ID, &c.Code, &c.Title, &c.Description, &c.RequiredForCertification,
			&c.PassScore, &c.Active, &c.CreatedAt)
	if err != nil {
		return Course{}, fmt.Errorf("training: insert course: %w", err)
	}

	for mi, m := range in.Modules {
		var moduleID string
		if err := tx.QueryRow(ctx,
			`INSERT INTO course_modules (course_id, position, title) VALUES ($1, $2, $3) RETURNING id`,
			c.ID, mi+1, m.Title).Scan(&moduleID); err != nil {
			return Course{}, fmt.Errorf("training: insert module: %w", err)
		}
		for li, l := range m.Lessons {
			if _, err := tx.Exec(ctx,
				`INSERT INTO course_lessons (module_id, position, title, body) VALUES ($1, $2, $3, $4)`,
				moduleID, li+1, l.Title, l.Body); err != nil {
				return Course{}, fmt.Errorf("training: insert lesson: %w", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Course{}, fmt.Errorf("training: commit: %w", err)
	}
	return c, nil
}

// ListCourses returns courses (newest first) with content counts. Admin
// lists include inactive courses.
func (r *Repository) ListCourses(ctx context.Context, includeInactive bool) ([]Course, error) {
	where := ""
	if !includeInactive {
		where = "WHERE c.active"
	}
	rows, err := r.pool.Query(ctx,
		`SELECT c.id, c.code, c.title, c.description, c.required_for_certification,
		        c.pass_score, c.active, c.created_at,
		        (SELECT COUNT(*)::int FROM course_modules m WHERE m.course_id = c.id),
		        (SELECT COUNT(*)::int FROM course_lessons l
		           JOIN course_modules m ON m.id = l.module_id WHERE m.course_id = c.id),
		        jsonb_array_length(c.quiz)
		 FROM courses c `+where+` ORDER BY c.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("training: list courses: %w", err)
	}
	defer rows.Close()

	var out []Course
	for rows.Next() {
		var c Course
		if err := rows.Scan(&c.ID, &c.Code, &c.Title, &c.Description,
			&c.RequiredForCertification, &c.PassScore, &c.Active, &c.CreatedAt,
			&c.ModuleCount, &c.LessonCount, &c.QuizLength); err != nil {
			return nil, fmt.Errorf("training: scan course: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCourse loads one course with its module/lesson tree and quiz.
func (r *Repository) GetCourse(ctx context.Context, id string) (Course, []Module, []QuizQuestion, error) {
	var c Course
	var quizJSON []byte
	err := r.pool.QueryRow(ctx,
		`SELECT c.id, c.code, c.title, c.description, c.required_for_certification,
		        c.pass_score, c.active, c.created_at, c.quiz,
		        (SELECT COUNT(*)::int FROM course_modules m WHERE m.course_id = c.id),
		        (SELECT COUNT(*)::int FROM course_lessons l
		           JOIN course_modules m ON m.id = l.module_id WHERE m.course_id = c.id),
		        jsonb_array_length(c.quiz)
		 FROM courses c WHERE c.id = $1`, id).
		Scan(&c.ID, &c.Code, &c.Title, &c.Description, &c.RequiredForCertification,
			&c.PassScore, &c.Active, &c.CreatedAt, &quizJSON,
			&c.ModuleCount, &c.LessonCount, &c.QuizLength)
	if errors.Is(err, pgx.ErrNoRows) {
		return Course{}, nil, nil, ErrNotFound
	}
	if err != nil {
		return Course{}, nil, nil, fmt.Errorf("training: get course: %w", err)
	}

	var quiz []QuizQuestion
	if err := json.Unmarshal(quizJSON, &quiz); err != nil {
		return Course{}, nil, nil, fmt.Errorf("training: decode quiz: %w", err)
	}

	rows, err := r.pool.Query(ctx,
		`SELECT m.id, m.course_id, m.position, m.title,
		        l.id, l.module_id, l.position, l.title, l.body
		 FROM course_modules m
		 LEFT JOIN course_lessons l ON l.module_id = m.id
		 WHERE m.course_id = $1
		 ORDER BY m.position, l.position`, id)
	if err != nil {
		return Course{}, nil, nil, fmt.Errorf("training: course tree: %w", err)
	}
	defer rows.Close()

	var modules []Module
	byID := map[string]int{}
	for rows.Next() {
		var m Module
		var lessonID, lessonModuleID, lessonTitle, lessonBody *string
		var lessonPos *int
		if err := rows.Scan(&m.ID, &m.CourseID, &m.Position, &m.Title,
			&lessonID, &lessonModuleID, &lessonPos, &lessonTitle, &lessonBody); err != nil {
			return Course{}, nil, nil, fmt.Errorf("training: scan tree: %w", err)
		}
		idx, ok := byID[m.ID]
		if !ok {
			m.Lessons = []Lesson{}
			modules = append(modules, m)
			idx = len(modules) - 1
			byID[m.ID] = idx
		}
		if lessonID != nil {
			modules[idx].Lessons = append(modules[idx].Lessons, Lesson{
				ID: *lessonID, ModuleID: *lessonModuleID, Position: *lessonPos,
				Title: *lessonTitle, Body: *lessonBody,
			})
		}
	}
	return c, modules, quiz, rows.Err()
}

// CoursePatch carries optional admin updates (nil = unchanged).
type CoursePatch struct {
	Title                    *string
	Description              *string
	RequiredForCertification *bool
	PassScore                *int
	Active                   *bool
}

// UpdateCourse applies a patch, returning the updated row.
func (r *Repository) UpdateCourse(ctx context.Context, id string, p CoursePatch) (Course, error) {
	var c Course
	err := r.pool.QueryRow(ctx,
		`UPDATE courses SET
		   title                      = COALESCE($2, title),
		   description                = COALESCE($3, description),
		   required_for_certification = COALESCE($4, required_for_certification),
		   pass_score                 = COALESCE($5, pass_score),
		   active                     = COALESCE($6, active),
		   updated_at                 = now()
		 WHERE id = $1
		 RETURNING id, code, title, description, required_for_certification, pass_score, active, created_at`,
		id, p.Title, p.Description, p.RequiredForCertification, p.PassScore, p.Active).
		Scan(&c.ID, &c.Code, &c.Title, &c.Description, &c.RequiredForCertification,
			&c.PassScore, &c.Active, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Course{}, ErrNotFound
	}
	if err != nil {
		return Course{}, fmt.Errorf("training: update course: %w", err)
	}
	return c, nil
}

// Enroll inserts the enrollment; ErrConflict when it already exists (the
// UNIQUE guide/course pair makes re-enroll idempotent, P8-01).
func (r *Repository) Enroll(ctx context.Context, guideID, courseID string) (Enrollment, error) {
	var e Enrollment
	err := r.pool.QueryRow(ctx,
		`INSERT INTO enrollments (guide_id, course_id) VALUES ($1, $2)
		 ON CONFLICT (guide_id, course_id) DO NOTHING
		 RETURNING id, guide_id, course_id, status, completed_at, created_at`,
		guideID, courseID).
		Scan(&e.ID, &e.GuideID, &e.CourseID, &e.Status, &e.CompletedAt, &e.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Enrollment{}, ErrConflict
	}
	if err != nil {
		return Enrollment{}, fmt.Errorf("training: enroll: %w", err)
	}
	return e, nil
}

const enrollmentSelect = `
	SELECT e.id, e.guide_id, NULL, e.course_id, e.status, e.completed_at, e.created_at,
	       (SELECT COUNT(*)::int FROM lesson_progress lp WHERE lp.enrollment_id = e.id),
	       (SELECT COUNT(*)::int FROM course_lessons l
	          JOIN course_modules m ON m.id = l.module_id WHERE m.course_id = e.course_id),
	       EXISTS (SELECT 1 FROM quiz_attempts qa WHERE qa.enrollment_id = e.id AND qa.passed),
	       (SELECT MAX(qa.score)::int FROM quiz_attempts qa WHERE qa.enrollment_id = e.id),
	       (SELECT cert.serial FROM certificates cert WHERE cert.enrollment_id = e.id)
	FROM enrollments e`

func scanEnrollment(row interface{ Scan(dest ...any) error }) (Enrollment, error) {
	var e Enrollment
	err := row.Scan(&e.ID, &e.GuideID, &e.GuideName, &e.CourseID, &e.Status,
		&e.CompletedAt, &e.CreatedAt, &e.LessonsDone, &e.LessonsTotal,
		&e.QuizPassed, &e.BestScore, &e.CertificateSerial)
	return e, err
}

// GetEnrollment loads one guide's enrollment for a course with progress.
func (r *Repository) GetEnrollment(ctx context.Context, guideID, courseID string) (Enrollment, error) {
	e, err := scanEnrollment(r.pool.QueryRow(ctx,
		enrollmentSelect+` WHERE e.guide_id = $1 AND e.course_id = $2`, guideID, courseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Enrollment{}, ErrNotFound
	}
	if err != nil {
		return Enrollment{}, fmt.Errorf("training: enrollment: %w", err)
	}
	return e, nil
}

// GetEnrollmentByID loads one enrollment with progress.
func (r *Repository) GetEnrollmentByID(ctx context.Context, id string) (Enrollment, error) {
	e, err := scanEnrollment(r.pool.QueryRow(ctx,
		enrollmentSelect+` WHERE e.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Enrollment{}, ErrNotFound
	}
	if err != nil {
		return Enrollment{}, fmt.Errorf("training: enrollment by id: %w", err)
	}
	return e, nil
}

// EnrollmentForLesson resolves the caller's enrollment for the course a
// lesson belongs to — enrolment is required before progress can be made.
func (r *Repository) EnrollmentForLesson(ctx context.Context, guideID, lessonID string) (enrollmentID, courseID string, err error) {
	err = r.pool.QueryRow(ctx,
		`SELECT e.id, e.course_id FROM enrollments e
		 JOIN course_modules m ON m.course_id = e.course_id
		 JOIN course_lessons l ON l.module_id = m.id
		 WHERE e.guide_id = $1 AND l.id = $2`, guideID, lessonID).Scan(&enrollmentID, &courseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("training: enrollment for lesson: %w", err)
	}
	return enrollmentID, courseID, nil
}

// ListEnrollments returns a guide's enrollments, newest first.
func (r *Repository) ListEnrollments(ctx context.Context, guideID string) ([]Enrollment, error) {
	rows, err := r.pool.Query(ctx,
		enrollmentSelect+` WHERE e.guide_id = $1 ORDER BY e.created_at DESC`, guideID)
	if err != nil {
		return nil, fmt.Errorf("training: my enrollments: %w", err)
	}
	defer rows.Close()
	var out []Enrollment
	for rows.Next() {
		e, err := scanEnrollment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Roster returns a course's enrollments with guide names (admin).
func (r *Repository) Roster(ctx context.Context, courseID string) ([]Enrollment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT e.id, e.guide_id, gp.public_name, e.course_id, e.status, e.completed_at, e.created_at,
		       (SELECT COUNT(*)::int FROM lesson_progress lp WHERE lp.enrollment_id = e.id),
		       (SELECT COUNT(*)::int FROM course_lessons l
		          JOIN course_modules m ON m.id = l.module_id WHERE m.course_id = e.course_id),
		       EXISTS (SELECT 1 FROM quiz_attempts qa WHERE qa.enrollment_id = e.id AND qa.passed),
		       (SELECT MAX(qa.score)::int FROM quiz_attempts qa WHERE qa.enrollment_id = e.id),
		       (SELECT cert.serial FROM certificates cert WHERE cert.enrollment_id = e.id)
		FROM enrollments e
		JOIN guide_profiles gp ON gp.user_id = e.guide_id
		WHERE e.course_id = $1 ORDER BY e.created_at DESC`, courseID)
	if err != nil {
		return nil, fmt.Errorf("training: roster: %w", err)
	}
	defer rows.Close()
	var out []Enrollment
	for rows.Next() {
		e, err := scanEnrollment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// CompletedLessonIDs returns the lessons one enrollment has completed.
func (r *Repository) CompletedLessonIDs(ctx context.Context, enrollmentID string) (map[string]bool, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT lesson_id FROM lesson_progress WHERE enrollment_id = $1`, enrollmentID)
	if err != nil {
		return nil, fmt.Errorf("training: progress: %w", err)
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("training: scan progress: %w", err)
		}
		out[id] = true
	}
	return out, rows.Err()
}

// CompleteLesson marks one lesson done — only when it belongs to the
// enrollment's course (ErrNotFound otherwise). Idempotent via the PK.
func (r *Repository) CompleteLesson(ctx context.Context, enrollmentID, lessonID string) error {
	tag, err := r.pool.Exec(ctx,
		`INSERT INTO lesson_progress (enrollment_id, lesson_id)
		 SELECT $1, l.id FROM course_lessons l
		 JOIN course_modules m ON m.id = l.module_id
		 JOIN enrollments e ON e.course_id = m.course_id
		 WHERE l.id = $2 AND e.id = $1
		 ON CONFLICT DO NOTHING`, enrollmentID, lessonID)
	if err != nil {
		return fmt.Errorf("training: complete lesson: %w", err)
	}
	if tag.RowsAffected() == 0 {
		// Either already done or not part of the course — distinguish.
		var exists bool
		if err := r.pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM lesson_progress WHERE enrollment_id = $1 AND lesson_id = $2)`,
			enrollmentID, lessonID).Scan(&exists); err != nil {
			return fmt.Errorf("training: progress check: %w", err)
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

// RecordQuizAttempt stores one scored attempt.
func (r *Repository) RecordQuizAttempt(ctx context.Context, enrollmentID string, score int, passed bool) error {
	if _, err := r.pool.Exec(ctx,
		`INSERT INTO quiz_attempts (enrollment_id, score, passed) VALUES ($1, $2, $3)`,
		enrollmentID, score, passed); err != nil {
		return fmt.Errorf("training: quiz attempt: %w", err)
	}
	return nil
}

// CompleteEnrollment marks the enrollment completed and issues the
// certificate atomically. The certificate UNIQUE(enrollment_id) makes a
// double completion a no-op that still returns success.
func (r *Repository) CompleteEnrollment(ctx context.Context, enrollmentID, serial string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("training: begin completion: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`UPDATE enrollments SET status = 'completed', completed_at = now(), updated_at = now()
		 WHERE id = $1`, enrollmentID); err != nil {
		return "", fmt.Errorf("training: complete enrollment: %w", err)
	}
	var existing string
	err = tx.QueryRow(ctx,
		`INSERT INTO certificates (enrollment_id, serial) VALUES ($1, $2)
		 ON CONFLICT (enrollment_id) DO NOTHING
		 RETURNING serial`, enrollmentID, serial).Scan(&existing)
	if errors.Is(err, pgx.ErrNoRows) {
		// Already issued — return the existing serial.
		if err := tx.QueryRow(ctx,
			`SELECT serial FROM certificates WHERE enrollment_id = $1`, enrollmentID).Scan(&existing); err != nil {
			return "", fmt.Errorf("training: existing certificate: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("training: issue certificate: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("training: commit completion: %w", err)
	}
	return existing, nil
}

// ListCertificates returns a guide's certificates, newest first.
func (r *Repository) ListCertificates(ctx context.Context, guideID string) ([]Certificate, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT cert.id, cert.serial, c.code, c.title, cert.issued_at
		 FROM certificates cert
		 JOIN enrollments e ON e.id = cert.enrollment_id
		 JOIN courses c ON c.id = e.course_id
		 WHERE e.guide_id = $1 ORDER BY cert.issued_at DESC`, guideID)
	if err != nil {
		return nil, fmt.Errorf("training: certificates: %w", err)
	}
	defer rows.Close()
	var out []Certificate
	for rows.Next() {
		var cert Certificate
		if err := rows.Scan(&cert.ID, &cert.Serial, &cert.CourseCode, &cert.CourseTitle, &cert.IssuedAt); err != nil {
			return nil, fmt.Errorf("training: scan certificate: %w", err)
		}
		out = append(out, cert)
	}
	return out, rows.Err()
}
