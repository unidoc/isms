# Multi-tenant routing

ISMS serves multiple organizations from a single deployment. Each request is
mapped to exactly one org context (or none — the "landing" / apex context).
This document describes the rules the platform enforces end-to-end.

## Two routing modes

A deployment can serve orgs in either or both of two modes:

### Subdomain mode — `<slug>.<apex>`

Each tenant gets its own host: `acme.isms.sh`, `unidoc.isms.sh`,
`acme-logistics.localhost`. **The subdomain IS the org boundary.** Users on
a tenant subdomain are bound to that one org:

- The `/organizations` picker is unreachable (router guard redirects to
  `/overview` or `/login`).
- The header org-switcher does not show "Switch to other orgs" or
  "Switch organization".
- `api.getMyOrgs()` is not called — the SPA never loads the user's other
  memberships. Even if a future code path tried to render them, there's
  nothing to render.
- Stale-token refresh re-auths into THIS subdomain's org, never into the
  picker (would otherwise leak other org memberships into the tenant UI).

Enabled with `ISMS_SUBDOMAIN_ROUTING=1` plus `ISMS_DOMAIN=<apex>`.

### Path mode — `<apex>/<slug>/...`

Org is part of the URL path: `isms.sh/acme/risks`,
`commandvector.net/verkis/documents`. **The path tells you which org is
active, but the user can switch.**

- The `/organizations` picker is reachable.
- The header org-switcher shows other org memberships.
- The `:org` path param is the single source of truth for the active org —
  no `localStorage` fallback, no JWT-only context.
- If the user lands on `/acme/...` with a JWT scoped to `unidoc`, the SPA
  calls `auth/switch-org` to swap the token before loading any org data.

Path mode works on every host — apex (`isms.sh`), localhost, container
hosts. It is the only mode on deployments without wildcard DNS / TLS.

## Resolution order (server)

`OrgResolverMiddleware` runs BEFORE auth and sets the org context on the
request:

1. **Skip** `/git/...` — git wire protocol resolves by UUID in the handler.
2. **Subdomain** — `hostname` ends with `.<apex>` and `ISMS_SUBDOMAIN_ROUTING=1`
   → look up the slug as the first label.
3. **Custom domain** — `hostname` matches an `organizations.domain` column.
4. **Path-based** — for non-API, non-static paths only: the first path
   segment is treated as the slug, the path is rewritten (`/acme/dashboard`
   becomes `/dashboard` with `org_id` set on context).
5. **No match** — apex context, `landing: true` flag set.

API requests (`/api/v1/...`) bypass path-based resolution. API consumers
must either be on a subdomain Host header or attach a JWT scoped to an org.

## UI rules (frontend)

The SPA mirrors the server-side model. See `web/src/composables/useCurrentOrg.js`
for the canonical helpers.

| Concern | Subdomain mode | Path mode |
|---|---|---|
| `route.params.org` | not used | source of truth |
| `orgFromSubdomain()` | returns the slug | returns `''` |
| `/organizations` reachable | NO (router guard) | YES |
| Header switcher dropdown | current org + settings only | full switcher |
| `api.getMyOrgs()` called | NO | YES |
| `orgEntryURL(slug)` returns | `https://<slug>.<apex>/...` | `/<slug>/...` |

The router guard in `web/src/router.js` intercepts navigation to
`/organizations` on a subdomain unconditionally — defense in depth in case
some future code path tries to push there.

## Authentication

### Signup — `/signup`

Local password account only. **Never OIDC.**

The first owner of an org must be a local password account so that if Entra
ID / Google access is later removed, contracts end, MFA devices are lost, or
the IT admin revokes the user, there is still a break-glass account that
can sign in and recover. OIDC is configured AFTER the org exists, by the
local owner.

The signup form is never combined with the login form. They are separate
pages — never tabs, never toggles, never a shared form.

### Login — `/login`

Password, OIDC, passkey, OTP. On a subdomain the org is implicit (Host
header binds it); on path/apex the user first enters their org slug.

### Verification — `/verify-email?token=...`

Sets password from the email link and returns a JWT. If the user has no org
yet, redirects to `/organizations` (picker) to create their first org; users
joining via an invite already belong to an org — the response carries their
`organization_slug` and they land on `/overview` directly.

## Server-served paths and the global anchor interceptor

`App.vue` installs a `document.addEventListener('click', ...)` that
intercepts internal anchor clicks and routes them through Vue Router with
the correct org prefix. This makes markdown-rendered links like
`/documents/foo` or `/risks/RISK-1` work as SPA navigations regardless of
which view rendered them.

**The interceptor must not capture clicks for server-served paths** like
`/docs` (Scalar API reference), `/api/openapi.yaml`, `/healthz`,
`/branding/...`. The Vue Router has no route for these; if the interceptor
pushes them, the auth guard rejects them and the user lands on `/login`.

The current implementation calls `router.resolve(candidate)` first and
only calls `preventDefault()` when `resolved.matched.length > 0`. Anything
else falls through to native browser navigation, which hits the server
handler directly. Test coverage: `tests/test_e2e_routing.py::TestDocsLinkServedNatively`.

## Configuration

| Env var | Required when | Effect |
|---|---|---|
| `ISMS_DOMAIN` | subdomain mode | Apex hostname — `acme.<ISMS_DOMAIN>` resolves to org `acme`. |
| `ISMS_SUBDOMAIN_ROUTING` | subdomain mode | Set to `1` to enable. Default off. |
| `organizations.domain` | custom domain mode | Per-org column; an exact hostname match resolves to that org. |

The `/api/v1/config` endpoint surfaces `subdomain_routing` and `apex_host`
to the SPA so `orgEntryURL()` can construct the right URL when switching
between orgs.

## Test coverage

| File | Layer | What it covers |
|---|---|---|
| `tests/test_routing.py` | Backend (HTTP) | `OrgResolverMiddleware` resolves Host headers; apex and unknown subdomains fall through cleanly; `/docs` is public Scalar HTML |
| `tests/test_e2e_routing.py` | Playwright | Subdomain hides the picker and the switcher's other-orgs section; stale token redirects to `/login` not `/organizations`; `/docs` link navigates natively |
| `tests/test_multi_tenant.py` | Backend (data) | Org A cannot read org B's data — RLS + application-layer enforcement |

The devenv container sets the routing env vars and adds
`acme-logistics.localhost` to `/etc/hosts` so Playwright can drive the
browser against a real subdomain URL.
