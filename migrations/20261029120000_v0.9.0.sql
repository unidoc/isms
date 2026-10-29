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

-- #414: risk acceptance (accepted_at, accepted_by_id, the API's auto-set-on-
-- transition logic, the CLI's `risk treat --decision accept`) has existed since
-- the initial schema, but 'accepted' was never in the allowed status list, so
-- none of it could ever run. This only widens the set (full list kept
-- explicit), so every existing row still satisfies it. DROP IF EXISTS + re-ADD
-- is safe on fresh databases and on a second run.
ALTER TABLE risks DROP CONSTRAINT IF EXISTS risks_status_check;
ALTER TABLE risks ADD CONSTRAINT risks_status_check
    CHECK (status IN ('draft', 'open', 'accepted', 'closed'));

-- #394: an audit programme is a link source/target like an audit or a finding
-- (display id AP-<row id>, stored as such), and its Discussion tab comments on
-- it. Neither CHECK listed 'audit_programme', so the API refused the link and
-- the comment insert failed at the database. These only widen the sets (full
-- lists kept explicit), so every existing row still satisfies them. DROP IF
-- EXISTS + re-ADD is safe on fresh databases and on a second run.
ALTER TABLE entity_references DROP CONSTRAINT IF EXISTS entity_references_source_type_check;
ALTER TABLE entity_references ADD CONSTRAINT entity_references_source_type_check
    CHECK (source_type IN ('document','risk','legal_requirement','asset','supplier','system','incident','corrective_action','objective','program','audit','audit_finding','audit_programme','change_request','task'));
ALTER TABLE entity_references DROP CONSTRAINT IF EXISTS entity_references_target_type_check;
ALTER TABLE entity_references ADD CONSTRAINT entity_references_target_type_check
    CHECK (target_type IN ('document','risk','legal_requirement','asset','supplier','system','incident','corrective_action','objective','program','audit','audit_finding','audit_programme','change_request','task'));
ALTER TABLE entity_comments DROP CONSTRAINT IF EXISTS entity_comments_entity_type_check;
ALTER TABLE entity_comments ADD CONSTRAINT entity_comments_entity_type_check
    CHECK (entity_type IN ('risk','supplier','incident','legal_requirement','change_request','corrective_action','objective','task','system','asset','audit','audit_finding','audit_programme','program','checkin','access_review','document','review'));

-- #438: entity comments are stored, and listed, under the same canonical id
-- /references uses: AUDIT-/FIND-/AP-<row id> for audits, findings and
-- programmes, a program's key, an objective's display id. The UI used to post
-- the bare row id for these, and those threads would vanish from the UI now
-- that reads canonicalise. Rewrite only rows whose bare numeric id names a real
-- row in the SAME organisation (deleted or not); orphans and every other type
-- are left alone. Ids are compared as text so a junk value never hits a cast.
-- Re-runnable: after one pass nothing matches '^[0-9]+$' (program keys and
-- display ids start with a letter).
UPDATE entity_comments e SET entity_id = 'AUDIT-' || e.entity_id
WHERE e.entity_type = 'audit' AND e.entity_id ~ '^[0-9]{1,18}$'
  AND EXISTS (SELECT 1 FROM audits a WHERE a.id::text = e.entity_id AND a.organization_id = e.organization_id);
UPDATE entity_comments e SET entity_id = 'FIND-' || e.entity_id
WHERE e.entity_type = 'audit_finding' AND e.entity_id ~ '^[0-9]{1,18}$'
  AND EXISTS (SELECT 1 FROM audit_findings f WHERE f.id::text = e.entity_id AND f.organization_id = e.organization_id);
UPDATE entity_comments e SET entity_id = 'AP-' || e.entity_id
WHERE e.entity_type = 'audit_programme' AND e.entity_id ~ '^[0-9]{1,18}$'
  AND EXISTS (SELECT 1 FROM audit_programmes p WHERE p.id::text = e.entity_id AND p.organization_id = e.organization_id);
UPDATE entity_comments e SET entity_id = p.key
FROM programs p
WHERE e.entity_type = 'program' AND e.entity_id ~ '^[0-9]{1,18}$'
  AND p.id::text = e.entity_id AND p.organization_id = e.organization_id;
UPDATE entity_comments e SET entity_id = o.display_id
FROM objectives o
WHERE e.entity_type = 'objective' AND e.entity_id ~ '^[0-9]{1,18}$'
  AND o.id::text = e.entity_id AND o.organization_id = e.organization_id;

-- #437: an audit programme's History tab was always empty: the programme
-- handlers never wrote entity_changelog rows, and the table's entity_type CHECK
-- did not list 'audit_programme', so such rows would have been refused. Widen
-- the CHECK to every type it already allowed plus 'audit_programme'; it only
-- adds a value, so no existing row can violate it. Re-runnable: DROP IF EXISTS
-- then re-ADD leaves the same constraint behind on a second run.
ALTER TABLE entity_changelog DROP CONSTRAINT IF EXISTS entity_changelog_entity_type_check;
ALTER TABLE entity_changelog ADD CONSTRAINT entity_changelog_entity_type_check
    CHECK (entity_type IN ('risk','supplier','incident','legal_requirement','change_request','corrective_action','objective','task','system','asset','audit','audit_finding','audit_programme','program','checkin','access_review','document','review'));

-- #422: the "Implement CR-n" task created on approval was tied to its change
-- only by title, so revoking the approval left it open and the next approval
-- created a second one. Link the task to its change instead. No backfill:
-- tasks created before this column stay unlinked. Re-runnable throughout.
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS change_request_id INTEGER;
-- Composite so a task can never point at another organisation's change.
-- Change requests are only soft-deleted, so the FK never blocks a delete.
ALTER TABLE tasks DROP CONSTRAINT IF EXISTS tasks_change_request_fk;
ALTER TABLE tasks ADD CONSTRAINT tasks_change_request_fk
    FOREIGN KEY (organization_id, change_request_id) REFERENCES change_requests(organization_id, id);
-- At most one live (open / in_progress) task per change; cancelled and done
-- ones are history and may sit beside a live one.
CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_change_request_live
    ON tasks(organization_id, change_request_id)
    WHERE change_request_id IS NOT NULL AND deleted_at IS NULL AND status IN ('open','in_progress');

-- #367: deleting a program only marked the program deleted, so its objectives
-- stayed live (lists, stats, overdue check-in reminders). Soft-delete every
-- live objective whose program is deleted, stamping it with the program's own
-- deleted_at, and give each one an objective 'delete' History row with the same
-- reason the app now writes. The actor is copied from the program's most recent
-- delete History row (an old double delete could leave several); a program with
-- none gets 'system'. Re-runnable: the UPDATE only matches live objectives, and
-- after one pass none remain under a deleted program, so a second run inserts
-- nothing.
WITH orphaned AS (
    UPDATE objectives o SET deleted_at = p.deleted_at, updated_at = now()
    FROM programs p
    WHERE p.id = o.program_id AND p.organization_id = o.organization_id
      AND o.deleted_at IS NULL AND p.deleted_at IS NOT NULL
    RETURNING o.id AS objective_id, o.organization_id, p.id AS program_id, p.key AS program_key
)
INSERT INTO entity_changelog (organization_id, entity_type, entity_id, action, changed_by, changed_by_user_id, reason)
SELECT x.organization_id, 'objective', x.objective_id, 'delete',
       COALESCE(c.changed_by, 'system'), c.changed_by_user_id,
       'program ' || x.program_key || ' deleted'
FROM orphaned x
LEFT JOIN LATERAL (
    SELECT changed_by, changed_by_user_id FROM entity_changelog
    WHERE organization_id = x.organization_id AND entity_type = 'program'
      AND action = 'delete' AND entity_id = x.program_id
    ORDER BY created_at DESC, id DESC LIMIT 1
) c ON true;

-- #316: external evidence links on register records (a certificate, an audit
-- report, a pen-test summary hosted elsewhere), with an optional valid-until
-- date that feeds the overdue summary. Keyed by the parent's row id, like
-- entity_changelog; the CHECK already lists the other register types so wiring
-- them later needs no migration (the API accepts suppliers only for now).
-- Soft-deleted rows stay for history. Re-runnable throughout.
CREATE TABLE IF NOT EXISTS external_links (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    entity_type     TEXT NOT NULL CHECK (entity_type IN ('supplier', 'asset', 'system', 'legal_requirement')),
    entity_id       BIGINT NOT NULL,
    url             TEXT NOT NULL CHECK (length(url) BETWEEN 1 AND 2048),
    label           TEXT NOT NULL CHECK (length(label) BETWEEN 1 AND 200),
    link_type       TEXT NOT NULL CHECK (link_type IN ('certificate', 'audit_report', 'penetration_test', 'attestation', 'contract', 'other')),
    valid_until     DATE,
    verified_at     DATE,
    verified_by_id  INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_by      TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_external_links_entity
    ON external_links(organization_id, entity_type, entity_id) WHERE deleted_at IS NULL;

ALTER TABLE external_links ENABLE ROW LEVEL SECURITY;
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies
                    WHERE tablename = 'external_links' AND policyname = 'tenant_isolation') THEN
        CREATE POLICY tenant_isolation ON external_links
            USING (organization_id = current_setting('app.current_org_id', true)::INTEGER)
            WITH CHECK (organization_id = current_setting('app.current_org_id', true)::INTEGER);
    END IF;
END $$;

-- #232: the weekly personal digest.
--
-- digest_enabled is the member's per-organization opt-out. Default on: a
-- digest is only sent when something is waiting on the member, and every mail
-- links to Settings, where it can be turned off.
ALTER TABLE organization_members
    ADD COLUMN IF NOT EXISTS digest_enabled BOOLEAN NOT NULL DEFAULT true;

-- One row per (organization, member, ISO week) records that the week's digest
-- was handled, so two runs of `isms server manager`, or two replicas, cannot
-- send the same week twice. The row is claimed with INSERT ... ON CONFLICT DO
-- NOTHING before the mail goes out. outcome 'sent' is a mail that went out;
-- 'empty' records that the member had nothing pending, so an item arriving
-- later the same week does not trigger a mid-week digest.
CREATE TABLE IF NOT EXISTS digest_sends (
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    period          TEXT NOT NULL,
    outcome         TEXT NOT NULL CHECK (outcome IN ('sent', 'empty')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (organization_id, user_id, period)
);

-- Same tenant_isolation policy as every org-scoped table. CREATE POLICY has no
-- IF NOT EXISTS, so it is guarded for re-runs.
ALTER TABLE digest_sends ENABLE ROW LEVEL SECURITY;
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies
                    WHERE tablename = 'digest_sends' AND policyname = 'tenant_isolation') THEN
        CREATE POLICY tenant_isolation ON digest_sends
            USING (organization_id = current_setting('app.current_org_id', true)::INTEGER)
            WITH CHECK (organization_id = current_setting('app.current_org_id', true)::INTEGER);
    END IF;
END $$;
