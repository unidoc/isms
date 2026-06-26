# Operator Guide

Running isms.sh in production. Covers deployment, configuration, backup, upgrades, and day-to-day operations.

## Prerequisites

| Component | Version | Notes |
|-----------|---------|-------|
| PostgreSQL | 14+ | Primary data store for all collaboration, registers, and auth |
| Go | 1.25+ | Only needed if building from source |
| Data directory | Writable path | Git repos and local blob storage |
| Templates | Disk directory | Separate git repo with standard templates (ISO 27001, etc.) |
| S3 (optional) | Any S3-compatible | Cloudflare R2, AWS S3, MinIO — for blob storage instead of local disk |

The server ships as a single binary with the web UI embedded. No runtime dependencies beyond Postgres.

## Deployment

### Build from source

```bash
go build -o isms ./cmd/isms/

# With version info baked in:
go build -ldflags "-X main.version=1.0.0 -X main.commitHash=$(git rev-parse --short HEAD) -X main.commitCount=$(git rev-list --count HEAD)" -o isms ./cmd/isms/
```

### Single binary

The `isms` binary contains: API server, web UI, CLI, TUI, MCP server, and migration runner. No separate processes needed.

```bash
# Start the server
isms server serve --addr :8080
```

The `--addr` flag defaults to `:8080`. The server auto-runs database migrations on startup.

### Environment file

The server loads environment from a file specified by `ISMS_SERVER_ENV`:

```bash
export ISMS_SERVER_ENV=/etc/isms/server.env
isms server serve
```

CLI (non-server) commands use `ISMS_ENV` instead. You can also pass `--env /path/to/file.env` to any command.

Use `contrib/unidoc.env` as a starting template. Set file permissions to `600`.

### systemd unit

```ini
[Unit]
Description=isms.sh — Intelligent Management System
After=network.target postgresql.service
Requires=postgresql.service

[Service]
Type=simple
User=isms
Group=isms
EnvironmentFile=/etc/isms/server.env
ExecStart=/usr/local/bin/isms server serve --addr :8080
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

# Hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/isms
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now isms
```

### Container

There is no production Dockerfile shipped (only a dev environment in `devenv/`). For container deployment, build from source inside a multi-stage Dockerfile:

```dockerfile
FROM golang:1.25 AS builder
WORKDIR /src
COPY . .
RUN go build -o /isms ./cmd/isms/

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates git && rm -rf /var/lib/apt/lists/*
COPY --from=builder /isms /usr/local/bin/isms
RUN useradd -r -m isms
USER isms
EXPOSE 8080
ENTRYPOINT ["isms", "server", "serve", "--addr", ":8080"]
```

Mount your data directory and pass configuration via environment variables. The container needs `git` installed because the server uses go-git which may shell out for wire protocol operations.

### Reverse proxy

The server is an HTTP service — put it behind a reverse proxy for TLS termination.

**nginx:**

```nginx
server {
    listen 443 ssl http2;
    server_name isms.company.com;

    ssl_certificate     /etc/letsencrypt/live/isms.company.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/isms.company.com/privkey.pem;

    client_max_body_size 50m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

**Caddy:**

```
isms.company.com {
    reverse_proxy localhost:8080
}
```

Caddy handles TLS automatically via Let's Encrypt.

Set `ISMS_BASE_URL=https://isms.company.com` so the server generates correct links for emails, passkeys, and CORS.

## Configuration

All configuration is via environment variables. See `contrib/unidoc.env` for the complete template.

### Required

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string. Example: `postgres://user:pass@localhost/isms?sslmode=disable` |
| `ISMS_SECRET` | Secret key for JWT signing and AES-256-GCM encryption of secrets at rest (OIDC client secrets, OTP secrets, sensitive settings). Minimum 32 characters. Generate with: `openssl rand -hex 32` |
| `ISMS_STORAGE_BACKEND` | `file` (local disk) or `s3` (S3-compatible). Required — the server will not start without it. |
| `ISMS_DATA_DIR` | Base directory for git repos and local blob storage. Default: `./data`. When an org is created, its bare git repo goes to `$ISMS_DATA_DIR/repos/{slug}.git`. |

### Templates

| Variable | Description |
|----------|-------------|
| `ISMS_TEMPLATE_PATH` | Path to the template directory on disk (a separate git repo containing standard templates). Required for scaffolding new templates into orgs. Without it, template listing and scaffolding will fail. |

### Server / Web

| Variable | Description |
|----------|-------------|
| `ISMS_BASE_URL` | Public URL of the instance (e.g. `https://isms.company.com`). Used for CORS origins, passkey RP origins, email links, and notification links. |
| `ISMS_WEB_DIR` | Path to the Vue `dist/` directory on disk. If unset, the server uses the embedded web UI built into the binary. Only needed for development or custom builds. |
| `ISMS_CORS_ORIGIN` | Override CORS allowed origins (comma-separated). Defaults to `ISMS_BASE_URL`. Falls back to `http://localhost:*` if neither is set. |
| `ISMS_DOMAIN` | Base domain for subdomain-based org resolution (e.g. `isms.sh` makes `acme.isms.sh` resolve to the `acme` org). Only needed for multi-tenant SaaS deployment. |
| `ISMS_RATE_LIMIT` | Set to `0` to disable per-IP rate limiting (20 req/min default). For testing only. |
| `ISMS_JWT_LIFETIME` | JWT session duration. Default: `24h`. Go duration format (e.g. `720h` for 30 days). |

### S3 storage (required when `ISMS_STORAGE_BACKEND=s3`)

| Variable | Description |
|----------|-------------|
| `ISMS_S3_BUCKET` | S3 bucket name |
| `ISMS_S3_REGION` | AWS region or `auto` for Cloudflare R2 |
| `ISMS_S3_ENDPOINT` | Custom endpoint URL (e.g. `https://xyz.r2.cloudflarestorage.com` for R2) |
| `ISMS_S3_ACCESS_KEY` | Access key ID |
| `ISMS_S3_SECRET_KEY` | Secret access key |

### Authentication

| Variable | Description |
|----------|-------------|
| `CLOUDFLARE_TEAM_DOMAIN` | Cloudflare Access team domain (e.g. `mycompany.cloudflareaccess.com`). Enables CF Zero Trust authentication. |
| `ISMS_CF_AUDIENCE` | CF Access Application Audience (AUD) tag. **Set this when using CF Access** — without it, any CF Access JWT from any application is accepted. |
| `ISMS_CF_AUTO_PROVISION` | Set to `1`/`true` to create the **user record** automatically on their first Cloudflare Access login (JIT). Off by default. The user is created with no org membership — an admin (or the CLI) then adds them to the right organization and role. **Only safe when your CF Access policy is the source of truth for who may reach ISMS** — see the warning below. |
| `ISMS_USER_SIGNUP` | Set to `1` to enable self-registration. Off by default. |
| `ISMS_SKIP_EMAIL_VERIFY` | Set to `1` to skip email verification on signup (users are active immediately). For dev/eval only. |

### SMTP (email notifications)

| Variable | Description |
|----------|-------------|
| `SMTP_HOST` | SMTP server hostname |
| `SMTP_PORT` | SMTP port (default: 587) |
| `SMTP_USER` | SMTP username |
| `SMTP_PASSWORD` | SMTP password |
| `SMTP_FROM` | From address for outgoing emails |

Email is used for review notifications, invite links, and verification. Without SMTP configured, email-dependent features silently skip sending.

### Notifications

Slack and Matrix notifications are configured **per-organization** in the web UI under **Admin > Settings**. There are no server-level env vars for notifications — each org manages its own webhook URLs and tokens.

### Git commit signing (optional)

| Variable | Description |
|----------|-------------|
| `ISMS_SIGNING_KEY` | Path to an SSH private key for signing git commits |
| `ISMS_SIGNING_NAME` | Committer name for signed commits (default: `isms.sh`) |
| `ISMS_SIGNING_EMAIL` | Committer email for signed commits (default: `git@isms.sh`) |

When set, all git commits made by the server are signed with this SSH key.

### Platform branding (optional)

| Variable | Description |
|----------|-------------|
| `ISMS_TERMS_FILE` | Path to a markdown file with Terms of Service |
| `ISMS_PRIVACY_FILE` | Path to a markdown file with Privacy Policy |
| `ISMS_HIDE_POWERED_BY` | Set to `1` to hide the "Powered by isms.sh" branding |

### CLI-only variables

These are used by the CLI client, not the server:

| Variable | Description |
|----------|-------------|
| `ISMS_API_URL` | API endpoint URL. If unset, derived from `ISMS_BASE_URL` + `/api`. |
| `ISMS_API_TOKEN` | Bearer token for CLI authentication (also accepted as `ISMS_API_KEY`) |
| `ISMS_ORGANIZATION` | Organization UUID for CLI operations |
| `ISMS_ROOT` | Local git repo root directory for CLI |
| `ISMS_USER` | User email for CLI task attribution |
| `ISMS_ENV` | Path to env file for CLI commands |
| `ISMS_SERVER_ENV` | Path to env file for `isms server` commands |
| `CF_ACCESS_CLIENT_ID` | Cloudflare Access service token client ID (for CLI through CF Zero Trust) |
| `CF_ACCESS_CLIENT_SECRET` | Cloudflare Access service token client secret |

## First run

### 1. Create the database

```bash
createdb isms
```

### 2. Set minimum configuration

```bash
cat > /etc/isms/server.env << 'EOF'
DATABASE_URL=postgres://isms:password@localhost/isms?sslmode=disable
ISMS_SECRET=<output-of-openssl-rand-hex-32>
ISMS_STORAGE_BACKEND=file
ISMS_DATA_DIR=/var/lib/isms/data
ISMS_TEMPLATE_PATH=/opt/isms-templates
ISMS_BASE_URL=https://isms.company.com
EOF
chmod 600 /etc/isms/server.env
```

### 3. Start the server

```bash
export ISMS_SERVER_ENV=/etc/isms/server.env
isms server serve --addr :8080
```

On first start, the server automatically creates the `schema_migrations` table and applies all pending migrations. You will see output like:

```
Running database migrations...
  Applying 20260327000000_initial_schema.sql...
  Applied 1 migration(s).
```

Migrations are embedded in the binary and tracked in the `schema_migrations` table. They are idempotent — running the server again skips already-applied migrations.

You can also run migrations separately without starting the server:

```bash
isms server migrate
```

### 4. Create the first organization and admin user

```bash
# Create a user
isms server user create --email admin@company.com --name "Admin Name" --password 'changeme'

# Create an organization (auto-initializes a bare git repo at $ISMS_DATA_DIR/repos/myco.git)
isms server org create --name "My Company" --slug myco

# Add the user as admin
isms server org add-member --org myco --email admin@company.com --role admin
```

### 5. Create an API token for CLI access

```bash
isms server api-key create --name cli --email admin@company.com --org myco
```

Save the printed token. It will not be shown again.

### 6. Scaffold a template

Log into the web UI at your `ISMS_BASE_URL`, navigate to Documents, and click a template card (e.g. ISO 27001). Alternatively, use the CLI:

```bash
export ISMS_ENV=/path/to/client.env
isms init --template iso27001
```

## Backup and restore

### What to back up

Three data stores need coordinated backup:

1. **PostgreSQL** — users, orgs, reviews, risks, assets, all registers, audit trail
2. **Git repositories** — `$ISMS_DATA_DIR/repos/` — bare git repos (one per org)
3. **Blob store** — `$ISMS_DATA_DIR/` (if `ISMS_STORAGE_BACKEND=file`) or your S3 bucket — branding assets, evidence files

### Backup procedure

```bash
#!/bin/bash
# Coordinated backup — run from cron
BACKUP_DIR=/var/backups/isms
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

mkdir -p "$BACKUP_DIR"

# 1. PostgreSQL dump
pg_dump -Fc isms > "$BACKUP_DIR/isms-pg-$TIMESTAMP.dump"

# 2. Git repos (rsync for incremental)
rsync -a /var/lib/isms/data/repos/ "$BACKUP_DIR/repos/"

# 3. Blob store (local file backend)
rsync -a /var/lib/isms/data/ "$BACKUP_DIR/blobs/" --exclude repos/

# For S3 backend, use your provider's backup mechanism or:
# aws s3 sync s3://your-bucket "$BACKUP_DIR/s3/"
```

### Restore procedure

```bash
# 1. Restore PostgreSQL
pg_restore -d isms --clean --if-exists isms-pg-20260416_030000.dump

# 2. Restore git repos
rsync -a /var/backups/isms/repos/ /var/lib/isms/data/repos/

# 3. Restore blobs
rsync -a /var/backups/isms/blobs/ /var/lib/isms/data/ --exclude repos/

# 4. Start the server — migrations will run if the binary is newer
isms server serve --addr :8080
```

The git repos must match the Postgres state. The `organizations` table stores the `repo_path` for each org. If you move repos, update those paths.

## Upgrade procedure

1. Stop the server (or let systemd handle the restart):

```bash
sudo systemctl stop isms
```

2. Replace the binary:

```bash
sudo cp isms-new /usr/local/bin/isms
sudo chmod +x /usr/local/bin/isms
```

3. Start the server:

```bash
sudo systemctl start isms
```

Migrations run automatically on startup. The server logs which migrations are applied. If no new migrations exist, you'll see `No pending migrations.`

4. Verify:

```bash
isms version
curl -s https://isms.company.com/healthz
# → ok
```

There is no downgrade mechanism. Take a backup before upgrading.

## Monitoring

### Health endpoint

```
GET /healthz → 200 "ok"
```

This endpoint is unauthenticated (excluded from auth middleware). Use it for load balancer health checks and uptime monitoring.

```bash
curl -f http://localhost:8080/healthz || echo "ISMS is down"
```

### Logs

The server uses Echo's logger middleware and Go's `log` package. Output goes to stdout/stderr. With systemd, logs go to the journal:

```bash
journalctl -u isms -f
```

Structured log lines include HTTP method, path, status code, and latency for every request.

### What to watch

- **Health endpoint** — `/healthz` returning 200
- **Process alive** — systemd restart count, process uptime
- **PostgreSQL connections** — the server uses pgx connection pooling; watch `pg_stat_activity` for connection count
- **Disk space** — git repos grow with every document edit; blob store grows with evidence uploads
- **Migration failures** — if the server fails to start after upgrade, check logs for migration errors

## Secret rotation

### ISMS_SECRET

`ISMS_SECRET` is used for two things:

1. **JWT signing** — all active web sessions are signed with HMAC-SHA256 using this secret
2. **AES-256-GCM encryption** — OIDC client secrets, OTP (TOTP) secrets, and sensitive org settings are encrypted at rest with a key derived from this secret

**Changing ISMS_SECRET will:**

- Invalidate all active JWT sessions (users must log in again)
- Make all encrypted OIDC client secrets unreadable (SSO login will break until re-entered)
- Make all OTP secrets unreadable (users with TOTP enabled will be locked out until OTP is disabled and re-enrolled)
- Make any encrypted org settings unreadable

**Rotation procedure:**

1. Back up the database
2. Note all OIDC provider configurations (you will need to re-enter client secrets)
3. Identify users with TOTP enabled
4. Stop the server
5. Update `ISMS_SECRET` to the new value
6. Start the server
7. Re-enter OIDC client secrets in Admin > SSO for each organization
8. Notify users with TOTP to re-enroll (admin can disable OTP for a user via the database, then the user re-enables it)

There is no automatic re-encryption tool. Plan secret rotation during a maintenance window.

### API tokens

API tokens are stored as SHA-256 hashes. They are not affected by `ISMS_SECRET` rotation. To revoke a token:

```bash
isms server api-key list
isms server api-key revoke <key-id> --email owner@company.com
```

### OIDC client secrets

OIDC client secrets are encrypted in the database with `ISMS_SECRET`. To rotate an OIDC secret, update it in Admin > SSO in the web UI (which re-encrypts with the current secret).

## Disaster recovery

### Full restore from backup

1. Provision a new server with PostgreSQL
2. Create the database: `createdb isms`
3. Restore the Postgres dump: `pg_restore -d isms isms-pg-TIMESTAMP.dump`
4. Restore git repos to the same path (or update `organizations.repo_path` rows in Postgres)
5. Restore blob store files
6. Deploy the same (or newer) binary
7. Set environment variables (same `ISMS_SECRET` — if you lost it, see secret rotation above)
8. Start the server

### Re-creating from scratch

If you have no backup but need to get running again:

1. Deploy the binary and start with a fresh database — migrations create the schema
2. Create org and users via the CLI
3. Scaffold templates
4. All Postgres data (reviews, risks, audit trail, etc.) is lost
5. If you have the git repos, document content is preserved — the server will read documents from the existing repos

## Daily, weekly, and monthly operations

### Daily

- Check `/healthz` is responding (automated monitoring)
- Review the **Inbox** in the web UI or via `isms inbox` for pending reviews and notifications
- Check `isms overdue` or the **Dashboard** for overdue tasks and review cycles

### Weekly

- Review open tasks: `isms status` or Dashboard > Tasks
- Check for stale reviews (open for too long without action)
- Review incident register for anything needing follow-up
- Run `isms server api-key list` to check for unused tokens

### Monthly

- Review risk register — re-assess risks approaching review dates
- Check supplier review dates
- Review legal register for upcoming compliance deadlines
- Verify backups by test-restoring to a staging instance
- Check disk usage on the data directory
- Review the audit programme calendar

### Annually

- Full document review cycle — use `isms overdue` to generate tasks for documents past their review date
- Internal audit execution
- Management review preparation
- Rotate `ISMS_SECRET` if required by your security policy (see secret rotation section)

## Troubleshooting

### Server won't start: "DATABASE_URL is required"

The `DATABASE_URL` environment variable is not set. Ensure your env file is loaded:

```bash
export ISMS_SERVER_ENV=/etc/isms/server.env
```

### Server won't start: "ISMS_SECRET is required" or "ISMS_SECRET must be at least 32 characters"

Generate a secret: `openssl rand -hex 32` (produces 64 hex characters).

### Server won't start: "ISMS_STORAGE_BACKEND is required"

Set `ISMS_STORAGE_BACKEND=file` or `ISMS_STORAGE_BACKEND=s3` in your environment.

### Migration failure on startup

Check the log output for the specific SQL error. Common causes:
- Database user lacks `CREATE TABLE` privileges
- Connecting to the wrong database
- Partial migration from a previous interrupted run — check `schema_migrations` table for the last applied version

If a migration is partially applied, you may need to manually fix the database state and insert the version into `schema_migrations`.

### "ISMS_TEMPLATE_PATH not set" when scaffolding

The template directory is not configured. Set `ISMS_TEMPLATE_PATH` to the directory containing your templates (typically a cloned `isms-templates` repo).

### SSO / OIDC login fails after secret rotation

OIDC client secrets are encrypted with `ISMS_SECRET`. After rotation, re-enter the client secret in Admin > SSO for each OIDC provider.

### Users locked out after secret rotation (TOTP)

OTP secrets are encrypted with `ISMS_SECRET`. An admin needs to disable OTP for affected users (via direct database update on the `users` table, clearing the `otp_secret` and `otp_enabled` columns), then users re-enroll.

### WebAuthn / passkeys not working

Passkeys require `ISMS_BASE_URL` to be set correctly. The Relying Party ID is derived from the hostname in `ISMS_BASE_URL`. If this doesn't match the domain users access, passkey registration and login will fail.

### CORS errors in browser

Set `ISMS_BASE_URL` to the URL users access (e.g. `https://isms.company.com`). The server uses this as the CORS allowed origin. For multiple origins, use `ISMS_CORS_ORIGIN=https://origin1.com,https://origin2.com`.

### Rate limiting in development/testing

Disable with `ISMS_RATE_LIMIT=0`. The default is 20 requests per minute per IP.

### CF Access warning: "ISMS_CF_AUDIENCE not set"

When `CLOUDFLARE_TEAM_DOMAIN` is set but `ISMS_CF_AUDIENCE` is not, the server logs a warning on startup. Without the audience check, any CF Access JWT from any application on your team domain is accepted. Set `ISMS_CF_AUDIENCE` to the Application Audience (AUD) tag from your CF Access dashboard.

### Cloudflare Access as web login (Zero Trust SSO)

With `CLOUDFLARE_TEAM_DOMAIN` + `ISMS_CF_AUDIENCE` set and ISMS published behind a CF Access application, a user who has already passed CF Access (e.g. via Entra at the CF layer) is logged into the **web app** automatically — no separate ISMS login. On load the SPA calls `GET /api/v1/auth/cf-session`; behind CF Access the proxy adds the identity headers, the server **validates the Access JWT** (not just the email header), resolves the user, and mints an ISMS session. The same CF identity also authenticates the API, CLI, and git.

By default the user must already exist in ISMS and be a member of the org. To skip the manual "create the user" step, enable JIT provisioning:

```
ISMS_CF_AUTO_PROVISION=1
```

With this on, the first CF Access login **creates the user record** (no org, no role). They can authenticate, but can't load an org's data until an **admin adds them to the right organization** — Admin → Members, or:

```
isms server org add-member --org acme --email user@acme.com --role reader
```

There is intentionally **no default org/role**: which org someone belongs to, and at what role, is a deliberate decision, not an env-var guess.

**Security tradeoff — read before enabling JIT.** Auto-provisioning is safe **only when your CF Access policy is the source of truth for who may reach ISMS.** If a route is left unprotected, or a CF Access token leaks, an unintended user could create an account. Mitigations: it's opt-in (default off); a provisioned user has **no org access at all** until an admin grants it (so a stray account can see nothing); and the Access JWT is always cryptographically verified — a missing/invalid token never provisions.

### Git repo not found for organization

The `organizations` table stores `repo_path` for each org. If the path doesn't exist, the server initializes it on first access. If you moved data directories, update the paths:

```sql
UPDATE organizations SET repo_path = '/new/path/repos/myco.git' WHERE slug = 'myco';
```

### Checking database migration state

```sql
SELECT version, applied_at FROM schema_migrations ORDER BY version;
```
