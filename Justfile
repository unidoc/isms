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

# ── Maintenance ──────────────────────────────────────────────────────────────

# Run go mod tidy
tidy:
    go mod tidy

# Clean build artifacts
clean:
    rm -rf bin/ web/dist/ coverage.out coverage.html
