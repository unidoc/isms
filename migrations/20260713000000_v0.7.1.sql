-- 0.7.1 release migration. One migration file per release (see v0.7.0.sql).
-- All schema changes shipping in 0.7.1 accumulate here until it releases.
--
-- #161: saving a CIA reading for a Supplier failed with
--   entity_readings_entity_type_check violated (SQLSTATE 23514)
-- The supplier reading path (POST /suppliers/:id/readings -> entity_type
-- 'supplier') shipped, but entity_readings' entity_type CHECK never listed
-- 'supplier' — even though the status CHECK on the same table already carries
-- supplier statuses. This widens it (full set kept explicit).
-- DROP IF EXISTS + re-ADD is safe on fresh DBs too; all existing rows satisfy it.
ALTER TABLE entity_readings DROP CONSTRAINT IF EXISTS entity_readings_entity_type_check;
ALTER TABLE entity_readings ADD CONSTRAINT entity_readings_entity_type_check
    CHECK (entity_type IN ('risk', 'legal_requirement', 'asset', 'system', 'supplier'));
