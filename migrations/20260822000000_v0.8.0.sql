-- 0.8.0 release migration. One migration file per release (see v0.7.1.sql).
-- All schema changes shipping in 0.8.0 accumulate here until it releases.
--
-- #212: i18n / localization foundation.
--
-- Two tiers of locale preference, resolved by internal/isms/i18n.Resolve():
--   1. users.locale          — the user's explicit choice, NULL = never chose
--   2. default_locale        — per-org default, via the settings registry
--   3. 'en'                  — hard fallback in Go, not stored anywhere
--
-- users.locale is a real column rather than a setting because settings are
-- per-organization and this is per-user; a user in two orgs has one language.
-- It is nullable on purpose: NULL means "no explicit choice", which is what lets
-- the org default (and, pre-login, Accept-Language) apply. A DEFAULT 'en' here
-- would make every existing user look like they had deliberately chosen English
-- and permanently shadow the org default.
--
-- No CHECK constraint on the value: the supported-locale set lives in
-- internal/isms/i18n and changes with the binary, not the schema. A locale that
-- stops being supported must degrade to the fallback (Resolve() re-validates
-- every tier), not wedge writes to the users table.
ALTER TABLE users ADD COLUMN IF NOT EXISTS locale TEXT;

-- default_locale is a setting rather than an organizations column: per-org
-- configuration already lives in the settings registry, which brings the
-- existing admin settings API, UI and audit path with it for free. A new column
-- would need all of that rebuilt.
--
-- The description is admin-facing copy rendered in the settings UI, so it says
-- "users who have not chosen one" rather than "new users": resolution applies
-- this to EVERY user whose locale is NULL, which immediately after this migration
-- is all of them. Calling it a new-user default would tell an admin that changing
-- it is safe for existing staff, when it re-languages the whole org.
INSERT INTO settings (key, description, category, default_value, sensitive) VALUES
    ('default_locale', 'Default language for users who have not chosen one, and for org-wide notifications (BCP 47 tag, e.g. en, id-ID)', 'localization', 'en', false)
ON CONFLICT (key) DO NOTHING;
