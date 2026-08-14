-- 0002_identity.down.sql — revert Phase 1 identity schema and seed data.

DROP TABLE IF EXISTS mfa_secrets;
DROP TABLE IF EXISTS otp_codes;
DROP TABLE IF EXISTS refresh_sessions;

-- Remove the seeded role/permission reference data (only rows whose codes
-- came from the 0002 seed; Phase 0 applied no seeds before this).
DELETE FROM role_permissions;
DELETE FROM user_roles;
DELETE FROM permissions;
DELETE FROM roles;
