# ISMS Platform — Claude Code Guide

## What is this?

A versioned document management platform. Documents live in git (markdown + YAML frontmatter). Collaboration (reviews, comments, approvals, tasks) lives in PostgreSQL. Web UI (Vue) for readers and management. CLI for managers.

The core is a **generic document engine** — it knows nothing about ISO 27001, clauses, controls, or any specific standard. All standard-specific content comes from **templates** that are loaded from disk. Templates provide the starting document structure; after scaffolding, everything is just documents.

## Architecture

### Core = Document Engine

The core provides:
- **Git-backed documents** — markdown files with YAML frontmatter, versioned
- **Review workflow** — send for review, inline comments, approve/reject, merge
- **Registers** — risks, assets, suppliers, systems, legal requirements, incidents (all in PostgreSQL)
- **Multi-tenant** — organizations with RLS, per-org identifiers, white-label branding
- **Authentication** — OIDC, password+TOTP, passkeys, API tokens

The core does NOT know about:
- Specific standards (ISO 27001, SOC 2, etc.)
- Folder names (clauses, controls, policies — those are template choices)
- SoA generation (that's a template-level document)
- Document types — everything is a "document" with a `document_id`

### Templates = Standard-Specific Content

Templates live on disk at `ISMS_TEMPLATE_PATH` (separate git repo). Each template is a directory of markdown files that get scaffolded into an org's git repo. Templates provide:
- Folder structure (template maintainer chooses names)
- Document content (markdown with TODO placeholders)
- `.title` files for folder display names
- `meta.yaml` with template identity and version

After scaffolding, the org owns the content. Templates are not referenced again — the org's git repo is the source of truth.

### Data Split

```
Git repo (per org)          PostgreSQL (shared)           Blob store (per org)
├── README.md               ├── users & organizations     {org-uuid}/
├── documents/              ├── reviews, comments         ├── branding/
│   ├── <template>/         ├── approvals, decision log   │   ├── logo.svg
│   │   ├── <folders>/      ├── tasks, changes            │   └── favicon.ico
│   │   │   ├── .title      ├── suggestions               └── evidence/
│   │   │   └── *.md        ├── risks, assets, suppliers      └── {checkin-id}/
│   │   └── ...             ├── systems, incidents
│   └── <other-template>/   ├── corrective actions        ISMS_STORAGE_BACKEND=file → disk
                            ├── audit programmes, findings ISMS_STORAGE_BACKEND=s3 → S3/R2
                            ├── objectives, programs
                            ├── legal requirements
                            ├── notifications, activity
                            ├── entity changelog
                            ├── approval policies
                            └── entity references
```

Git stores ONLY documents (markdown + frontmatter). No images, no branding, no binary files. Branding and evidence go to the blob store (`internal/isms/blob/`).

## CRITICAL RULES

### Document versioning model
Three layers of history, each with a distinct purpose:
- **Git commits** = raw edit history. Every save is a commit. This is the working log.
- **`document_versions`** = official milestones only. Created on merge/publish and confirm. NOT on draft edits. This is what the Version History UI shows.
- **`decision_log`** = governance trail. Approvals, rejections, merges, confirmations with content hashes.

Version numbers increment when an approved document is edited into a new draft (status goes from `approved` → `draft`). They do NOT increment on every save. Draft edits are working state, not milestones.

### Review status transitions
Review status is a state machine. Only dedicated endpoints may transition between states:
- `open` → `approved` (via approve endpoint)
- `open` → `changes_requested` (via approve endpoint with changes_requested/proposed_revision)
- `changes_requested` → `open` (via resubmit)
- `approved` → `merged` (via merge endpoint)
- Any active status → `closed` (via status endpoint — the ONLY transition it allows)

The `PUT /reviews/:id/status` endpoint accepts ONLY `closed`. All other transitions go through their dedicated handlers. Never bypass this.

### NEVER use git CLI on the server
All git operations on the server MUST use go-git library. The ONLY exception is `api_git.go` for the wire protocol.

### Everything is a document
There are no "clauses", "policies", or "controls" in the core. All documents have `document_id` in frontmatter. Use `store.FindDocumentByID()` to resolve paths. Never hardcode folder names.

### Templates from disk, not embedded
Templates are loaded from `ISMS_TEMPLATE_PATH`. Use `scaffold.ListTemplates()` and `scaffold.IsValidTemplate()`. Never embed template content in the binary.

### Document IDs are lowercase
All document_ids are lowercase with hyphens: `iso27001-4-1`, `iso27001-a-5-1`. Normalized to lowercase on read (`LoadDocument`). Case-insensitive uniqueness enforced.

### No standard-specific logic in core
If something is specific to ISO 27001, PCI DSS, or any other standard, it belongs in the template or in a future plugin — not in the core engine. The core is agnostic.

## Key Types

```go
// store/document.go
type DocumentFile struct {
    Path        string
    Frontmatter model.DocumentFrontmatter
    Body        string
}

// model/document.go
type DocumentFrontmatter struct {
    DocumentID string   `yaml:"document_id"`
    Title      string   `yaml:"title"`
    Version    string   `yaml:"version,omitempty"`
    Status     string   `yaml:"status"`       // draft, in_review, approved, retired
    Author     string   `yaml:"author,omitempty"`
    // ... other optional fields
}
```

## Key Functions

```go
store.LoadDocument(path)          // read document from git
store.SaveDocument(doc)           // write document to git
store.FindDocumentByID(id)       // resolve document_id → path (cached)
store.LoadDocumentsFromDir(dir)   // list all docs in a folder
store.ListDocFolders()            // list top-level doc folders

scaffold.ListTemplates()          // available templates from disk
scaffold.IsValidTemplate(id)      // check template exists
scaffold.Init(root, template)     // scaffold template into repo
scaffold.ScaffoldToRepo(st, ...)  // scaffold into bare git repo
```

## API Endpoints (documents)

```
GET  /documents/all              — list all documents with folder tree
GET  /documents/:docId/body      — get document content by ID
PUT  /documents/:docId/metadata  — update document metadata
PUT  /documents/:docId/content   — update document content
GET  /documents/search           — search documents
GET  /documents/needs-review     — documents changed since approval
GET  /templates/available        — list available templates
POST /templates                  — scaffold template into org
```

## CLI

```bash
isms document list               # list all documents
isms document list --folder X    # list documents in folder X
isms document show <id>          # show document details + content
isms document cat <id>           # print document body only
isms init --template iso27001    # scaffold new repo from template
isms sync                        # push/pull git
isms review send <id> --to email # send for review
isms status                      # show overdue reviews
```

## Roles

admin > manager > contributor > reader

Review assignment grants approve/comment rights to any role.

## Entity Suggestions

Generic suggestion primitive — one `suggestions` table serves all modules. AI or human proposes change → manager reviews and applies atomically.

- 22 apply handlers across 11 entity types (risk, incident, supplier, legal, change, CA, task, objective, system, asset, audit_finding)
- Atomic apply via `WithOrgTx` — entity mutation + changelog + suggestion mark in one transaction
- Stale detection via `entity_updated_at` snapshot
- RBAC: any user can create, only manager/admin can apply/reject

## MCP Server

`isms server mcp` — 22 tools over stdio. Agents read entities, suggest changes, review documents.

Key tools: `get_isms_overview`, `create_suggestion`, `comment_on_review`, `approve_review`, `merge_review`, `edit_review_content`, `confirm_document_review`, `get_pending_actions`.

## Agent Identity

- `users.is_agent` boolean — set via `isms server user create --agent`
- `suggested_by_type: agent` on suggestions — auto-set by MCP
- `ai_enabled` org setting — kill switch blocks all agent API tokens
- Approval policies: `require_human` and `auto_merge` flags

## Project Structure

```
cmd/isms/           CLI commands + MCP server
internal/isms/
  api/              REST API (Echo)
  db/               PostgreSQL (pgx)
  mcp/              MCP server (stdio JSON-RPC)
  model/            Data types
  store/            Git document storage
  scaffold/         Template scaffolding
  tui/              Terminal UI
  notify/           Slack + Matrix
  mail/             SMTP email
web/                Vue 3 + Vite + Tailwind
migrations/         PostgreSQL schema
docs/               Architecture, AI strategy, specs
```
