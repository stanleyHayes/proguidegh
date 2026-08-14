-- 0001_init.up.sql — ProGuideGH Phase 0 schema (spec §8.1 core tables).
--
-- Conventions:
--   * UUID primary keys via gen_random_uuid() (pgcrypto, supported on
--     managed Postgres per spec §26 — only create extensions when needed).
--   * timestamptz created_at/updated_at on mutable entities; updated_at is
--     maintained by the application (no trigger magic in Phase 0).
--   * MONEY: NUMERIC(14,2) in major currency units (GHS). Floats are
--     forbidden by spec §1.2. Ledger entries additionally CHECK amount > 0.
--   * Financial and audit tables (payments, refunds, ledger_*,
--     booking_status_events, certification_events, sos_events, audit_logs)
--     are APPEND-ONLY by intent: application code must never UPDATE or
--     DELETE their rows; corrections are new rows (spec §8).
--   * Status columns are TEXT with CHECK constraints against the state
--     machines in spec §8.2–8.4; transitions are enforced in domain code.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Reference data
-- ---------------------------------------------------------------------------

CREATE TABLE regions (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code       text NOT NULL UNIQUE,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE specialties (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code       text NOT NULL UNIQUE,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE roles (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code       text NOT NULL UNIQUE,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE permissions (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code       text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE role_permissions (
    role_id       uuid NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    permission_id uuid NOT NULL REFERENCES permissions (id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

-- Controlled configuration; changes are versioned and audited (spec §27).
CREATE TABLE system_settings (
    key        text PRIMARY KEY,
    value_json jsonb NOT NULL,
    version    integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Identity & RBAC
-- ---------------------------------------------------------------------------

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         text NOT NULL UNIQUE,
    phone_e164    text,
    password_hash text NOT NULL,
    status        text NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'suspended', 'deactivated')),
    last_login_at timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE user_roles (
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id    uuid NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE tourist_profiles (
    user_id                    uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    full_name                  text NOT NULL,
    nationality                text,
    preferred_language         text,
    emergency_contact_name     text,
    emergency_contact_phone_e164 text,
    created_at                 timestamptz NOT NULL DEFAULT now(),
    updated_at                 timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Guides
-- ---------------------------------------------------------------------------

CREATE TABLE guide_profiles (
    user_id      uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    public_name  text NOT NULL,
    bio          text,
    status       text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'in_review', 'certified', 'suspended', 'disabled')),
    rating_avg   NUMERIC(3, 2) NOT NULL DEFAULT 0 CHECK (rating_avg >= 0 AND rating_avg <= 5),
    rating_count integer NOT NULL DEFAULT 0 CHECK (rating_count >= 0),
    elite_status boolean NOT NULL DEFAULT false,
    region_id    uuid REFERENCES regions (id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- §26 query-pattern indexes: guide status / region / rating.
CREATE INDEX idx_guide_profiles_status ON guide_profiles (status);
CREATE INDEX idx_guide_profiles_region ON guide_profiles (region_id);
CREATE INDEX idx_guide_profiles_rating ON guide_profiles (rating_avg DESC);

CREATE TABLE guide_languages (
    guide_id      uuid NOT NULL REFERENCES guide_profiles (user_id) ON DELETE CASCADE,
    language_code text NOT NULL,
    proficiency   text NOT NULL DEFAULT 'conversational'
        CHECK (proficiency IN ('basic', 'conversational', 'fluent', 'native')),
    PRIMARY KEY (guide_id, language_code)
);

CREATE TABLE guide_specialties (
    guide_id     uuid NOT NULL REFERENCES guide_profiles (user_id) ON DELETE CASCADE,
    specialty_id uuid NOT NULL REFERENCES specialties (id) ON DELETE CASCADE,
    PRIMARY KEY (guide_id, specialty_id)
);

-- object_key points into private R2 storage; access via signed URLs only.
CREATE TABLE guide_documents (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    guide_id   uuid NOT NULL REFERENCES guide_profiles (user_id) ON DELETE CASCADE,
    type       text NOT NULL,
    object_key text NOT NULL,
    status     text NOT NULL DEFAULT 'uploaded'
        CHECK (status IN ('uploaded', 'under_review', 'approved', 'rejected', 'expired')),
    expires_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_guide_documents_guide ON guide_documents (guide_id);

-- ---------------------------------------------------------------------------
-- Certification pipeline (spec §5)
-- ---------------------------------------------------------------------------

CREATE TABLE certification_cases (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    guide_id     uuid NOT NULL REFERENCES guide_profiles (user_id) ON DELETE CASCADE,
    status       text NOT NULL DEFAULT 'submitted'
        CHECK (status IN ('submitted', 'under_review', 'interview', 'exam',
                          'approved', 'rejected', 'expired')),
    assigned_to  uuid REFERENCES users (id),
    opened_at    timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_certification_cases_status ON certification_cases (status);

-- APPEND-ONLY: immutable certification workflow history.
CREATE TABLE certification_events (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    case_id      uuid NOT NULL REFERENCES certification_cases (id) ON DELETE CASCADE,
    from_status  text,
    to_status    text NOT NULL,
    actor_id     uuid REFERENCES users (id),
    reason       text,
    evidence_ref text,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_certification_events_case ON certification_events (case_id, created_at);

-- ---------------------------------------------------------------------------
-- Tour catalog & pricing
-- ---------------------------------------------------------------------------

CREATE TABLE tour_packages (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code             text NOT NULL UNIQUE,
    name             text NOT NULL,
    duration_minutes integer NOT NULL CHECK (duration_minutes > 0),
    base_price       NUMERIC(14, 2) NOT NULL CHECK (base_price >= 0),
    currency         char(3) NOT NULL DEFAULT 'GHS',
    active           boolean NOT NULL DEFAULT true,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

-- Effective-dated prices (spec §27); region_id NULL means all regions.
CREATE TABLE pricing_rules (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    package_id     uuid NOT NULL REFERENCES tour_packages (id) ON DELETE CASCADE,
    region_id      uuid REFERENCES regions (id),
    amount         NUMERIC(14, 2) NOT NULL CHECK (amount >= 0),
    effective_from timestamptz NOT NULL,
    effective_to   timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    CHECK (effective_to IS NULL OR effective_to > effective_from)
);
CREATE INDEX idx_pricing_rules_package ON pricing_rules (package_id, effective_from);

-- ---------------------------------------------------------------------------
-- Bookings
-- ---------------------------------------------------------------------------

CREATE TABLE bookings (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    reference        text NOT NULL UNIQUE,
    tourist_id       uuid NOT NULL REFERENCES users (id),
    guide_id         uuid REFERENCES users (id),
    package_id       uuid NOT NULL REFERENCES tour_packages (id),
    starts_at        timestamptz NOT NULL,
    ends_at          timestamptz,
    status           text NOT NULL DEFAULT 'DRAFT'
        CHECK (status IN ('DRAFT', 'PAYMENT_PENDING', 'CONFIRMED', 'GUIDE_EN_ROUTE',
                          'GUIDE_ARRIVED', 'IN_PROGRESS', 'COMPLETED', 'PAYMENT_FAILED',
                          'CANCELLED_BY_TOURIST', 'CANCELLED_BY_GUIDE',
                          'CANCELLED_BY_ADMIN', 'NO_SHOW', 'REFUND_PENDING', 'REFUNDED')),
    meeting_point_text text,
    meeting_latitude   NUMERIC(9, 6),
    meeting_longitude  NUMERIC(9, 6),
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at IS NULL OR ends_at > starts_at)
);

-- §26 query-pattern indexes: booking start / status / guide.
CREATE INDEX idx_bookings_starts_at ON bookings (starts_at);
CREATE INDEX idx_bookings_status ON bookings (status);
CREATE INDEX idx_bookings_guide ON bookings (guide_id);
CREATE INDEX idx_bookings_tourist ON bookings (tourist_id);

-- APPEND-ONLY: immutable booking state history (spec §8.2).
CREATE TABLE booking_status_events (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id uuid NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    from_status text,
    to_status  text NOT NULL,
    actor_id   uuid REFERENCES users (id),
    metadata   jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_booking_status_events_booking ON booking_status_events (booking_id, created_at);

-- ---------------------------------------------------------------------------
-- Payments & refunds
-- ---------------------------------------------------------------------------

-- Raw card data is never stored; provider_reference is the provider-hosted
-- transaction token (spec §1.2, §16.1).
CREATE TABLE payments (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id         uuid NOT NULL REFERENCES bookings (id),
    provider           text NOT NULL,
    provider_reference text NOT NULL UNIQUE,
    amount             NUMERIC(14, 2) NOT NULL CHECK (amount >= 0),
    currency           char(3) NOT NULL DEFAULT 'GHS',
    status             text NOT NULL DEFAULT 'CREATED'
        CHECK (status IN ('CREATED', 'PENDING', 'SUCCEEDED', 'FAILED', 'EXPIRED',
                          'REFUND_PENDING', 'PARTIALLY_REFUNDED', 'REFUNDED')),
    paid_at            timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_payments_booking ON payments (booking_id);
CREATE INDEX idx_payments_status ON payments (status);

CREATE TABLE refunds (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id         uuid NOT NULL REFERENCES payments (id),
    provider_reference text,
    amount             NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    status             text NOT NULL DEFAULT 'PENDING'
        CHECK (status IN ('PENDING', 'SUCCEEDED', 'FAILED')),
    reason             text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_refunds_payment ON refunds (payment_id);

-- ---------------------------------------------------------------------------
-- Ledger (immutable double-entry; spec §9)
-- ---------------------------------------------------------------------------

-- owner_type/owner_id identify whose logical account this is (e.g. guide,
-- platform, tourism_levy); owner_id is NULL for platform-owned accounts.
CREATE TABLE ledger_accounts (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_type text NOT NULL,
    owner_id   uuid,
    currency   char(3) NOT NULL DEFAULT 'GHS',
    code       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (owner_type, owner_id, code)
);

-- APPEND-ONLY: immutable transaction header.
CREATE TABLE ledger_transactions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    reference   text NOT NULL UNIQUE,
    type        text NOT NULL,
    booking_id  uuid REFERENCES bookings (id),
    occurred_at timestamptz NOT NULL DEFAULT now(),
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- APPEND-ONLY: balanced debit/credit entries; the sum of debits must equal
-- the sum of credits per transaction (enforced in domain code, spec §9.2).
CREATE TABLE ledger_entries (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    transaction_id uuid NOT NULL REFERENCES ledger_transactions (id),
    account_id     uuid NOT NULL REFERENCES ledger_accounts (id),
    direction      text NOT NULL CHECK (direction IN ('debit', 'credit')),
    amount         NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_ledger_entries_transaction ON ledger_entries (transaction_id);
CREATE INDEX idx_ledger_entries_account ON ledger_entries (account_id);

-- ---------------------------------------------------------------------------
-- Payouts
-- ---------------------------------------------------------------------------

-- account_ref_tokenized holds the provider-tokenized destination reference,
-- never raw account numbers.
CREATE TABLE payout_accounts (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    guide_id              uuid NOT NULL REFERENCES guide_profiles (user_id) ON DELETE CASCADE,
    provider              text NOT NULL,
    network               text,
    account_ref_tokenized text NOT NULL,
    verified_at           timestamptz,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE payouts (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    guide_id           uuid NOT NULL REFERENCES guide_profiles (user_id),
    amount             NUMERIC(14, 2) NOT NULL CHECK (amount > 0),
    currency           char(3) NOT NULL DEFAULT 'GHS',
    status             text NOT NULL DEFAULT 'PENDING_ELIGIBILITY'
        CHECK (status IN ('PENDING_ELIGIBILITY', 'ELIGIBLE', 'QUEUED', 'PROCESSING',
                          'PAID', 'FAILED', 'RETRY_QUEUED', 'MANUAL_REVIEW')),
    provider_reference text,
    scheduled_for      date,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

-- §26 query-pattern indexes: payout status / date.
CREATE INDEX idx_payouts_status ON payouts (status);
CREATE INDEX idx_payouts_scheduled_for ON payouts (scheduled_for);
CREATE INDEX idx_payouts_guide ON payouts (guide_id);

-- ---------------------------------------------------------------------------
-- Reviews
-- ---------------------------------------------------------------------------

-- One verified review per booking (UNIQUE booking_id, spec §4.4).
CREATE TABLE reviews (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id uuid NOT NULL UNIQUE REFERENCES bookings (id),
    tourist_id uuid NOT NULL REFERENCES users (id),
    guide_id   uuid NOT NULL REFERENCES users (id),
    rating     integer NOT NULL CHECK (rating BETWEEN 1 AND 5),
    body       text,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_reviews_guide ON reviews (guide_id);

-- Tag values are seeded constants (Knowledgeable / Punctual / Friendly …),
-- stored as TEXT to keep the dictionary in seed data rather than a table.
CREATE TABLE review_tags (
    review_id uuid NOT NULL REFERENCES reviews (id) ON DELETE CASCADE,
    tag       text NOT NULL,
    PRIMARY KEY (review_id, tag)
);

-- ---------------------------------------------------------------------------
-- Safety & support
-- ---------------------------------------------------------------------------

CREATE TABLE incidents (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id  uuid REFERENCES bookings (id),
    type        text NOT NULL,
    severity    text NOT NULL DEFAULT 'low'
        CHECK (severity IN ('low', 'medium', 'high', 'critical')),
    status      text NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'acknowledged', 'in_progress', 'resolved', 'closed')),
    reported_by uuid REFERENCES users (id),
    assigned_to uuid REFERENCES users (id),
    occurred_at timestamptz NOT NULL DEFAULT now(),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_incidents_status ON incidents (status);

-- APPEND-ONLY: SOS evidence (spec §12).
CREATE TABLE sos_events (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id      uuid REFERENCES bookings (id),
    user_id         uuid NOT NULL REFERENCES users (id),
    latitude        NUMERIC(9, 6) NOT NULL,
    longitude       NUMERIC(9, 6) NOT NULL,
    accuracy        real,
    triggered_at    timestamptz NOT NULL DEFAULT now(),
    acknowledged_at timestamptz
);
CREATE INDEX idx_sos_events_booking ON sos_events (booking_id);

-- ---------------------------------------------------------------------------
-- Notifications, audit, idempotency
-- ---------------------------------------------------------------------------

CREATE TABLE notifications (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            uuid NOT NULL REFERENCES users (id),
    channel            text NOT NULL
        CHECK (channel IN ('email', 'sms', 'push', 'in_app')),
    template           text NOT NULL,
    status             text NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'sent', 'delivered', 'failed')),
    provider_reference text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notifications_user ON notifications (user_id, created_at);

-- APPEND-ONLY: privileged/financially significant action audit (spec §1.2).
CREATE TABLE audit_logs (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    actor_id    uuid REFERENCES users (id),
    action      text NOT NULL,
    entity_type text NOT NULL,
    entity_id   uuid,
    before_json jsonb,
    after_json  jsonb,
    ip          inet,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- §26 query-pattern indexes: audit entity / time.
CREATE INDEX idx_audit_logs_entity ON audit_logs (entity_type, entity_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs (created_at);

-- Mutation replay protection (spec §1.2, §14). Rows expire and may be
-- reaped after expires_at.
CREATE TABLE idempotency_keys (
    key                text NOT NULL,
    scope              text NOT NULL,
    response_code      integer,
    response_body_hash text,
    expires_at         timestamptz NOT NULL,
    created_at         timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (key, scope)
);
CREATE INDEX idx_idempotency_keys_expiry ON idempotency_keys (expires_at);
