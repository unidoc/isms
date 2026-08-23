-- #213: per-org custom risk categories.
--
-- Standalone migration, not folded into the v0.8.0 release file (see
-- change_type_check.sql for the same precedent). v0.7.1 is already released, and
-- the v0.8.0 file is open on the in-flight i18n branch (#212) — accumulating
-- there would guarantee a merge conflict for a change that is independent of it.
--
-- Categories are stored as a JSON array in organization_settings under the
-- 'risk_categories' key: [{"key":"people_process","label":"People & Process"}].
-- Only the settings registry row is seeded here — GetOrgSetting does a QueryRow
-- against `settings`, so without this row every lookup errors with "no rows".
--
-- default_value is NULL on purpose: defaults are virtual. An unset (or empty)
-- value falls back to db.DefaultRiskCategories() in Go, which keeps existing
-- orgs unchanged and lets the default list improve in later releases.
--
-- Deliberately NO change to risks.category — it is plain TEXT with no CHECK
-- constraint, so a risk holding a category that was later removed stays legal
-- at the DB level (categories orphan, they do not cascade).
INSERT INTO settings (key, description, category, default_value, sensitive)
VALUES (
    'risk_categories',
    'Risk categories available to this organization, as a JSON array of {key, label}. Empty falls back to the built-in defaults.',
    'risk',
    NULL,
    false
)
ON CONFLICT (key) DO NOTHING;
