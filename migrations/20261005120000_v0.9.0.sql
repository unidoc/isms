-- 0.9.0 release migration. One migration file per minor release (see
-- docs/releasing.md); every schema change that lands in 0.9.0 adds to this file.
--
-- #269: the Legal register's web form offered categories the
-- legal_requirements_category_check constraint rejected. The register now uses
-- one list of ten, the original five plus employment, intellectual_property,
-- financial, environmental and corporate. This only widens the set (full list
-- kept explicit), so every existing row still satisfies it. DROP IF EXISTS +
-- re-ADD is safe on fresh databases and on a second run.
ALTER TABLE legal_requirements DROP CONSTRAINT IF EXISTS legal_requirements_category_check;
ALTER TABLE legal_requirements ADD CONSTRAINT legal_requirements_category_check
    CHECK (category IN ('privacy', 'security', 'sector', 'contractual',
                        'employment', 'intellectual_property', 'financial',
                        'environmental', 'corporate', 'other'));

-- #366: audit and audit-finding references were stored in two forms depending
-- on the screen that made them: AUDIT-<id> / FIND-<id> (create-from-finding,
-- the finding delete guard, API callers) or the bare row id (the audit and
-- finding Links tabs). Reads match the literal string, so links made one way
-- were invisible the other way, and the "corrective actions still linked"
-- guard on finding delete missed bare-id links. The API now stores the prefixed
-- form only; this rewrites existing bare-id rows to it. They name real rows,
-- so they are rewritten, not deleted. A bare-id row whose prefixed twin already
-- exists is deleted first, or the rewrite would hit the unique key. Only
-- '^[1-9][0-9]*$' is touched (what the Links tabs sent), so a second run finds
-- nothing to do.
DELETE FROM entity_references r
 WHERE r.target_type = 'audit_finding' AND r.target_id ~ '^[1-9][0-9]*$'
   AND EXISTS (SELECT 1 FROM entity_references p
                WHERE p.organization_id = r.organization_id
                  AND p.source_type = r.source_type AND p.source_id = r.source_id
                  AND p.target_type = r.target_type AND p.target_id = 'FIND-' || r.target_id);
UPDATE entity_references SET target_id = 'FIND-' || target_id
 WHERE target_type = 'audit_finding' AND target_id ~ '^[1-9][0-9]*$';

DELETE FROM entity_references r
 WHERE r.source_type = 'audit_finding' AND r.source_id ~ '^[1-9][0-9]*$'
   AND EXISTS (SELECT 1 FROM entity_references p
                WHERE p.organization_id = r.organization_id
                  AND p.target_type = r.target_type AND p.target_id = r.target_id
                  AND p.source_type = r.source_type AND p.source_id = 'FIND-' || r.source_id);
UPDATE entity_references SET source_id = 'FIND-' || source_id
 WHERE source_type = 'audit_finding' AND source_id ~ '^[1-9][0-9]*$';

DELETE FROM entity_references r
 WHERE r.target_type = 'audit' AND r.target_id ~ '^[1-9][0-9]*$'
   AND EXISTS (SELECT 1 FROM entity_references p
                WHERE p.organization_id = r.organization_id
                  AND p.source_type = r.source_type AND p.source_id = r.source_id
                  AND p.target_type = r.target_type AND p.target_id = 'AUDIT-' || r.target_id);
UPDATE entity_references SET target_id = 'AUDIT-' || target_id
 WHERE target_type = 'audit' AND target_id ~ '^[1-9][0-9]*$';

DELETE FROM entity_references r
 WHERE r.source_type = 'audit' AND r.source_id ~ '^[1-9][0-9]*$'
   AND EXISTS (SELECT 1 FROM entity_references p
                WHERE p.organization_id = r.organization_id
                  AND p.target_type = r.target_type AND p.target_id = r.target_id
                  AND p.source_type = r.source_type AND p.source_id = 'AUDIT-' || r.source_id);
UPDATE entity_references SET source_id = 'AUDIT-' || source_id
 WHERE source_type = 'audit' AND source_id ~ '^[1-9][0-9]*$';
