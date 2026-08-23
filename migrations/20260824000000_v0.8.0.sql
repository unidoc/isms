-- The single migration for the 0.8.0 release. One migration per minor release,
-- named for the release (not for any one change). ALL schema changes shipping in
-- 0.8.0 accumulate in this file, appended in the order they land on master.
--
-- NB: this replaces 20260823000000_v0.8.0.sql, a second 0.8.0 file that briefly
-- shipped #213 on master. Appending under that name would have been skipped
-- (runner tracks by filename) on any DB that already ran it, so the file was
-- renamed instead — safe only because 0.8.0 is unreleased and every statement
-- here is idempotent.

-- Per-org custom risk categories (#213, merged first via #215): a JSON array in
-- organization_settings under 'risk_categories', e.g.
-- [{"key":"people_process","label":"People & Process"}]. Only the registry row is
-- seeded — GetOrgSetting queries `settings`, so without it every lookup errors
-- with "no rows". default_value stays NULL because defaults are virtual: unset
-- falls back to db.DefaultRiskCategories(), which leaves existing orgs unchanged
-- and lets the default list improve in later releases. risks.category is
-- deliberately untouched plain TEXT — a removed category orphans, it does not
-- cascade.
INSERT INTO settings (key, description, category, default_value, sensitive)
VALUES (
    'risk_categories',
    'Risk categories available to this organization, as a JSON array of {key, label}. Empty falls back to the built-in defaults.',
    'risk',
    NULL,
    false
)
ON CONFLICT (key) DO NOTHING;

-- Per-user language choice (#212): a column rather than a setting because
-- settings are per-organization and a user in two orgs has one language.
-- Nullable with no default on purpose — NULL means "never chose", which is what
-- lets the org default (and, pre-login, Accept-Language) apply; DEFAULT 'en'
-- would make every existing user look like they had picked English and shadow the
-- org default permanently. No CHECK on the value: the supported set lives in
-- internal/isms/i18n and changes with the binary, so a dropped locale must
-- degrade to the fallback, not wedge writes to users.
ALTER TABLE users ADD COLUMN IF NOT EXISTS locale TEXT;

-- The org-wide default locale (#212), a setting rather than an organizations
-- column: per-org config already lives in the settings registry, which brings the
-- admin settings API, UI and audit path with it. The description says "users who
-- have not chosen one" rather than "new users" because resolution applies it to
-- EVERY user whose locale is NULL — which right after this migration is all of
-- them — and this copy is what an admin reads in the settings UI.
INSERT INTO settings (key, description, category, default_value, sensitive) VALUES
    ('default_locale', 'Default language for users who have not chosen one, and for org-wide notifications (BCP 47 tag, e.g. en, id-ID)', 'localization', 'en', false)
ON CONFLICT (key) DO NOTHING;
