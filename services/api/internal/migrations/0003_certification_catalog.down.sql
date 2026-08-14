-- 0003_certification_catalog.down.sql — roll back Phase 2 catalog/certification
-- changes. Seed rows are removed; dependent rows (guide_specialties,
-- guide_languages, pricing_rules) are cleared first.

DELETE FROM system_settings WHERE key IN
    ('platform_fee_pct', 'tourism_levy_pct', 'payout_delay_days',
     'quality_retraining_threshold', 'elite_rating_threshold');

DELETE FROM pricing_rules WHERE package_id IN
    (SELECT id FROM tour_packages WHERE code IN ('CITY_TOUR_4H', 'HERITAGE_TOUR_8H', 'MULTI_REGION_24H'));
DELETE FROM tour_packages WHERE code IN ('CITY_TOUR_4H', 'HERITAGE_TOUR_8H', 'MULTI_REGION_24H');

DELETE FROM guide_specialties WHERE specialty_id IN (SELECT id FROM specialties);
DELETE FROM specialties;

DELETE FROM regions;

DELETE FROM review_tag_defs;
DROP TABLE review_tag_defs;

ALTER TABLE guide_languages DROP CONSTRAINT IF EXISTS guide_languages_language_code_fkey;
DELETE FROM guide_languages;
DROP TABLE languages;

DROP INDEX IF EXISTS idx_certification_cases_open;

-- New-state rows cannot satisfy the restored Phase 0 CHECK; the pipeline is
-- dev-stage data, so the rollback clears it (events cascade).
DELETE FROM certification_cases;

ALTER TABLE certification_cases DROP CONSTRAINT IF EXISTS certification_cases_status_check;
ALTER TABLE certification_cases ALTER COLUMN status SET DEFAULT 'submitted';
ALTER TABLE certification_cases ADD CONSTRAINT certification_cases_status_check
    CHECK (status IN ('submitted', 'under_review', 'interview', 'exam',
                      'approved', 'rejected', 'expired'));
