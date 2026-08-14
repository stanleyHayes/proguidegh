DROP TABLE IF EXISTS legal_documents;
DROP TABLE IF EXISTS consent_records;
DROP TABLE IF EXISTS account_deletions;

DROP INDEX IF EXISTS idx_users_anonymized;
ALTER TABLE users DROP COLUMN IF EXISTS anonymized_at;

-- Restore the pre-0010 status domain. Any rows already anonymized would
-- violate it, so they are returned to 'deactivated' first; their personal
-- data is not recoverable either way.
UPDATE users SET status = 'deactivated' WHERE status = 'deleted';
ALTER TABLE users DROP CONSTRAINT users_status_check;
ALTER TABLE users
    ADD CONSTRAINT users_status_check
    CHECK (status IN ('active', 'suspended', 'deactivated'));
