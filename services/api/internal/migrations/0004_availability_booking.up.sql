-- 0004_availability_booking.up.sql — Phase 3: guide availability (weekly
-- schedule + time off), booking columns needed for search/booking (spec §8.2,
-- §10.1–10.2, §13.3), guide coordinates for radius search, and the
-- double-booking guard.

-- ---------------------------------------------------------------------------
-- Guide availability: recurring weekly schedule (spec §10.1 "date/time
-- availability"). One row per weekly window; weekday follows Postgres
-- extract(dow) (0 = Sunday). end_time > start_time: overnight windows are not
-- supported in V1 (split them across two rows). timezone is per-row so guides
-- travelling across zones can keep a home schedule; Africa/Accra (UTC, no DST)
-- is the default and the coverage logic assumes no-DST zones in V1.
-- ---------------------------------------------------------------------------

CREATE TABLE guide_availability (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    guide_id   uuid NOT NULL REFERENCES guide_profiles (user_id) ON DELETE CASCADE,
    weekday    smallint NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    start_time time NOT NULL,
    end_time   time NOT NULL,
    timezone   text NOT NULL DEFAULT 'Africa/Accra',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (end_time > start_time)
);
CREATE INDEX idx_guide_availability_lookup ON guide_availability (guide_id, weekday);

-- ---------------------------------------------------------------------------
-- One-off unavailability (leave, personal appointments). Wins over the weekly
-- schedule when the requested interval intersects [starts_at, ends_at).
-- ---------------------------------------------------------------------------

CREATE TABLE guide_time_off (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    guide_id   uuid NOT NULL REFERENCES guide_profiles (user_id) ON DELETE CASCADE,
    starts_at  timestamptz NOT NULL,
    ends_at    timestamptz NOT NULL,
    reason     text,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (ends_at > starts_at)
);
CREATE INDEX idx_guide_time_off_guide ON guide_time_off (guide_id, starts_at);

-- ---------------------------------------------------------------------------
-- Booking columns required by Phase 3 (spec §13.3):
--   * num_guests / notes — checkout payload.
--   * amount / currency — server-authoritative price snapshot taken at
--     creation (spec §14: never trust client totals). Phase 4 payment
--     initiation charges exactly bookings.amount. amount is NULL only for
--     pre-Phase-3 rows, which are dev-stage data.
-- ---------------------------------------------------------------------------

ALTER TABLE bookings ADD COLUMN num_guests integer NOT NULL DEFAULT 1 CHECK (num_guests > 0);
ALTER TABLE bookings ADD COLUMN notes text;
ALTER TABLE bookings ADD COLUMN amount NUMERIC(14, 2) CHECK (amount >= 0);
ALTER TABLE bookings ADD COLUMN currency char(3) NOT NULL DEFAULT 'GHS';

-- ---------------------------------------------------------------------------
-- Guide operating-base coordinates for the §10.1 coordinates/radius search
-- filter (haversine). Nullable: guides without coordinates only appear in
-- region-scoped searches. Never exposed on public responses.
-- ---------------------------------------------------------------------------

ALTER TABLE guide_profiles ADD COLUMN latitude NUMERIC(9, 6);
ALTER TABLE guide_profiles ADD COLUMN longitude NUMERIC(9, 6);

-- ---------------------------------------------------------------------------
-- Double-booking guard (spec §10.2, task decision): a guide cannot hold two
-- bookings whose [starts_at, ends_at) intersect while both are in an active
-- (on-calendar) status. Enforced at two layers:
--
--   1. Transactional check in internal/bookings (SELECT ... FOR UPDATE over
--      the guide's overlapping active rows) — gives a clean 409 at creation
--      and on transition into CONFIRMED.
--   2. This exclusion constraint as the race-proof backstop at the database
--      layer (requires btree_gist for the uuid equality operator).
--
-- Only CONFIRMED..IN_PROGRESS rows participate: DRAFT/PAYMENT_PENDING holds
-- may coexist for the same slot until payment lands (Phase 4 confirms the
-- winner; confirming the loser violates this constraint and fails).
-- ends_at IS NOT NULL is explicit because tstzrange treats NULL as
-- unbounded; active rows always carry ends_at via domain code.
-- ---------------------------------------------------------------------------

CREATE EXTENSION IF NOT EXISTS btree_gist;

ALTER TABLE bookings ADD CONSTRAINT bookings_no_guide_overlap EXCLUDE USING gist (
    guide_id WITH =,
    tstzrange(starts_at, ends_at, '[)') WITH &&
) WHERE (guide_id IS NOT NULL
         AND ends_at IS NOT NULL
         AND status IN ('CONFIRMED', 'GUIDE_EN_ROUTE', 'GUIDE_ARRIVED', 'IN_PROGRESS'));

-- Supports the transactional overlap check and the search exclusion filter.
CREATE INDEX idx_bookings_guide_active ON bookings (guide_id, starts_at)
    WHERE status IN ('CONFIRMED', 'GUIDE_EN_ROUTE', 'GUIDE_ARRIVED', 'IN_PROGRESS');

-- ---------------------------------------------------------------------------
-- Idempotency replay mapping (spec §14): idempotency_keys stores only a
-- response hash, which cannot replay the created booking. entity_id points at
-- the booking created under this key so a replayed request returns the same
-- record. response_body_hash carries the sha256 of the canonical REQUEST
-- payload for booking.create (conflict detection: same key, different
-- payload); the schema comment in 0001 documents the original intent.
-- ---------------------------------------------------------------------------

ALTER TABLE idempotency_keys ADD COLUMN entity_id uuid;
