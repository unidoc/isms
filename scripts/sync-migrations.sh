#!/usr/bin/env sh
# Single source of truth for the compile-time migration sync.
#
# cmd/isms/migrations/ is a build artifact (gitignored except a .keep placeholder):
# //go:embed all:migrations bakes it into the binary, so every path that COMPILES
# the source — just build-go, the CI jobs, goreleaser, the dev/test containers —
# must copy the real migrations in first. This is that copy, in one place.
set -eu

root="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$root/cmd/isms/migrations"
# Clear stale .sql first, then copy. `cp -f` alone only ever adds: a migration
# RENAMED in migrations/ (which is how an append to an unreleased release file
# has to be done, since the runner records applied migrations by filename) would
# leave the old name behind in a dev's embed dir, and the binary would then
# embed and apply both copies. CI and goreleaser start from an empty dir and
# never saw this; a local checkout that has built before does.
rm -f "$root"/cmd/isms/migrations/*.sql
cp -f "$root"/migrations/*.sql "$root/cmd/isms/migrations/"
