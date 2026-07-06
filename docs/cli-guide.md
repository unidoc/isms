# ISMS CLI Guide

## Why a CLI

ISMS keeps its documents in **git** (markdown + YAML frontmatter) and its
collaboration state (reviews, registers, tasks, approvals) in PostgreSQL. The web
UI is where readers and managers work day to day. The CLI exists for the things a
browser is bad at:

- **Bulk and raw-markdown work.** The web editor is deliberately WYSIWYG-only.
  When you need to touch many documents at once, script edits, or work in your own
  editor, you `isms clone` the repo, edit the markdown directly, and `isms sync`.
- **Managers and automation.** Registers (risks, assets, suppliers, incidents,
  changes, …) and the review workflow are scriptable from the terminal and CI.
- **Operations.** Server-side user/org/token administration and diagnostics.

**How `sync` relates to governance.** A push lands on the repo's `main` ref and
is served to readers **immediately** — it does not sit in a draft/pending state,
and the push path does not enforce frontmatter `status` (you can edit an
`approved` document's body directly). This is exactly why pushing is restricted to
**managers/admins**. Changes made to a document since its last approval are
surfaced after the fact by `GET /documents/needs-review` (and the web UI), so
reviewers can catch them — but the CLI is a trusted, high-privilege path, not a
gated one. Use it accordingly.

## Setup

The CLI talks to the server over HTTP. Point it at your deployment and give it a
token:

```sh
export ISMS_API_URL=https://your-org.isms.sh/api   # or ISMS_BASE_URL=https://your-org.isms.sh
export ISMS_API_TOKEN=<api-token>                  # or ISMS_API_KEY
export ISMS_ORGANIZATION=<org-slug>                # if your token spans multiple orgs
```

Create a token from the server side with `isms server api-key create` (see
[Server administration](#server-administration)). For Cloudflare Access–fronted
deployments, set `CF_ACCESS_CLIENT_ID` / `CF_ACCESS_CLIENT_SECRET` as well.

Verify the connection:

```sh
isms whoami
```

## The clone → edit → sync workflow

```sh
isms clone ./my-isms      # git-clone the org's document repo
cd my-isms
$EDITOR documents/iso27001/a-5-1.md   # edit markdown directly, in bulk if you like
isms sync                  # push local commits to the server's main ref
```

`isms clone` gives you the whole document tree as plain markdown — ideal for
find-and-replace across many files, migrations, or drafting in your own tools.
`isms sync` pushes your commits to the server's `main` ref; readers see the
content **immediately** (push is manager/admin-only for this reason). Documents
changed since their last approval show up in the needs-review view so reviewers
can follow up — the push itself is not gated on review.

Use `isms diff` to preview what a document change looks like before syncing, and
`isms status` to see what's outstanding.

## Command reference

Run `isms <command> --help` for full flags on any command.

### Documents & git
| Command | Purpose |
|---|---|
| `isms clone [dir]` | Clone the org's document repo locally |
| `isms sync` | Push local commits to `main` (live immediately; manager/admin-only) |
| `isms document list\|show\|cat` | List/inspect documents |
| `isms diff` | Show a document diff |
| `isms export` | Export documents |

### Registers
| Command | Purpose |
|---|---|
| `isms risk` | Risk register (add, list, assess, treat, matrix) |
| `isms asset` | Asset register |
| `isms supplier` | Supplier register |
| `isms system` | Systems register |
| `isms incident` | Incident register |
| `isms legal` | Legal & regulatory requirements |
| `isms objective` / `isms checkin` | Objectives and their check-ins |
| `isms program` | Programmes |

### Change & corrective management
| Command | Purpose |
|---|---|
| `isms change list\|show\|create\|status` | Change requests (raise, inspect, transition status) |
| `isms corrective` | Corrective actions |
| `isms audit` | Audit programmes & findings |

### Review workflow
| Command | Purpose |
|---|---|
| `isms review` | Send for review, approve, merge |
| `isms inbox` | Items awaiting your action |
| `isms overdue` | Overdue reviews |
| `isms status` | Outstanding work overview |

### Terminal UI
| Command | Purpose |
|---|---|
| `isms tui` | Read-only terminal document browser/reader |

### Meta
| Command | Purpose |
|---|---|
| `isms whoami` | Show current user, verify API connection |
| `isms version` | Version info |

### Server administration
Run on the server host (`isms server …`, uses the server's env / `DATABASE_URL`):

| Command | Purpose |
|---|---|
| `isms server user create\|list\|set-password\|verify` | Manage users |
| `isms server user test-auth` | Read-only login credential check (diagnostics) |
| `isms server user reset-otp` | Clear a user's 2FA so they can re-enroll |
| `isms server org …` | Manage organizations and members |
| `isms server api-key create` | Mint CLI/automation tokens |
| `isms server test-email [--org <slug>]` | Verify SMTP; `--org` previews a tenant's branded From |

## Module coverage

**Scriptable today:** documents, risks, assets, suppliers, systems, incidents,
legal requirements, objectives (+check-ins), programmes, **change requests**,
corrective actions, and audit programmes/findings. Reviews, inbox, overdue, and
status cover the approval workflow.

**Known gaps (API exists, no CLI yet):**

| Area | Status |
|---|---|
| Suggestions (create / apply / reject) | API only — apply governs the contributor/agent write path; no `isms suggestion` command |
| Entity comments & @-mentions | API only |
| Cross-entity references / tags | API only |
| Notifications (list / mark read) | API only |
| Register `update` beyond the fields each command exposes | partial — some commands cover a subset of fields |

If you need one of these scriptable, open an issue.
