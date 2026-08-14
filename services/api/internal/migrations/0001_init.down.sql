-- 0001_init.down.sql — drop the Phase 0 schema (reverse of 0001_init.up.sql).

DROP TABLE IF EXISTS idempotency_keys;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS sos_events;
DROP TABLE IF EXISTS incidents;
DROP TABLE IF EXISTS review_tags;
DROP TABLE IF EXISTS reviews;
DROP TABLE IF EXISTS payouts;
DROP TABLE IF EXISTS payout_accounts;
DROP TABLE IF EXISTS ledger_entries;
DROP TABLE IF EXISTS ledger_transactions;
DROP TABLE IF EXISTS ledger_accounts;
DROP TABLE IF EXISTS refunds;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS booking_status_events;
DROP TABLE IF EXISTS bookings;
DROP TABLE IF EXISTS pricing_rules;
DROP TABLE IF EXISTS tour_packages;
DROP TABLE IF EXISTS certification_events;
DROP TABLE IF EXISTS certification_cases;
DROP TABLE IF EXISTS guide_documents;
DROP TABLE IF EXISTS guide_specialties;
DROP TABLE IF EXISTS guide_languages;
DROP TABLE IF EXISTS guide_profiles;
DROP TABLE IF EXISTS tourist_profiles;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS system_settings;
DROP TABLE IF EXISTS role_permissions;
DROP TABLE IF EXISTS permissions;
DROP TABLE IF EXISTS roles;
DROP TABLE IF EXISTS specialties;
DROP TABLE IF EXISTS regions;

DROP EXTENSION IF EXISTS pgcrypto;
