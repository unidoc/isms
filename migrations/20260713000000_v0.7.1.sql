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

-- Supplier register: add 'contractor' to supplier_type. The IT-centric set had
-- no home for physical works contractors (rock/concrete, blasting, tank cleaning,
-- coating, marine), which were landing in 'other'. DROP IF EXISTS + re-ADD is
-- safe on fresh DBs; existing rows all satisfy the wider set.
ALTER TABLE suppliers DROP CONSTRAINT IF EXISTS suppliers_supplier_type_check;
ALTER TABLE suppliers ADD CONSTRAINT suppliers_supplier_type_check
    CHECK (supplier_type IN ('cloud', 'saas', 'consulting', 'hosting',
        'infrastructure', 'software', 'contractor', 'other'));

-- Task visibility: per-task privacy flag. PUBLIC by default, so existing tasks
-- and existing deployments are unchanged. When true, a task is visible only to
-- its assignee, its creator, and managers/admins — enforced in the API task read
-- paths (NOT via RLS). Org setting `task_default_private` decides the value for
-- new tasks when a create request omits it.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS private BOOLEAN NOT NULL DEFAULT false;

-- Org default for new-task visibility, surfaced in Admin -> Settings. The
-- settings UI is catalog-driven and renders this as a boolean toggle because
-- default_value is 'true'/'false'. Public by default; an org opts into privacy
-- by flipping this. Idempotent so a re-run / fresh DB both behave.
INSERT INTO settings (key, description, category, default_value, sensitive) VALUES
    ('task_default_private', 'New tasks default to private (visible only to the assignee, creator, and managers)', 'tasks', 'false', false)
ON CONFLICT (key) DO NOTHING;
