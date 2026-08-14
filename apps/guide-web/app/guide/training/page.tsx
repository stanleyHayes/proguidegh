"use client";

import { useCallback, useEffect, useState } from "react";
import { Alert, Badge, Button, Card } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";

/**
 * Guide training (spec §4.3, P8-01): the course catalog with enrollment,
 * per-lesson progress, the server-scored quiz and issued certificates.
 * Quiz answers are scored on the server — they never reach this client.
 */

interface Course {
  id: string;
  code: string;
  title: string;
  description?: string | null;
  required_for_certification: boolean;
  pass_score: number;
  module_count: number;
  lesson_count: number;
  quiz_length: number;
}

interface Enrollment {
  id: string;
  status: string;
  lessons_done: number;
  lessons_total: number;
  quiz_passed: boolean;
  certificate_serial?: string | null;
}

interface CourseItem {
  course: Course;
  enrollment?: Enrollment | null;
}

interface Lesson {
  id: string;
  title: string;
  body: string;
}

interface Module {
  id: string;
  title: string;
  lessons: Lesson[];
}

interface QuizQuestion {
  question: string;
  options: string[];
}

interface Certificate {
  id: string;
  serial: string;
  course_code: string;
  course_title: string;
  issued_at: string;
}

type LoadState = "loading" | "unauthenticated" | "error" | "ready";

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

export default function TrainingPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [items, setItems] = useState<CourseItem[]>([]);
  const [certificates, setCertificates] = useState<Certificate[]>([]);
  const [open, setOpen] = useState<{
    course: Course;
    modules: Module[];
    quiz: QuizQuestion[];
    enrollment: Enrollment | null;
    completed: Record<string, boolean>;
  } | null>(null);
  const [answers, setAnswers] = useState<Record<number, number>>({});
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const [courseData, certData] = await Promise.all([
        api("/me/training/courses"),
        api("/me/training/certificates"),
      ]);
      const list = asRecord(courseData)?.courses;
      setItems(Array.isArray(list) ? (list as CourseItem[]) : []);
      const certs = asRecord(certData)?.certificates;
      setCertificates(Array.isArray(certs) ? (certs as Certificate[]) : []);
      setState("ready");
      setError(null);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthenticated");
      } else {
        setError(errorMessage(err, "Could not load training."));
        setState("error");
      }
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function openCourse(course: Course) {
    setError(null);
    try {
      const data = await api(`/me/training/courses/${course.id}`);
      const record = asRecord(data);
      const done = asRecord(record?.completed_lessons) ?? {};
      setOpen({
        course,
        modules: (record?.modules as Module[]) ?? [],
        quiz: (record?.quiz as QuizQuestion[]) ?? [],
        enrollment: (record?.enrollment as Enrollment | null) ?? null,
        completed: done as Record<string, boolean>,
      });
      setAnswers({});
    } catch (err) {
      setError(errorMessage(err, "Could not open the course."));
    }
  }

  async function refreshOpen(courseID: string) {
    const course = items.find((i) => i.course.id === courseID)?.course;
    if (course) await openCourse(course);
    await load();
  }

  async function enroll(course: Course) {
    setBusy(true);
    setError(null);
    try {
      await api(`/me/training/courses/${course.id}/enroll`, {
        method: "POST",
        body: {},
      });
      await refreshOpen(course.id);
      setNotice(`Enrolled on “${course.title}”.`);
    } catch (err) {
      setError(errorMessage(err, "Could not enroll."));
    } finally {
      setBusy(false);
    }
  }

  async function completeLesson(lessonID: string) {
    setBusy(true);
    setError(null);
    try {
      await api(`/me/training/lessons/${lessonID}/complete`, {
        method: "POST",
        body: {},
      });
      if (open) await refreshOpen(open.course.id);
    } catch (err) {
      setError(errorMessage(err, "Could not mark the lesson complete."));
    } finally {
      setBusy(false);
    }
  }

  async function submitQuiz() {
    if (!open) return;
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      const ordered = open.quiz.map((_, i) => answers[i] ?? -1);
      const data = asRecord(
        await api(`/me/training/courses/${open.course.id}/quiz`, {
          method: "POST",
          body: { answers: ordered },
        }),
      );
      if (data?.passed === true) {
        setNotice(`Quiz passed with ${String(data.score)}%.`);
      } else {
        setError(`Quiz scored ${String(data?.score ?? 0)}% — ${open.course.pass_score}% needed.`);
      }
      await refreshOpen(open.course.id);
    } catch (err) {
      setError(errorMessage(err, "Could not submit the quiz."));
    } finally {
      setBusy(false);
    }
  }

  if (state === "unauthenticated") {
    return (
      <div className="stack">
        <h1>Training</h1>
        <Alert tone="info">Sign in to view training.</Alert>
      </div>
    );
  }

  return (
    <div className="stack">
      <section aria-labelledby="training-heading">
        <h1 id="training-heading">Training</h1>
        <p className="muted">
          Complete required courses to keep your certification in good
          standing (§4.3).
        </p>
      </section>

      {notice ? <Alert tone="success">{notice}</Alert> : null}
      {error ? <Alert tone="error">{error}</Alert> : null}

      <Card title={`Courses (${items.length})`}>
        {items.length === 0 ? (
          <p className="muted">{state === "loading" ? "Loading…" : "No courses available yet."}</p>
        ) : (
          <ul className="stack" aria-label="Courses">
            {items.map(({ course, enrollment }) => (
              <li key={course.id} className="stack">
                <p>
                  {course.required_for_certification ? (
                    <Badge tone="warning">Required</Badge>
                  ) : (
                    <Badge tone="neutral">Optional</Badge>
                  )}{" "}
                  <strong>{course.title}</strong> · {course.lesson_count} lesson(s)
                  {course.quiz_length > 0 ? ` · quiz (pass ${course.pass_score}%)` : ""}
                  {enrollment ? (
                    <>
                      {" "}
                      <Badge tone={enrollment.status === "completed" ? "success" : "neutral"}>
                        {enrollment.status === "completed"
                          ? "Completed"
                          : `${enrollment.lessons_done}/${enrollment.lessons_total} lessons`}
                      </Badge>
                    </>
                  ) : null}
                </p>
                <p className="nav-actions">
                  <Button variant="secondary" onClick={() => void openCourse(course)}>
                    {enrollment ? "Continue" : "View"}
                  </Button>
                  {!enrollment ? (
                    <Button variant="primary" disabled={busy} onClick={() => void enroll(course)}>
                      Enroll
                    </Button>
                  ) : null}
                </p>
              </li>
            ))}
          </ul>
        )}
      </Card>

      {open ? (
        <Card title={open.course.title}>
          {open.modules.map((mod) => (
            <section key={mod.id} aria-label={mod.title}>
              <h3>{mod.title}</h3>
              <ul className="stack">
                {mod.lessons.map((lesson) => {
                  const done = open.completed[lesson.id] === true;
                  return (
                    <li key={lesson.id}>
                      <p>
                        {done ? <Badge tone="success">Done</Badge> : null} {lesson.title}
                      </p>
                      {lesson.body ? <p className="muted">{lesson.body}</p> : null}
                      {open.enrollment && !done ? (
                        <p>
                          <Button
                            variant="secondary"
                            disabled={busy}
                            onClick={() => void completeLesson(lesson.id)}
                          >
                            Mark complete
                          </Button>
                        </p>
                      ) : null}
                    </li>
                  );
                })}
              </ul>
            </section>
          ))}

          {open.enrollment && open.quiz.length > 0 ? (
            <section aria-label="Quiz">
              <h3>Quiz</h3>
              {open.quiz.map((q, qi) => (
                <fieldset key={qi}>
                  <legend>{q.question}</legend>
                  {q.options.map((option, oi) => (
                    <label key={oi}>
                      <input
                        type="radio"
                        name={`q-${qi}`}
                        checked={answers[qi] === oi}
                        onChange={() => setAnswers((prev) => ({ ...prev, [qi]: oi }))}
                      />{" "}
                      {option}
                    </label>
                  ))}
                </fieldset>
              ))}
              <p>
                <Button
                  variant="primary"
                  disabled={busy || Object.keys(answers).length < open.quiz.length}
                  onClick={() => void submitQuiz()}
                >
                  Submit quiz
                </Button>
              </p>
            </section>
          ) : null}
        </Card>
      ) : null}

      <Card title={`Certificates (${certificates.length})`}>
        {certificates.length === 0 ? (
          <p className="muted">Complete a course to earn your first certificate.</p>
        ) : (
          <ul aria-label="Certificates">
            {certificates.map((cert) => (
              <li key={cert.id}>
                <p>
                  <Badge tone="success">Certificate</Badge>{" "}
                  <strong>{cert.course_title}</strong> · {cert.serial} ·{" "}
                  {new Date(cert.issued_at).toLocaleDateString()}
                </p>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
