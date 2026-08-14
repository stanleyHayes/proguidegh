-- 0006_dispatch.up.sql — Phase 5: dispatch offers, coarse location
-- checkpoints and guide dispatch reliability stats (spec §10.3, §11.2).

-- ---------------------------------------------------------------------------
-- Dispatch offers (spec §10.3 step 3): one row per (booking, guide, batch).
-- A batch is one dispatch round; batch_seq increments when a batch expires or
-- is declined out and operations re-dispatches. score is the §10.3 step 2
-- composite; features stores the exact scoring inputs so a future model can
-- be evaluated offline against outcomes (ACCEPTED/DECLINED/EXPIRED).
--
-- expires_at is the authoritative deadline (Postgres is the source of truth,
-- ADR 0002); the Redis offer:{id} TTL key is only a cache/failure hint.
-- ---------------------------------------------------------------------------

CREATE TABLE dispatch_offers (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id   uuid NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    guide_id     uuid NOT NULL REFERENCES guide_profiles (user_id),
    batch_seq    integer NOT NULL DEFAULT 1 CHECK (batch_seq > 0),
    score        NUMERIC(7, 5) NOT NULL CHECK (score >= 0),
    features     jsonb NOT NULL DEFAULT '{}'::jsonb,
    status       text NOT NULL DEFAULT 'OFFERED'
        CHECK (status IN ('OFFERED', 'ACCEPTED', 'DECLINED', 'EXPIRED', 'SUPERSEDED')),
    offered_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    responded_at timestamptz,
    UNIQUE (booking_id, guide_id, batch_seq),
    CHECK (expires_at > offered_at)
);

-- Guide's offer inbox: current OFFERED rows by expiry (GET /me/guide/offers,
-- accept/decline lookups).
CREATE INDEX idx_dispatch_offers_guide_status_expiry
    ON dispatch_offers (guide_id, status, expires_at);
-- Batch history per booking (admin "why unmatched" view, next-batch lookup).
CREATE INDEX idx_dispatch_offers_booking ON dispatch_offers (booking_id, batch_seq);
-- Sweeper scan: live offers past their deadline.
CREATE INDEX idx_dispatch_offers_expiry ON dispatch_offers (expires_at)
    WHERE status = 'OFFERED';

-- ---------------------------------------------------------------------------
-- Location checkpoints (spec §11.2 privacy/retention): the COARSE persisted
-- safety/audit trail for active tours. High-frequency pings live only in
-- Redis (loc:booking:{id} / loc:guide:{id}, EX 60) and are never written here.
-- Persist policy (documented in internal/tracking): the first ping of a tour
-- leg, then at most one row per checkpoint interval (60s), plus one row per
-- tour event (en-route/arrived/start/complete) when a fresh position exists.
-- No endpoint exposes this table to tourists; historical movement stays
-- operational/audit only.
-- ---------------------------------------------------------------------------

CREATE TABLE location_checkpoints (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id  uuid NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    guide_id    uuid NOT NULL REFERENCES guide_profiles (user_id),
    latitude    NUMERIC(9, 6) NOT NULL,
    longitude   NUMERIC(9, 6) NOT NULL,
    accuracy_m  NUMERIC(8, 2),
    heading     NUMERIC(6, 2),
    speed_mps   NUMERIC(8, 2),
    captured_at timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_location_checkpoints_booking
    ON location_checkpoints (booking_id, captured_at);

-- ---------------------------------------------------------------------------
-- Acceptance reliability (spec §10.3 step 2 input): running counters per
-- guide. The ratio accepts/offers feeds the dispatch score; counters, not
-- derived queries, so scoring stays O(1) per candidate.
-- ---------------------------------------------------------------------------

CREATE TABLE guide_dispatch_stats (
    guide_id   uuid PRIMARY KEY REFERENCES guide_profiles (user_id) ON DELETE CASCADE,
    offers     integer NOT NULL DEFAULT 0 CHECK (offers >= 0),
    accepts    integer NOT NULL DEFAULT 0 CHECK (accepts >= 0),
    declines   integer NOT NULL DEFAULT 0 CHECK (declines >= 0),
    expiries   integer NOT NULL DEFAULT 0 CHECK (expiries >= 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Dispatch policy configuration (spec §10.3). dispatch_weights is the JSON
-- weight map for scoring (keys: distance, rating, specialty, language,
-- workload, reliability; must sum ~1). Scores are recomputed per offer, so
-- weight changes apply to the next batch only.
-- ---------------------------------------------------------------------------

INSERT INTO system_settings (key, value_json) VALUES
    ('dispatch_weights', '{"distance": 0.30, "rating": 0.25, "specialty": 0.15, "language": 0.10, "workload": 0.10, "reliability": 0.10}'),
    ('dispatch_batch_size', '3'),
    ('dispatch_offer_ttl_seconds', '30'),
    ('dispatch_radius_km', '50'),
    ('dispatch_presence_window_minutes', '120')
ON CONFLICT (key) DO NOTHING;
