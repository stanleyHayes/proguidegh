-- 0004_availability_booking.down.sql — roll back Phase 3 availability/booking
-- changes. Dev-stage data: bookings created with the new columns lose them;
-- availability/time-off rows are dropped with their tables.

ALTER TABLE idempotency_keys DROP COLUMN IF EXISTS entity_id;

DROP INDEX IF EXISTS idx_bookings_guide_active;
ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_no_guide_overlap;
DROP EXTENSION IF EXISTS btree_gist;

ALTER TABLE guide_profiles DROP COLUMN IF EXISTS latitude;
ALTER TABLE guide_profiles DROP COLUMN IF EXISTS longitude;

ALTER TABLE bookings DROP COLUMN IF EXISTS currency;
ALTER TABLE bookings DROP COLUMN IF EXISTS amount;
ALTER TABLE bookings DROP COLUMN IF EXISTS notes;
ALTER TABLE bookings DROP COLUMN IF EXISTS num_guests;

DROP TABLE IF EXISTS guide_time_off;
DROP TABLE IF EXISTS guide_availability;
