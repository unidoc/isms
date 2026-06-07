# isms.sh — The Intelligent Management System

An open-source management system platform. Documents live in git, collaboration lives in PostgreSQL, AI operates through suggestions and review — and everything ships as a single binary.

The core is a **generic versioned document engine** — it knows nothing about specific standards. All standard-specific content is provided through **templates**: collections of markdown files that get scaffolded into your organization's git repository. Templates exist for ISO 27001, ISO 9001, ISO 14001, ISO 42001, SOC 2, NIS2, PCI DSS, HIPAA, NIST CSF, NIST 800-53, and DORA — and creating your own is as simple as writing markdown files in a folder.

**[isms.sh](https://isms.sh)** — Built by [UniDoc](https://unidoc.io).

![Overview](docs/screenshots/overview.png)

## Try the live demo

A hosted demo runs at **[demo.isms.sh](https://demo.isms.sh)** with a sample
organization — **ACME Logistics** — pre-populated so you can explore every role.
Every demo account uses the password `demo`:

| Role | Email | Password |
|------|-------|----------|
| Admin | `sarah.chen@acme-logistics.demo` | `demo` |
| Manager | `maria.rodriguez@acme-logistics.demo` | `demo` |
| Contributor | `david.walsh@acme-logistics.demo` | `demo` |

> The demo is a sandbox: data resets periodically and holds nothing real.
> Please don't store anything sensitive there.

## Round-Based Document Review

![Review workflow](docs/screenshots/review.png)

Most compliance tools treat review as a checkbox. isms.sh treats it as a proper workflow with tracked rounds — because a lawyer reviewing a data processing agreement needs to know exactly what changed since their last feedback, not scroll through a generic diff.

- **Round-based review** — Send for review, get feedback, resubmit. Each round is explicit.
- **What changed since last round** — Default view shows only changes in the current round. Toggle to see everything since the review began.
- **Clear reviewer status per round** — See who approved, who's pending, and who requested changes — for this specific round.
- **Three reviewer actions** — Approve. Request changes with comments. Or propose a revision — edit the document directly and send your version back.
- **Inline suggestions** — Reviewers suggest replacement text on individual paragraphs. Author accepts or rejects each one.
- **Auto-merge** — Approval policies with `require_human` and `auto_merge` flags. When all requirements are met, the document publishes automatically.
- **Immutable audit trail** — Every approval, every round transition, every proposed revision gets a decision record with a SHA-256 content hash.

## AI-First

AI is a first-class participant, not a bolt-on.

- **MCP server** — `isms server mcp` exposes 22 tools over stdio. Any MCP-compatible AI (Claude Code, etc.) can read entities, suggest changes, review documents, and propose operational actions.
- **Entity suggestions** — Generic suggestion primitive across all modules. AI (or any user) proposes changes → manager reviews and applies atomically.
- **Agent identity** — Agent users are explicit (`is_agent` flag). Every AI action is attributed. UI shows AI badge. Audit trail distinguishes human from agent.
- **AI document review** — Agent users participate in review as assigned reviewers: inline comments, paragraph suggestions, approve/reject decisions.
- **AI review loop** — Two agents iterate (writer + reviewer) with automatic notification suppression. Escalates to human after max rounds.
- **Annual review confirmation** — AI reviews document against registers, human owner confirms. Creates audit evidence without full review cycle.
- **AI kill switch** — `ai_enabled` org setting. Set to `false` and all agent API tokens are blocked. Core works identically without AI.
- **Three approval modes** — Human required (default), AI confidence + human confirm, full autopilot.

## Features

- **Template scaffolding** — Pick a standard, scaffold the documents, start writing. Templates provide structure; you own the content.
- **Multi-standard** — Run ISO 27001 + ISO 14001 + HIPAA simultaneously in one organization.
- **Git-based documents** — Markdown with YAML frontmatter. Full version history with diffs and blame.

  ![Documents](docs/screenshots/documents.png)
- **Review workflow** — Round-based review, inline suggestions, approval policies, auto-merge. See above.
- **Risk management** — Risk register with 5×5 matrix, inherent/residual scoring, CIA impact, treatment plans, auto-calculated review dates.

  ![Risk Register](docs/screenshots/risks.png)
- **Asset register** — Information assets with CIA ratings, ownership, classification, linked to risks and systems.
- **Legal & statutory requirements** — Regulatory obligations with jurisdiction, compliance status, review cycles.
- **Internal audit** — Audit programmes, scope-based assessments, findings, corrective actions with full lifecycle.
- **Supplier management** — Supplier register with criticality, certifications, data access tracking, review cycles.
- **Business impact analysis** — Systems with RPO/RTO, criticality levels, access reviews, supplier/asset linkage.
- **Incident management** — Incident register with severity, timeline, root cause, lessons learned.
- **Change management** — Change requests with priority, risk level, rollback plan, approval workflow.
- **Corrective actions** — Full lifecycle from assessment through implementation to verification.
- **Objectives & programs** — Measurable objectives with check-ins, evidence upload, review cycles.
- **Tasks** — Operational work items with auto-generation from overdue review cycles.
- **Entity references** — Link anything to anything: risks to documents, incidents to assets, legal requirements to controls.
- **Dashboard** — Annual plan calendar, overdue items, module overview.
- **Notifications** — In-app notifications, Slack webhooks, Matrix integration (configured per-org in Admin).
- **Web UI** — Vue 3 dark-themed SPA for readers and management.
- **CLI** — Full-featured command-line interface for ISMS managers.
- **TUI** — Terminal UI for quick operational access.
- **One binary** — Single Go binary: serve, CLI, TUI, MCP, migrate, manage.

## Security

- **5 auth methods** — OIDC/SSO, password+TOTP, passkeys (WebAuthn), API tokens, Cloudflare Zero Trust
- **Secrets encrypted at rest** — AES-256-GCM on OTP secrets, OIDC client secrets, sensitive settings
- **Two-layer tenant isolation** — Application-layer `organization_id` filtering on all queries, with Postgres RLS policies as defense-in-depth on transactional operations via `WithOrgTx`
- **Multi-tenant isolation** — composite FKs, org membership validation, UUID-only org resolution, org-scoped API tokens
- **Role-based access control** — admin, manager, contributor, reader (per-org). Review assignment grants approve rights.
- **Agent identity** — `is_agent` flag on users, enforced in policy evaluation and audit trail
- **AI kill switch** — `ai_enabled` org setting blocks all agent API tokens at middleware level
- **SVG sanitization** — Regex-based stripping of scripts, event handlers, dangerous URIs on branding uploads
- **Repo protection** — Path allowlist, size limits, symlink/exec rejection on git push
- **Audit trail** — Activity log + entity changelog with field-level diffs on all operations

See [Architecture](docs/architecture.md) for details on the core vs templates split and multi-tenancy design.

## Quick Start

### Prerequisites

- Go 1.25+
- PostgreSQL 14+
- Node.js 20+ (for web UI development)

### Build

```bash
go build -o isms ./cmd/isms/
```

### Setup

```bash
# Set required environment
export DATABASE_URL="postgres://user:pass@localhost/isms?sslmode=disable"
export ISMS_SECRET="$(openssl rand -hex 32)"  # min 32 characters
export ISMS_STORAGE_BACKEND="file"
export ISMS_TEMPLATE_PATH="/path/to/isms-templates"

# Start the server (runs migrations automatically)
isms server serve --addr :9090

# Create the first user and organization
isms server user create --email admin@company.com --name "Admin" --password changeme
isms server org create --name "My Company" --slug myco
isms server org add-member --org myco --email admin@company.com --role admin
```

### Configure CLI

Create an env file (e.g. `company.env`):

```env
ISMS_BASE_URL=http://localhost:9090
ISMS_API_TOKEN=isms_your_token_here
ISMS_ORGANIZATION=<org-uuid>
```

```bash
export ISMS_ENV=company.env
isms whoami
isms document list
isms risk list
```

### Setup AI Agent

```bash
# Create agent user
isms server user create --email ai@company.com --name "Claude Agent" --agent
isms server org add-member --org myco --email ai@company.com --role contributor
isms server api-key create --email ai@company.com --name "mcp"

# Configure Claude Code (.claude/settings.json)
{
  "mcpServers": {
    "isms": {
      "command": "isms",
      "args": ["server", "mcp"],
      "env": {
        "ISMS_API_URL": "http://localhost:9090",
        "ISMS_API_TOKEN": "tok_..."
      }
    }
  }
}
```

## Scope — what ISMS is, and isn't

ISMS is a generic, versioned engine for management systems. The core is
deliberately small: it does a few things well and pushes everything else to its
edges. That smallness is the point — it keeps the engine clean, and the value of
a managed deployment lives at the edges.

**Core** (in the binary) — generic primitives every deployment needs: git-backed
documents (markdown + frontmatter), the review/approval workflow, and structured
registers (risks, assets, suppliers, systems, incidents, legal requirements,
corrective actions, audits, objectives). Multi-tenant, white-label,
authentication. The core knows nothing about any specific standard.

**Templates** — standard-specific content is documents, not core. ISO 27001
clauses, controls, a Statement of Applicability, management-review minutes,
competence records are markdown documents scaffolded from
[isms-templates](https://github.com/unidoc/isms-templates) and owned per
organization. Templates get you started fast and make the core/content
separation concrete. Once scaffolded, documents and operational entities (risks,
incidents, …) cross-link freely.

**Integrations** — external tools are sources; ISMS is the system of record.
Evidence and objectives flow in through the integration layer, with objective
check-ins capturing evidence against the objectives they support. The direction
is first-party connectors for major systems alongside private, customer-specific
integrations.

**Hosting** — AI/agent wiring, deployment, and operations are hosting concerns,
not the engine. A managed, hosted ISMS is operated by UniDoc at
[isms.sh](https://isms.sh); self-hosting is fully supported.

The test for any new capability: *does every deployment need it, generically?* →
core. Standard-specific → template. External system → integration. Deployment or
AI wiring → hosting.

## Architecture

```
                    ┌─────────────────────────┐
                    │      ISMS Server         │
 CLI ──┐            │                          │
       │  Bearer    │  ┌─────┐    ┌────────┐   │
 TUI ──┼──token───▶ │  │ API │───▶│  Git   │   │
       │            │  │     │    │  Store  │   │
 Web ──┤            │  │Echo │    └────────┘   │
       │  CF Zero   │  │     │    ┌────────┐   │
 MCP ──┘  Trust     │  │     │───▶│Postgres│   │
                    │  └─────┘    └────────┘   │
                    └─────────────────────────┘
```

**Git repository** stores all documents:
- `documents/` — All documents organized by template-defined folders
- `branding/` — Logo, favicon (optional)

**PostgreSQL** stores collaboration and operational data:
- Reviews, comments, approvals, decision log
- Risks, incidents, suppliers, systems, assets, legal requirements
- Audit programmes, findings, corrective actions
- Objectives, programs, check-ins, evidence
- Tasks, change requests, suggestions
- Notifications, activity log, entity changelog
- Users, API tokens, approval policies

## Authentication

Five authentication methods:

1. **OIDC / SSO** — Microsoft 365, Google, Okta, any OIDC provider — per-org configuration
2. **Password + TOTP** — Local users, auditors, external parties
3. **Passkeys (WebAuthn)** — Modern passwordless authentication
4. **API tokens** — `Authorization: Bearer isms_xxx` — for CLI and AI agents. Tokens are org-scoped by default.
5. **Cloudflare Zero Trust** — Optional reverse proxy authentication (`ISMS_CF_AUDIENCE` required)

### Roles

- **Admin** — full access, user management, SSO/settings
- **Manager** — operates the ISMS: risks, documents, audits, reviews, templates
- **Contributor** — operational input: incidents, change requests, tasks, comments
- **Reader** — read-only access. Any role can review documents when assigned.

## Environment Variables

**Required:**

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |
| `ISMS_SECRET` | Secret key for JWT signing and encryption (minimum 32 characters) |
| `ISMS_STORAGE_BACKEND` | Blob storage backend: `file` (local disk) or `s3` (S3-compatible/R2) |
| `ISMS_TEMPLATE_PATH` | Path to template directory on disk. Required for template scaffolding. |

**Server:**

| Variable | Description |
|----------|-------------|
| `ISMS_BASE_URL` | Public URL (e.g. `https://isms.company.com`) — used for CORS, passkeys, notification links |
| `ISMS_DATA_DIR` | Data directory for git repos and local file storage |
| `ISMS_WEB_DIR` | Path to Vue web frontend `dist/` directory |
| `ISMS_ROOT` | Git repository root (where the org git repo lives) |

**S3 storage** (required if `ISMS_STORAGE_BACKEND=s3`):

| Variable | Description |
|----------|-------------|
| `ISMS_S3_BUCKET` | S3 bucket name |
| `ISMS_S3_REGION` | S3 region (e.g. `auto` for R2) |
| `ISMS_S3_ENDPOINT` | S3 endpoint URL |
| `ISMS_S3_ACCESS_KEY` | S3 access key |
| `ISMS_S3_SECRET_KEY` | S3 secret key |

**CLI:**

| Variable | Description |
|----------|-------------|
| `ISMS_API_URL` | API URL for CLI (default: `ISMS_BASE_URL/api`) |
| `ISMS_API_TOKEN` | API token for CLI authentication |
| `ISMS_ORGANIZATION` | Organization UUID for CLI |
| `ISMS_USER` | User email for CLI identity |

**Authentication:**

| Variable | Description |
|----------|-------------|
| `CLOUDFLARE_TEAM_DOMAIN` | Cloudflare Access team domain |
| `ISMS_CF_AUDIENCE` | Cloudflare Access application audience tag (required for CF Zero Trust) |
| `ISMS_USER_SIGNUP` | Set to `1` to enable self-signup (dev mode) |
| `ISMS_SKIP_EMAIL_VERIFY` | Set to `1` to skip email verification (dev mode) |
| `ISMS_RATE_LIMIT` | Rate limit override (`0` to disable, dev mode) |

**SMTP** (optional):

| Variable | Description |
|----------|-------------|
| `SMTP_HOST` | SMTP server for email notifications |
| `SMTP_PORT` | SMTP port (default: 587) |
| `SMTP_USER` | SMTP username |
| `SMTP_PASSWORD` | SMTP password |
| `SMTP_FROM` | From address for emails |

**Git commit signing** (optional):

| Variable | Description |
|----------|-------------|
| `ISMS_SIGNING_KEY` | Path to SSH key for git commit signing |
| `ISMS_SIGNING_NAME` | Signer name for git commits |
| `ISMS_SIGNING_EMAIL` | Signer email for git commits |

**Platform branding** (optional):

| Variable | Description |
|----------|-------------|
| `ISMS_TERMS_FILE` | Path to terms of service markdown file |
| `ISMS_PRIVACY_FILE` | Path to privacy policy markdown file |
| `ISMS_HIDE_POWERED_BY` | Set to `1` to hide "Powered by" footer |

Slack and Matrix notifications are configured per-organization in **Admin → Settings**, not via environment variables.

## Documentation

- [Evaluate in 10 Minutes](docs/evaluate.md) — Quick start guide
- [Architecture](docs/architecture.md) — Core vs templates, multi-tenancy, entity references
- [AI-First Strategy](docs/ai-first.md) — AI architecture, MCP tools, agent identity
- [Suggestions](docs/suggestions.md) — Entity suggestion system specification
- [AI Review Loop](docs/ai-review-loop.md) — Multi-agent document review design
- [Releasing](docs/releasing.md) — Cadence, versioning discipline, and house style

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines. Please open an issue first to discuss what you'd like to change.

How we ship — a weekly release train, honest semver, and a deliberate release
discipline — is documented in [docs/releasing.md](docs/releasing.md). The short
version: rigor lives in the process (CI, signed gates), so the people can stay
welcoming.

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.

Copyright 2026 [UniDoc ehf.](https://unidoc.io) — [isms.sh](https://isms.sh)
