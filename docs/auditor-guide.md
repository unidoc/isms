# Auditor Guide

This guide is for external auditors, compliance officers, and anyone reviewing the integrity and governance controls of an isms.sh deployment.

## What is isms.sh

isms.sh is a document management and compliance platform for running information security management systems. Documents are stored as markdown files in git repositories, with collaboration features (reviews, approvals, comments, tasks) tracked in PostgreSQL.

The platform supports any compliance framework through templates. ISO 27001, HIPAA, NIST CSF, SOC 2, and custom frameworks all use the same underlying engine. The core has no knowledge of specific standards -- it manages documents, reviews, and operational registers generically.

Organizations interact with the system through a web interface, a CLI, and a REST API.

## Document versioning

Every document is a markdown file with YAML frontmatter stored in a per-organization git repository. Git provides the raw edit history: every save creates a commit, forming a complete log of all changes to every document.

On top of git, the platform maintains a `document_versions` table in PostgreSQL that records **official milestones only**. A new version entry is created when:

- A review is **merged** (approved changes are published)
- A document review is **confirmed** (annual or periodic review with no changes)

Draft edits do not create version entries. Saving a document while editing it creates git commits, but those are working state. The version history shows only the points where a document was officially published or confirmed.

Each version record captures:

| Field | Description |
|-------|-------------|
| `document_id` | The document's permanent identifier |
| `version` | Version string (e.g. "1.0", "2.0") |
| `commit_hash` | The exact git commit for this version |
| `file_path` | Path to the document in the repository |
| `content_hash` | SHA-256 hash of the file content at this version |
| `message` | Description of what changed |
| `owner` | Document owner at time of version (snapshot) |
| `review_cycle_months` | Review cycle at time of version (snapshot) |
| `created_by` | Who published or confirmed this version |
| `created_at` | Timestamp |

Version numbers increment when an approved document enters a new draft cycle (status transitions from `approved` to `draft`). This means the version number reflects meaningful governance milestones, not incremental edits.

## Review workflow

Documents go through a formal review process before publication. The review workflow uses **rounds** to track iterations between author and reviewers.

### Sending for review

An author sends a document for review by specifying one or more reviewers and a message describing what changed. The system creates a review record and assigns the reviewers. The document's status changes to `in_review`.

### Reviewer actions

Each assigned reviewer can take one of three actions:

- **Approve** -- the reviewer accepts the document as-is
- **Request changes** -- the reviewer asks the author to address specific issues
- **Propose revision** -- the reviewer edits the document directly and submits their version

Reviewers are locked after acting in a round. They cannot change their decision within the same round, preventing accidental double-approval.

### Rounds

When a reviewer requests changes, the review enters a `changes_requested` state. The author addresses the feedback and resubmits, which increments the round counter and resets all reviewer assignments to `pending` for the new round.

The review tracks:
- Current round number
- Per-reviewer status for the current round
- What changed since the last round ("This round" diff)
- What changed since the original ("All changes" diff)

### Merging

Once all required reviewers have approved, the review can be merged. Merging publishes the document: the approved content replaces the main version, frontmatter is updated (status, approved_by, version), and a new entry is written to both the `document_versions` table and the `decision_log`.

### Review status transitions

The review status follows a strict state machine:

```
open --> approved (via approve endpoint)
open --> changes_requested (via approve endpoint with changes_requested decision)
changes_requested --> open (via resubmit)
approved --> merged (via merge endpoint)
Any active status --> closed (via status endpoint)
```

Each transition goes through its dedicated handler. There is no generic status update endpoint that can bypass the state machine.

### Inline comments and suggestions

Reviewers can post comments on specific paragraphs of a document. Comments can include an inline suggestion with replacement text, which the author can accept or reject individually. Each comment tracks:

- The paragraph it references (by index and content hash)
- Whether it is open or resolved
- Who resolved it and when
- The suggestion text and its accept/reject status

## Decision log

The `decision_log` table is an immutable audit trail of governance decisions. A new record is created whenever:

- A reviewer **approves** a document
- A reviewer **requests changes**
- A reviewer **proposes a revision**
- A review is **merged** (document published)
- A review is **closed**
- A document is **confirmed** (periodic review with no changes)

Each decision record contains:

| Field | Description |
|-------|-------------|
| `review_id` | The review this decision belongs to (NULL for confirmations) |
| `document_id` | The document's permanent identifier |
| `decision` | One of: `approved`, `changes_requested`, `proposed_revision`, `merged`, `closed`, `confirmed` |
| `decided_by` | Email of the person who made the decision |
| `commit_ref` | Git commit hash at the time of the decision |
| `version` | Document version string |
| `comment` | Any comment provided with the decision |
| `content_hash` | SHA-256 hash of the document content at decision time |
| `created_at` | Timestamp |

Decision records are append-only. They cannot be updated or deleted. The `content_hash` field is computed from the raw file bytes using SHA-256, providing a tamper-evident seal: if the document content were modified after the fact, the hash would no longer match.

## Version history

The `document_versions` table provides a clean timeline of official document milestones. It is separate from the decision log and serves a different purpose: the decision log records every governance action (approvals, rejections, merges), while the version history records published states of the document.

A new version is created only when:

1. A review is **merged** -- the approved content becomes the new official version
2. A document review is **confirmed** -- the existing content is re-affirmed without changes

Draft saves, comments, and intermediate review actions do not trigger new versions.

Each version snapshots the document's owner and review cycle at that point in time, so you can see what the governance parameters were when the version was published.

To view version history for a document, use the Version History panel in the web UI (clock icon) or query the API:

```
GET /documents/{docId}/versions
```

## Audit trail

The platform maintains three layers of audit history:

### 1. Git commit log

Every document edit is a git commit. The commit log shows who changed what, when, with the full diff. This is the raw working history.

### 2. Entity changelog

The `entity_changelog` table tracks field-level changes to all operational entities (risks, incidents, suppliers, assets, systems, legal requirements, changes, corrective actions, objectives, tasks, audit findings). Each entry records:

- Entity type and ID
- Action (`create`, `update`, `delete`)
- Field name (for updates)
- Old value and new value
- Who made the change (email snapshot)
- Which API key was used (if applicable, NULL for browser sessions)
- Optional reason for the change
- Timestamp

This provides a complete diff-level trail for every register mutation. You can query it per-entity to see the full history of how a risk score evolved, when a supplier was reassessed, or who changed an incident severity.

### 3. Activity log

The `activity` table logs high-level actions on documents and reviews:

- Document edits, sends, approvals, merges
- Review comments, status changes
- Actor identity (email + user ID)
- Timestamp

The activity log and decision log together provide a complete governance trail for document lifecycle events.

## Evidence

Evidence files are stored in a pluggable blob store (local filesystem or S3-compatible object storage like Cloudflare R2). They are attached to objective check-ins, which represent periodic progress measurements against management objectives.

Each evidence file is tracked in the `checkin_evidence` table with:

| Field | Description |
|-------|-------------|
| `checkin_id` | The objective check-in this evidence belongs to |
| `title` | Human-readable name |
| `object_key` | Storage path: `{org-uuid}/evidence/{file-uuid}.ext` |
| `content_type` | MIME type |
| `size_bytes` | File size |
| `sha256` | SHA-256 checksum of the file contents |
| `created_at` | Upload timestamp |

The SHA-256 checksum is computed at upload time. To verify evidence integrity, download the file and compare its SHA-256 hash against the stored value.

Evidence files are scoped to organizations. Storage keys include the organization's UUID, ensuring tenant isolation. When using S3-compatible storage, presigned URLs provide time-limited download access.

## Exporting data

### CLI export

The `isms export` command provides structured data export:

```
isms export policy <doc-id>       # Single document
isms export documents             # All documents
isms export documents --folder X  # Documents in a folder
isms export manual                # Full ISMS manual with TOC
isms export risks                 # Risk register
isms export assets                # Asset register
isms export suppliers             # Supplier register
isms export audit-pack <id>       # Audit report with findings
```

Export currently produces markdown. PDF and DOCX export (via UniDoc) is planned.

### API access

All data is accessible through the REST API with Bearer token authentication. Key endpoints for bulk export:

```
GET /documents/all                 # Full document tree
GET /documents/{docId}/body        # Single document content
GET /documents/{docId}/versions    # Version history
GET /documents/{docId}/decisions   # Decision log
GET /risks                         # Risk register
GET /assets                        # Asset register
GET /suppliers                     # Supplier register
GET /incidents                     # Incident register
GET /legal                         # Legal requirements
GET /changelog                     # Entity changelog
GET /activity/{docId}              # Activity log for a document
```

API tokens are created per-user and carry the user's organizational role and permissions.

## Roles and permissions

The platform has four roles, in descending order of privilege:

| Role | Documents | Reviews | Registers | Suggestions | Admin |
|------|-----------|---------|-----------|-------------|-------|
| **Admin** | Read, write | Send, approve, merge | Full CRUD | Apply, reject | Full org management |
| **Manager** | Read, write | Send, approve, merge | Full CRUD | Apply, reject | -- |
| **Contributor** | Read, write | Send, comment | Full CRUD | Create, edit own | -- |
| **Reader** | Read only | Comment (when assigned) | Read only | Create | -- |

Important details:

- **Review assignment grants review rights to any role.** A reader assigned as a reviewer can approve or request changes on that specific review.
- Roles are per-organization. A user can be admin in one org and reader in another.
- API tokens inherit the permissions of the user who created them.

## AI governance

The platform has first-class support for AI agent users, with explicit identity and policy controls.

### Agent identity

- Users have an `is_agent` boolean flag, set at creation time
- Agent users are visually identified in the UI
- Suggestions created by agents carry `suggested_by_type: agent`
- All agent actions appear in the audit trail with full attribution
- Agents cannot be hidden behind human accounts

### Suggestions

The suggestion system is the primary write mechanism for AI. Agents propose changes through structured suggestions (create, update, reassess entities). Suggestions are reviewed and applied by human managers or admins. Every suggestion records who created it, whether the creator is human or agent, and the full rationale.

### Kill switch

Organizations have an `ai_enabled` setting. When disabled, all API tokens belonging to agent users are blocked. This provides an immediate, org-wide kill switch for all AI activity.

### Approval policies

The `approval_policies` table defines rules per document path pattern:

| Field | Description |
|-------|-------------|
| `path_pattern` | Folder/path prefix to match (e.g. `iso27001/policies`, `*` for all) |
| `min_approvals` | Minimum number of approvals required |
| `required_roles` | Roles that must approve (e.g. manager, admin) |
| `required_users` | Specific users who must approve |
| `require_human` | At least one human must approve (true by default) |
| `auto_merge` | Automatically merge when all required approvals are met |

The `require_human` flag ensures that even when agents participate in review, a human decision-maker is always in the loop for governance-critical documents.

## Verifying integrity

### Verifying content hashes on decision records

Every decision record and document version includes a `content_hash` field containing the SHA-256 hash of the raw document file at the time of the decision. To verify:

1. Retrieve the decision record (via API or database)
2. Note the `commit_ref` (git commit hash) and `content_hash`
3. Check out the document at that commit:
   ```
   git show <commit_ref>:documents/<path>
   ```
4. Compute the SHA-256 hash of the file contents:
   ```
   git show <commit_ref>:documents/<path> | sha256sum
   ```
5. Compare the computed hash with the `content_hash` in the decision record

If they match, the document content has not been altered since the decision was recorded.

### Verifying the decision log

The decision log is append-only in PostgreSQL. Records cannot be updated or deleted through the application. To verify completeness:

1. Query all decision records for a document:
   ```
   GET /documents/{docId}/decisions
   ```
2. Verify that the timeline is consistent: approvals precede merges, rounds are sequential, timestamps are monotonically increasing per review
3. Cross-reference with the `document_versions` table: each merged review should have a corresponding version entry with matching content hash

### Verifying evidence checksums

Evidence files store a SHA-256 hash at upload time. To verify:

1. Download the evidence file
2. Compute `sha256sum` on the downloaded file
3. Compare with the `sha256` field in the `checkin_evidence` record

### Verifying git history

Each organization has its own git repository. The full commit history is available and can be cloned or inspected. Git's content-addressable storage provides inherent integrity: any modification to historical commits would change all subsequent commit hashes.

For signed commits (when SSH signing is configured), verify signatures using standard git tooling:

```
git log --show-signature
```
