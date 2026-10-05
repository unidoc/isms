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

-- #194: a #RISK-1 mention in a comment now saves a link between the comment's
-- subject and the mentioned record, and deleting the comment removes the link
-- again. Two pieces make that safe:
--
-- entity_references.origin says who created a link. Every existing writer
-- (the Links tab, create-with-references, incident link suggestions) is
-- 'manual', the default, so existing rows are manual. Only comment mentions
-- write 'comment', and a manual write on an existing pair upgrades it to
-- 'manual', so a link someone also added by hand is never removed with a
-- comment.
ALTER TABLE entity_references ADD COLUMN IF NOT EXISTS origin TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE entity_references DROP CONSTRAINT IF EXISTS entity_references_origin_check;
ALTER TABLE entity_references ADD CONSTRAINT entity_references_origin_check
    CHECK (origin IN ('manual', 'comment'));

-- comment_references records which comment mentions which pair, so a link
-- two comments share survives deleting one of them. comment_type names the
-- table comment_id lives in: 'comment' (document and review comments) or
-- 'entity_comment' (register records). No FK, since the id is polymorphic;
-- the delete path removes a comment's rows in the same transaction.
CREATE TABLE IF NOT EXISTS comment_references (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    comment_type    TEXT NOT NULL CHECK (comment_type IN ('comment', 'entity_comment')),
    comment_id      BIGINT NOT NULL,
    source_type     TEXT NOT NULL,
    source_id       TEXT NOT NULL,
    target_type     TEXT NOT NULL,
    target_id       TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, comment_type, comment_id, source_type, source_id, target_type, target_id)
);
CREATE INDEX IF NOT EXISTS idx_comment_refs_pair
    ON comment_references(organization_id, source_type, source_id, target_type, target_id);

-- Same tenant_isolation policy as every org-scoped table in the initial
-- schema. CREATE POLICY has no IF NOT EXISTS, so it is guarded for re-runs.
ALTER TABLE comment_references ENABLE ROW LEVEL SECURITY;
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies
                    WHERE tablename = 'comment_references' AND policyname = 'tenant_isolation') THEN
        CREATE POLICY tenant_isolation ON comment_references
            USING (organization_id = current_setting('app.current_org_id', true)::INTEGER)
            WITH CHECK (organization_id = current_setting('app.current_org_id', true)::INTEGER);
    END IF;
END $$;
