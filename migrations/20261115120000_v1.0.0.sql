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
-- can violate it. #519 adds 'trust_center_settings' (clearing the overview
-- document when it is retired or deleted is recorded under it).
ALTER TABLE entity_changelog DROP CONSTRAINT IF EXISTS entity_changelog_entity_type_check;
ALTER TABLE entity_changelog ADD CONSTRAINT entity_changelog_entity_type_check
    CHECK (entity_type IN ('risk','supplier','incident','legal_requirement','change_request','corrective_action','objective','task','system','asset','audit','audit_finding','audit_programme','program','checkin','access_review','document','review','trust_center_item','trust_center_settings'));

-- #518: a published document stays published across edits, but a document that
-- is retired or deleted must not come back by itself when it is un-retired or
-- re-created with the same id. The item is hidden instead of deleted; only
-- publishing it again clears the flag. hidden_reason says why.
ALTER TABLE trust_center_items ADD COLUMN IF NOT EXISTS hidden_at TIMESTAMPTZ;
ALTER TABLE trust_center_items ADD COLUMN IF NOT EXISTS hidden_reason TEXT;
ALTER TABLE trust_center_items DROP CONSTRAINT IF EXISTS trust_center_items_hidden_reason_check;
ALTER TABLE trust_center_items ADD CONSTRAINT trust_center_items_hidden_reason_check
    CHECK (hidden_reason IN ('retired','deleted'));

-- #521: certifications and audit reports shown in the trust center. A
-- certification is a record in its own right (it can exist unpublished);
-- publishing it adds a trust_center_items row of kind 'certification' with
-- reference = this table's id, and that row owns the access level, the order
-- and the public id, so there is no access_level column here.
--
-- The source is exactly one of a url or an uploaded file. The file's key, name,
-- type, size and sha256 are set together or not at all, and the type is limited
-- to what the server accepts after sniffing the bytes. valid_from / valid_until
-- / verified_at are DATE columns: the certificate is valid through its
-- valid_until day, and expiry is always judged by Postgres's CURRENT_DATE.
CREATE TABLE IF NOT EXISTS trust_center_certifications (
    id                BIGSERIAL PRIMARY KEY,
    organization_id   INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    label             TEXT NOT NULL CHECK (length(label) BETWEEN 1 AND 200),
    link_type         TEXT NOT NULL
                      CHECK (link_type IN ('certificate','audit_report','penetration_test','attestation','other')),
    issuer            TEXT NOT NULL DEFAULT '' CHECK (length(issuer) <= 200),
    valid_from        DATE,
    valid_until       DATE,
    url               TEXT CHECK (url IS NULL OR length(url) BETWEEN 1 AND 2048),
    file_key          TEXT,
    file_name         TEXT CHECK (file_name IS NULL OR length(file_name) <= 255),
    file_content_type TEXT CHECK (file_content_type IS NULL
                      OR file_content_type IN ('application/pdf','image/png','image/jpeg')),
    file_size         BIGINT,
    file_sha256       TEXT,
    verified_at       DATE,
    verified_by_id    INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_by        TEXT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT trust_center_certifications_dates_check
        CHECK (valid_from IS NULL OR valid_until IS NULL OR valid_from <= valid_until),
    CONSTRAINT trust_center_certifications_source_check
        CHECK ((url IS NOT NULL) <> (file_key IS NOT NULL)),
    CONSTRAINT trust_center_certifications_file_check
        CHECK ((file_key IS NULL AND file_name IS NULL AND file_content_type IS NULL
                AND file_size IS NULL AND file_sha256 IS NULL)
            OR (file_key IS NOT NULL AND file_name IS NOT NULL AND file_content_type IS NOT NULL
                AND file_size IS NOT NULL AND file_sha256 IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_trust_center_certifications_org
    ON trust_center_certifications(organization_id);

ALTER TABLE trust_center_certifications ENABLE ROW LEVEL SECURITY;
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies
                    WHERE tablename = 'trust_center_certifications' AND policyname = 'tenant_isolation') THEN
        CREATE POLICY tenant_isolation ON trust_center_certifications
            USING (organization_id = current_setting('app.current_org_id', true)::INTEGER)
            WITH CHECK (organization_id = current_setting('app.current_org_id', true)::INTEGER);
    END IF;
END $$;

-- #521: creating, editing, verifying and deleting a certification is recorded
-- in the entity changelog under 'trust_center_certification'. Widen the
-- entity_type CHECK to every type it already allowed plus that one; it only
-- adds a value, so no existing row can violate it.
ALTER TABLE entity_changelog DROP CONSTRAINT IF EXISTS entity_changelog_entity_type_check;
ALTER TABLE entity_changelog ADD CONSTRAINT entity_changelog_entity_type_check
    CHECK (entity_type IN ('risk','supplier','incident','legal_requirement','change_request','corrective_action','objective','task','system','asset','audit','audit_finding','audit_programme','program','checkin','access_review','document','review','trust_center_item','trust_center_settings','trust_center_certification'));

-- #524: trust center visitor sign-in by one-time email link.
--
-- A visitor (prospect, auditor) is NOT an ISMS user: never a row in users or
-- organization_members. A visitor belongs to one organization; the same email
-- at two organizations is two visitors. Name and company are asked once, at
-- the first sign-in. blocked stops sign-in and revokes the sessions.
CREATE TABLE IF NOT EXISTS trust_visitors (
    id              BIGSERIAL PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email           TEXT NOT NULL CHECK (email = lower(email) AND length(email) <= 320),
    name            TEXT NOT NULL DEFAULT '' CHECK (length(name) <= 200),
    company         TEXT NOT NULL DEFAULT '' CHECK (length(company) <= 200),
    verified_at     TIMESTAMPTZ,
    first_seen      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen       TIMESTAMPTZ NOT NULL DEFAULT now(),
    blocked         BOOLEAN NOT NULL DEFAULT false,
    blocked_at      TIMESTAMPTZ,
    blocked_by      TEXT,
    UNIQUE (organization_id, email)
);
-- (organization_id, id) is unique so a session can reference its visitor
-- together with the organization: a session can never belong to another
-- organization's visitor.
CREATE UNIQUE INDEX IF NOT EXISTS trust_visitors_org_id_uq ON trust_visitors(organization_id, id);

-- Sign-in links. Only the sha256 of the token is stored. A link is valid
-- until expires_at and until used_at is set; redeeming sets used_at in one
-- atomic UPDATE. The indexes serve the per-email and per-organization send caps.
CREATE TABLE IF NOT EXISTS trust_visitor_links (
    id              BIGSERIAL PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email           TEXT NOT NULL,
    token_hash      TEXT NOT NULL UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL,
    used_at         TIMESTAMPTZ
);
ALTER TABLE trust_visitor_links DROP CONSTRAINT IF EXISTS trust_visitor_links_email_check;
ALTER TABLE trust_visitor_links ADD CONSTRAINT trust_visitor_links_email_check
    CHECK (email = lower(email) AND length(email) <= 320);
CREATE INDEX IF NOT EXISTS idx_trust_visitor_links_org_email
    ON trust_visitor_links(organization_id, email, created_at);
CREATE INDEX IF NOT EXISTS idx_trust_visitor_links_org_created
    ON trust_visitor_links(organization_id, created_at);
-- The per-address cap across all organizations counts by email alone.
CREATE INDEX IF NOT EXISTS idx_trust_visitor_links_email_created
    ON trust_visitor_links(email, created_at);

-- Visitor sessions: opaque token stored hashed, revocable at once.
CREATE TABLE IF NOT EXISTS trust_visitor_sessions (
    id              BIGSERIAL PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    visitor_id      BIGINT NOT NULL,
    token_hash      TEXT NOT NULL UNIQUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,
    last_seen_at    TIMESTAMPTZ
);
-- The visitor reference carries the organization, replacing the single-column
-- foreign key an earlier version of this file created.
ALTER TABLE trust_visitor_sessions DROP CONSTRAINT IF EXISTS trust_visitor_sessions_visitor_id_fkey;
ALTER TABLE trust_visitor_sessions DROP CONSTRAINT IF EXISTS trust_visitor_sessions_visitor_fk;
ALTER TABLE trust_visitor_sessions ADD CONSTRAINT trust_visitor_sessions_visitor_fk
    FOREIGN KEY (organization_id, visitor_id) REFERENCES trust_visitors(organization_id, id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_trust_visitor_sessions_visitor
    ON trust_visitor_sessions(visitor_id);

-- blocked_email_domains: NULL = the built-in list of personal providers, an
-- array = the organization's own list, an empty array = allow every domain.
ALTER TABLE trust_center_settings ADD COLUMN IF NOT EXISTS blocked_email_domains TEXT[];
ALTER TABLE trust_center_settings ADD COLUMN IF NOT EXISTS privacy_notice_url TEXT NOT NULL DEFAULT '';
ALTER TABLE trust_center_settings DROP CONSTRAINT IF EXISTS trust_center_settings_privacy_notice_url_check;
ALTER TABLE trust_center_settings ADD CONSTRAINT trust_center_settings_privacy_notice_url_check
    CHECK (length(privacy_notice_url) <= 2000);

ALTER TABLE trust_visitors ENABLE ROW LEVEL SECURITY;
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies
                    WHERE tablename = 'trust_visitors' AND policyname = 'tenant_isolation') THEN
        CREATE POLICY tenant_isolation ON trust_visitors
            USING (organization_id = current_setting('app.current_org_id', true)::INTEGER)
            WITH CHECK (organization_id = current_setting('app.current_org_id', true)::INTEGER);
    END IF;
END $$;

ALTER TABLE trust_visitor_links ENABLE ROW LEVEL SECURITY;
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies
                    WHERE tablename = 'trust_visitor_links' AND policyname = 'tenant_isolation') THEN
        CREATE POLICY tenant_isolation ON trust_visitor_links
            USING (organization_id = current_setting('app.current_org_id', true)::INTEGER)
            WITH CHECK (organization_id = current_setting('app.current_org_id', true)::INTEGER);
    END IF;
END $$;

ALTER TABLE trust_visitor_sessions ENABLE ROW LEVEL SECURITY;
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies
                    WHERE tablename = 'trust_visitor_sessions' AND policyname = 'tenant_isolation') THEN
        CREATE POLICY tenant_isolation ON trust_visitor_sessions
            USING (organization_id = current_setting('app.current_org_id', true)::INTEGER)
            WITH CHECK (organization_id = current_setting('app.current_org_id', true)::INTEGER);
    END IF;
END $$;
