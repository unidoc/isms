"""Migration hygiene — fail `just test` / CI on a badly-named migration.

Convention: ONE migration file per release, named after the version —
<14-digit-timestamp>_vX.Y.Z.sql (see migrations/v0.7.0.sql). The runner records
applied migrations by FILENAME (schema_migrations.version), so a shipped file can
never be renamed or amended without re-running or silently skipping it on live
DBs. Version-pinned names keep migration history 1:1 with releases and stop
descriptive one-off files from scattering it.

This is a pure filesystem check — no server/fixtures — so it runs (and fails)
in `just test` locally and in the CI pytest suite.
"""
import re
from pathlib import Path

MIGRATIONS_DIR = Path(__file__).resolve().parent.parent / "migrations"

# <14-digit timestamp>_vX.Y.Z.sql
VERSION_NAME = re.compile(r"^\d{14}_v\d+\.\d+\.\d+\.sql$")

# Grandfathered: these shipped before the rule and their names are already locked
# into schema_migrations on live DBs (renaming would re-run or skip them). FROZEN
# — do NOT extend this set; every new migration must be <timestamp>_vX.Y.Z.sql.
GRANDFATHERED = {
    "20260327000000_initial_schema.sql",
    "20260707120000_change_type_check.sql",
}


def test_migration_files_named_after_version():
    files = sorted(p.name for p in MIGRATIONS_DIR.glob("*.sql"))
    assert files, f"no migrations found in {MIGRATIONS_DIR}"
    offenders = [f for f in files if f not in GRANDFATHERED and not VERSION_NAME.match(f)]
    assert not offenders, (
        "migration file(s) must be named <timestamp>_vX.Y.Z.sql — one migration "
        "per release, named after the version (e.g. 20260713000000_v0.7.1.sql). "
        "Offending: " + ", ".join(offenders)
    )


def test_one_migration_file_per_version():
    # The name check above accepts two files for one version, and git merges two
    # branches that each renamed the release file to a different later timestamp
    # without a conflict, so both copies (each holding the whole release) land.
    versions = {}
    for p in sorted(MIGRATIONS_DIR.glob("*.sql")):
        if p.name in GRANDFATHERED or not VERSION_NAME.match(p.name):
            continue
        versions.setdefault(p.name.split("_", 1)[1], []).append(p.name)
    duplicated = {v: names for v, names in versions.items() if len(names) > 1}
    assert not duplicated, (
        "one migration file per release: merge these into the latest-timestamped "
        "file (and keep it re-runnable). Duplicated: " + "; ".join(
            f"{v}: {', '.join(names)}" for v, names in sorted(duplicated.items()))
    )
