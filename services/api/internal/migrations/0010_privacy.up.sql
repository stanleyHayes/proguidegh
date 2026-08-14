-- Phase M compliance: data-subject rights (M-20)
--
-- Drives three obligations that all demand the same machinery:
--   * Apple App Store Review 5.1.1(v) — an app with account creation MUST
--     offer in-app account deletion.
--   * Google Play "Data deletion" policy — same, plus a web-reachable route.
--   * Ghana Data Protection Act, 2012 (Act 843) — data subjects may access,
--     correct and object to processing of their personal data.
--
-- Deletion is ANONYMIZATION, not row removal. Spec §8 makes financial and
-- audit records append-only, and bookings/ledger_entries/receipts/audit_logs
-- all reference users.id. Dropping the row would either cascade away
-- immutable financial history or break referential integrity. Act 843 and
-- GDPR Art 17(3) both permit continued retention where another legal
-- obligation (tax, tourism levy reconciliation, fraud) requires it — what
-- must go is the personal data, which is exactly what anonymization removes.

-- 'deleted' is terminal: an anonymized account can never be reactivated,
-- because the credentials and contact details needed to prove ownership are
-- gone by design.
ALTER TABLE users DROP CONSTRAINT users_status_check;
ALTER TABLE users
    ADD CONSTRAINT users_status_check
    CHECK (status IN ('active', 'suspended', 'deactivated', 'deleted'));

ALTER TABLE users ADD COLUMN anonymized_at timestamptz;

CREATE INDEX idx_users_anonymized ON users (anonymized_at)
    WHERE anonymized_at IS NOT NULL;

-- Append-only record that an erasure happened. Deliberately holds NO personal
-- data — it exists to prove to a regulator or store reviewer that the request
-- was honoured, and would defeat its own purpose if it retained identifiers.
CREATE TABLE account_deletions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id        uuid NOT NULL REFERENCES users (id),
    requested_at   timestamptz NOT NULL DEFAULT now(),
    completed_at   timestamptz,
    -- Which classes of data were cleared, e.g.
    -- ["profile","documents","payout_accounts","location_checkpoints"].
    cleared        jsonb NOT NULL DEFAULT '[]'::jsonb,
    -- Set when deletion was refused, e.g. "active_booking".
    blocked_reason text,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_account_deletions_user ON account_deletions (user_id);

-- Act 843 s.20: consent must be demonstrable, so record which version of
-- which document the user accepted, and when. Append-only: a later
-- acceptance is a new row, never an update, so consent history is auditable.
CREATE TABLE consent_records (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    document     text NOT NULL CHECK (document IN ('terms', 'privacy', 'location')),
    version      text NOT NULL,
    accepted_at  timestamptz NOT NULL DEFAULT now(),
    -- Coarse provenance only; no device fingerprinting.
    source       text NOT NULL DEFAULT 'app'
        CHECK (source IN ('app', 'web', 'import')),
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_consent_records_user ON consent_records (user_id, document, accepted_at DESC);

-- Current published versions, so the apps can detect that a user is on an
-- outdated policy and re-prompt. Effective-dated rather than hard-coded, per
-- spec §31.23 (configurable values live in config tables).
CREATE TABLE legal_documents (
    document     text NOT NULL CHECK (document IN ('terms', 'privacy', 'location')),
    version      text NOT NULL,
    url          text NOT NULL,
    published_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (document, version)
);

-- Placeholder rows so the endpoint is well-formed before Legal signs off
-- (P9-06). The URLs 404 until the pages are published — that is the visible
-- reminder, and the launch checklist blocks on it.
INSERT INTO legal_documents (document, version, url) VALUES
    ('terms',    '2026-08-13', 'https://proguidegh.com/legal/terms'),
    ('privacy',  '2026-08-13', 'https://proguidegh.com/legal/privacy'),
    ('location', '2026-08-13', 'https://proguidegh.com/legal/location')
ON CONFLICT DO NOTHING;
