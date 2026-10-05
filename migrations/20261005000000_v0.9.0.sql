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
