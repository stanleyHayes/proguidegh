-- 0007_safety_reviews.down.sql — reverse 0007_safety_reviews.up.sql.

DELETE FROM system_settings WHERE key IN (
    'quality_low_rating_threshold',
    'quality_min_review_count',
    'elite_rating_threshold',
    'elite_min_completed_tours'
);
DROP TABLE IF EXISTS quality_flags;
DROP TABLE IF EXISTS incident_events;
