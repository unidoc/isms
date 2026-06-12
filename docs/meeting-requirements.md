# How ISMS meets a requirement

When someone asks "does ISMS have X?", the answer almost always lands in one of
four layers. ISMS is a generic engine — it does **not** hardcode a bespoke module
per standard or per requirement. A requirement is met by composing primitives,
not by shipping a new feature for every standard.

## The four layers

1. **Documents** — versioned, git-backed, AI-assisted. Policies, procedures, the
   Statement of Applicability (SoA), a Record of Processing Activities (ROPA),
   risk-treatment narratives, and most written compliance artifacts. Templates
   scaffold the structure; the org owns the content; every change is versioned
   (audit trail); AI can draft from the data and a human reviews and approves.

2. **Registers** — structured, queryable, governed. Risks, assets, suppliers,
   systems, legal requirements, incidents, corrective actions, objectives, audit
   programmes/findings. Use these when the data is rows-with-fields you want to
   query, link, and report on. Entities reference each other (e.g. a processing
   activity → the system that holds the data).

3. **Built-in features** — configured, not built. SSO/OIDC (Microsoft 365,
   Google, Okta, any OIDC provider), password+TOTP, passkeys (WebAuthn), API
   tokens, Cloudflare Zero Trust; multi-tenant orgs; review/approval workflow;
   white-label branding; Slack/Matrix notifications; the AI/MCP layer.

4. **Integrations** — the API. When you need to pull data in from, or push out
   to, an external system automatically. The HTTP API + MCP exist today; a
   first-class ingest layer is on the roadmap (1.0.0). This is the only place you
   "program against" ISMS.

## Worked examples

| Ask | Answer | Layer |
|-----|--------|-------|
| **ROPA** (GDPR Art. 30 record) | A versioned document with the Art. 30 fields, optionally linking processing activities to systems / suppliers / legal requirements. | Document (+ references) |
| **Statement of Applicability (SoA)** | A versioned document. AI drafts applicability + justification from each control's implementation status (maturity); a human reviews and owns the applicability decisions. Versioning gives the SoA its audit trail. | Document (AI-assisted) |
| **Policies / procedures** | Documents from a template — reviewed, approved, versioned. | Document |
| **Risk register / asset inventory** | Structured registers. | Register |
| **SSO** | Built in — OIDC, per-org. Configure in Admin; no code. | Feature |
| **"Do I need to program against you?"** | Not for any of the above. Only for automated data sync with external systems. | Integration |

## The honest line on AI

AI is a **contributor, not a decision-maker**. It reads your entities (via MCP)
and *proposes* content — a SoA draft, a suggested risk treatment — and a human
reviews and approves. For audited artifacts like the SoA, accountability stays
with the person: the AI removes the blank-page work, and versioning records who
changed what.

## Why it's built this way

One generic engine — documents + registers + a few primitives — means the same
system serves ISO 27001, GDPR, SOC 2, NIS2, internal governance, and custom
frameworks without a bespoke module per standard. See also
[architecture.md](architecture.md) and the Scope section of the README.
