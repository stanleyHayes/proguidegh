package training

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"proguidegh/api/internal/platform/audit"
	pauth "proguidegh/api/internal/platform/auth"
)

// Service is the training application service.
type Service struct {
	repo  *Repository
	audit *audit.Recorder
}

// NewService builds the service. audit may be nil in tests.
func NewService(repo *Repository, auditor *audit.Recorder) *Service {
	return &Service{repo: repo, audit: auditor}
}

func (s *Service) record(ctx context.Context, actorID, action, entityID string, after any) {
	if s.audit != nil {
		_ = s.audit.Record(ctx, audit.Entry{
			ActorID:    actorID,
			Action:     action,
			EntityType: "course",
			EntityID:   entityID,
			After:      after,
		})
	}
}

// CreateCourse validates and stores a course (admin, training.manage).
func (s *Service) CreateCourse(ctx context.Context, in CourseInput, actorID string) (Course, error) {
	in.Code = strings.TrimSpace(in.Code)
	in.Title = strings.TrimSpace(in.Title)
	if in.Code == "" || in.Title == "" {
		return Course{}, fmt.Errorf("%w: code and title are required", ErrValidation)
	}
	if len(in.Modules) == 0 {
		return Course{}, fmt.Errorf("%w: at least one module is required", ErrValidation)
	}
	for _, m := range in.Modules {
		if strings.TrimSpace(m.Title) == "" || len(m.Lessons) == 0 {
			return Course{}, fmt.Errorf("%w: every module needs a title and at least one lesson", ErrValidation)
		}
	}
	if in.PassScore < 0 || in.PassScore > 100 {
		return Course{}, fmt.Errorf("%w: pass_score must be 0..100", ErrValidation)
	}
	if in.PassScore == 0 {
		in.PassScore = 80
	}
	for _, q := range in.Quiz {
		if strings.TrimSpace(q.Question) == "" || len(q.Options) < 2 ||
			q.AnswerIndex < 0 || q.AnswerIndex >= len(q.Options) {
			return Course{}, fmt.Errorf("%w: each quiz question needs 2+ options and a valid answer_index", ErrValidation)
		}
	}
	quizJSON, err := json.Marshal(in.Quiz)
	if err != nil {
		return Course{}, fmt.Errorf("training: encode quiz: %w", err)
	}
	c, err := s.repo.CreateCourse(ctx, in, quizJSON)
	if err != nil {
		if strings.Contains(err.Error(), "courses_code_key") {
			return Course{}, fmt.Errorf("%w: course code already exists", ErrConflict)
		}
		return Course{}, err
	}
	s.record(ctx, actorID, "training.course_created", c.ID, map[string]any{"code": c.Code, "title": c.Title})
	return c, nil
}

// ListCourses returns the admin course list.
func (s *Service) ListCourses(ctx context.Context) ([]Course, error) {
	return s.repo.ListCourses(ctx, true)
}

// CourseDetail returns the full tree with answers (admin).
func (s *Service) CourseDetail(ctx context.Context, id string) (Course, []Module, []QuizQuestion, error) {
	return s.repo.GetCourse(ctx, id)
}

// UpdateCourse applies the admin patch.
func (s *Service) UpdateCourse(ctx context.Context, id string, p CoursePatch, actorID string) (Course, error) {
	c, err := s.repo.UpdateCourse(ctx, id, p)
	if err != nil {
		return Course{}, err
	}
	s.record(ctx, actorID, "training.course_updated", c.ID, map[string]any{
		"active": c.Active, "pass_score": c.PassScore,
	})
	return c, nil
}

// Roster returns a course's enrollments (admin).
func (s *Service) Roster(ctx context.Context, courseID string) ([]Enrollment, error) {
	return s.repo.Roster(ctx, courseID)
}

// MyCourses returns active courses with the caller's enrollment overlaid
// (nil enrollment = not enrolled).
func (s *Service) MyCourses(ctx context.Context, guideID string) ([]Course, map[string]Enrollment, error) {
	courses, err := s.repo.ListCourses(ctx, false)
	if err != nil {
		return nil, nil, err
	}
	enrollments, err := s.repo.ListEnrollments(ctx, guideID)
	if err != nil {
		return nil, nil, err
	}
	byCourse := map[string]Enrollment{}
	for _, e := range enrollments {
		byCourse[e.CourseID] = e
	}
	return courses, byCourse, nil
}

// GuideCourseDetail returns the tree for a guide: the quiz arrives with
// answer_index stripped (answers never leave the server, P8-01).
func (s *Service) GuideCourseDetail(ctx context.Context, guideID, courseID string) (Course, []Module, []PublicQuestion, *Enrollment, map[string]bool, error) {
	c, modules, quiz, err := s.repo.GetCourse(ctx, courseID)
	if err != nil {
		return Course{}, nil, nil, nil, nil, err
	}
	public := make([]PublicQuestion, 0, len(quiz))
	for _, q := range quiz {
		public = append(public, PublicQuestion{Question: q.Question, Options: q.Options})
	}
	enrollment, err := s.repo.GetEnrollment(ctx, guideID, courseID)
	if err != nil && err != ErrNotFound {
		return Course{}, nil, nil, nil, nil, err
	}
	var enrollmentArg *Enrollment
	var done map[string]bool
	if err == nil {
		enrollmentArg = &enrollment
		done, err = s.repo.CompletedLessonIDs(ctx, enrollment.ID)
		if err != nil {
			return Course{}, nil, nil, nil, nil, err
		}
	}
	return c, modules, public, enrollmentArg, done, nil
}

// Enroll registers the caller on an active course; re-enrolling returns
// the existing enrollment (idempotent, P8-01).
func (s *Service) Enroll(ctx context.Context, guideID, courseID string) (Enrollment, error) {
	c, _, _, err := s.repo.GetCourse(ctx, courseID)
	if err != nil {
		return Enrollment{}, err
	}
	if !c.Active {
		return Enrollment{}, fmt.Errorf("%w: course is not active", ErrValidation)
	}
	e, err := s.repo.Enroll(ctx, guideID, courseID)
	if err == ErrConflict {
		return s.repo.GetEnrollment(ctx, guideID, courseID)
	}
	return e, err
}

// CompleteLesson marks a lesson done and completes the enrollment when
// every lesson is finished and the quiz is passed (or no quiz exists).
func (s *Service) CompleteLesson(ctx context.Context, guideID, lessonID string) (Enrollment, error) {
	enrollmentID, courseID, err := s.enrollmentForLesson(ctx, guideID, lessonID)
	if err != nil {
		return Enrollment{}, err
	}
	if err := s.repo.CompleteLesson(ctx, enrollmentID, lessonID); err != nil {
		return Enrollment{}, err
	}
	return s.maybeComplete(ctx, enrollmentID, courseID)
}

func (s *Service) enrollmentForLesson(ctx context.Context, guideID, lessonID string) (enrollmentID, courseID string, err error) {
	return s.repo.EnrollmentForLesson(ctx, guideID, lessonID)
}

// SubmitQuiz scores the attempt server-side and completes the enrollment
// when the attempt passes and every lesson is done.
func (s *Service) SubmitQuiz(ctx context.Context, guideID, courseID string, answers []int) (Enrollment, int, bool, error) {
	e, err := s.repo.GetEnrollment(ctx, guideID, courseID)
	if err != nil {
		return Enrollment{}, 0, false, err
	}
	c, _, quiz, err := s.repo.GetCourse(ctx, courseID)
	if err != nil {
		return Enrollment{}, 0, false, err
	}
	if len(quiz) == 0 {
		return Enrollment{}, 0, false, fmt.Errorf("%w: course has no quiz", ErrValidation)
	}
	if len(answers) != len(quiz) {
		return Enrollment{}, 0, false, fmt.Errorf("%w: expected %d answers", ErrValidation, len(quiz))
	}
	correct := 0
	for i, q := range quiz {
		if answers[i] == q.AnswerIndex {
			correct++
		}
	}
	score := correct * 100 / len(quiz)
	passed := score >= c.PassScore
	if err := s.repo.RecordQuizAttempt(ctx, e.ID, score, passed); err != nil {
		return Enrollment{}, 0, false, err
	}
	updated, err := s.maybeComplete(ctx, e.ID, courseID)
	return updated, score, passed, err
}

// maybeComplete completes the enrollment when all lessons are done and the
// quiz is passed (vacuously true when the course has no quiz).
func (s *Service) maybeComplete(ctx context.Context, enrollmentID, courseID string) (Enrollment, error) {
	e, err := s.repo.GetEnrollmentByID(ctx, enrollmentID)
	if err != nil {
		return Enrollment{}, err
	}
	if e.Status == "completed" {
		return e, nil
	}
	_, _, quiz, err := s.repo.GetCourse(ctx, courseID)
	if err != nil {
		return Enrollment{}, err
	}
	quizOK := len(quiz) == 0 || e.QuizPassed
	if e.LessonsTotal > 0 && e.LessonsDone >= e.LessonsTotal && quizOK {
		token, err := pauth.NewOpaqueToken()
		if err != nil {
			return Enrollment{}, fmt.Errorf("training: serial: %w", err)
		}
		if _, err := s.repo.CompleteEnrollment(ctx, enrollmentID, "PGH-CERT-"+strings.ToUpper(token[:8])); err != nil {
			return Enrollment{}, err
		}
		return s.repo.GetEnrollmentByID(ctx, enrollmentID)
	}
	return e, nil
}

// Certificates returns the caller's issued certificates.
func (s *Service) Certificates(ctx context.Context, guideID string) ([]Certificate, error) {
	return s.repo.ListCertificates(ctx, guideID)
}
