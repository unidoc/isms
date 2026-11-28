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
-- (field access_level). The entity_type CHECK has to allow 'trust_center_item'
-- (#517) and 'trust_center_settings' (#519: clearing the overview document when
-- it is retired or deleted is recorded under it); it is widened once, with the
-- final list, further down. It is deliberately NOT re-added here with a shorter
-- list: the runner records migrations by file name, so a database that already
-- ran an earlier name of this file runs the whole file again, and a shorter
-- list would be violated by rows written since.

-- #518: a published document stays published across edits, but a document that
-- is retired or deleted must not come back by itself when it is un-retired or
-- re-created with the same id. The item is hidden instead of deleted; only
-- publishing it again clears the flag. hidden_reason says why.
ALTER TABLE trust_center_items ADD COLUMN IF NOT EXISTS hidden_at TIMESTAMPTZ;
ALTER TABLE trust_center_items ADD COLUMN IF NOT EXISTS hidden_reason TEXT;
-- The hidden_reason CHECK is set once, with its final list, at the end of this
-- file (see the note on entity_type above for why it is not added here).

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

-- Suppliers: public-facing fields for the trust center subprocessor list (#522).
-- All free text; empty means "not set". trust_public_changed_at is internal: it
-- moves only when the visible projection (public name falling back to name,
-- purpose, location, data categories) changes, and is written by the UPDATE SQL.
ALTER TABLE suppliers ADD COLUMN IF NOT EXISTS public_name TEXT NOT NULL DEFAULT '';
ALTER TABLE suppliers ADD COLUMN IF NOT EXISTS public_purpose TEXT NOT NULL DEFAULT '';
ALTER TABLE suppliers ADD COLUMN IF NOT EXISTS processing_location TEXT NOT NULL DEFAULT '';
ALTER TABLE suppliers ADD COLUMN IF NOT EXISTS data_categories TEXT NOT NULL DEFAULT '';
ALTER TABLE suppliers ADD COLUMN IF NOT EXISTS trust_public_changed_at TIMESTAMPTZ;
UPDATE suppliers SET trust_public_changed_at = COALESCE(updated_at, now()) WHERE trust_public_changed_at IS NULL;

-- #522: subprocessors are published from the Suppliers register. An item whose
-- supplier is terminated, loses data access or is deleted is hidden with its own
-- reason (it is not served until a manager publishes it again). Widen the CHECK
-- to the reasons already allowed plus these two.
ALTER TABLE trust_center_items DROP CONSTRAINT IF EXISTS trust_center_items_hidden_reason_check;
ALTER TABLE trust_center_items ADD CONSTRAINT trust_center_items_hidden_reason_check
    CHECK (hidden_reason IN ('retired','deleted','terminated','no_data_access'));

-- When a visitor-visible change to the subprocessor list last happened and is not
-- derivable from the items or the suppliers: unpublishing and reordering.
ALTER TABLE trust_center_settings ADD COLUMN IF NOT EXISTS subprocessors_changed_at TIMESTAMPTZ;

-- #525: click-through NDA for gated items. nda_document_id names the document
-- whose last approved version a visitor accepts before reading a gated item
-- (NULL = no NDA, signing in is enough). It is never cleared when the document
-- is retired or deleted: an NDA that is set but cannot be served locks gated
-- items for everyone. nda_skip_domains lists email domains that do not need to
-- accept: "example.com" matches that domain only, "*.example.com" any subdomain.
ALTER TABLE trust_center_settings ADD COLUMN IF NOT EXISTS nda_document_id TEXT;
ALTER TABLE trust_center_settings ADD COLUMN IF NOT EXISTS nda_skip_domains TEXT[] NOT NULL DEFAULT '{}';

-- One row per visitor, NDA version (content_hash of the served document_versions
-- row, version_id is its id; commit_hash and file_path locate the accepted bytes
-- in git, which stay provable if that row is overwritten later) and method. 'accepted' is a click-through;
-- 'domain_skip' is a log row written the first time a visitor is let in by the
-- skip list. A skip row is never a grant: the skip list is evaluated on every
-- request. company, ip and user_agent are snapshots taken at that moment.
CREATE TABLE IF NOT EXISTS trust_nda_acceptances (
    id              BIGSERIAL PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    visitor_id      BIGINT NOT NULL,
    document_id     TEXT NOT NULL,
    version_id      BIGINT NOT NULL,
    version         TEXT NOT NULL DEFAULT '',
    content_hash    TEXT NOT NULL CHECK (content_hash <> ''),
    commit_hash     TEXT NOT NULL DEFAULT '',
    file_path       TEXT NOT NULL DEFAULT '',
    method          TEXT NOT NULL CHECK (method IN ('accepted','domain_skip')),
    company         TEXT NOT NULL DEFAULT '',
    ip              TEXT NOT NULL DEFAULT '',
    user_agent      TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, visitor_id, content_hash, method)
);
ALTER TABLE trust_nda_acceptances DROP CONSTRAINT IF EXISTS trust_nda_acceptances_visitor_fk;
ALTER TABLE trust_nda_acceptances ADD CONSTRAINT trust_nda_acceptances_visitor_fk
    FOREIGN KEY (organization_id, visitor_id) REFERENCES trust_visitors(organization_id, id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_trust_nda_acceptances_visitor
    ON trust_nda_acceptances(organization_id, visitor_id, created_at);

ALTER TABLE trust_nda_acceptances ENABLE ROW LEVEL SECURITY;
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies
                    WHERE tablename = 'trust_nda_acceptances' AND policyname = 'tenant_isolation') THEN
        CREATE POLICY tenant_isolation ON trust_nda_acceptances
            USING (organization_id = current_setting('app.current_org_id', true)::INTEGER)
            WITH CHECK (organization_id = current_setting('app.current_org_id', true)::INTEGER);
    END IF;
END $$;

-- ═══════════════════════════════════════════════════════════════════════
-- Versions of a document that no longer exists
-- ═══════════════════════════════════════════════════════════════════════
-- A deleted document keeps its document_versions rows for audit, but they must
-- never be read again, and a new document that reuses the id must not inherit
-- or overwrite them. A row is archived (archived_at set) when its document id
-- leaves the repository; every reader ignores archived rows, and the unique
-- key on (organization, document, version) applies to active rows only.
ALTER TABLE document_versions ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;
ALTER TABLE document_versions DROP CONSTRAINT IF EXISTS document_versions_organization_id_document_id_version_key;
CREATE UNIQUE INDEX IF NOT EXISTS document_versions_active_key
    ON document_versions (organization_id, document_id, version) WHERE archived_at IS NULL;

-- One-time archiving of the versions of documents that were deleted before
-- versions were archived: the server does it once per organization at start and
-- records it here.
CREATE TABLE IF NOT EXISTS document_version_backfill (
    organization_id INTEGER PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    done_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE document_version_backfill ENABLE ROW LEVEL SECURITY;
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_policies
                    WHERE tablename = 'document_version_backfill' AND policyname = 'tenant_isolation') THEN
        CREATE POLICY tenant_isolation ON document_version_backfill
            USING (organization_id = current_setting('app.current_org_id', true)::INTEGER)
            WITH CHECK (organization_id = current_setting('app.current_org_id', true)::INTEGER);
    END IF;
END $$;
