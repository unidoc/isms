-- #213: per-org custom risk categories.
--
-- Named <timestamp>_v0.8.0.sql per the migration convention enforced by
-- tests/test_migrations.py: one migration per release, named after the version.
-- (change_type_check.sql is grandfathered, not a precedent — that set is frozen.)
-- v0.7.1 is already released, so this ships in v0.8.0. The i18n branch (#212) has
-- its own v0.8.0 file at an earlier timestamp; both apply independently, since the
-- runner records applied migrations by filename.
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
