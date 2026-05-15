# AI Review Loop

## Problem

When two AI agents collaborate on a document review (writer agent + reviewer agent), the current system notifies the human owner after every action. That means the human sees intermediate states — half-finished edits, unresolved reviewer comments, back-and-forth that should have been settled between the agents before a human ever looks at it.

The goal is: **AI agents resolve what they can between themselves, and the human only sees the finished result.**

## Design Principle

The review system already has rounds, comments, suggestions, approve/reject decisions, and merge.

The AI review loop should reuse those primitives. It adds **automation rules** that determine:

- when to notify humans
- when to notify agents
- when to let agents continue iterating
- when to escalate

## Current Flow (Without Loop)

```
Claude writes document
  → sends for review
    → GPT is assigned reviewer
      → GPT requests changes
        → human gets notification ← too early
          → human has to understand and relay back to Claude
```

## Desired Flow (With Loop)

```
Claude writes document
  → sends for review
    → GPT is assigned reviewer
      → GPT requests changes + adds inline comments
        → system detects: author is agent, reviewer is agent
          → system notifies Claude agent (not human yet)
            → Claude reads GPT comments, makes edits, resubmits
              → GPT reviews again → approves
                → NOW human gets notification: "AI review complete, ready for your confirmation"
```

## When To Involve The Human

The human should be notified when:

1. **All agent reviewers have approved** — the AI loop is done, human confirms
2. **An agent reviewer rejects twice on the same issue** — agents are stuck, human decides
3. **Max rounds exceeded** — safety limit (e.g. 3 AI rounds), escalate to human
4. **A human reviewer is assigned** — human always gets their own notifications regardless of agent activity

The human should NOT be notified when:

1. An agent reviewer requests changes and the author is also an agent
2. An agent author resubmits after addressing agent comments
3. Agents are still iterating within the round limit

## Preconditions

Before this loop is trusted for governance decisions, agent identity must be first-class.

That means `users.is_agent` cannot just exist in schema. It must be:

- settable at user creation time
- editable in admin/server flows
- visible in UI and audit views where relevant
- used consistently by policy evaluation

Without that, `require_human` and agent-to-agent loop rules are weaker than they look.

## Data Model

No new domain tables. Reuse existing review primitives and user identity:

- `users.is_agent` — identifies agent accounts
- `review_assignments.reviewer` — email, can look up `is_agent`
- `reviews.requested_by` — author, can look up `is_agent`
- `reviews.round` — current round number

Small supporting additions are still fine:

- org settings
- notification flags / types

New org settings:

```
ai_review_max_rounds = 3        -- max AI-to-AI rounds before escalation
ai_review_auto_resubmit = true  -- let author agent auto-resubmit after addressing changes
```

## Implementation

### In the approve/reject handler

After a reviewer submits a decision:

1. Check if the reviewer is an agent (`users.is_agent`)
2. Check if the review author is an agent (`users.is_agent` on `reviews.requested_by`)
3. If both are agents AND decision is `changes_requested`:
   - Check current round against `ai_review_max_rounds`
   - If under limit: **suppress owner-facing notification**, create agent notification only
   - If at/over limit: **escalate to human** with summary of what agents disagreed on
4. If decision is `approved` AND all assigned reviewers have approved:
   - If `require_human` policy: notify human owner for final confirmation
   - If autopilot: auto-merge

Important boundary:

- if any human reviewer is assigned, do **not** suppress that human's own review notifications
- suppression applies only to "owner, please look now" notifications during a pure agent loop
- human reviewers always remain visible participants, never hidden behind the loop

### Agent notification

A new notification type or flag:

```sql
-- Add to notifications table
agent_actionable BOOLEAN NOT NULL DEFAULT false
```

When `agent_actionable = true`, the notification is intended for an agent to act on, not for a human to read.

This does not need a new table. It can be:

- a boolean on notifications
- or an equivalent typed notification attribute

MCP tools should be able to filter for these.

### MCP: get_pending_actions tool

New MCP tool that returns "what should this agent do next":

```
get_pending_actions
  → returns: reviews awaiting resubmit, reviews awaiting re-review, suggestions awaiting response
```

This lets an agent poll for work without the orchestration layer needing to push.

It must be **actor-scoped**:

- returns only work relevant to the authenticated agent
- not a generic org-wide inbox dump
- can optionally support a manager-only org-wide mode later, but that is not the default

### Resubmit flow

When an author agent receives a `changes_requested` notification:

1. Agent reads the review comments (`get_review` + timeline or comments)
2. Agent reads the current document (`get_review_content`)
3. Agent edits the document on the review branch (via `PUT /reviews/:id/content` — needs MCP tool)
4. Agent resubmits using the **existing review send/resubmit flow**:
   - call `POST /documents/:docId/reviews`
   - include the same reviewer set (from `get_review` assignments)
   - when the existing review is `changes_requested`, the server already:
     - reopens the same review
     - resets assignments to `pending`
     - increments the effective round
     - logs `review_resubmitted`

Missing MCP tool: `edit_review_content` — writes to review branch.

Important:

- do **not** model resubmit as `PUT /reviews/:id/status`
- status update is not the author resubmit path
- the loop should use the same send/resubmit behavior humans already use

## Escalation

When agents are stuck (max rounds exceeded):

1. System creates a notification for the human owner:
   - "AI review of [document] reached round 3 without agreement"
   - links to the review with full comment history
   - human can see all AI comments and decide
2. System stops auto-resubmitting
3. Human takes over: edits, approves, or closes

Escalation should also happen immediately if:

- a human reviewer is added mid-loop
- policy requirements change and now require explicit human participation
- agent identity cannot be trusted for one of the participants

## Round Tracking

The existing `round` column on reviews tracks this. Each resubmit increments the round. The system checks:

```
if review.round >= ai_review_max_rounds AND both author and reviewer are agents:
    escalate to human
```

## Summary Of What Needs Building

### MCP tools (2 new)

- `get_pending_actions` — what should this agent do next
- `edit_review_content` — write to review branch (enables agent to address comments)

### Backend logic (in approve handler)

- Agent-to-agent detection (both `is_agent`)
- Suppress owner-facing notification when agents are still iterating
- Escalation when max rounds exceeded
- Agent-targeted notifications with `agent_actionable` flag
- Never suppress notifications for assigned human reviewers

### Org settings (2 new)

- `ai_review_max_rounds` (default 3)
- `ai_review_auto_resubmit` (default true)

### Frontend

- Show "AI review in progress" badge on reviews where agents are iterating
- Show "Escalated — AI review stalled" when max rounds exceeded
- Show round-by-round AI conversation in timeline with clear "AI round" markers

## What This Enables

### Workflow 1: AI writes, AI reviews, human confirms

```
Claude writes policy update
  → GPT reviews, requests 2 changes
    → Claude fixes both, resubmits
      → GPT approves
        → Human sees: "AI review complete (2 rounds). Claude updated, GPT approved."
          → Human confirms in 20 seconds
```

### Workflow 2: AI writes, AI reviews, stuck → human decides

```
Claude writes risk assessment
  → GPT disagrees on likelihood rating
    → Claude argues back with evidence
      → GPT still disagrees
        → Round 3: system escalates to human
          → Human reads both positions, makes final call
```

### Workflow 3: Annual document review (no changes)

```
Claude reads document, compares with registers
  → Claude: "document is still valid, no changes needed"
    → GPT reviews Claude's assessment
      → GPT: "agreed, but note that supplier list in section 3 could mention new vendor"
        → Claude adds one sentence, resubmits
          → GPT approves
            → Human confirms: "AI reviewed, minor update, both models agree"
```

## Design Rules

- Agents use the same review primitives as humans — no parallel system
- Agent-to-agent iteration is bounded by max rounds
- Human is always the final authority when `require_human = true`
- Escalation is automatic, not dependent on agent behavior
- Every action has full audit trail regardless of who (human or agent) performed it
- MCP tools are pull-based (agent checks for work), not push-based (no webhooks to agents)
- Agent loops are only trusted if agent identity is explicit and policy evaluation treats agents and humans consistently

## Implementation Order

1. `edit_review_content` MCP tool (enables agents to address review comments)
2. Agent-to-agent detection in approve handler + notification suppression
3. `get_pending_actions` MCP tool
4. `ai_review_max_rounds` setting + escalation logic
5. Frontend: "AI review in progress" badge + escalation indicator
