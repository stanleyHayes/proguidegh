-- 0002_identity.up.sql — Phase 1: sessions, OTP and MFA (spec §15), plus
-- role/permission reference seed data (spec §3, Appendix A).

-- ---------------------------------------------------------------------------
-- Rotating refresh sessions (spec §15.1)
-- ---------------------------------------------------------------------------

-- token_hash stores sha256 hex of the opaque refresh token; the raw token is
-- only ever sent as an HttpOnly cookie. rotated_to points at the replacement
-- session row so a rotated-out token that is presented again (reuse) can be
-- detected and the whole chain revoked.
CREATE TABLE refresh_sessions (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    rotated_to uuid,
    revoked_at timestamptz,
    ip         inet,
    user_agent text,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_sessions_user ON refresh_sessions (user_id);
CREATE INDEX idx_refresh_sessions_expiry ON refresh_sessions (expires_at);

-- ---------------------------------------------------------------------------
-- One-time codes (spec §15.2)
-- ---------------------------------------------------------------------------

-- Codes are stored sha256-hashed; the plaintext code is delivered via
-- sms/email channel (or logged to stdout in local dev only). purpose is one
-- of login | verify_contact | password_reset.
CREATE TABLE otp_codes (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid REFERENCES users (id) ON DELETE CASCADE,
    destination text,
    channel     text NOT NULL CHECK (channel IN ('sms', 'email')),
    purpose     text NOT NULL CHECK (purpose IN ('login', 'verify_contact', 'password_reset')),
    code_hash   text NOT NULL,
    attempts    integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    expires_at  timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (user_id IS NOT NULL OR destination IS NOT NULL)
);
CREATE INDEX idx_otp_codes_destination ON otp_codes (destination, purpose, created_at);
CREATE INDEX idx_otp_codes_user ON otp_codes (user_id, purpose, created_at);
CREATE INDEX idx_otp_codes_expiry ON otp_codes (expires_at);

-- ---------------------------------------------------------------------------
-- TOTP MFA secrets (spec §15.2)
-- ---------------------------------------------------------------------------

-- totp_secret_encrypted holds the base32 TOTP secret encrypted at rest
-- (AES-GCM with the app secret; see internal/platform/auth). enabled_at NULL
-- means enrollment started but not yet verified. backup_codes_hash stores
-- sha256 hashes of one-time recovery codes.
CREATE TABLE mfa_secrets (
    user_id               uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    totp_secret_encrypted text NOT NULL,
    enabled_at            timestamptz,
    backup_codes_hash     jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Reference seed data: roles (spec §3) and permissions (Appendix A)
-- ---------------------------------------------------------------------------

INSERT INTO roles (code, name) VALUES
    ('tourist',           'Tourist'),
    ('guide_applicant',   'Guide Applicant'),
    ('guide',             'Certified Guide'),
    ('elite_guide',       'Elite Guide'),
    ('operations_agent',  'Operations Agent'),
    ('verifier',          'Verifier / Certification Officer'),
    ('finance_officer',   'Finance Officer'),
    ('content_admin',     'Content / Training Admin'),
    ('administrator',     'Administrator'),
    ('super_admin',       'Super Admin')
ON CONFLICT (code) DO NOTHING;

INSERT INTO permissions (code) VALUES
    ('guides.read'),
    ('guides.manage'),
    ('certification.read'),
    ('certification.review'),
    ('bookings.read'),
    ('bookings.manage'),
    ('dispatch.manage'),
    ('incidents.read'),
    ('incidents.manage'),
    ('payments.read'),
    ('payments.refund'),
    ('payouts.read'),
    ('payouts.manage'),
    ('ledger.read'),
    ('reviews.moderate'),
    ('training.manage'),
    ('pricing.manage'),
    ('reports.read'),
    ('reports.export'),
    ('users.read'),
    ('users.manage'),
    ('rbac.manage'),
    ('audit.read'),
    ('settings.manage')
ON CONFLICT (code) DO NOTHING;

-- Role -> permission mapping. Tourist/guide roles are self-scoped and need no
-- staff permission codes; staff roles get the minimum set per spec §3.
WITH grants (role_code, permission_codes) AS (
    VALUES
    ('operations_agent', ARRAY[
        'guides.read', 'bookings.read', 'bookings.manage', 'dispatch.manage',
        'incidents.read', 'incidents.manage', 'reports.read'
    ]),
    ('verifier', ARRAY[
        'certification.read', 'certification.review', 'guides.read'
    ]),
    ('finance_officer', ARRAY[
        'payments.read', 'payments.refund', 'payouts.read', 'payouts.manage',
        'ledger.read', 'bookings.read', 'reports.read', 'reports.export'
    ]),
    ('content_admin', ARRAY[
        'training.manage', 'reviews.moderate', 'reports.read'
    ]),
    ('administrator', ARRAY[
        'guides.read', 'guides.manage', 'certification.read',
        'bookings.read', 'bookings.manage', 'dispatch.manage',
        'incidents.read', 'incidents.manage', 'payments.read',
        'payouts.read', 'reviews.moderate', 'training.manage',
        'pricing.manage', 'reports.read', 'users.read', 'users.manage',
        'settings.manage'
    ]),
    ('super_admin', ARRAY[
        'guides.read', 'guides.manage', 'certification.read', 'certification.review',
        'bookings.read', 'bookings.manage', 'dispatch.manage',
        'incidents.read', 'incidents.manage', 'payments.read', 'payments.refund',
        'payouts.read', 'payouts.manage', 'ledger.read', 'reviews.moderate',
        'training.manage', 'pricing.manage', 'reports.read', 'reports.export',
        'users.read', 'users.manage', 'rbac.manage', 'audit.read', 'settings.manage'
    ])
)
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM grants g
JOIN roles r ON r.code = g.role_code
JOIN permissions p ON p.code = ANY (g.permission_codes)
ON CONFLICT DO NOTHING;
