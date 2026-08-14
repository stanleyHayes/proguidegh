-- 0007_safety_reviews.up.sql — Phase 6: incident workflow audit trail,
-- quality flags (low-rating retraining / Elite qualification review) and
-- quality policy settings (spec §4.4, §12).

-- APPEND-ONLY: every acknowledgement, note, escalation, assignment and
-- closure on an incident is timestamped and attributed (spec §12 step 11).
CREATE TABLE incident_events (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id uuid NOT NULL REFERENCES incidents (id) ON DELETE CASCADE,
    actor_id    uuid REFERENCES users (id),
    kind        text NOT NULL
        CHECK (kind IN ('acknowledged', 'note', 'escalated', 'assigned',
                        'resolved', 'closed', 'reopened')),
    body        text,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_incident_events_incident ON incident_events (incident_id, created_at);

-- Quality flags (spec §4.4): rating below the rolling threshold opens a
-- low_rating flag (the retraining queue); rating above the Elite threshold
-- with enough completed tours opens an elite_review flag. At most one OPEN
-- flag per (guide, kind) — re-flagging an already-flagged guide is a no-op.
CREATE TABLE quality_flags (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    guide_id           uuid NOT NULL REFERENCES guide_profiles (user_id),
    kind               text NOT NULL CHECK (kind IN ('low_rating', 'elite_review')),
    status             text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
    rating_avg_at_flag NUMERIC(3, 2) NOT NULL,
    detail             text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    resolved_at        timestamptz,
    resolved_by        uuid REFERENCES users (id),
    resolution_note    text
);
CREATE UNIQUE INDEX idx_quality_flags_open ON quality_flags (guide_id, kind)
    WHERE status = 'open';
CREATE INDEX idx_quality_flags_status ON quality_flags (status, created_at);

-- Quality policy (spec §4.4): thresholds are configuration, not code.
INSERT INTO system_settings (key, value_json) VALUES
    ('quality_low_rating_threshold', '4.0'),
    ('quality_min_review_count', '3'),
    ('elite_rating_threshold', '4.8'),
    ('elite_min_completed_tours', '20')
ON CONFLICT (key) DO NOTHING;
