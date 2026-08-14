-- 0003_certification_catalog.up.sql — Phase 2: certification pipeline state
-- machine statuses (spec §5) plus catalog/reference seed data (spec §27,
-- Appendices B and C).

-- ---------------------------------------------------------------------------
-- Certification case statuses: replace the Phase 0 placeholder set with the
-- explicit state machine from spec §5. certification_cases is the state
-- machine root (spec §8.1); certification_events stays append-only history.
-- ---------------------------------------------------------------------------

ALTER TABLE certification_cases DROP CONSTRAINT IF EXISTS certification_cases_status_check;
ALTER TABLE certification_cases ALTER COLUMN status SET DEFAULT 'APPLIED';
ALTER TABLE certification_cases ADD CONSTRAINT certification_cases_status_check
    CHECK (status IN ('APPLIED', 'IDENTITY_PENDING', 'IDENTITY_VERIFIED',
                      'BACKGROUND_CHECK_PENDING', 'BACKGROUND_VERIFIED',
                      'TRAINING', 'EXAM_PENDING', 'CERTIFIED',
                      'INSURANCE_ACTIVE', 'ACTIVE',
                      'REJECTED', 'SUSPENDED', 'EXPIRED', 'REQUIRES_RETRAINING'));

-- One open pipeline per guide: the latest case (by opened_at) is the current
-- one. A partial unique index blocks concurrent double-opens.
CREATE UNIQUE INDEX idx_certification_cases_open
    ON certification_cases (guide_id)
    WHERE status NOT IN ('REJECTED');

-- ---------------------------------------------------------------------------
-- Language reference table: validates guide_languages.language_code and the
-- PATCH /me/guide/profile payload (spec §4.2).
-- ---------------------------------------------------------------------------

CREATE TABLE languages (
    code       text PRIMARY KEY,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO languages (code, name) VALUES
    ('en',  'English'),
    ('fr',  'French'),
    ('ak',  'Akan'),
    ('tw',  'Twi'),
    ('ga',  'Ga'),
    ('ee',  'Ewe'),
    ('dag', 'Dagbani'),
    ('dga', 'Dagaare'),
    ('gur', 'Frafra'),
    ('ha',  'Hausa')
ON CONFLICT (code) DO NOTHING;

ALTER TABLE guide_languages
    ADD CONSTRAINT guide_languages_language_code_fkey
    FOREIGN KEY (language_code) REFERENCES languages (code);

-- ---------------------------------------------------------------------------
-- Review tag dictionary (spec Appendix B). review_tags.tag stays TEXT; this
-- table is the canonical dictionary the values are validated against.
-- ---------------------------------------------------------------------------

CREATE TABLE review_tag_defs (
    code       text PRIMARY KEY,
    name       text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO review_tag_defs (code, name) VALUES
    ('knowledgeable',         'Knowledgeable'),
    ('punctual',              'Punctual'),
    ('friendly',              'Friendly'),
    ('professional',          'Professional'),
    ('helpful',               'Helpful'),
    ('great_storyteller',     'Great Storyteller'),
    ('safety_conscious',      'Safety Conscious'),
    ('good_communicator',     'Good Communicator'),
    ('local_expert',          'Local Expert'),
    ('exceeded_expectations', 'Exceeded Expectations')
ON CONFLICT (code) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Regions of Ghana (16, ISO 3166-2:GH codes without the GH- prefix).
-- ---------------------------------------------------------------------------

INSERT INTO regions (code, name) VALUES
    ('AA', 'Greater Accra'),
    ('AH', 'Ashanti'),
    ('AF', 'Ahafo'),
    ('BE', 'Bono East'),
    ('BO', 'Bono'),
    ('CP', 'Central'),
    ('EP', 'Eastern'),
    ('NE', 'North East'),
    ('NP', 'Northern'),
    ('OT', 'Oti'),
    ('SV', 'Savannah'),
    ('TV', 'Volta'),
    ('UE', 'Upper East'),
    ('UW', 'Upper West'),
    ('WN', 'Western North'),
    ('WP', 'Western')
ON CONFLICT (code) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Specialties (spec Appendix C — 13 minimum seed specialties).
-- ---------------------------------------------------------------------------

INSERT INTO specialties (code, name) VALUES
    ('city_tours',           'City Tours'),
    ('heritage_history',     'Heritage & History'),
    ('culture_arts',         'Culture & Arts'),
    ('food_culinary',        'Food & Culinary'),
    ('nature_ecotourism',    'Nature & Ecotourism'),
    ('adventure',            'Adventure'),
    ('nightlife',            'Nightlife'),
    ('religious_heritage',   'Religious Heritage'),
    ('business_conference',  'Business/Conference Support'),
    ('photography_tours',    'Photography Tours'),
    ('family_tours',         'Family Tours'),
    ('accessible_tourism',   'Accessible Tourism'),
    ('multi_region_tours',   'Multi-region Tours')
ON CONFLICT (code) DO NOTHING;

-- ---------------------------------------------------------------------------
-- Initial tour catalog + effective-dated prices (spec §27), effective from
-- 2026-01-01 with no end date and no region override (region_id NULL = all
-- regions, per the pricing_rules comment in 0001).
-- ---------------------------------------------------------------------------

INSERT INTO tour_packages (code, name, duration_minutes, base_price, currency, active) VALUES
    ('CITY_TOUR_4H',     'City Tour (4 hours)',          240,  250.00, 'GHS', true),
    ('HERITAGE_TOUR_8H', 'Heritage Tour (8 hours)',      480,  450.00, 'GHS', true),
    ('MULTI_REGION_24H', 'Multi-region Tour (24 hours)', 1440, 900.00, 'GHS', true)
ON CONFLICT (code) DO NOTHING;

INSERT INTO pricing_rules (package_id, region_id, amount, effective_from, effective_to)
SELECT p.id, NULL, p.base_price, TIMESTAMPTZ '2026-01-01 00:00:00+00', NULL
FROM tour_packages p
WHERE p.code IN ('CITY_TOUR_4H', 'HERITAGE_TOUR_8H', 'MULTI_REGION_24H')
  AND NOT EXISTS (
      SELECT 1 FROM pricing_rules pr
      WHERE pr.package_id = p.id AND pr.region_id IS NULL
        AND pr.effective_from = TIMESTAMPTZ '2026-01-01 00:00:00+00'
  );

-- ---------------------------------------------------------------------------
-- Initial policy configuration (spec §27). Stored as versioned JSON config
-- now; effective-dating for financial rules lands with Phase 4.
-- ---------------------------------------------------------------------------

INSERT INTO system_settings (key, value_json) VALUES
    ('platform_fee_pct',             '15'),
    ('tourism_levy_pct',             '3'),
    ('payout_delay_days',            '7'),
    ('quality_retraining_threshold', '4.0'),
    ('elite_rating_threshold',       '4.8')
ON CONFLICT (key) DO NOTHING;
