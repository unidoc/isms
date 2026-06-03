# isms.sh — The Intelligent Management System

set dotenv-load := false

default:
    @just --list

# Build everything (Vue frontend + Go binary)
build: build-web build-go

# Build Go binary with version info
build-go:
    #!/usr/bin/env bash
    set -euo pipefail
    # Sync migrations and web dist into cmd/isms for embedding
    cp -f migrations/*.sql cmd/isms/migrations/
    rm -rf cmd/isms/web/dist && mkdir -p cmd/isms/web/dist
    [ -d web/dist ] && cp -r web/dist/* cmd/isms/web/dist/ || true
    VERSION=$(cat version.txt | tr -d '[:space:]')
    COMMIT_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
    COMMIT_COUNT=$(git rev-list --count HEAD 2>/dev/null || echo "0")
    CGO_ENABLED=0 go build \
        -ldflags "-X 'main.version=${VERSION}' -X 'main.commitHash=${COMMIT_HASH}' -X 'main.commitCount=${COMMIT_COUNT}'" \
        -o bin/isms ./cmd/isms/

# Build Vue frontend
build-web:
    cd web && npm run build

# Dev mode: Go API + Vite hot reload in one command
dev:
    go run ./cmd/isms/ serve --dev

# ── Testing ──────────────────────────────────────────────────────────────────

# Run Go unit tests
test-go:
    CGO_ENABLED=0 go test ./internal/isms/... -count=1 -timeout 120s

# Run Go tests with coverage summary
test-go-cover:
    CGO_ENABLED=0 go test ./internal/isms/... -count=1 -cover -timeout 120s

# Generate Go coverage report (HTML)
coverage:
    #!/usr/bin/env bash
    set -euo pipefail
    CGO_ENABLED=0 go test ./internal/isms/... -coverprofile=coverage.out -timeout 120s
    go tool cover -func=coverage.out | tail -1
    go tool cover -html=coverage.out -o coverage.html
    echo "Coverage report: coverage.html"

# Run API integration tests (requires running server)
test URL="http://localhost:9090":
    ISMS_TEST_URL={{URL}} pytest tests/ -v --tb=short

# Run E2E browser tests (requires running server + Playwright)
test-e2e URL="http://localhost:9090":
    ISMS_TEST_URL={{URL}} pytest tests/test_e2e_browser.py -v --tb=short

# Run all tests (Go unit + API integration)
test-all URL="http://localhost:9090":
    #!/usr/bin/env bash
    set -euo pipefail
    echo "=== Go unit tests ==="
    CGO_ENABLED=0 go test ./internal/isms/... -count=1 -cover -timeout 120s
    echo ""
    echo "=== API integration tests ==="
    ISMS_TEST_URL={{URL}} pytest tests/ -v --tb=short
    echo ""
    echo "=== E2E browser tests ==="
    ISMS_TEST_URL={{URL}} pytest tests/test_e2e_browser.py -v --tb=short

# ── Server ───────────────────────────────────────────────────────────────────

# Start the web server
serve:
    go run ./cmd/isms/ serve

# Run database migrations
migrate:
    go run ./cmd/isms/ migrate --dir migrations

# ── Release ──────────────────────────────────────────────────────────────────
#
# Two-step, PR-based release flow:
#   1. just release-pr 0.6.0   → branch + version.txt bump + PR (review, CI)
#   2. merge the PR
#   3. just release 0.6.0      → verifies master carries 0.6.0, signs the tag,
#                                pushes — CI (goreleaser) publishes binaries,
#                                checksums and changelog to a GitHub Release.

# Step 1: open the version-bump PR.
release-pr VERSION:
    #!/usr/bin/env bash
    set -euo pipefail
    [ -z "$(git status --porcelain)" ] || { echo "✗ working tree not clean"; exit 1; }
    git fetch origin
    git checkout -b "release/v{{VERSION}}" origin/master
    echo "{{VERSION}}" > version.txt
    git add version.txt
    git commit -m "Release v{{VERSION}}"
    git push -u origin "release/v{{VERSION}}"
    gh pr create --title "Release v{{VERSION}}" \
        --body "Bumps version.txt to {{VERSION}}. After merge: \`just release {{VERSION}}\` tags master and CI publishes the release."
    echo "✓ release PR opened — merge it, then run: just release {{VERSION}}"

# Local test-build of the release pipeline — same artifacts as a real release
# (dist/), nothing published. Requires built frontend (just build-web) first.
snapshot:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p cmd/isms/web/dist && cp -r web/dist/* cmd/isms/web/dist/ 2>/dev/null || true
    cp -f migrations/*.sql cmd/isms/migrations/
    COMMIT_COUNT=$(git rev-list --count HEAD) goreleaser release --snapshot --clean --skip=validate
    echo "✓ snapshot in dist/ — try: tar -xzf dist/*linux_amd64.tar.gz -C /tmp isms && /tmp/isms version"

# Step 2 (after the PR is merged): tag master and push the tag.
release VERSION:
    #!/usr/bin/env bash
    set -euo pipefail
    [ -z "$(git status --porcelain)" ] || { echo "✗ working tree not clean"; exit 1; }
    git checkout master
    git pull --ff-only
    [ "$(tr -d '[:space:]' < version.txt)" = "{{VERSION}}" ] || \
        { echo "✗ version.txt is '$(cat version.txt)' — merge the release PR first"; exit 1; }
    git rev-parse "v{{VERSION}}" >/dev/null 2>&1 && { echo "✗ tag v{{VERSION}} already exists"; exit 1; }
    git tag -s "v{{VERSION}}" -m "v{{VERSION}}"
    git push origin "v{{VERSION}}"
    echo "✓ v{{VERSION}} tagged — CI builds the release: gh run watch"

# ── Maintenance ──────────────────────────────────────────────────────────────

# Run go mod tidy
tidy:
    go mod tidy

# Clean build artifacts
clean:
    rm -rf bin/ web/dist/ coverage.out coverage.html
