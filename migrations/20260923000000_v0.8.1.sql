-- 0.8.1 release migration. One migration file per release (see v0.7.0.sql).
-- 0.8.1 is a patch; it carries a migration under the data-loss exception in
-- docs/releasing.md, because without it every register reading silently loses
-- its audit-trail row.
--
-- #295: recording a reading on a risk, asset, legal requirement, supplier or
-- system returned 201, but its entity_changelog row failed with
--   entity_changelog_action_check violated (SQLSTATE 23514)
-- The readings handlers (internal/isms/api/api_readings.go) log action
-- 'reading', which the web History panel already renders, but the CHECK never
-- listed it. The API's changelog write is best-effort, so the failure was only
-- logged and no reading ever appeared in an entity's history. This widens the
-- set (full list kept explicit). DROP IF EXISTS + re-ADD is safe on fresh DBs
-- and on a second run; all existing rows satisfy the wider set.
ALTER TABLE entity_changelog DROP CONSTRAINT IF EXISTS entity_changelog_action_check;
ALTER TABLE entity_changelog ADD CONSTRAINT entity_changelog_action_check
    CHECK (action IN ('create', 'update', 'delete', 'reading',
                      'suggestion_created', 'suggestion_applied',
                      'suggestion_rejected', 'suggestion_deleted'));
