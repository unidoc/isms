# isms.sh Editions

## Positioning

`isms.sh` is not open core with artificial limits.

The core product is already a serious, self-hostable management system:

- git-backed documents
- round-based review and publish workflow
- risks, incidents, suppliers, systems, legal, changes, corrective actions, objectives, audit
- tasks and annual planning
- cross-linking between operational entities

The editions model is:

- **Open Source Core** for ownership, trust, and self-hosting
- **isms.sh Cloud** as the best everyday experience
- **Enterprise** for AI, automation, integrations, and premium controls

This is closer to:

- GitHub + GitHub Enterprise

than:

- crippled open core

---

## Product Thesis

The category is full of compliance systems that treat documents as attachments and frameworks as hardcoded forms.

That is not our direction.

The thesis is:

1. Documents are first-class operational assets, not dead Word files.
2. Reviews are first-class decisions, not email threads.
3. Tasks are the execution layer that runs the system through the year.
4. AI should remove drudge work, not replace human responsibility.
5. The same engine should work for ISO 27001, GDPR, NIS2, SOC 2, internal governance, and custom frameworks without hardcoding the framework into the product model.

The result is:

- living documents
- versioned history
- evidence trail
- recurring operational cadence
- human accountability

---

## Open Source Core

The open source core should remain fully credible on its own.

That includes:

- multi-tenant organizations
- auth, RBAC, API tokens, passkeys, OIDC
- git-backed document system
- review workflow with rounds, suggestions, proposed revision, publish
- operational modules
- tasks
- annual planning / dashboard workload view
- branding
- repo protection
- REST API, CLI, TUI
- MCP server for AI agents (22 tools — entity read, suggestions, document review)
- entity suggestions as core primitive (generic, auditable, role-aware)

Core should not feel like a trial.

Self-hosters should be able to run a real ISMS/IMS with no artificial ceiling.

That credibility is part of the hosted story too.

---

## Why Hosted Still Wins

Even with strong self-hosting support, many customers will still prefer `isms.sh`.

Hosted should be the best experience because it can offer:

- zero setup
- managed upgrades and migrations
- managed backups
- working email delivery and reminders
- AI features out of the box
- integrations without glue work
- smoother branding and onboarding
- less operational risk for the customer

The right posture is:

- **self-hosted is possible**
- **hosted is easier**
- **enterprise hosted is the premium experience**

That gives us both:

- trust from customers who want control
- revenue from customers who want convenience and intelligence

---

## AI-First, But Not AI-Autopilot

The platform should be AI-first in workflow design, not in the sense of replacing human judgment.

The correct model is:

- AI drafts
- AI suggests
- AI summarizes
- AI links context
- AI proposes actions
- humans remain accountable

That matters because the human side is the real system:

- people must understand what is happening
- people must communicate
- people must act
- people must stand behind the record
- people must speak to auditors

So the rule is:

- **AI is an operator and assistant**
- **humans remain the accountable actors**

---

## The Three-Layer AI Model

### 1. Working Layer

AI can operate in working state:

- edit documents on main
- update draft registers
- draft incidents, changes, corrective actions
- prepare objective check-ins
- prepare risk reassessments

### 2. Suggestion Layer

AI should mostly produce first-class suggestions:

- proposed document revision
- inline suggestion
- risk assessment suggestion
- supplier reassessment suggestion
- incident summary draft
- corrective action draft
- change request draft
- legal update suggestion

The user should be able to:

- accept
- reject
- edit

### 3. Official Layer

Official truth still requires human confirmation where it matters:

- publish document version
- approve review
- close or accept significant operational decisions
- confirm important risk posture changes

This preserves auditability and trust.

---

## Enterprise AI Features

These are the features that belong naturally in Enterprise.

### AI Suggestions

AI should be able to generate suggestions from operational context across the full system.

Examples:

- suggest a document update after an incident
- suggest a new risk or a reassessment of an existing risk
- suggest a corrective action from an audit finding
- suggest a change request from a supplier or legal update
- suggest a version note or review note
- suggest linked entities the user may have missed

This is a natural enterprise feature because:

- it costs inference money
- it needs careful controls
- it adds most value in larger teams

### Inbound Intake

Intake is the process of receiving external input and turning it into operational records.

The core supports manual intake: any user can create an incident, change request, or corrective action from external information. That works and is sufficient for self-hosted core.

Enterprise AI intake accelerates the same process:

- `security@company.isms.sh` receives an incident report
- the system ingests the email and attachments
- an AI agent drafts:
  - incident summary
  - severity suggestion
  - affected systems/assets/suppliers
  - risk reassessment suggestion
  - corrective action suggestion
  - change request suggestion
  - document review suggestion
- everything is linked and saved as drafts or suggestions
- a human confirms the next steps

This same pattern applies to:

- supplier evidence intake
- audit evidence intake
- legal or contractual notices
- customer complaints or requests
- change request intake

The principle: manual intake is core, AI-accelerated intake is enterprise.

### Multi-Model Review

Enterprise can also offer:

1. model A drafts
2. model B reviews and challenges
3. human approves

That is useful when customers want stronger separation of duties or higher confidence.

### AI-Generated Evidence Support

AI should help with evidence preparation, not invent evidence.

Good examples:

- summarize what communication happened this quarter
- draft the evidence note for a completed awareness activity
- organize incoming attachments under the right objective, check-in, audit item, or incident
- explain why a linked record supports a control or requirement

Bad model:

- fabricated evidence
- silent automatic approval
- fake completion

---

## AI Identity And Permissions

Enterprise AI must have explicit identity.

That means:

- agent users or service accounts
- scoped API tokens
- full audit trail
- separate identity from the human who later approves

Recommended permission tiers:

- `ai_read_only`
- `ai_draft_writer`
- `ai_reviewer`
- `ai_intake_agent`

Never hide AI actions behind a human account.

The UI should make it obvious when:

- this was written by a person
- this was suggested by AI
- this was reviewed by AI

---

## Enterprise Integrations

Enterprise should focus on integrations that feed operational work into the system.

High-value examples:

- inbound email intake
- Microsoft 365 and Google Workspace
- HRIS or identity lifecycle events
- ticketing systems
- chat notifications
- awareness and training systems such as KnowBe4
- evidence sources and storage integrations

The integration principle should be:

- pull useful facts in
- create or enrich product objects
- keep the final workflow inside the platform

The system should not become a thin wrapper around other tools.

---

## Documents Stay At The Center

A key differentiator is that important governance content stays in the document engine.

That means many things that other systems hardcode as modules can remain documents first:

- communication plan
- interested parties / stakeholders
- competence requirements
- emergency plans
- tabletop exercise plans
- framework-specific interpretations

Why that matters:

- versioning
- review cadence
- ownership
- publish history
- AI-assisted drafting
- evidence that a document was reviewed

Framework-specific artifacts like Statement of Applicability, control matrices, or maturity assessments are documents with review cycles, not hardcoded modules. A template provides the starting structure; the document engine provides versioning, ownership, and evidence that it was reviewed.

The product should resist the temptation to hardcode every framework concept into CRUD screens.

Documents, templates, and references should carry much of the semantic weight.

---

## Tasks And Annual Planning

Tasks should not become a giant project-management subsystem.

Their role is to run the management system:

- overdue document reviews
- objective check-ins
- supplier reviews
- legal reviews
- risk reviews
- access reviews
- corrective follow-up

That means:

- tasks are first-class
- they appear in Dashboard, Annual Plan, Inbox, and Tasks
- they are generated from real review cycles and operational due dates

This is one of the biggest product advantages over Excel:

- the system tells you what to do this month
- the system distributes workload across the year
- the system proves that work happened

---

## Evidence And Auditors

Enterprise should help with the painful, repetitive evidence work auditors ask for.

Typical examples:

- proof of communication according to communication plan
- proof of awareness activity
- proof of periodic review
- proof that a supplier was reassessed
- proof that an objective was checked in

The right product response is:

- keep the source content and operational record in one system
- attach or link evidence where work happens
- let AI help summarize and package that evidence

This is much stronger than:

- separate Word files
- separate Excel trackers
- ad hoc folders of screenshots

---

## Deployment Options

### 1. Open Source Self-Hosted

For customers who want:

- full control
- internal hosting
- no vendor dependency

### 2. isms.sh Cloud

For customers who want:

- fastest onboarding
- no ops work
- managed updates
- branding and email out of the box

### 3. Enterprise Self-Hosted

For customers who want:

- advanced AI and integrations
- but must keep the platform on their own infrastructure

This should feel like a real enterprise offering, not a second-class path.

---

## Moat

The moat is not hiding the code.

The moat is:

- the hosted operational experience
- the AI workflows
- the integrations
- the audit and evidence ergonomics
- the product semantics around documents, reviews, and operational cadence
- the team's understanding of how ISMS work actually happens in real companies

Someone can copy screens.

It is much harder to copy:

- workflow design
- the AI layer
- the operational polish
- customer trust

---

## Practical Editions Split

### Open Source Core

- full management system
- self-hostable
- no artificial crippling

### Hosted Cloud

- same core value
- best setup and operations experience

### Enterprise

- AI suggestions
- inbound AI intake
- advanced automations
- multi-model review
- premium integrations
- premium support and rollout help

That is the clean split.

---

## Summary

The core is already powerful enough to stand on its own.

Enterprise should not try to make the core usable.
Enterprise should make it:

- smarter
- faster
- more connected
- easier to run at scale

The right message is:

- your system
- your documents
- your history
- your evidence
- our hosted experience and intelligence on top

That is a much stronger story than a locked-down compliance SaaS.
