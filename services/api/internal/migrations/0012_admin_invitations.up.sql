-- Auditable, single-use invitations for privileged staff accounts.
CREATE TABLE admin_invitations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email text NOT NULL,
    roles text[] NOT NULL CHECK (cardinality(roles) > 0),
    token_hash text NOT NULL UNIQUE,
    invited_by uuid REFERENCES users (id) ON DELETE SET NULL,
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (accepted_at IS NULL OR revoked_at IS NULL)
);

CREATE INDEX idx_admin_invitations_email ON admin_invitations (lower(email), created_at DESC);
CREATE INDEX idx_admin_invitations_expiry ON admin_invitations (expires_at) WHERE accepted_at IS NULL AND revoked_at IS NULL;
CREATE UNIQUE INDEX idx_admin_invitations_one_pending ON admin_invitations (lower(email)) WHERE accepted_at IS NULL AND revoked_at IS NULL;
