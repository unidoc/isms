# Suggestions

## Purpose

Suggestions are a first-class core primitive.

They are not an AI-only feature.
They are not a module-specific hack.
They are the generic mechanism for proposing change without directly mutating the source record.

That means suggestions can be created by:

- readers
- contributors
- managers
- admins
- agent users / AI accounts

The goal is simple:

- direct edits remain available where appropriate
- suggestions provide a safe path for lower-permission users and AI
- reviewable change proposals become part of the audit trail

---

## Core Principle

Do not build a separate suggestion system for every module.

Do not create:

- `risk_suggestions`
- `supplier_suggestions`
- `legal_suggestions`
- `incident_suggestions`
- etc.

Instead, create one generic `suggestions` table in core.

Modules then define:

- how suggestions are displayed
- how payload is validated
- how `apply` becomes a real change in that module

This keeps the architecture coherent and lets Enterprise reuse the same primitive for AI, agents, and MCP.

---

## What A Suggestion Is

A suggestion is a structured proposal to:

- create something new
- update an existing entity
- reassess an existing entity
- link entities together
- request review or follow-up

A suggestion is not official truth.
It becomes official only if a user with sufficient authority applies it.

---

## Suggestion Types

Generic types:

- `create`
- `update`
- `reassess`
- `link`
- `review`

Examples:

- `create` + `risk`: suggest a new risk
- `create` + `supplier`: suggest a new supplier
- `create` + `incident`: suggest a new incident
- `update` + `supplier`: suggest changing supplier notes, owner, status, review date
- `reassess` + `risk`: suggest new likelihood/impact/level with rationale
- `link` + `incident`: suggest linking an incident to a system, supplier, risk, or document
- `review` + `document`: suggest sending a document for review or revisiting a document after an event

The system should be able to support suggestions for effectively all operational entities.

That includes:

- incidents
- risks
- suppliers
- legal requirements
- change requests
- corrective actions
- objectives
- tasks
- documents

The module decides what `apply` means.
The suggestion model stays generic.

---

## Data Model

Table: `suggestions`

```sql
CREATE TABLE suggestions (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    organization_id   INTEGER NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    entity_type       TEXT NOT NULL,
    entity_id         TEXT,
    suggestion_type   TEXT NOT NULL CHECK (suggestion_type IN ('create','update','reassess','link','review')),
    title             TEXT NOT NULL,
    payload           JSONB NOT NULL DEFAULT '{}',
    rationale         TEXT,
    source_refs       JSONB,
    entity_updated_at TIMESTAMPTZ,            -- snapshot for stale detection (null for create)
    status            TEXT NOT NULL DEFAULT 'open'
                      CHECK (status IN ('open','in_review','applied','rejected','withdrawn')),
    suggested_by      TEXT NOT NULL,
    suggested_by_user_id INTEGER REFERENCES users(id),
    suggested_by_type TEXT NOT NULL DEFAULT 'user' CHECK (suggested_by_type IN ('user','agent')),
    reviewed_by       TEXT,
    reviewed_by_user_id INTEGER REFERENCES users(id),
    reviewed_at       TIMESTAMPTZ,
    applied_at        TIMESTAMPTZ,
    applied_entity_id TEXT,                   -- ID of entity created/updated by apply (closes audit loop)
    reject_reason     TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_suggestions_org ON suggestions(organization_id);
CREATE INDEX idx_suggestions_entity ON suggestions(organization_id, entity_type, entity_id);
CREATE INDEX idx_suggestions_status ON suggestions(organization_id, status);
```

Notes:

- `entity_type` is the target module type: `risk`, `supplier`, `legal_requirement`, `incident`, `change_request`, `corrective_action`, `objective`, `task`, `document`
- `entity_id` is nullable because a `create` suggestion has no target yet
- `payload` contains the structured proposed change and stays module-specific
- `source_refs` contains evidence or linked context used to support the suggestion
- `suggested_by_type` distinguishes human from agent
- `reject_reason` is required when a suggestion is rejected

---

## Lifecycle

```text
open ──> in_review ──> applied
  │          │
  │          └──> rejected
  │
  └──> withdrawn
```

States:

| State | Meaning | Who can transition |
|-------|---------|-------------------|
| `open` | Submitted, awaiting review | Created automatically |
| `in_review` | A reviewer has claimed it | Manager, admin |
| `applied` | Applied to the real entity | Manager, admin |
| `rejected` | Declined with reason | Manager, admin |
| `withdrawn` | Author pulled it back | Original author |

Rules:

- `open` -> `in_review`: optional claim, not a required workflow step
- `open` or `in_review` -> `applied`: reviewer applies the change atomically
- `open` or `in_review` -> `rejected`: reviewer rejects and must provide `reject_reason`
- `open` -> `withdrawn`: author retracts before review
- `in_review` -> `open`: reviewer unclaims and returns it to the queue

Terminal states:

- `applied`
- `rejected`
- `withdrawn`

### Apply Semantics

When a suggestion is applied:

1. Validate the payload against the target module
2. Create or update the target entity
3. Write entity changelog entries, including suggestion ID reference
4. Mark the suggestion `applied`
5. Record `applied_at`, `reviewed_by`, `reviewed_at`, `applied_entity_id`
6. Send notification to the original suggester

This is one atomic operation.

There is no separate `accepted` state.
If the reviewer wants to adjust the payload before applying it, they edit the suggestion first, then apply it.

### Stale Detection

A suggestion can become stale if the target entity changes after the suggestion was created.

The schema stores `entity_updated_at` — a snapshot of the target entity's `updated_at` at suggestion creation time. Null for `create` suggestions that have no target yet.

At apply time:

1. Load the current entity
2. Compare its `updated_at` against the suggestion's `entity_updated_at`
3. If the entity has changed since the suggestion was created:
   - Show a stale warning to the reviewer with what fields changed
   - Reviewer can **apply anyway** (force), **reject as stale**, or **edit the suggestion** to account for changes, then apply

This is a safety check, not an automatic rejection. The reviewer decides.

The diff comes from the existing `entity_changelog` table — query changelog entries for the target entity where `created_at > suggestion.created_at`. This shows exactly which fields changed, who changed them, and when. No separate snapshot needed.

The UI should show a yellow banner ("Entity changed since this was suggested") with the changelog entries that occurred after the suggestion was created.

### Delete Semantics

Suggestions can be deleted only in non-final states.

| Status | Who can delete |
|--------|---------------|
| `open` | Author, manager, admin |
| `in_review` | Manager, admin |
| `withdrawn` | Author, manager, admin |
| `applied` | Nobody |
| `rejected` | Nobody |

Delete is hard delete for `open` and `withdrawn` suggestions.
Applied and rejected suggestions are permanent audit records.

---

## Payload Shape

`payload` is module-specific JSONB inside one generic container.

Examples:

### New Risk

```json
{
  "title": "Key supplier lacks formal offboarding process",
  "description": "Supplier retains privileged access after contract end.",
  "category": "supplier",
  "current_likelihood": 3,
  "current_impact": 4,
  "treatment_plan": "Add contract offboarding check and quarterly access review."
}
```

### Risk Reassessment

```json
{
  "current_likelihood": 2,
  "current_impact": 5,
  "reason": "Recent incident shows lower likelihood but severe impact remains."
}
```

### New Supplier

```json
{
  "name": "Example Vendor Ltd",
  "service_type": "email delivery",
  "owner": "ops@example.com",
  "risk_level": "medium",
  "notes": "Processes operational email and support notifications."
}
```

### New Incident

```json
{
  "title": "Suspicious admin login from foreign IP",
  "summary": "Inbound alert from email gateway and SSO logs.",
  "severity": "high",
  "affected_systems": ["SYSTEM-12"],
  "impact_description": "Potential privileged account compromise."
}
```

### Link Suggestion

```json
{
  "links": [
    { "type": "risk", "id": "RISK-14" },
    { "type": "document", "id": "supplier-access-review" }
  ]
}
```

### Update Fields

```json
{
  "fields": {
    "owner": "new-owner@example.com",
    "next_review": "2027-01-15",
    "notes": "Updated after quarterly reassessment."
  }
}
```

---

## Source References

A suggestion should be explainable.

`source_refs` points at the context used to make the suggestion:

```json
[
  { "type": "incident", "id": "INC-12" },
  { "type": "audit_finding", "id": "FINDING-5" },
  { "type": "document", "id": "supplier-management-policy" }
]
```

The reviewer should be able to answer:

- why was this suggested?
- what evidence supports it?
- what records does it relate to?

---

## Roles And Permissions

### Reader

- Create suggestions
- View suggestions visible to them
- Withdraw their own open suggestions
- Delete their own open suggestions

### Contributor

- Everything reader can do
- Edit their own open suggestions (`payload`, `rationale`, `source_refs`)

### Manager / Admin

- Everything contributor can do
- Transition any suggestion (`in_review`, `apply`, `reject`)
- Edit any open or in_review suggestion before applying
- Delete open, in_review, or withdrawn suggestions

### Agent User

- Create suggestions according to token scope
- `suggested_by_type` must be `agent`
- Never hide AI behind a human identity

---

## Notifications

Notifications are required for suggestions to work in practice.

| Event | Notify |
|-------|--------|
| Suggestion created | Entity owner if known, managers |
| Suggestion moved to in_review | Original suggester |
| Suggestion applied | Original suggester |
| Suggestion rejected | Original suggester, with reason |
| Suggestion withdrawn | Optional, owner/managers if relevant |

Without notifications, suggestions accumulate unseen.

---

## UI Model

Suggestions are visible in two places.

### 1. In Context

Each module detail view shows suggestions relevant to the current entity.

Examples:

- risk detail shows reassessment suggestions
- supplier detail shows update or reassessment suggestions
- incident detail shows suggested linked risks, corrective actions, and document reviews
- change request detail shows suggested follow-up or reassessment

Render as a collapsible section in the entity detail, below activity history.

### 2. Central Queue

A filterable list of all suggestions for the organization.

Filters:

- status: `open`, `in_review`, `applied`, `rejected`
- `mine`
- `awaiting review`
- `from AI`
- entity type
- suggestion type

This becomes the review inbox for proposed operational change.

---

## Module Apply Rules

Modules do not need separate suggestion tables.
They need module-specific apply logic.

Each module registers an apply handler that:

1. Receives the suggestion payload
2. Validates required fields
3. Creates or updates the entity
4. Returns the resulting entity ID for changelog linkage

### Risks

- `create`: create new risk from payload fields
- `reassess`: update likelihood, impact, level, treatment notes
- `update`: update owner, status, treatment, notes

### Suppliers

- `create`: create new supplier
- `reassess`: update criticality, risk level, review date
- `update`: update owner, notes, status, assessment state

### Legal

- `create`: create new legal requirement
- `update`: update compliance status, owner, notes

### Incidents

- `create`: create new incident from payload
- `update`: update severity, summary, impact, assignee
- `link`: create entity references from payload links

### Changes

- `create`: create new change request
- `update`: update risk level, priority, rollback plan

### Corrective Actions

- `create`: create new corrective action
- `update`: update assignee, status, evidence notes

### Objectives

- `create`: create new objective if desired later
- `update`: update target, description, owner, measurement method
- `reassess`: suggest new check-in interpretation

### Tasks

- `create`: create a task
- `update`: update assignee, due date, priority, type, title

### Documents

Documents already have native review primitives:

- comments
- inline suggestions
- proposed revision
- review rounds

Do not replace those.

Generic suggestions for documents are limited to higher-level triggers such as:

- `review`: suggest sending a document for review
- `update`: suggest a document needs updating because of an incident, finding, supplier change, or legal change

The actual content-edit flow continues to use the document review system.

---

## Relation To Tasks

Suggestions and tasks are different things.

- a suggestion proposes change
- a task assigns work

Sometimes applying a suggestion may also create a task.
But they remain separate objects with separate lifecycles.

---

## Relation To Document Review Cycle

Recurring document review remains simple by default:

- document `owner` is responsible for periodic review
- default `review_cycle = 12 months`
- system generates review tasks for the owner
- owner reviews and confirms, or updates the document

This does not require suggestions.
Suggestions add an optional path for others to propose that a document needs review outside the normal cycle.

This should continue to support:

- simple owner-led recurring review
- optional human second review
- optional AI review assistance later

---

## API Surface

### Core Endpoints (MVP)

```text
POST   /suggestions              create suggestion
GET    /suggestions              list suggestions
GET    /suggestions/:id          get suggestion
PUT    /suggestions/:id          edit open or in_review suggestion
DELETE /suggestions/:id          delete non-terminal suggestion (open, in_review, withdrawn)
POST   /suggestions/:id/claim    move open -> in_review or in_review -> open
POST   /suggestions/:id/apply    atomically apply suggestion
POST   /suggestions/:id/reject   reject with required reason
```

This is the intentionally narrow MVP surface.

Convenience endpoints can be added later, but the canonical model stays generic.

---

## MCP And Agent Model

The suggestion system should be directly usable by MCP servers and agents.

That is one of the reasons it must live in core.

An MCP server does not need special hidden AI-only mutation paths.
It should be able to talk to the same suggestion model as humans.

Recommended pattern:

- MCP server reads entities, links, comments, history, and documents
- MCP server creates suggestions
- humans review and apply suggestions
- Enterprise later adds richer automation and inbound intake on top

### Why This Matters

This lets us prototype early without waiting for the full Enterprise stack.

A simple MCP server can already:

- suggest a new incident
- suggest a new risk
- suggest a risk reassessment
- suggest a supplier update
- suggest a change request
- suggest a corrective action
- suggest that a document should be reviewed

That is enough to prototype agent workflows while keeping the product model clean.

### MCP Tool Shape (22 tools, implemented)

`isms server mcp` exposes 22 tools:

**Entity read:**
- `list_entities` — list any entity type with status filter
- `get_entity` — single entity details
- `get_entity_history` — changelog / audit trail
- `get_entity_links` — cross-references
- `list_documents` — all documents with metadata
- `get_document` — full markdown body + frontmatter

**Operational awareness:**
- `get_isms_overview` — counts, overdue, open reviews, active suggestions
- `get_overdue_items` — everything past its review cycle

**Suggestions:**
- `list_suggestions` — filter by status, entity type, entity ID
- `create_suggestion` — propose changes (auto-sets `suggested_by_type: agent`)
- `apply_suggestion` — atomically apply with stale detection
- `reject_suggestion` — reject with required reason

**Document review:**
- `list_reviews` — reviews with status filter
- `get_review` — details, assignments, round
- `get_review_diff` — what changed
- `get_review_content` — current document on review branch
- `comment_on_review` — inline comments with optional suggestion_body
- `approve_review` — approve, request changes, or propose revision
- `merge_review` — publish approved document version
- `edit_review_content` — write to review branch
- `confirm_document_review` — lightweight annual review with audit evidence
- `get_pending_actions` — actor-scoped agent work queue

### MCP Example: Suggest New Incident

```json
{
  "tool": "create_suggestion",
  "arguments": {
    "entity_type": "incident",
    "suggestion_type": "create",
    "title": "Create incident from inbound security email",
    "payload": {
      "title": "Suspicious admin login from foreign IP",
      "summary": "Inbound alert from security mailbox and SSO logs.",
      "severity": "high",
      "affected_systems": ["SYSTEM-12"]
    },
    "rationale": "The email describes a privileged login anomaly that should be tracked as an incident.",
    "source_refs": [
      { "type": "email", "id": "msg-123" },
      { "type": "system", "id": "SYSTEM-12" }
    ]
  }
}
```

### MCP Example: Suggest Risk Reassessment

```json
{
  "tool": "create_suggestion",
  "arguments": {
    "entity_type": "risk",
    "entity_id": "RISK-14",
    "suggestion_type": "reassess",
    "title": "Reassess supplier offboarding risk after incident",
    "payload": {
      "current_likelihood": 4,
      "current_impact": 4,
      "reason": "Recent incident and audit evidence show higher practical exposure than previously assessed."
    },
    "rationale": "The current rating likely underestimates real-world exposure.",
    "source_refs": [
      { "type": "incident", "id": "INC-12" },
      { "type": "audit_finding", "id": "FINDING-5" }
    ]
  }
}
```

### MCP Example: Suggest Document Review

```json
{
  "tool": "create_suggestion",
  "arguments": {
    "entity_type": "document",
    "entity_id": "supplier-management-policy",
    "suggestion_type": "review",
    "title": "Review supplier management policy after incident and supplier reassessment",
    "payload": {
      "reason": "Related incident and reassessment suggest the policy may need an update."
    },
    "source_refs": [
      { "type": "incident", "id": "INC-12" },
      { "type": "supplier", "id": "SUPPLIER-8" }
    ]
  }
}
```

### AI Document Review

Document review is a separate system from entity suggestions, but agent users participate in both.

An agent user assigned as reviewer on a document review can:

- read the document content via API
- post inline comments on specific paragraphs
- post paragraph-level suggestions (replacement text with accept/reject)
- submit a proposed revision (edit + decision in one step)
- approve or request changes

This uses the existing review assignment and comment system. No special AI path needed — agent users are first-class reviewers.

### Multi-Model Review (Confidence Stacking)

Enterprise can chain multiple models for higher confidence:

1. Model A reviews the document and creates inline suggestions and comments
2. Model B reviews Model A's suggestions and either endorses or challenges them
3. Human reviewer sees both perspectives and makes the final decision

This works because:

- each agent user has its own identity (`suggested_by_type = 'agent'`)
- comments and suggestions carry author attribution
- the UI already shows who wrote what
- the human sees "Agent A suggested X, Agent B agreed/disagreed" before deciding

The same pattern applies to entity suggestions:

1. Model A creates a suggestion (e.g., reassess risk after incident)
2. Model B reviews the suggestion and adds a comment with its assessment
3. Human applies, rejects, or edits based on both inputs

This is not a new system. It is two agent users interacting with the same suggestion and comment primitives that humans use.

The `source_refs` on the second agent's comment can point back to the first agent's suggestion, creating a reviewable chain of reasoning.

### Design Rule

MCP is an interface layer, not a separate domain model.

The rule should be:

- core owns suggestions
- MCP exposes suggestions to agents and external assistants
- Enterprise adds better AI generation, orchestration, and automations

---

## Enterprise Layer

Enterprise builds on top of the same core suggestion primitive.

Enterprise adds:

- AI-generated suggestions
- inbound email intake
- multi-model review
- automation rules
- richer integrations
- suggestion generation from incidents, audits, supplier notices, and legal changes

The rule:

- Enterprise adds who and what generates suggestions
- Core owns what a suggestion is and how it is reviewed and applied

---

## MVP Scope

### Phase 1: Core

- `suggestions` table with full schema
- CRUD + lifecycle endpoints
- RBAC for reader, contributor, manager, admin, agent user
- changelog linkage from applied suggestion to entity change
- notifications on create, apply, reject
- queue UI plus in-context suggestion panels
- apply handlers for first operational modules

### Phase 1 Modules

Start with:

- risks
- suppliers
- incidents
- legal

These give the highest practical value quickly.

### Phase 2: Enterprise

- AI-generated suggestions via agent API tokens
- inbound email intake -> suggestions
- suggested document review triggers
- batch review workflows
- multi-model review

### Phase 2.5: MCP Prototype Path

Even before full Enterprise, a simple MCP server can prototype the model by:

- reading entities and history
- creating suggestions
- listing open suggestions
- applying or rejecting suggestions through the same core endpoints

This is the fastest path to experimenting with agent workflows without corrupting the product model.

---

## Why This Split Is Right

This split gives us three big benefits.

### 1. Humans And AI Use The Same Primitive

Readers, contributors, and AI all propose change using the same mechanism.
That keeps the model clean.

### 2. Core Stays Valuable

Suggestions are useful even without AI.
A lower-permission user can still contribute meaningfully without direct write rights.

### 3. Enterprise Gets Stronger

Enterprise can add AI and automation without inventing a parallel hidden workflow.

---

## Summary

Suggestions should be a first-class core capability.

They should be:

- generic
- auditable
- role-aware
- usable by both humans and AI
- easy to expose through MCP and agent tooling

The correct architecture is:

- one core suggestion primitive
- module-specific apply logic
- MCP and agent interfaces on top
- Enterprise AI built on top of that

That gives us a clean base for:

- reader and contributor proposals
- AI-assisted operational work
- future incident, risk, and supplier suggestion flows
- stronger review and auditability
- future automation without architectural regret
