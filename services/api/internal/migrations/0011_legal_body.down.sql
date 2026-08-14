-- The 2026-08-14 versions exist only because of this migration; rolling back
-- removes them. Consent recorded against them goes too — that is correct, the
-- text those users accepted no longer exists.
DELETE FROM consent_records WHERE version = '2026-08-14';
DELETE FROM legal_documents WHERE version = '2026-08-14';

DROP INDEX IF EXISTS idx_legal_documents_published;
ALTER TABLE legal_documents DROP CONSTRAINT IF EXISTS legal_documents_approval_complete;
ALTER TABLE legal_documents
    DROP COLUMN IF EXISTS approved_by,
    DROP COLUMN IF EXISTS approved_at,
    DROP COLUMN IF EXISTS approved,
    DROP COLUMN IF EXISTS body,
    DROP COLUMN IF EXISTS summary;
