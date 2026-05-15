# Template Authoring Guide

This guide is for anyone creating a new compliance template for isms.sh -- whether for a published standard (ISO 27001, HIPAA, NIST CSF), an industry framework, or a custom internal governance structure.

## What is a template

A template is a folder of markdown files with YAML frontmatter, plus a `meta.yaml` file that identifies the template. Templates live on disk at the path configured by `ISMS_TEMPLATE_PATH` (typically a separate git repository such as `isms-templates`).

When an organization adds a template, the platform copies the template files into the organization's git repository under `documents/<template-id>/`. After scaffolding, the organization owns the content. The template is never referenced again -- the org's repository is the source of truth.

Templates provide:

- Folder structure (you choose the names)
- Document content (markdown with TODO placeholders for org-specific content)
- `.title` files for folder display names in the UI
- `meta.yaml` with template identity and version

The core engine is standard-agnostic. It renders whatever folder structure is in git. There are no hardcoded folder names, document types, or framework-specific concepts in the core. Everything framework-specific lives in the template.

## Template structure

A template is a directory containing `meta.yaml` and one or more folders of markdown documents:

```
isms-templates/
├── iso27001/
│   ├── meta.yaml
│   ├── clauses/
│   │   ├── .title
│   │   ├── 04-context-of-the-organisation/
│   │   │   ├── .title
│   │   │   ├── 4.01-external-and-internal-issues.md
│   │   │   └── 4.02-interested-parties.md
│   │   └── 05-leadership/
│   │       ├── .title
│   │       └── 5.01-leadership-commitment.md
│   └── controls/
│       ├── .title
│       ├── a.05-organizational-controls/
│       │   ├── .title
│       │   ├── a.5.01-policies-for-information-security.md
│       │   └── a.5.02-information-security-roles.md
│       └── a.08-technological-controls/
│           ├── .title
│           └── a.8.01-user-endpoint-devices.md
├── hipaa/
│   ├── meta.yaml
│   ├── safeguards/
│   │   └── ...
│   └── requirements/
│       └── ...
└── nist-csf/
    ├── meta.yaml
    ├── functions/
    │   └── ...
    └── subcategories/
        └── ...
```

### meta.yaml

Every template must have a `meta.yaml` at its root. This file identifies the template to the platform:

```yaml
id: iso27001
name: ISO/IEC 27001:2022
description: Information security management system based on ISO/IEC 27001:2022
version: "1.0"
maintainer: isms.sh
```

| Field | Required | Description |
|-------|----------|-------------|
| `id` | Yes | Unique identifier. Lowercase, hyphens allowed. Becomes the folder name under `documents/`. If omitted, the directory name is used. |
| `name` | Yes | Human-readable name shown in the UI template picker |
| `description` | Yes | Brief description of what the template covers |
| `version` | Yes | Template version (for tracking updates to the template itself) |
| `maintainer` | No | Who maintains this template |

### .title files

Each folder can contain a `.title` file with a single line of text. This text is used as the folder's display name in the web UI. Without a `.title` file, the UI shows the raw directory name.

Example `.title` file for `04-context-of-the-organisation/`:

```
4. Context of the Organisation
```

Example `.title` file for `a.05-organizational-controls/`:

```
A.5 Organizational Controls
```

### Folder naming

Folder names should be lowercase with hyphens. Use numeric prefixes to control sort order:

```
04-context-of-the-organisation/
05-leadership/
06-planning/
07-support/
a.05-organizational-controls/
a.06-people-controls/
a.07-physical-controls/
a.08-technological-controls/
```

The numeric prefix ensures folders appear in the correct order in the UI and in file listings. The platform sorts folders alphabetically, so prefixes are the mechanism for controlling display order.

## Document frontmatter

Every markdown document in a template must start with YAML frontmatter between `---` delimiters:

```markdown
---
document_id: "iso27001-4-1"
title: "4.1 Understanding the Organisation and Its Context"
status: "draft"
version: "1.0"
author: ""
owner: ""
reviewer: ""
review_cycle: 12
---

## Purpose

TODO: Describe the purpose of this document.

## Scope

TODO: Define the scope of this requirement within your organisation.
```

### Required fields

| Field | Description |
|-------|-------------|
| `document_id` | Permanent identifier for this document. Lowercase with hyphens. Must be unique across all documents in the organization. |
| `title` | Human-readable title displayed in the UI |
| `status` | Initial status. Templates should use `draft`. Valid values: `draft`, `in_review`, `approved`, `retired` |

### Optional fields

| Field | Description |
|-------|-------------|
| `version` | Version string, e.g. `"1.0"`. Set in template, incremented by the platform on publish cycles. |
| `author` | Author email. Usually left empty in templates (filled when the org customizes). |
| `owner` | Owner email. The person responsible for periodic review. Usually left empty in templates. |
| `reviewer` | Default reviewer email. Usually left empty in templates. |
| `approved_by` | Who approved the current version. Left empty in templates. |
| `approved_date` | Date of approval. Left empty in templates. |
| `effective_date` | When this version becomes effective. Left empty in templates. |
| `next_review` | Next review date. Left empty in templates. |
| `review_cycle` | Review cycle in months (integer). Determines how often the document should be reviewed. Common values: `6`, `12`, `24`. |
| `classification` | Document classification level. |
| `changelog` | List of version history entries (see below). |

### Changelog in frontmatter

Templates can include an initial changelog entry:

```yaml
changelog:
  - version: "1.0"
    date: "2025-01-01"
    author: "template"
    description: "Initial template version"
```

The platform's `document_versions` table and `decision_log` provide the authoritative version history. The frontmatter changelog is a convenience for document-level display.

## Creating a new template

### Step 1: Create the template directory

Create a new directory under your `ISMS_TEMPLATE_PATH`:

```
mkdir -p /path/to/isms-templates/my-framework
```

### Step 2: Write meta.yaml

```yaml
id: my-framework
name: My Custom Framework
description: Internal governance framework for Acme Corp
version: "1.0"
maintainer: compliance@acme.com
```

### Step 3: Plan your folder structure

Decide on the top-level folders. These become the main navigation categories in the UI. Common patterns:

- **Standard-based**: `clauses/` + `controls/` (ISO 27001), `safeguards/` + `requirements/` (HIPAA)
- **Policy-based**: `policies/` + `procedures/` + `guidelines/`
- **Function-based**: `govern/` + `identify/` + `protect/` + `detect/` + `respond/` + `recover/` (NIST CSF)

Create folders and add `.title` files:

```
mkdir -p my-framework/policies
echo "Policies" > my-framework/policies/.title

mkdir -p my-framework/procedures
echo "Procedures" > my-framework/procedures/.title

mkdir -p my-framework/policies/01-security
echo "1. Information Security" > my-framework/policies/01-security/.title
```

### Step 4: Write documents

Create markdown files with frontmatter in each folder:

```markdown
---
document_id: "myfw-pol-001"
title: "Information Security Policy"
status: "draft"
version: "1.0"
review_cycle: 12
---

## Purpose

TODO: State the purpose of your information security policy.

## Scope

TODO: Define who and what this policy applies to.

## Policy Statements

### 1. Management Commitment

TODO: Describe top management's commitment to information security.

### 2. Roles and Responsibilities

TODO: Define key roles and their security responsibilities.

## Review

This document is reviewed annually or when significant changes occur.
```

### Step 5: Verify the template

The platform discovers templates at startup by scanning `ISMS_TEMPLATE_PATH`. To verify your template is recognized:

- Ensure `meta.yaml` exists and contains valid YAML
- Ensure at least one `.md` file exists in the template
- Check the template appears in the web UI under "Add Template" or via the API:
  ```
  GET /templates/available
  ```

## Best practices

### Document ID format

Use a consistent prefix derived from the template ID, followed by a meaningful identifier:

```
iso27001-4-1          # ISO 27001 clause 4.1
iso27001-a-5-1        # ISO 27001 control A.5.1
hipaa-2-1             # HIPAA section 2.1
nistcsf-gv-1          # NIST CSF Govern subcategory 1
myfw-pol-001          # Custom framework policy 001
```

Rules:
- **Lowercase only.** Document IDs are normalized to lowercase on read.
- **Hyphens for separators.** No underscores, no dots, no spaces.
- **Unique across the organization.** Since multiple templates can coexist, prefix with the template/framework ID to avoid collisions.

### File naming

Name markdown files to reflect their content and sort correctly:

```
4.01-external-and-internal-issues.md
4.02-interested-parties.md
a.5.01-policies-for-information-security.md
```

Use zero-padded numbers to ensure correct alphabetical sorting (e.g. `01-` not `1-`).

### TODO placeholders

Use `TODO:` markers in template content to indicate where organizations need to fill in their own information:

```markdown
TODO: List your organization's key interested parties and their requirements.

TODO: Define your risk acceptance criteria.

TODO: Describe your internal audit programme schedule.
```

This makes it clear what needs customization after scaffolding. Teams can search for `TODO` to find all outstanding items.

### Review cycles

Set appropriate `review_cycle` values based on the document type:

- **High-change documents** (risk register summary, incident response plan): `6` months
- **Standard policies**: `12` months
- **Stable foundational documents** (scope, context): `12` to `24` months

The platform uses review cycles to generate overdue review tasks and notifications.

### Status

All template documents should start with `status: "draft"`. The organization moves documents through the lifecycle (`draft` -> `in_review` -> `approved`) as they customize and review each one.

### Keep templates focused

A template should cover one standard or framework. Do not try to merge ISO 27001 and HIPAA into a single template. Organizations that need both can scaffold both templates -- they coexist as separate folder trees under `documents/`.

## Scaffolding

When an organization adds a template through the web UI or API, the scaffolding process:

1. Reads all files from the template directory on disk
2. Copies each file into the organization's git repository under `documents/<template-id>/`
3. Skips `meta.yaml` (it is template metadata, not a document)
4. Commits all files in a single git commit: `chore: scaffold <template> template (N files)`

After scaffolding:

- The organization owns all the content. Editing, deleting, or reorganizing documents has no effect on the template.
- The template is not referenced again. Future template updates do not automatically propagate to existing organizations.
- Documents retain their `document_id` values from the template. These are the permanent identifiers used throughout the platform.

To scaffold via the API:

```
POST /templates
{
  "template": "iso27001"
}
```

To scaffold via the CLI:

```
isms init --template iso27001
```

## Multi-standard

An organization can run multiple templates simultaneously. Each template scaffolds into its own folder:

```
documents/
├── iso27001/
│   ├── clauses/
│   └── controls/
├── iso14001/
│   ├── clauses/
│   └── controls/
└── nist-csf/
    ├── functions/
    └── subcategories/
```

The web UI shows all document folders in the sidebar. The core engine treats them identically -- they are all just folders of documents with frontmatter.

Cross-referencing between templates works through the entity reference system. A risk can link to documents from any template. A legal requirement can reference both an ISO 27001 control and a HIPAA safeguard.

Considerations for multi-standard deployments:

- **Document IDs must be unique across all templates.** Use template-specific prefixes (e.g. `iso27001-4-1`, `iso14001-4-1`).
- **Folder names are independent.** Both ISO 27001 and ISO 14001 can have a `clauses/` folder because they live under different template directories.
- **Review cycles and ownership are per-document.** Different standards can have different review schedules.
- **Export works per-folder or across all.** The `isms export documents --folder iso27001` command exports one template's documents; `isms export manual` exports everything.
