# AI-First Strategy

## Core Principle

The platform is **AI-first in draft space** and **human-approved in official space**.

- AI may read all operational data, documents, and history
- AI may create suggestions for any operational change
- AI may comment on and review documents inline
- AI does **not** become the source of accepted truth by itself
- Official state flows through review, approval, and publish decisions

## What Is Built

### Suggestions (Core Primitive)

Entity suggestions are a first-class generic primitive. One `suggestions` table serves all modules.

AI creates suggestions → humans review and apply → entity mutations happen atomically.

22 apply handlers across 11 entity types:
- risks (create, reassess, update)
- incidents (create, update, link)
- suppliers (create, update, reassess)
- legal requirements (create, update)
- change requests (create, update)
- corrective actions (create, update)
- tasks (create, update)
- objectives (update)
- systems (update)
- assets (update)
- audit findings (create, update)

All apply handlers run inside `WithOrgTx` — entity mutation + changelog + suggestion mark in one transaction with RLS.

### MCP Server (Core)

`isms server mcp` runs an MCP server on stdio. 22 tools:

**Read (8 tools):**
- `get_isms_overview` — full system status: counts, overdue, open reviews, open suggestions
- `get_overdue_items` — everything past its review cycle
- `list_entities` — list any entity type with optional status filter
- `get_entity` — single entity with full details
- `get_entity_history` — changelog / audit trail
- `get_entity_links` — cross-references
- `list_documents` — all documents with metadata
- `get_document` — full markdown body + frontmatter

**Suggestions (3 tools):**
- `list_suggestions` — filter by status, entity type, entity ID
- `create_suggestion` — propose operational changes (auto-sets `suggested_by_type: agent`)
- `reject_suggestion` — reject with required reason

**Document review (9 tools):**
- `list_reviews` — reviews with status filter
- `get_review` — review details, assignments, round
- `get_review_diff` — what changed in the document
- `get_review_content` — current document on review branch
- `comment_on_review` — paragraph-level comments with optional `suggestion_body` for inline edits
- `approve_review` — approve, request changes, or propose revision
- `merge_review` — publish the approved document version (final step)
- `edit_review_content` — write to review branch (address reviewer comments)
- `confirm_document_review` — lightweight annual review confirmation with audit evidence

**Apply + workflow (2 tools):**
- `apply_suggestion` — atomically apply with optional force for stale entities
- `get_pending_actions` — actor-scoped: what should this agent do next

### Agent Identity

Agent users are first-class. Every AI action carries:
- `suggested_by_type: agent` on suggestions
- Author identity on comments and reviews
- Full audit trail in activity log and entity changelog
- UI shows AI badge on agent-authored content

### Document Review

AI agents participate in document review using the same workflow as humans:
- Read document content on review branch
- Post inline comments on specific paragraphs
- Suggest replacement text (paragraph-level inline suggestions)
- Approve, request changes, or propose revision
- Multi-round review with round tracking

### Stale Detection

When an entity changes after a suggestion is created, apply shows a stale warning with the changelog entries that occurred since. Reviewer can force-apply, reject, or edit.

## Three-Layer Model

### 1. Working State

AI operates freely:
- Create suggestions for new risks, incidents, suppliers, legal requirements
- Update and reassess existing entities via suggestions
- Comment on document reviews
- Link entities together

### 2. Suggested State

AI proposals await human review:
- Entity suggestions with rationale and source references
- Inline document suggestions with replacement text
- Proposed revisions on document reviews

### 3. Official State

Human approval remains the trust boundary:
- Published document versions
- Approved review decisions
- Applied suggestions (manager/admin only)
- Closed incidents, implemented changes, resolved corrective actions

## Setup

### 1. Create agent user

```bash
isms server user create --email ai@company.com --name "Claude Agent" --role contributor
isms server api-key create --email ai@company.com --name "mcp" --permissions "read,write"
```

### 2. Configure Claude Code

In `.claude/settings.json` or project `.claude/settings.json`:

```json
{
  "mcpServers": {
    "isms": {
      "command": "isms",
      "args": ["server", "mcp"],
      "env": {
        "ISMS_API_URL": "https://isms.example.com",
        "ISMS_API_TOKEN": "tok_..."
      }
    }
  }
}
```

### 3. Use

Tell Claude what to do:

- "Review the risk register and suggest any reassessments based on recent incidents"
- "Read the supplier management policy and comment on the open review"
- "Create a new risk for the SSO provider outage we had last week"
- "What is overdue in our ISMS right now?"
- "Suggest a corrective action for FINDING-3"

Claude uses MCP tools to read, suggest, comment, and review. Humans apply suggestions and approve reviews in the web UI.

## Multi-Model Review (Enterprise)

For higher confidence:

1. Model A reviews document and creates inline suggestions
2. Model B reviews Model A's suggestions and endorses or challenges
3. Human reviewer sees both perspectives and decides

This works because each agent has its own identity — the UI shows who wrote what.

## Module Coverage

| Module | AI Can Read | AI Can Suggest | AI Can Review |
|--------|:-----------:|:--------------:|:-------------:|
| Documents | Y | Y (via review) | Y (comments, approve) |
| Risks | Y | Y (create, reassess, update) | — |
| Incidents | Y | Y (create, update, link) | — |
| Suppliers | Y | Y (create, update, reassess) | — |
| Legal | Y | Y (create, update) | — |
| Changes | Y | Y (create, update) | — |
| Corrective Actions | Y | Y (create, update) | — |
| Systems | Y | Y (update) | — |
| Objectives | Y | Y (update) | — |
| Tasks | Y | Y (create, update) | — |
| Audit | Y | Y (finding create, update) | — |

## What Is Not Built Yet

### Enterprise Features
- Inbound email intake (email → draft suggestions)
- Event-driven triggers (incident created → auto-suggest risks)
- Multi-model orchestration (automated model A → model B pipeline)
- Cost controls and AI observability
- Batch suggestion review

### Test Coverage
- Entity suggestion + MCP integration tests
- Auto-merge / require_human policy tests

## Design Rules

- MCP is core, not enterprise. Every self-hosted instance gets AI tools.
- Suggestions are the universal write primitive for AI. No hidden mutation paths.
- Agent identity is always explicit. Never hide AI behind a human account.
- Official state requires human approval. AI proposes, humans decide.
- The same tools work for readers, contributors, and AI. No separate AI workflow.
