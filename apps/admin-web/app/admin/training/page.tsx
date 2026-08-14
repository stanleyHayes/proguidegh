"use client";

import { useCallback, useEffect, useState } from "react";
import { Alert, Badge, Button, Card } from "@proguidegh/ui";
import { api, ApiError, errorMessage } from "../../lib/api";
import { Unauthorized } from "../../components/Unauthorized";

/**
 * Training admin (P8-01): course list, a minimal course-authoring form and
 * per-course rosters with progress. Quiz answers are only visible on this
 * admin surface; guides never receive them.
 */

interface Course {
  id: string;
  code: string;
  title: string;
  required_for_certification: boolean;
  pass_score: number;
  active: boolean;
  module_count: number;
  lesson_count: number;
  quiz_length: number;
}

interface RosterEntry {
  id: string;
  guide_name?: string | null;
  status: string;
  lessons_done: number;
  lessons_total: number;
  quiz_passed: boolean;
  best_score?: number | null;
  certificate_serial?: string | null;
}

type LoadState = "loading" | "unauthorized" | "forbidden" | "error" | "ready";

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object"
    ? (value as Record<string, unknown>)
    : null;
}

function parseCourses(data: unknown): Course[] {
  const list = asRecord(data)?.courses;
  if (!Array.isArray(list)) return [];
  return list
    .map((entry) => asRecord(entry))
    .filter((r): r is Record<string, unknown> => r !== null && typeof r.id === "string")
    .map((r) => ({
      id: r.id as string,
      code: String(r.code ?? ""),
      title: String(r.title ?? ""),
      required_for_certification: Boolean(r.required_for_certification),
      pass_score: Number(r.pass_score ?? 80),
      active: Boolean(r.active),
      module_count: Number(r.module_count ?? 0),
      lesson_count: Number(r.lesson_count ?? 0),
      quiz_length: Number(r.quiz_length ?? 0),
    }));
}

export default function TrainingPage() {
  const [state, setState] = useState<LoadState>("loading");
  const [courses, setCourses] = useState<Course[]>([]);
  const [roster, setRoster] = useState<{ course: Course; entries: RosterEntry[] } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const [code, setCode] = useState("");
  const [title, setTitle] = useState("");
  const [lessonTitle, setLessonTitle] = useState("Introduction");
  const [required, setRequired] = useState(false);

  const load = useCallback(async () => {
    try {
      const data = await api("/admin/training/courses");
      setCourses(parseCourses(data));
      setState("ready");
      setError(null);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setState("unauthorized");
      } else if (err instanceof ApiError && err.status === 403) {
        setState("forbidden");
      } else {
        setError(errorMessage(err, "Could not load courses."));
        setState("error");
      }
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function createCourse(event: React.FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    setNotice(null);
    try {
      await api("/admin/training/courses", {
        method: "POST",
        body: {
          code,
          title,
          required_for_certification: required,
          modules: [{ title: "Module 1", lessons: [{ title: lessonTitle, body: "" }] }],
        },
      });
      setNotice(`Course “${title}” created.`);
      setCode("");
      setTitle("");
      await load();
    } catch (err) {
      setError(errorMessage(err, "Could not create the course."));
    } finally {
      setBusy(false);
    }
  }

  async function toggleActive(course: Course) {
    setBusy(true);
    setError(null);
    try {
      await api(`/admin/training/courses/${course.id}`, {
        method: "PATCH",
        body: { active: !course.active },
      });
      await load();
    } catch (err) {
      setError(errorMessage(err, "Could not update the course."));
    } finally {
      setBusy(false);
    }
  }

  async function openRoster(course: Course) {
    try {
      const data = await api(`/admin/training/courses/${course.id}/enrollments`);
      const list = asRecord(data)?.enrollments;
      setRoster({
        course,
        entries: Array.isArray(list) ? (list as RosterEntry[]) : [],
      });
    } catch (err) {
      setError(errorMessage(err, "Could not load the roster."));
    }
  }

  if (state === "unauthorized" || state === "forbidden") {
    return <Unauthorized />;
  }

  return (
    <div className="stack">
      <section aria-labelledby="training-heading">
        <h1 id="training-heading">Training</h1>
        <p className="muted">
          Course authoring and enrollment oversight (spec §4.3). Guides
          self-enroll; completion requires every lesson plus the quiz.
        </p>
      </section>

      {notice ? <Alert tone="success">{notice}</Alert> : null}
      {error ? <Alert tone="error">{error}</Alert> : null}

      <Card title="New course">
        <form className="stack" onSubmit={(e) => void createCourse(e)}>
          <label>
            Code
            <input value={code} onChange={(e) => setCode(e.target.value)} required placeholder="safety-101" />
          </label>
          <label>
            Title
            <input value={title} onChange={(e) => setTitle(e.target.value)} required placeholder="Safety Fundamentals" />
          </label>
          <label>
            First lesson title
            <input value={lessonTitle} onChange={(e) => setLessonTitle(e.target.value)} required />
          </label>
          <label>
            <input
              type="checkbox"
              checked={required}
              onChange={(e) => setRequired(e.target.checked)}
            />{" "}
            Required for certification
          </label>
          <p>
            <Button variant="primary" disabled={busy}>
              {busy ? "Saving…" : "Create course"}
            </Button>
          </p>
        </form>
      </Card>

      <Card title={`Courses (${courses.length})`}>
        {courses.length === 0 ? (
          <p className="muted">No courses yet.</p>
        ) : (
          <ul className="stack" aria-label="Courses">
            {courses.map((course) => (
              <li key={course.id} className="stack">
                <p>
                  <Badge tone={course.active ? "success" : "neutral"}>
                    {course.active ? "Active" : "Inactive"}
                  </Badge>{" "}
                  <strong>{course.title}</strong> ({course.code}) ·{" "}
                  {course.module_count} module(s), {course.lesson_count} lesson(s)
                  {course.quiz_length > 0 ? `, quiz ×${course.quiz_length}` : ""}
                  {course.required_for_certification ? (
                    <>
                      {" "}
                      <Badge tone="warning">Required</Badge>
                    </>
                  ) : null}
                </p>
                <p className="nav-actions">
                  <Button variant="secondary" disabled={busy} onClick={() => void toggleActive(course)}>
                    {course.active ? "Deactivate" : "Activate"}
                  </Button>
                  <Button variant="secondary" onClick={() => void openRoster(course)}>
                    Roster
                  </Button>
                </p>
              </li>
            ))}
          </ul>
        )}
      </Card>

      {roster ? (
        <Card title={`Roster — ${roster.course.title} (${roster.entries.length})`}>
          {roster.entries.length === 0 ? (
            <p className="muted">No enrollments yet.</p>
          ) : (
            <ul className="stack" aria-label="Roster">
              {roster.entries.map((entry) => (
                <li key={entry.id}>
                  <p>
                    <Badge tone={entry.status === "completed" ? "success" : "neutral"}>
                      {entry.status}
                    </Badge>{" "}
                    <strong>{entry.guide_name ?? "Guide"}</strong> · lessons{" "}
                    {entry.lessons_done}/{entry.lessons_total} · quiz{" "}
                    {entry.quiz_passed ? "passed" : "not passed"}
                    {entry.best_score != null ? ` (best ${entry.best_score})` : ""}
                    {entry.certificate_serial ? ` · ${entry.certificate_serial}` : ""}
                  </p>
                </li>
              ))}
            </ul>
          )}
        </Card>
      ) : null}
    </div>
  );
}
