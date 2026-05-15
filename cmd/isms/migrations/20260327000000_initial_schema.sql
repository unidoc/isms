-- ISMS platform schema (multi-tenant)
-- Documents live in git (per org). This DB handles collaboration, workflow, and audit trail.
-- Every tenant-scoped table has organization_id. Users are global; roles are per-org.

-- ═══════════════════════════════════════════════════════════════════════
-- ORGANIZATIONS
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS organizations (
    id         INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    uuid       UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL,
    repo_path  TEXT NOT NULL,
    domain     TEXT,                              -- custom domain (e.g. isms.mycompany.com)
    deleted_at TIMESTAMPTZ,                       -- soft delete (NULL = active)
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_organizations_domain ON organizations(lower(domain)) WHERE domain IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_organizations_slug_lower ON organizations(lower(slug)) WHERE deleted_at IS NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- SETTINGS (registry of known settings + per-org values)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS settings (
    key            TEXT PRIMARY KEY,
    description    TEXT NOT NULL,
    category       TEXT NOT NULL,            -- notifications, integrations, branding, etc.
    default_value  TEXT,
    sensitive      BOOLEAN NOT NULL DEFAULT false  -- true = value is encrypted at rest
);

CREATE TABLE IF NOT EXISTS organization_settings (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    setting_key     TEXT NOT NULL REFERENCES settings(key),
    value           TEXT NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(organization_id, setting_key)
);

CREATE INDEX IF NOT EXISTS idx_org_settings_org ON organization_settings(organization_id);

-- Seed known settings
INSERT INTO settings (key, description, category, default_value, sensitive) VALUES
    ('slack_webhook',               'Slack incoming webhook URL',                               'notifications', NULL, true),
    ('matrix_room_id',              'Matrix room ID for notifications',                         'notifications', NULL, false),
    ('matrix_token',                'Matrix access token',                                      'notifications', NULL, true),
    ('matrix_server',               'Matrix server URL',                                        'notifications', NULL, false),
    ('risk_review_cycle_critical',  'Review cycle for critical risks (months)',                  'review_cycles', '1', false),
    ('risk_review_cycle_high',      'Review cycle for high risks (months)',                      'review_cycles', '3', false),
    ('risk_review_cycle_medium',    'Review cycle for medium risks (months)',                    'review_cycles', '6', false),
    ('risk_review_cycle_low',       'Review cycle for low risks (months)',                       'review_cycles', '12', false),
    ('risk_appetite',               'Risk appetite threshold (current_score above this requires treatment)', 'risk', '9', false),
    ('branding_name',               'Organization display name (overrides org.name in UI)',             'branding', NULL, false),
    ('branding_color',              'Primary brand color (hex, e.g. #1a365d)',                          'branding', NULL, false),
    ('branding_footer',             'Footer text shown in UI and exports',                              'branding', NULL, false),
    ('show_powered_by',             'Show "Powered by isms.sh" in footer (true/false)',                 'branding', 'true', false),
    ('terms_url',                   'Custom Terms of Service URL (overrides platform file)',             'branding', NULL, false),
    ('privacy_url',                 'Custom Privacy Policy URL (overrides platform file)',               'branding', NULL, false),
    ('ai_enabled',                      'Enable AI features (agent users, suggestions, MCP). Set to false to disable all AI.', 'ai', 'true', false),
    ('ai_review_max_rounds',            'Maximum AI-to-AI review rounds before escalation to human',       'ai', '3', false)
ON CONFLICT (key) DO NOTHING;

-- ═══════════════════════════════════════════════════════════════════════
-- USERS (global — not org-scoped)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS users (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email           TEXT NOT NULL,
    name            TEXT NOT NULL,
    password_hash   TEXT,                            -- bcrypt, NULL = external auth only
    otp_secret      TEXT,                            -- TOTP secret (base32), NULL = OTP not enabled
    otp_verified    BOOLEAN NOT NULL DEFAULT false,
    email_verified  BOOLEAN NOT NULL DEFAULT false,
    is_agent        BOOLEAN NOT NULL DEFAULT false,   -- true for AI/bot accounts
    active          BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen       TIMESTAMPTZ,
    last_totp_at    TIMESTAMPTZ                      -- TOTP replay protection: last successful TOTP window
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_users_email_lower ON users(lower(email));

-- ═══════════════════════════════════════════════════════════════════════
-- USER IDENTITIES (OIDC / external IdP links — replaces users.oidc_subject)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS user_identities (
    id          INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider    TEXT NOT NULL,              -- 'microsoft', 'google', 'okta'
    subject     TEXT NOT NULL,              -- OIDC sub claim
    email       TEXT,                       -- email from IdP at time of link
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(provider, subject)
);
CREATE INDEX IF NOT EXISTS idx_user_identities_user ON user_identities(user_id);

-- ═══════════════════════════════════════════════════════════════════════
-- ORGANIZATION MEMBERS (role is per-org)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS organization_members (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            TEXT NOT NULL DEFAULT 'reader' CHECK (role IN ('admin','manager','contributor','reader')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(organization_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_org_members_org ON organization_members(organization_id);
CREATE INDEX IF NOT EXISTS idx_org_members_user ON organization_members(user_id);

-- ═══════════════════════════════════════════════════════════════════════
-- API KEYS (Personal Access Tokens — belong to user, not org)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS api_keys (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name            TEXT NOT NULL,
    token_hash      TEXT NOT NULL UNIQUE,
    user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    organization_id INTEGER REFERENCES organizations(id) ON DELETE CASCADE,  -- NULL = all orgs, set = org-scoped
    permissions     TEXT NOT NULL DEFAULT 'read-write' CHECK (permissions IN ('read','write','read-write')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at      TIMESTAMPTZ,
    last_used_at    TIMESTAMPTZ,
    expires_at      TIMESTAMPTZ
);

-- idx_api_keys_hash is redundant: UNIQUE constraint on token_hash already creates an index
CREATE INDEX IF NOT EXISTS idx_api_keys_active ON api_keys(token_hash) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id);

-- ═══════════════════════════════════════════════════════════════════════
-- EMAIL VERIFICATION TOKENS (global — tied to user, not org)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS email_verifications (
    id           INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL UNIQUE,
    purpose      TEXT NOT NULL DEFAULT 'verify' CHECK (purpose IN ('verify', 'reset')),
    expires_at   TIMESTAMPTZ NOT NULL,
    used_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_verify_hash ON email_verifications(token_hash);
CREATE INDEX IF NOT EXISTS idx_email_verify_user ON email_verifications(user_id);

-- ═══════════════════════════════════════════════════════════════════════
-- REVIEWS & APPROVAL WORKFLOW
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS reviews (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    document_id     TEXT NOT NULL,
    document_type   TEXT NOT NULL,
    title           TEXT NOT NULL,
    version         TEXT NOT NULL,
    commit_hash     TEXT,
    sent_head       TEXT,
    merge_commit    TEXT,
    round           INTEGER NOT NULL DEFAULT 1,
    requested_by_id INTEGER NOT NULL REFERENCES users(id),
    message         TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','approved','changes_requested','closed','merged')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id)
);

CREATE INDEX IF NOT EXISTS idx_reviews_org ON reviews(organization_id);
CREATE INDEX IF NOT EXISTS idx_reviews_document ON reviews(organization_id, document_id);
CREATE INDEX IF NOT EXISTS idx_reviews_status ON reviews(organization_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_reviews_active_doc ON reviews(organization_id, document_id) WHERE status IN ('open', 'changes_requested', 'approved');

CREATE TABLE IF NOT EXISTS review_assignments (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    review_id       INTEGER NOT NULL,
    reviewer_id     INTEGER NOT NULL REFERENCES users(id),
    status          TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','changes_requested','proposed_revision')),
    due_date        DATE,
    reviewed_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(review_id, reviewer_id),
    FOREIGN KEY (organization_id, review_id) REFERENCES reviews(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_review_assignments_org ON review_assignments(organization_id);
CREATE INDEX IF NOT EXISTS idx_review_assignments_reviewer ON review_assignments(reviewer_id);
CREATE INDEX IF NOT EXISTS idx_review_assignments_review ON review_assignments(review_id);

CREATE TABLE IF NOT EXISTS approvals (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    review_id       INTEGER,
    document_id     TEXT NOT NULL,
    version         TEXT NOT NULL,
    round           INTEGER NOT NULL DEFAULT 1,
    decision        TEXT NOT NULL CHECK (decision IN ('approved','changes_requested','proposed_revision','confirmed')),
    approved_by     TEXT NOT NULL,
    approved_by_user_id INTEGER REFERENCES users(id),
    comment         TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (organization_id, review_id) REFERENCES reviews(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_approvals_org ON approvals(organization_id);
CREATE INDEX IF NOT EXISTS idx_approvals_document ON approvals(organization_id, document_id);
CREATE INDEX IF NOT EXISTS idx_approvals_review ON approvals(review_id);
-- Prevent duplicate approvals from the same reviewer on the same review round (race condition guard).
CREATE UNIQUE INDEX IF NOT EXISTS idx_approvals_unique_reviewer_round ON approvals(organization_id, review_id, approved_by, round);

-- ═══════════════════════════════════════════════════════════════════════
-- DOCUMENT VERSIONS (git snapshot per version)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS document_versions (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    document_id     TEXT NOT NULL,
    version         TEXT NOT NULL,
    commit_hash     TEXT NOT NULL,
    file_path       TEXT NOT NULL,
    content_hash    TEXT,
    message         TEXT,
    owner           TEXT,                             -- snapshot from frontmatter
    review_cycle_months INTEGER,                      -- snapshot from frontmatter
    created_by      TEXT NOT NULL,                    -- snapshot
    created_by_user_id INTEGER REFERENCES users(id),  -- FK to user
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(organization_id, document_id, version)
);

CREATE INDEX IF NOT EXISTS idx_doc_versions_org ON document_versions(organization_id);
CREATE INDEX IF NOT EXISTS idx_doc_versions_document ON document_versions(organization_id, document_id);

-- ═══════════════════════════════════════════════════════════════════════
-- COMMENTS
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS comments (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    review_id       INTEGER,
    document_id     TEXT NOT NULL,
    author          TEXT NOT NULL,
    author_user_id  INTEGER REFERENCES users(id),
    body            TEXT NOT NULL,
    section         TEXT,
    paragraph_index INTEGER,
    paragraph_hash  TEXT,
    quote           TEXT,
    parent_id       BIGINT,
    status          TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved')),
    resolved_by_id  INTEGER REFERENCES users(id),
    resolved_at     TIMESTAMPTZ,
    suggestion_body TEXT,
    suggestion_status TEXT CHECK (suggestion_status IN ('pending','accepted','rejected')),
    suggestion_resolved_by_id INTEGER REFERENCES users(id),
    suggestion_resolved_at TIMESTAMPTZ,
    is_outdated     BOOLEAN NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    edited_at       TIMESTAMPTZ,
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, review_id) REFERENCES reviews(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, parent_id) REFERENCES comments(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_comment_resolved CHECK (status != 'resolved' OR (resolved_at IS NOT NULL AND resolved_by_id IS NOT NULL)),
    CONSTRAINT chk_suggestion CHECK ((suggestion_body IS NULL AND suggestion_status IS NULL) OR (suggestion_body IS NOT NULL AND suggestion_status IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_comments_org ON comments(organization_id);
CREATE INDEX IF NOT EXISTS idx_comments_document ON comments(organization_id, document_id);
CREATE INDEX IF NOT EXISTS idx_comments_review ON comments(review_id);
CREATE INDEX IF NOT EXISTS idx_comments_status ON comments(organization_id, status);
CREATE INDEX IF NOT EXISTS idx_comments_suggestions ON comments(organization_id, review_id) WHERE suggestion_body IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_comments_parent ON comments(organization_id, parent_id);

-- ═══════════════════════════════════════════════════════════════════════
-- INCIDENTS
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS incidents (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    identifier      TEXT NOT NULL,                      -- INC-1, INC-2, ...
    title           TEXT NOT NULL,
    description     TEXT NOT NULL,
    severity        TEXT NOT NULL DEFAULT 'medium' CHECK (severity IN ('critical','high','medium','low')),
    status          TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('draft','open','investigating','contained','resolved','closed')),
    affects_c       BOOLEAN NOT NULL DEFAULT false,
    affects_i       BOOLEAN NOT NULL DEFAULT false,
    affects_a       BOOLEAN NOT NULL DEFAULT false,
    incident_type   TEXT NOT NULL DEFAULT 'event' CHECK (incident_type IN ('incident','event','weakness')),
    source          TEXT NOT NULL DEFAULT 'internal' CHECK (source IN ('internal','external','internal and external')),
    notes           TEXT,
    -- Data breach / GDPR fields
    data_breach     BOOLEAN NOT NULL DEFAULT false,
    gdpr_role       TEXT CHECK (gdpr_role IN ('controller','processor')),
    authority_notified   TEXT NOT NULL DEFAULT 'not_required' CHECK (authority_notified IN ('not_required','pending','notified')),
    authority_notified_at TIMESTAMPTZ,
    subjects_notified    TEXT NOT NULL DEFAULT 'not_required' CHECK (subjects_notified IN ('not_required','pending','notified')),
    subjects_notified_at TIMESTAMPTZ,
    reporter        TEXT NOT NULL,                     -- email of person who reported
    reporter_user_id INTEGER REFERENCES users(id),
    assignee_id     INTEGER REFERENCES users(id),       -- person handling
    detected_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    contained_at    TIMESTAMPTZ,
    resolved_at     TIMESTAMPTZ,
    closed_at       TIMESTAMPTZ,
    root_cause      TEXT,
    lessons_learned TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, identifier)
);

CREATE INDEX IF NOT EXISTS idx_incidents_org ON incidents(organization_id);
CREATE INDEX IF NOT EXISTS idx_incidents_status ON incidents(organization_id, status);
CREATE INDEX IF NOT EXISTS idx_incidents_severity ON incidents(organization_id, severity);
CREATE INDEX IF NOT EXISTS idx_incidents_not_deleted ON incidents(organization_id) WHERE deleted_at IS NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- TASKS
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS tasks (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    identifier      TEXT NOT NULL,                      -- TASK-1, TASK-2, ...
    title           TEXT NOT NULL,
    description     TEXT,
    task_type       TEXT NOT NULL CHECK (task_type IN ('general','review','incident_followup','audit_followup','ca_followup','change_followup','onboarding','offboarding','training','other')),
    -- cross-entity links (incident, document, control, etc.): use entity_references table
    assignee_id     INTEGER NOT NULL REFERENCES users(id),
    created_by      TEXT NOT NULL,
    created_by_user_id INTEGER REFERENCES users(id),
    status          TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','in_progress','done','cancelled')),
    priority        TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('critical','high','medium','low')),
    due_date        DATE,
    completed_at    TIMESTAMPTZ,
    recurrence_days INTEGER,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE (organization_id, id),
    UNIQUE (organization_id, identifier)
);

CREATE INDEX IF NOT EXISTS idx_tasks_org ON tasks(organization_id);
CREATE INDEX IF NOT EXISTS idx_tasks_assignee ON tasks(assignee_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(organization_id, status);
CREATE INDEX IF NOT EXISTS idx_tasks_due ON tasks(organization_id, due_date);
CREATE INDEX IF NOT EXISTS idx_tasks_not_deleted ON tasks(organization_id) WHERE deleted_at IS NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- CHANGE MANAGEMENT
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS change_requests (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    identifier      TEXT NOT NULL,                      -- CR-1, CR-2, ...
    title           TEXT NOT NULL,
    description     TEXT NOT NULL,
    justification   TEXT,
    priority        TEXT NOT NULL DEFAULT 'medium' CHECK (priority IN ('low','medium','high','critical')),
    category        TEXT NOT NULL DEFAULT 'process' CHECK (category IN ('process','technology','people','documentation','infrastructure','other')),
    risk_level      TEXT NOT NULL DEFAULT 'low' CHECK (risk_level IN ('low','medium','high','critical')),
    rollback_plan   TEXT,
    requested_by_id INTEGER NOT NULL REFERENCES users(id),
    assigned_to_id  INTEGER REFERENCES users(id),
    status          TEXT NOT NULL DEFAULT 'proposed' CHECK (status IN ('proposed','approved','rejected','in_progress','implemented','closed')),
    notes           TEXT,
    approved_by     TEXT,
    approved_by_user_id INTEGER REFERENCES users(id),
    approved_at     TIMESTAMPTZ,
    planned_at      TIMESTAMPTZ,
    implemented_at  TIMESTAMPTZ,
    -- document_ids: use entity_references table
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE(organization_id, id),
    UNIQUE(organization_id, identifier)
);

CREATE INDEX IF NOT EXISTS idx_changes_org ON change_requests(organization_id);
CREATE INDEX IF NOT EXISTS idx_changes_status ON change_requests(organization_id, status);
CREATE INDEX IF NOT EXISTS idx_change_requests_not_deleted ON change_requests(organization_id) WHERE deleted_at IS NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- IMPLEMENTATION TRACKING
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS implementation_status (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    item_id         TEXT NOT NULL,
    item_type       TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'not_started' CHECK (status IN ('not_started','in_progress','implemented','verified')),
    owner_id        INTEGER REFERENCES users(id),
    target_date     DATE,
    notes           TEXT,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(organization_id, item_type, item_id)
);

CREATE INDEX IF NOT EXISTS idx_impl_org ON implementation_status(organization_id);
CREATE INDEX IF NOT EXISTS idx_impl_status ON implementation_status(organization_id, status);
CREATE INDEX IF NOT EXISTS idx_impl_type ON implementation_status(organization_id, item_type);

-- ═══════════════════════════════════════════════════════════════════════
-- NOTIFICATIONS
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS notifications (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    recipient_id    INTEGER NOT NULL REFERENCES users(id),
    title           TEXT NOT NULL,
    body            TEXT,
    link            TEXT,
    read            BOOLEAN NOT NULL DEFAULT false,
    agent_actionable BOOLEAN NOT NULL DEFAULT false,  -- true = intended for agent, not human
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notifications_org ON notifications(organization_id);
CREATE INDEX IF NOT EXISTS idx_notifications_recipient ON notifications(organization_id, recipient_id, read);
CREATE INDEX IF NOT EXISTS idx_notifications_created ON notifications(created_at DESC);

-- ═══════════════════════════════════════════════════════════════════════
-- SUGGESTIONS
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS suggestions (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id     INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    entity_type         TEXT NOT NULL CHECK (entity_type IN ('risk','supplier','incident','legal_requirement','change_request','corrective_action','objective','task','system','asset','audit','audit_finding','program','checkin','access_review')),
    entity_id           TEXT,
    suggestion_type     TEXT NOT NULL CHECK (suggestion_type IN ('create','update','reassess','link','review','reading')),
    title               TEXT NOT NULL,
    payload             JSONB NOT NULL DEFAULT '{}',
    rationale           TEXT,
    source_refs         JSONB,
    entity_updated_at   TIMESTAMPTZ,
    status              TEXT NOT NULL DEFAULT 'open'
                        CHECK (status IN ('open','in_review','applied','rejected','withdrawn')),
    suggested_by        TEXT NOT NULL,
    suggested_by_user_id INTEGER REFERENCES users(id),
    suggested_by_type   TEXT NOT NULL DEFAULT 'user' CHECK (suggested_by_type IN ('user','agent')),
    reviewed_by         TEXT,
    reviewed_by_user_id INTEGER REFERENCES users(id),
    reviewed_at         TIMESTAMPTZ,
    applied_at          TIMESTAMPTZ,
    applied_entity_id   TEXT,
    reject_reason       TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_suggestions_org ON suggestions(organization_id);
CREATE INDEX IF NOT EXISTS idx_suggestions_entity ON suggestions(organization_id, entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_suggestions_status ON suggestions(organization_id, status);

-- ═══════════════════════════════════════════════════════════════════════
-- ENTITY COMMENTS (generic comments on any operational entity)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS entity_comments (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    entity_type     TEXT NOT NULL CHECK (entity_type IN ('risk','supplier','incident','legal_requirement','change_request','corrective_action','objective','task','system','asset','audit','audit_finding','program','checkin','access_review','document','review')),
    entity_id       TEXT NOT NULL,
    parent_id       BIGINT,
    author          TEXT NOT NULL,
    author_user_id  INTEGER REFERENCES users(id),
    body            TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved')),
    resolved_by     TEXT,
    resolved_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    edited_at       TIMESTAMPTZ,
    UNIQUE(organization_id, id),
    FOREIGN KEY (organization_id, parent_id) REFERENCES entity_comments(organization_id, id)
);

CREATE INDEX IF NOT EXISTS idx_entity_comments_entity ON entity_comments(organization_id, entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_entity_comments_parent ON entity_comments(parent_id);

-- ═══════════════════════════════════════════════════════════════════════
-- ENTITY REACTIONS (emoji reactions on comments and suggestions)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS entity_reactions (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    target_type     TEXT NOT NULL CHECK (target_type IN ('comment','suggestion','entity_comment')),
    target_id       BIGINT NOT NULL,
    emoji           TEXT NOT NULL,
    user_email      TEXT NOT NULL,
    user_id         INTEGER REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(organization_id, target_type, target_id, user_email, emoji)
);

CREATE INDEX IF NOT EXISTS idx_entity_reactions_target ON entity_reactions(organization_id, target_type, target_id);

-- ═══════════════════════════════════════════════════════════════════════
-- INTERNAL AUDITS
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS audit_programmes (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    year            INTEGER NOT NULL,
    description     TEXT,
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('draft','active','closed')),
    notes           TEXT,
    created_by      TEXT NOT NULL,
    created_by_user_id INTEGER REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE (organization_id, id)
);

CREATE INDEX IF NOT EXISTS idx_audit_programmes_org ON audit_programmes(organization_id);
CREATE INDEX IF NOT EXISTS idx_audit_programmes_not_deleted ON audit_programmes(organization_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS audits (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    programme_id    INTEGER,
    title           TEXT NOT NULL,
    scope           TEXT NOT NULL DEFAULT '',
    audit_type      TEXT NOT NULL DEFAULT 'internal' CHECK (audit_type IN ('internal','external','surveillance','certification','recertification')),
    auditor_id      INTEGER REFERENCES users(id),
    status          TEXT NOT NULL DEFAULT 'planned' CHECK (status IN ('planned','in_progress','completed')),
    planned_date    DATE,
    end_date        DATE,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    summary         TEXT,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, programme_id) REFERENCES audit_programmes(organization_id, id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_audits_org ON audits(organization_id);
CREATE INDEX IF NOT EXISTS idx_audits_programme ON audits(programme_id);
CREATE INDEX IF NOT EXISTS idx_audits_status ON audits(organization_id, status);
CREATE INDEX IF NOT EXISTS idx_audits_not_deleted ON audits(organization_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS audit_items (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    audit_id        INTEGER NOT NULL,
    item_id         TEXT NOT NULL,
    item_type       TEXT NOT NULL,
    title           TEXT NOT NULL,
    result          TEXT NOT NULL DEFAULT 'not_assessed' CHECK (result IN ('not_assessed','conforming','minor_nc','major_nc','observation','opportunity')),
    evidence        TEXT,
    notes           TEXT,
    assessed_at     TIMESTAMPTZ,
    assessed_by_user_id INTEGER REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, audit_id) REFERENCES audits(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_audit_items_org ON audit_items(organization_id);
CREATE INDEX IF NOT EXISTS idx_audit_items_audit ON audit_items(audit_id);
CREATE INDEX IF NOT EXISTS idx_audit_items_result ON audit_items(organization_id, result);

CREATE TABLE IF NOT EXISTS audit_findings (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    audit_id        INTEGER NOT NULL,
    audit_item_id   INTEGER,
    finding_type    TEXT NOT NULL CHECK (finding_type IN ('major_nc','minor_nc','observation','opportunity')),
    title           TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    -- cross-entity links (task, audit_item, etc.): use entity_references table + audit_item_id FK
    -- corrective action content lives in description (## Corrective Action heading)
    status          TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','closed')),
    due_date        DATE,
    owner_id        INTEGER REFERENCES users(id),
    closed_at       TIMESTAMPTZ,
    closed_by       TEXT,
    closed_by_user_id INTEGER REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, audit_id) REFERENCES audits(organization_id, id) ON DELETE CASCADE,
    FOREIGN KEY (organization_id, audit_item_id) REFERENCES audit_items(organization_id, id) ON DELETE SET NULL,
    CONSTRAINT chk_finding_closed CHECK (status != 'closed' OR closed_at IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_findings_org ON audit_findings(organization_id);
CREATE INDEX IF NOT EXISTS idx_findings_audit ON audit_findings(audit_id);
CREATE INDEX IF NOT EXISTS idx_findings_status ON audit_findings(organization_id, status);
CREATE INDEX IF NOT EXISTS idx_findings_due ON audit_findings(organization_id, due_date) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_findings_type ON audit_findings(organization_id, finding_type) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_audit_findings_not_deleted ON audit_findings(organization_id) WHERE deleted_at IS NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- LEGAL REGISTER (applicable legislation)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS legal_requirements (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    identifier      TEXT NOT NULL,                    -- per-org: LEGAL-001, set by app
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,                    -- e.g. "GDPR", "NIS2 Directive"
    description     TEXT,
    jurisdiction    TEXT NOT NULL DEFAULT 'EU',       -- EU, Iceland, US, Global, etc.
    category        TEXT NOT NULL DEFAULT 'privacy' CHECK (category IN ('privacy','security','sector','contractual','other')),
    reference       TEXT,                             -- article/section reference
    url             TEXT,                             -- link to legislation text
    status          TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('draft','open','closed')),
    owner_id        INTEGER REFERENCES users(id),      -- responsible person
    -- linked documents: use entity_references table
    -- Derived cache from entity_readings (canonical assessment log).
    -- Updated by app on reading insert; do not write directly.
    last_review     DATE,
    next_review     DATE,
    notes           TEXT,
    -- Risk assessment (current = the live assessment; CIA / inherent baseline live on linked risks)
    current_likelihood  INTEGER CHECK (current_likelihood BETWEEN 0 AND 5),  -- NULL = not assessed
    current_impact      INTEGER CHECK (current_impact BETWEEN 0 AND 5),
    current_score       INTEGER CHECK (current_score BETWEEN 0 AND 25),      -- auto: computed
    current_level       TEXT CHECK (current_level IN ('low','medium','high','critical')),
    treatment           TEXT CHECK (treatment IN ('mitigate','accept','transfer','avoid')),  -- NULL = not decided
    treatment_plan      TEXT,
    target_likelihood   INTEGER CHECK (target_likelihood BETWEEN 0 AND 5),
    target_impact       INTEGER CHECK (target_impact BETWEEN 0 AND 5),
    completion          INTEGER DEFAULT 0 CHECK (completion BETWEEN 0 AND 100),
    -- Assessment lifecycle: see entity_readings table for the canonical assessment log
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_legal_org ON legal_requirements(organization_id);
CREATE INDEX IF NOT EXISTS idx_legal_requirements_not_deleted ON legal_requirements(organization_id) WHERE deleted_at IS NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- ASSETS
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS assets (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    identifier      TEXT NOT NULL,                    -- per-org: ASSET-001, set by app
    name            TEXT NOT NULL,
    description     TEXT,
    asset_type      TEXT NOT NULL DEFAULT 'other' CHECK (asset_type IN ('infrastructure','processing_devices','software','financial_info','personal_data','ipr','sales_marketing','processing_facility','products_services','supply_chain','system','network','service','other')),
    status          TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('draft','open','archived')),
    owner_id        INTEGER REFERENCES users(id),
    primary_location TEXT,
    confidentiality INTEGER CHECK (confidentiality BETWEEN 0 AND 5),  -- NULL = not assessed
    integrity       INTEGER CHECK (integrity BETWEEN 0 AND 5),
    availability    INTEGER CHECK (availability BETWEEN 0 AND 5),
    -- Derived cache from entity_readings (canonical assessment log).
    -- Updated by app on reading insert; do not write directly.
    last_review     DATE,
    next_review     DATE,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE (organization_id, id)
);

CREATE INDEX IF NOT EXISTS idx_assets_org ON assets(organization_id);
CREATE INDEX IF NOT EXISTS idx_assets_type ON assets(organization_id, asset_type);
CREATE INDEX IF NOT EXISTS idx_assets_status ON assets(organization_id, status);
CREATE INDEX IF NOT EXISTS idx_assets_not_deleted ON assets(organization_id) WHERE deleted_at IS NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- RISKS
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS risks (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    identifier      TEXT NOT NULL,                    -- per-org: RISK-001, set by app
    title           TEXT NOT NULL,
    description     TEXT,
    risk_type       TEXT NOT NULL DEFAULT 'threat' CHECK (risk_type IN ('threat','opportunity')),
    origin          TEXT NOT NULL DEFAULT 'internal' CHECK (origin IN ('internal','external','internal and external')),
    category        TEXT,
    -- potential consequences live in description (## Potential consequences heading)
    -- Current/residual assessment (NULL = not assessed)
    current_likelihood      INTEGER CHECK (current_likelihood BETWEEN 0 AND 5),
    current_impact          INTEGER CHECK (current_impact BETWEEN 0 AND 5),
    current_score           INTEGER CHECK (current_score BETWEEN 0 AND 25),
    current_level           TEXT CHECK (current_level IN ('low','medium','high','critical')),
    confidentiality_impact  INTEGER CHECK (confidentiality_impact BETWEEN 0 AND 5),
    integrity_impact        INTEGER CHECK (integrity_impact BETWEEN 0 AND 5),
    availability_impact     INTEGER CHECK (availability_impact BETWEEN 0 AND 5),
    -- Inherent assessment
    inherent_likelihood              INTEGER CHECK (inherent_likelihood BETWEEN 0 AND 5),
    inherent_impact                  INTEGER CHECK (inherent_impact BETWEEN 0 AND 5),
    inherent_score                   INTEGER CHECK (inherent_score BETWEEN 0 AND 25),
    inherent_confidentiality_impact  INTEGER CHECK (inherent_confidentiality_impact BETWEEN 0 AND 5),
    inherent_integrity_impact        INTEGER CHECK (inherent_integrity_impact BETWEEN 0 AND 5),
    inherent_availability_impact     INTEGER CHECK (inherent_availability_impact BETWEEN 0 AND 5),
    -- Target
    target_likelihood   INTEGER CHECK (target_likelihood BETWEEN 0 AND 5),
    target_impact       INTEGER CHECK (target_impact BETWEEN 0 AND 5),
    target_score        INTEGER CHECK (target_score BETWEEN 0 AND 25),
    target_level        TEXT CHECK (target_level IN ('low','medium','high','critical')),
    -- Treatment
    treatment       TEXT CHECK (treatment IN ('mitigate','accept','transfer','avoid')),  -- NULL = not decided
    treatment_plan  TEXT,
    treatment_due_date TIMESTAMPTZ,
    -- linked entities: use entity_references table
    -- Assessment lifecycle: see entity_readings table for the canonical assessment log
    accepted_at     TIMESTAMPTZ,
    accepted_by_id  INTEGER REFERENCES users(id),
    -- Ownership
    owner_id        INTEGER REFERENCES users(id),
    status          TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('draft','open','closed')),
    -- Derived cache from entity_readings (canonical assessment log).
    -- Updated by app on reading insert; do not write directly.
    last_review     DATE,
    next_review     DATE,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE (organization_id, id),
    CONSTRAINT chk_risk_accepted CHECK ((accepted_at IS NULL) = (accepted_by_id IS NULL))
);

CREATE INDEX IF NOT EXISTS idx_risks_org ON risks(organization_id);
CREATE INDEX IF NOT EXISTS idx_risks_status ON risks(organization_id, status);
CREATE INDEX IF NOT EXISTS idx_risks_level ON risks(organization_id, current_level);
CREATE INDEX IF NOT EXISTS idx_risks_not_deleted ON risks(organization_id) WHERE deleted_at IS NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- SUPPLIERS
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS suppliers (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    identifier      TEXT NOT NULL,                    -- per-org: SUPPLIER-001, set by app
    name            TEXT NOT NULL,
    supplier_type   TEXT NOT NULL DEFAULT 'other' CHECK (supplier_type IN ('cloud','saas','consulting','hosting','infrastructure','software','other')),
    criticality     TEXT NOT NULL DEFAULT 'low' CHECK (criticality IN ('low','medium','high','critical')),
    -- services description lives in notes (## Services heading)
    data_access     BOOLEAN NOT NULL DEFAULT false,
    contact         TEXT,
    contract_ref    TEXT,
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','under_review','suspended','terminated')),
    owner_id        INTEGER REFERENCES users(id),
    contract_expiry DATE,
    -- assessment_status is derivable from last_review/next_review and supplier_reviews table
    confidentiality INTEGER CHECK (confidentiality BETWEEN 0 AND 5),  -- NULL = not assessed
    integrity       INTEGER CHECK (integrity BETWEEN 0 AND 5),
    availability    INTEGER CHECK (availability BETWEEN 0 AND 5),
    -- Derived cache from entity_readings (canonical assessment log).
    -- Updated by app on reading insert; do not write directly.
    last_review     DATE,
    next_review     DATE,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE (organization_id, id)
);

CREATE INDEX IF NOT EXISTS idx_suppliers_org ON suppliers(organization_id);
CREATE INDEX IF NOT EXISTS idx_suppliers_criticality ON suppliers(organization_id, criticality);
CREATE INDEX IF NOT EXISTS idx_suppliers_not_deleted ON suppliers(organization_id) WHERE deleted_at IS NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- SYSTEMS (IT systems register with recovery objectives, access control, and supplier link)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS systems (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    identifier      TEXT NOT NULL,                    -- per-org: SYSTEM-001, set by app
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    description     TEXT,
    supplier_id     BIGINT,
    department      TEXT,                     -- IT, Development, Marketing, Sales, General, etc.
    -- purpose lives in description (## Purpose heading)
    classification  TEXT NOT NULL DEFAULT 'confidential' CHECK (classification IN ('public','internal','confidential','restricted')),
    criticality     TEXT NOT NULL DEFAULT 'low' CHECK (criticality IN ('low','medium','high','critical')),
    status          TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','under_review','decommissioned')),
    -- Recovery objectives
    rpo_hours       INTEGER NOT NULL DEFAULT 0,
    rto_hours       INTEGER NOT NULL DEFAULT 0,
    -- CIA impact (NULL = not assessed, 1-5 = assessed)
    confidentiality INTEGER CHECK (confidentiality BETWEEN 0 AND 5),
    integrity       INTEGER CHECK (integrity BETWEEN 0 AND 5),
    availability    INTEGER CHECK (availability BETWEEN 0 AND 5),
    -- Access control: auth method lives in notes (## Access control heading)
    -- Review (cycle derived from criticality)
    -- Derived cache from entity_readings (canonical assessment log).
    -- Updated by app on reading insert; do not write directly.
    last_review     DATE,
    next_review     DATE,
    owner_id        INTEGER REFERENCES users(id),
    -- assets: use entity_references table
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, supplier_id) REFERENCES suppliers(organization_id, id)
);

CREATE INDEX IF NOT EXISTS idx_systems_org ON systems(organization_id);
CREATE INDEX IF NOT EXISTS idx_systems_supplier ON systems(organization_id, supplier_id) WHERE supplier_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_systems_criticality ON systems(organization_id, criticality);
CREATE INDEX IF NOT EXISTS idx_systems_not_deleted ON systems(organization_id) WHERE deleted_at IS NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- ACCESS REVIEWS (periodic access review per system)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS access_reviews (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    system_id       BIGINT NOT NULL,
    reviewed_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_by     TEXT NOT NULL,             -- user email (snapshot)
    reviewed_by_user_id INTEGER REFERENCES users(id),  -- FK to user
    users_added     INTEGER NOT NULL DEFAULT 0,
    users_removed   INTEGER NOT NULL DEFAULT 0,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (organization_id, system_id) REFERENCES systems(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_access_reviews_org ON access_reviews(organization_id);
CREATE INDEX IF NOT EXISTS idx_access_reviews_system ON access_reviews(system_id, reviewed_at DESC);

-- ═══════════════════════════════════════════════════════════════════════
-- ACTIVITY LOG
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS activity (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    document_id     TEXT,
    review_id       INTEGER,
    actor           TEXT NOT NULL,                    -- snapshot for audit trail
    actor_user_id   INTEGER REFERENCES users(id),    -- FK to user (NULL for system actions)
    action          TEXT NOT NULL,
    detail          TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (organization_id, review_id) REFERENCES reviews(organization_id, id)
);

CREATE INDEX IF NOT EXISTS idx_activity_org ON activity(organization_id);
CREATE INDEX IF NOT EXISTS idx_activity_document ON activity(organization_id, document_id);
CREATE INDEX IF NOT EXISTS idx_activity_created ON activity(organization_id, created_at DESC);

-- ═══════════════════════════════════════════════════════════════════════
-- CORRECTIVE ACTIONS
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS corrective_actions (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    identifier      TEXT NOT NULL,                      -- CA-1, CA-2, ...
    title           TEXT NOT NULL,
    description     TEXT NOT NULL,
    source          TEXT NOT NULL DEFAULT 'other' CHECK (source IN ('internal_audit','external_audit','risk_assessment','security_incident','objective','feedback','other')),
    severity        TEXT NOT NULL DEFAULT 'observation' CHECK (severity IN ('major_nc','minor_nc','observation','opportunity')),
    status          TEXT NOT NULL DEFAULT 'todo' CHECK (status IN ('todo','assessment','awaiting_approval','implementation','monitoring','resolved')),
    assignee_id     INTEGER REFERENCES users(id),
    created_by      TEXT NOT NULL,                    -- email
    created_by_user_id INTEGER REFERENCES users(id),
    due_date        DATE,
    root_cause      TEXT,
    -- cross-entity links (incident, audit_finding, risk, document, control, etc.): use entity_references table
    notes           TEXT,
    resolved_at     TIMESTAMPTZ,
    resolved_by_id  INTEGER REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE (organization_id, identifier)
);

CREATE INDEX IF NOT EXISTS idx_ca_org ON corrective_actions(organization_id);
CREATE INDEX IF NOT EXISTS idx_ca_status ON corrective_actions(organization_id, status);
CREATE INDEX IF NOT EXISTS idx_ca_assignee ON corrective_actions(assignee_id);
CREATE INDEX IF NOT EXISTS idx_ca_severity ON corrective_actions(organization_id, severity);
CREATE INDEX IF NOT EXISTS idx_corrective_actions_not_deleted ON corrective_actions(organization_id) WHERE deleted_at IS NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- PROGRAMS (objective grouping)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS programs (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    identifier      TEXT NOT NULL,                    -- per-org: PROG-001, set by app
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    key             TEXT NOT NULL CHECK (key ~ '^[A-Z][A-Z0-9_]{0,15}$'),  -- uppercase prefix: AWARE, TANK, SEC
    title           TEXT NOT NULL,
    description     TEXT,
    notes           TEXT,
    owner_id        INTEGER REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE (organization_id, id)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_programs_org_key ON programs(organization_id, key) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_programs_org ON programs(organization_id);
CREATE INDEX IF NOT EXISTS idx_programs_not_deleted ON programs(organization_id) WHERE deleted_at IS NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- OBJECTIVES (measurable targets within programs)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS objectives (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    program_id      BIGINT NOT NULL,
    display_id      TEXT NOT NULL,            -- "AWARE-1", "TANK-3" (program.key + seq)
    seq_number      INTEGER NOT NULL,
    title           TEXT NOT NULL,
    description     TEXT,
    owner_id        INTEGER REFERENCES users(id),
    source          TEXT,                     -- where requirement comes from
    measurement_method TEXT,                  -- how it's measured
    target_value    NUMERIC,                  -- KPI target number
    target_operator TEXT NOT NULL DEFAULT 'gte' CHECK (target_operator IN ('gte','lte','eq','gt','lt')),
    unit            TEXT,                     -- "%", "minutes", "incidents", "count", etc.
    window_seconds  INTEGER,                  -- measurement interval (e.g., 2592000 for monthly)
    grace_seconds   INTEGER NOT NULL DEFAULT 3600,  -- grace period before overdue
    checkin_cycle   INTEGER NOT NULL DEFAULT 12,  -- months between check-ins (default yearly)
    status          TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','active','at_risk','paused','complete')),
    started_at      TIMESTAMPTZ,             -- when measurement tracking began
    archived_at     TIMESTAMPTZ,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE(organization_id, display_id),
    UNIQUE (organization_id, id),
    UNIQUE (program_id, seq_number),
    FOREIGN KEY (organization_id, program_id) REFERENCES programs(organization_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_objectives_org ON objectives(organization_id);
CREATE INDEX IF NOT EXISTS idx_objectives_program ON objectives(program_id);
CREATE INDEX IF NOT EXISTS idx_objectives_status ON objectives(organization_id, status);
CREATE INDEX IF NOT EXISTS idx_objectives_not_deleted ON objectives(organization_id) WHERE deleted_at IS NULL;

-- ═══════════════════════════════════════════════════════════════════════
-- CHECKINS (time-series measurements for objectives)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS checkins (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    objective_id    BIGINT NOT NULL,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT now(),  -- when measurement happened
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT now(),  -- when it was logged
    created_by      TEXT,                                -- user email
    created_by_user_id INTEGER REFERENCES users(id),
    success         BOOLEAN,                             -- pass/fail (NULL = unspecified)
    value_numeric   NUMERIC,                             -- actual measured value
    message         TEXT,                                -- internal note
    public_note     TEXT,                                -- stakeholder-facing note
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (organization_id, id),
    FOREIGN KEY (organization_id, objective_id) REFERENCES objectives(organization_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_checkins_org ON checkins(organization_id);
CREATE INDEX IF NOT EXISTS idx_checkins_objective ON checkins(objective_id, occurred_at DESC);

-- ═══════════════════════════════════════════════════════════════════════
-- CHECKIN EVIDENCE (S3-backed file attachments)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS checkin_evidence (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    checkin_id      BIGINT NOT NULL,
    title           TEXT NOT NULL,
    object_key      TEXT NOT NULL,            -- S3 key: {org-uuid}/evidence/{uuid}.ext
    content_type    TEXT NOT NULL,
    size_bytes      BIGINT,
    sha256          TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (organization_id, checkin_id) REFERENCES checkins(organization_id, id) ON DELETE CASCADE,
    CONSTRAINT uq_evidence_org_object_key UNIQUE(organization_id, object_key)
);
CREATE INDEX IF NOT EXISTS idx_evidence_checkin ON checkin_evidence(checkin_id);

-- ═══════════════════════════════════════════════════════════════════════
-- ENTITY CHANGELOG (audit trail for all register changes)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS entity_changelog (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    entity_type     TEXT NOT NULL CHECK (entity_type IN ('risk','supplier','incident','legal_requirement','change_request','corrective_action','objective','task','system','asset','audit','audit_finding','program','checkin','access_review','document','review')),
    entity_id       BIGINT NOT NULL,  -- the entity's primary key
    action          TEXT NOT NULL CHECK (action IN ('create','update','delete','suggestion_created','suggestion_applied','suggestion_rejected','suggestion_deleted')),
    field           TEXT,             -- NULL for create/delete, field name for update
    old_value       TEXT,             -- NULL for create
    new_value       TEXT,             -- NULL for delete
    changed_by      TEXT NOT NULL,    -- user email (snapshot)
    changed_by_user_id INTEGER REFERENCES users(id),  -- FK to user
    api_key_id      INTEGER REFERENCES api_keys(id) ON DELETE SET NULL,  -- which API key was used (NULL for JWT sessions)
    reason          TEXT,             -- optional: why the change was made
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_changelog_org ON entity_changelog(organization_id);
CREATE INDEX IF NOT EXISTS idx_changelog_entity ON entity_changelog(organization_id, entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_changelog_created ON entity_changelog(organization_id, created_at DESC);

-- ═══════════════════════════════════════════════════════════════════════
-- SCHEMA VERSION TRACKING
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS schema_migrations (
    version     TEXT PRIMARY KEY,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ═══════════════════════════════════════════════════════════════════════
-- OIDC PROVIDERS (per-org SSO configuration)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS oidc_providers (
    id               INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id  INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    provider_name    TEXT NOT NULL,                -- 'microsoft', 'google', 'okta', 'custom'
    display_name     TEXT NOT NULL,                -- shown on login button
    client_id        TEXT NOT NULL,
    client_secret    TEXT NOT NULL,
    discovery_url    TEXT NOT NULL,                -- .well-known/openid-configuration URL
    scopes           TEXT NOT NULL DEFAULT 'openid email profile',
    auto_add_members BOOLEAN NOT NULL DEFAULT false,
    default_role     TEXT NOT NULL DEFAULT 'reader' CHECK (default_role IN ('admin','manager','contributor','reader')),
    enabled          BOOLEAN NOT NULL DEFAULT true,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(organization_id, provider_name),
    UNIQUE (organization_id, id)
);

CREATE INDEX IF NOT EXISTS idx_oidc_providers_org ON oidc_providers(organization_id);

-- ═══════════════════════════════════════════════════════════════════════
-- OIDC SESSIONS (state+nonce for CSRF protection during auth flow)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS oidc_sessions (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    state            TEXT NOT NULL UNIQUE,
    nonce            TEXT NOT NULL,
    provider_id      INTEGER NOT NULL,
    organization_id  INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    redirect_uri     TEXT,
    expires_at       TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (organization_id, provider_id) REFERENCES oidc_providers(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_oidc_sessions_state ON oidc_sessions(state);
CREATE INDEX IF NOT EXISTS idx_oidc_sessions_expires ON oidc_sessions(expires_at);

-- ═══════════════════════════════════════════════════════════════════════
-- WEBAUTHN CREDENTIALS (passkeys, per-user)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS webauthn_credentials (
    id               INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_id    BYTEA NOT NULL UNIQUE,
    public_key       BYTEA NOT NULL,
    attestation_type TEXT NOT NULL DEFAULT '',
    transport        TEXT[],
    sign_count       INTEGER NOT NULL DEFAULT 0,
    name             TEXT NOT NULL DEFAULT 'Passkey',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_webauthn_user ON webauthn_credentials(user_id);

-- ═══════════════════════════════════════════════════════════════════════
-- JWT BLOCKLIST (token revocation without short-lived tokens)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS jwt_blocklist (
    token_hash TEXT PRIMARY KEY,
    blocked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_jwt_blocklist_expires ON jwt_blocklist(expires_at);

-- ═══════════════════════════════════════════════════════════════════════
-- LOGIN ATTEMPTS (DB-backed brute-force protection)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS login_attempts (
    email        TEXT NOT NULL,
    ip_address   INET,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '24 hours')
);

CREATE INDEX IF NOT EXISTS idx_login_attempts_email ON login_attempts(email, attempted_at);
CREATE INDEX IF NOT EXISTS idx_login_attempts_ip ON login_attempts(ip_address, attempted_at);
CREATE INDEX IF NOT EXISTS idx_login_attempts_expires ON login_attempts(expires_at);

-- ═══════════════════════════════════════════════════════════════════════
-- ENTITY CROSS-REFERENCES (bidirectional links between documents, risks, etc.)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS entity_references (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL CHECK (source_type IN ('document','risk','legal_requirement','asset','supplier','system','incident','corrective_action','objective','program','audit','audit_finding','change_request','task')),
    source_id TEXT NOT NULL,    -- entity identifier (e.g. 'CLS-4.1', 'RISK-12', 'LEGAL-3')
    target_type TEXT NOT NULL CHECK (target_type IN ('document','risk','legal_requirement','asset','supplier','system','incident','corrective_action','objective','program','audit','audit_finding','change_request','task')),
    target_id TEXT NOT NULL,
    created_by TEXT,
    created_by_user_id INTEGER REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(organization_id, source_type, source_id, target_type, target_id)
);
CREATE INDEX IF NOT EXISTS idx_entity_refs_source ON entity_references(organization_id, source_type, source_id);
CREATE INDEX IF NOT EXISTS idx_entity_refs_target ON entity_references(organization_id, target_type, target_id);

-- ═══════════════════════════════════════════════════════════════════════
-- PER-ORG IDENTIFIER SEQUENCES
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS identifier_sequences (
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    entity_type     TEXT NOT NULL CHECK (entity_type IN ('risk','asset','supplier','system','legal_requirement','program','incident','change_request','task','corrective_action','objective','audit','audit_finding')),
    next_value      INTEGER NOT NULL DEFAULT 1,
    PRIMARY KEY (organization_id, entity_type)
);

-- Per-org identifier uniqueness (prevent app bugs from creating duplicate RISK-001)
ALTER TABLE risks ADD CONSTRAINT uq_risks_org_identifier UNIQUE(organization_id, identifier);
ALTER TABLE assets ADD CONSTRAINT uq_assets_org_identifier UNIQUE(organization_id, identifier);
ALTER TABLE suppliers ADD CONSTRAINT uq_suppliers_org_identifier UNIQUE(organization_id, identifier);
ALTER TABLE systems ADD CONSTRAINT uq_systems_org_identifier UNIQUE(organization_id, identifier);
ALTER TABLE legal_requirements ADD CONSTRAINT uq_legal_org_identifier UNIQUE(organization_id, identifier);
ALTER TABLE programs ADD CONSTRAINT uq_programs_org_identifier UNIQUE(organization_id, identifier);

-- ═══════════════════════════════════════════════════════════════════════
-- APPROVAL POLICIES (enforced review rules per document path)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS approval_policies (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    path_pattern    TEXT NOT NULL,  -- folder/path prefix like "iso27001/policies", "*" for all
    min_approvals   INTEGER NOT NULL DEFAULT 1,
    required_roles  TEXT[] DEFAULT '{}',  -- roles that must approve, e.g. {"manager","admin"}
    required_users  TEXT[] DEFAULT '{}',  -- specific emails that must approve
    require_human   BOOLEAN NOT NULL DEFAULT true,  -- at least one human must approve (false = agent-only OK)
    auto_merge      BOOLEAN NOT NULL DEFAULT false, -- auto-merge when all required approvals are met
    active          BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_approval_policies_org ON approval_policies(organization_id);
CREATE INDEX IF NOT EXISTS idx_approval_policies_active ON approval_policies(organization_id) WHERE active = true;

-- ═══════════════════════════════════════════════════════════════════════
-- DECISION LOG (immutable audit trail for review decisions)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS decision_log (
    id              INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    review_id       INTEGER,
    document_id     TEXT NOT NULL,
    decision        TEXT NOT NULL CHECK (decision IN ('approved','changes_requested','proposed_revision','merged','closed','confirmed')),
    decided_by      TEXT NOT NULL,
    decided_by_id   INTEGER REFERENCES users(id),
    commit_ref      TEXT,
    version         TEXT,
    comment         TEXT,
    content_hash    TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (organization_id, review_id) REFERENCES reviews(organization_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_decision_log_org ON decision_log(organization_id);
CREATE INDEX IF NOT EXISTS idx_decision_log_document ON decision_log(organization_id, document_id);
CREATE INDEX IF NOT EXISTS idx_decision_log_review ON decision_log(review_id);
CREATE INDEX IF NOT EXISTS idx_decision_log_created ON decision_log(organization_id, created_at DESC);

-- ═══════════════════════════════════════════════════════════════════════
-- ENTITY READINGS (periodic assessment records for risks, legal, suppliers)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS entity_readings (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    entity_type     TEXT NOT NULL CHECK (entity_type IN ('risk','legal_requirement','asset','system')),
    entity_id       BIGINT NOT NULL,
    current_likelihood INTEGER CHECK (current_likelihood BETWEEN 0 AND 5),
    current_impact     INTEGER CHECK (current_impact BETWEEN 0 AND 5),
    confidentiality    INTEGER CHECK (confidentiality BETWEEN 0 AND 5),
    integrity          INTEGER CHECK (integrity BETWEEN 0 AND 5),
    availability       INTEGER CHECK (availability BETWEEN 0 AND 5),
    status             TEXT CHECK (status IN ('draft','open','closed','archived','active','under_review','suspended','terminated','decommissioned') OR status IS NULL),
    treatment          TEXT CHECK (treatment IN ('mitigate','accept','transfer','avoid') OR treatment IS NULL),
    notes              TEXT,
    assessed_by        TEXT NOT NULL,
    assessed_by_user_id INTEGER REFERENCES users(id),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(organization_id, id)
);
CREATE INDEX IF NOT EXISTS idx_entity_readings_entity ON entity_readings(organization_id, entity_type, entity_id);

-- ═══════════════════════════════════════════════════════════════════════
-- SUPPLIER REVIEWS (periodic supplier assessments — "is everything still OK?")
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS supplier_reviews (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    supplier_id     BIGINT NOT NULL,
    outcome         TEXT NOT NULL DEFAULT 'satisfactory' CHECK (outcome IN ('satisfactory','concerns','unsatisfactory')),
    certifications_verified BOOLEAN NOT NULL DEFAULT false,
    data_handling_verified  BOOLEAN NOT NULL DEFAULT false,
    sla_met                BOOLEAN NOT NULL DEFAULT true,
    notes           TEXT,
    reviewed_by     TEXT NOT NULL,
    reviewed_by_user_id INTEGER REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(organization_id, id),
    FOREIGN KEY (organization_id, supplier_id) REFERENCES suppliers(organization_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_supplier_reviews_supplier ON supplier_reviews(organization_id, supplier_id);

-- ═══════════════════════════════════════════════════════════════════════
-- ASSET REVIEWS (periodic asset assessments)
-- ═══════════════════════════════════════════════════════════════════════

CREATE TABLE IF NOT EXISTS asset_reviews (
    id              BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    asset_id        BIGINT NOT NULL,
    outcome         TEXT NOT NULL DEFAULT 'satisfactory' CHECK (outcome IN ('satisfactory','concerns','unsatisfactory')),
    classification_verified BOOLEAN NOT NULL DEFAULT false,
    ownership_verified      BOOLEAN NOT NULL DEFAULT false,
    notes           TEXT,
    reviewed_by     TEXT NOT NULL,
    reviewed_by_user_id INTEGER REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(organization_id, id),
    FOREIGN KEY (organization_id, asset_id) REFERENCES assets(organization_id, id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_asset_reviews_asset ON asset_reviews(organization_id, asset_id);

-- ═══════════════════════════════════════════════════════════════════════
-- SLUG FORMAT VALIDATION
-- ═══════════════════════════════════════════════════════════════════════

ALTER TABLE organizations ADD CONSTRAINT chk_slug_format
    CHECK (slug ~ '^[a-z0-9][a-z0-9-]*[a-z0-9]$' AND length(slug) BETWEEN 2 AND 63);

-- ═══════════════════════════════════════════════════════════════════════
-- ROW LEVEL SECURITY (defense in depth for multi-tenant isolation)
-- App layer sets: SET LOCAL app.current_org_id = <org_id> per request.
-- RLS ensures queries can only see rows for the current org, even if
-- a bug in the WHERE clause omits organization_id filtering.
-- ═══════════════════════════════════════════════════════════════════════

DO $$ DECLARE t TEXT; BEGIN
    FOR t IN SELECT unnest(ARRAY[
        'organization_settings','organization_members',
        'reviews','review_assignments','approvals','document_versions','comments',
        'incidents','tasks','change_requests','implementation_status','notifications',
        'audit_programmes','audits','audit_items','audit_findings',
        'legal_requirements','assets','risks','suppliers','systems','access_reviews',
        'activity','corrective_actions','programs','objectives','checkins','checkin_evidence',
        'entity_changelog','entity_references','identifier_sequences',
        'oidc_providers','oidc_sessions','approval_policies','decision_log',
        'suggestions','entity_comments','entity_reactions','entity_readings'
    ])
    LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format(
            'CREATE POLICY tenant_isolation ON %I
             USING (organization_id = current_setting(''app.current_org_id'', true)::INTEGER)
             WITH CHECK (organization_id = current_setting(''app.current_org_id'', true)::INTEGER)',
            t
        );
    END LOOP;
END $$;
