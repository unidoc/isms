# Architecture: Core vs Templates

## The Split

The platform has two distinct layers:

**Core (`isms`)** — A generic versioned document engine. It manages documents in git, collaboration in PostgreSQL, and provides a web UI and API. It knows nothing about ISO 27001, SOC 2, or any specific standard.

**Templates (`isms-templates`)** — Standard-specific content packs. Each template provides a folder structure with markdown documents that get scaffolded into an organization's git repo. After scaffolding, the org owns the content.

## Core: What It Does

The core provides infrastructure:

- **Git-backed documents** — Markdown files with YAML frontmatter (`document_id`, `title`, `status`)
- **Round-based review** — Documents are reviewed in explicit rounds. Each round tracks what changed since the last round, who approved, who requested changes, and who proposed revisions. Three reviewer actions: approve, request changes, or propose revision (edit the document directly). Immutable decision records with SHA-256 content hashes provide tamper-evident audit trail. Inline paragraph-level suggestions with accept/reject. "This round" vs "All changes" diff views. Reviewers are locked after acting — no accidental double-approval.
- **Registers** — Risks, assets, suppliers, systems, legal requirements, incidents (PostgreSQL)
- **Entity references** — Generic linking between any entity (documents, risks, assets, etc.)
- **Multi-tenant** — Organizations with RLS, per-org identifiers, white-label branding
- **Authentication** — OIDC, password+TOTP, passkeys, API tokens
- **Automation** — Overdue detection, task creation, review cycle tracking

The core does NOT provide:

- Specific folder names (clauses, controls, policies)
- Standard-specific logic or validation
- SoA generation or compliance scoring
- Any knowledge of what "ISO 27001 clause 4.1" means

## Templates: What They Provide

A template is a directory of markdown files on disk:

```
isms-templates/
├── iso27001/
│   ├── meta.yaml
│   ├── clauses/
│   │   ├── .title                     # "Clauses"
│   │   ├── 04-context-of-the-organisation/
│   │   │   ├── .title                 # "4. Context of the Organisation"
│   │   │   ├── 4.01-external-and-internal-issues.md
│   │   │   └── 4.02-interested-parties.md
│   │   └── 05-leadership/
│   │       └── ...
│   └── controls/
│       ├── .title                     # "Controls"
│       ├── a.05-organizational-controls/
│       │   ├── .title                 # "A.5 Organizational Controls"
│       │   ├── a.5.01-policies-for-information-security.md
│       │   └── ...
│       └── a.08-technological-controls/
│           └── ...
├── hipaa/
│   ├── meta.yaml
│   ├── safeguards/                    # HIPAA uses different folder names
│   │   ├── 01-privacy-rule/
│   │   └── 02-administrative-safeguards/
│   └── requirements/
│       └── ...
└── nist-csf/
    ├── meta.yaml
    ├── functions/                     # NIST CSF uses "functions"
    │   ├── 01-govern/
    │   └── 02-identify/
    └── subcategories/                 # and "subcategories"
        └── ...
```

Each template chooses its own:
- **Folder names** — `clauses/` for ISO, `safeguards/` for HIPAA, `functions/` for NIST CSF
- **Document IDs** — `iso27001-4-1`, `hipaa-2-1`, `nistcsf-1-1`
- **Titles** — "4.1 External and Internal Issues"
- **Content** — Markdown body with TODO placeholders
- **`.title` files** — Display names for folders

The core renders whatever folder structure is in git. It never assumes specific folder names exist.

## How They Connect

### Scaffolding

When an org adds a template, the core copies the template files into the org's git repo:

```
Template (disk)                    Org Git Repo
iso27001/clauses/04-context/  →   documents/iso27001/clauses/04-context/
iso27001/controls/a.05-org/   →   documents/iso27001/controls/a.05-org/
```

After scaffold, the template is never referenced again. The org's git repo is the source of truth.

### Entity References

The `entity_references` table links anything to anything:

```
source_type: "risk"
source_id:   "RISK-1"
target_type: "document"
target_id:   "iso27001-a-5-1"
```

This connects a risk in PostgreSQL to a document in git. The core doesn't know that `iso27001-a-5-1` is an "Annex A control" — it just knows it's a document ID.

References work across the git/PostgreSQL boundary:
- Document ↔ Document (e.g., policy references a control)
- Risk ↔ Document (e.g., risk linked to a control)
- Incident ↔ Risk (both in PostgreSQL)
- Legal requirement ↔ Document (e.g., GDPR linked to privacy policy)

### SoA (Statement of Applicability)

SoA is NOT a core feature. It's a document that lives in the ISO 27001 template:

```
iso27001/
├── clauses/
├── controls/
└── soa.md          # SoA is just a document, maintained by the ISMS manager
```

The SoA document references controls via document IDs, includes maturity ratings and justifications. It's version-controlled in git like everything else. Claude Code can generate and update it.

## Example: ISO 27001 vs HIPAA

Both use the same core platform but with different templates:

| | ISO 27001 | HIPAA |
|---|-----------|-------|
| Template ID | `iso27001` | `hipaa` |
| Top folders | `clauses/`, `controls/` | `safeguards/`, `requirements/` |
| Document IDs | `iso27001-4-1`, `iso27001-a-5-1` | `hipaa-2-1`, `hipaa-r-1` |
| SoA | `soa.md` (template doc) | Not applicable |
| Review cycles | From risk level | From risk level |
| Audit workflow | Same core | Same core |
| Risk register | Same core | Same core |

The core platform is identical. Only the documents differ.

## Why This Matters

1. **No lock-in** — Templates are markdown files in git. Move to another platform by copying your repo.
2. **Community templates** — Anyone can create a template for any standard, framework, or internal process.
3. **Multi-standard** — One org can run ISO 27001 + ISO 14001 + HIPAA simultaneously. Same core, different document folders.
4. **Clean core** — No standard-specific code to maintain. The core evolves independently of templates.
5. **AI-friendly** — Claude Code works with markdown files and document IDs. No special API needed per standard.
