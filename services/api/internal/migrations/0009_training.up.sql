-- Phase 8: light LMS (P8-01) + versioned notification templates (P8-03)

-- Courses are the certification/upskill unit. quiz holds the assessment as
-- JSONB: [{"question": str, "options": [str], "answer_index": int}] —
-- answer_index is stripped from every guide-facing response.
CREATE TABLE courses (
    id                          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code                        text NOT NULL UNIQUE,
    title                       text NOT NULL,
    description                 text,
    required_for_certification  boolean NOT NULL DEFAULT false,
    pass_score                  int NOT NULL DEFAULT 80 CHECK (pass_score BETWEEN 0 AND 100),
    quiz                        jsonb NOT NULL DEFAULT '[]'::jsonb,
    active                      boolean NOT NULL DEFAULT true,
    created_at                  timestamptz NOT NULL DEFAULT now(),
    updated_at                  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE course_modules (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id  uuid NOT NULL REFERENCES courses (id) ON DELETE CASCADE,
    position   int NOT NULL,
    title      text NOT NULL,
    UNIQUE (course_id, position)
);

CREATE TABLE course_lessons (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    module_id  uuid NOT NULL REFERENCES course_modules (id) ON DELETE CASCADE,
    position   int NOT NULL,
    title      text NOT NULL,
    body       text NOT NULL DEFAULT '',
    UNIQUE (module_id, position)
);

-- One enrollment per guide per course (UNIQUE is the idempotent-enroll
-- backstop, P8-01).
CREATE TABLE enrollments (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    guide_id     uuid NOT NULL REFERENCES guide_profiles (user_id) ON DELETE CASCADE,
    course_id    uuid NOT NULL REFERENCES courses (id) ON DELETE CASCADE,
    status       text NOT NULL DEFAULT 'enrolled' CHECK (status IN ('enrolled', 'completed')),
    completed_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (guide_id, course_id)
);

CREATE TABLE lesson_progress (
    enrollment_id uuid NOT NULL REFERENCES enrollments (id) ON DELETE CASCADE,
    lesson_id     uuid NOT NULL REFERENCES course_lessons (id) ON DELETE CASCADE,
    completed_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (enrollment_id, lesson_id)
);

CREATE TABLE quiz_attempts (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    enrollment_id uuid NOT NULL REFERENCES enrollments (id) ON DELETE CASCADE,
    score         int NOT NULL CHECK (score BETWEEN 0 AND 100),
    passed        boolean NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_quiz_attempts_enrollment ON quiz_attempts (enrollment_id, created_at);

-- One certificate per completed enrollment; the serial is the public
-- verification handle.
CREATE TABLE certificates (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    enrollment_id uuid NOT NULL UNIQUE REFERENCES enrollments (id) ON DELETE CASCADE,
    serial        text NOT NULL UNIQUE,
    issued_at     timestamptz NOT NULL DEFAULT now()
);

-- Versioned notification templates (P8-03): a new version is inserted
-- inactive; activation supersedes the previous active version atomically
-- (partial unique index enforces one active version per key).
CREATE TABLE notification_templates (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    key        text NOT NULL,
    version    int NOT NULL,
    channel    text NOT NULL CHECK (channel IN ('email', 'sms', 'push', 'in_app')),
    subject    text NOT NULL DEFAULT '',
    body       text NOT NULL,
    active     boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (key, version)
);
CREATE UNIQUE INDEX idx_notification_templates_active
    ON notification_templates (key) WHERE active;

INSERT INTO notification_templates (key, version, channel, subject, body, active) VALUES
    ('booking.confirmed', 1, 'email', 'Your tour is confirmed',
     'Hi {{tourist_name}}, your booking {{booking_reference}} is confirmed for {{starts_at}}. Your guide is {{guide_name}}.', true),
    ('dispatch.offer', 1, 'push', 'New job offer',
     'New offer: {{package_title}} on {{starts_at}} — accept within {{expires_in}}.', true),
    ('sos.triggered', 1, 'sms', '',
     'SOS raised on booking {{booking_reference}} at {{location}}. Respond immediately.', true),
    ('payout.paid', 1, 'email', 'Your payout is on its way',
     'Hi {{guide_name}}, your payout of {{amount}} for {{scheduled_for}} has been paid. Reference: {{provider_reference}}.', true);
