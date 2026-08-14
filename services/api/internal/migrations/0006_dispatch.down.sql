-- 0006_dispatch.down.sql — revert Phase 5 dispatch/location tables and
-- configuration.

DELETE FROM system_settings WHERE key IN (
    'dispatch_weights',
    'dispatch_batch_size',
    'dispatch_offer_ttl_seconds',
    'dispatch_radius_km',
    'dispatch_presence_window_minutes'
);

DROP TABLE IF EXISTS guide_dispatch_stats;
DROP TABLE IF EXISTS location_checkpoints;
DROP TABLE IF EXISTS dispatch_offers;
