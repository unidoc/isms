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
cp -f "$root"/migrations/*.sql "$root/cmd/isms/migrations/"
