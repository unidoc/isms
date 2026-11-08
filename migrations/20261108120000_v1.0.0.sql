-- v1.0.0 schema changes. Every statement is re-runnable (IF NOT EXISTS,
-- guarded policies, DROP IF EXISTS then ADD) so a second run changes nothing.

-- #517: the trust center, the first feature that serves an organization's data
-- to people who are not signed in.
--
-- trust_center_settings holds one row per organization. A missing row means
-- the trust center was never set up, which the application reads as disabled;
-- `enabled` defaults to false so nothing becomes public by creating the row.
-- default_access_level is the level a newly published item starts with.
CREATE TABLE IF NOT EXISTS trust_center_settings (
    organization_id      INTEGER PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    enabled              BOOLEAN NOT NULL DEFAULT false,
    title                TEXT NOT NULL DEFAULT '' CHECK (length(title) <= 200),
    overview_document_id TEXT,
    security_contact     TEXT NOT NULL DEFAULT '' CHECK (length(security_contact) <= 320),
    default_access_level TEXT NOT NULL DEFAULT 'gated'
                         CHECK (default_access_level IN ('public','gated','restricted')),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_by           TEXT
);

-- trust_center_items is what an organization chose to publish. `reference`
-- points at the thing published (a document_id, a register id, ...), whose
-- meaning depends on `kind`; the public API never returns it. public_id is
-- the only identifier shown to the outside, so the serial id (which reveals
-- how many items exist) stays internal.
CREATE TABLE IF NOT EXISTS trust_center_items (
    id              BIGSERIAL PRIMARY KEY,
    public_id       UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL
                    CHECK (kind IN ('document','certification','subprocessor','card','faq','update')),
    reference       TEXT NOT NULL CHECK (length(reference) BETWEEN 1 AND 200),
    access_level    TEXT NOT NULL CHECK (access_level IN ('public','gated','restricted')),
    section         TEXT NOT NULL DEFAULT '' CHECK (length(section) <= 100),
    sort_order      INTEGER NOT NULL DEFAULT 0,
    published_by    TEXT NOT NULL,
    published_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, kind, reference)
);
CREATE INDEX IF NOT EXISTS idx_trust_center_items_org_section
    ON trust_center_items(organization_id, section, sort_order);

ALTER TABLE trust_center_settings ENABLE ROW LEVEL SECURITY;
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies
                    WHERE tablename = 'trust_center_settings' AND policyname = 'tenant_isolation') THEN
        CREATE POLICY tenant_isolation ON trust_center_settings
            USING (organization_id = current_setting('app.current_org_id', true)::INTEGER)
            WITH CHECK (organization_id = current_setting('app.current_org_id', true)::INTEGER);
    END IF;
END $$;

ALTER TABLE trust_center_items ENABLE ROW LEVEL SECURITY;
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies
                    WHERE tablename = 'trust_center_items' AND policyname = 'tenant_isolation') THEN
        CREATE POLICY tenant_isolation ON trust_center_items
            USING (organization_id = current_setting('app.current_org_id', true)::INTEGER)
            WITH CHECK (organization_id = current_setting('app.current_org_id', true)::INTEGER);
    END IF;
END $$;

-- #517: publishing, unpublishing and changing the access level of a trust
-- center item are recorded in the entity changelog as create / delete / update
-- (field access_level). Widen the entity_type CHECK to every type it already
-- allowed plus 'trust_center_item'; it only adds a value, so no existing row
-- can violate it.
ALTER TABLE entity_changelog DROP CONSTRAINT IF EXISTS entity_changelog_entity_type_check;
ALTER TABLE entity_changelog ADD CONSTRAINT entity_changelog_entity_type_check
    CHECK (entity_type IN ('risk','supplier','incident','legal_requirement','change_request','corrective_action','objective','task','system','asset','audit','audit_finding','audit_programme','program','checkin','access_review','document','review','trust_center_item'));
