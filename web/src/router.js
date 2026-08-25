import { createRouter, createWebHistory } from 'vue-router'
import { getApiToken, setApiToken, clearApiToken } from './api'
import api from './api'
import Login from './views/Login.vue'
import Signup from './views/Signup.vue'
import ForgotPassword from './views/ForgotPassword.vue'
import VerifyEmail from './views/VerifyEmail.vue'
import VerifyEmailChange from './views/VerifyEmailChange.vue'
import Landing from './views/Landing.vue'
import { isSubdomainMode, orgFromSubdomain } from './composables/useCurrentOrg'

const Dashboard = () => import('./views/Dashboard.vue')
const Documents = () => import('./views/Documents.vue')
const Inbox = () => import('./views/Inbox.vue')
const Risks = () => import('./views/Risks.vue')
const Suppliers = () => import('./views/Suppliers.vue')
const Systems = () => import('./views/Systems.vue')
const Assets = () => import('./views/Assets.vue')
const Audit = () => import('./views/Audit.vue')
const Incidents = () => import('./views/Incidents.vue')
const CorrectiveActions = () => import('./views/CorrectiveActions.vue')
const Changes = () => import('./views/Changes.vue')
const Tasks = () => import('./views/Tasks.vue')
const Legal = () => import('./views/Legal.vue')
const Objectives = () => import('./views/Objectives.vue')
const Programs = () => import('./views/Programs.vue')
const Reviews = () => import('./views/Reviews.vue')
const Settings = () => import('./views/Settings.vue')
const Admin = () => import('./views/Admin.vue')
const Organizations = () => import('./views/Organizations.vue')

// Org-scoped route definitions, given as suffixes (no /:org prefix). The router
// boot logic below decides whether to mount them under `/:org` (apex / dev) or
// at the top level (tenant subdomain).
const orgScopedRoutes = [
  { path: '/overview', component: Dashboard },
  { path: '/documents', component: Documents },
  { path: '/documents/:docId', component: Documents },
  { path: '/inbox', component: Inbox },
  { path: '/inbox/:tab?', component: Inbox },
  { path: '/risks', component: Risks },
  { path: '/risks/:id', component: Risks },
  { path: '/suppliers', component: Suppliers },
  { path: '/suppliers/:id', component: Suppliers },
  { path: '/assets', component: Assets },
  { path: '/assets/:id', component: Assets },
  { path: '/systems', component: Systems },
  { path: '/systems/:id', component: Systems },
  { path: '/legal', component: Legal },
  { path: '/legal/:id', component: Legal },
  { path: '/objectives', component: Objectives },
  { path: '/objectives/:objId', component: Objectives },
  { path: '/programs', component: Programs },
  { path: '/programs/:id', component: Programs },
  { path: '/audit', component: Audit },
  { path: '/audit/:tab', component: Audit },
  { path: '/audit/:tab/:itemId', component: Audit },
  { path: '/corrective-actions', component: CorrectiveActions },
  { path: '/corrective-actions/:id', component: CorrectiveActions },
  { path: '/tasks', component: Tasks },
  { path: '/tasks/:id', component: Tasks },
  { path: '/incidents/:id', component: Incidents },
  { path: '/changes', component: Changes },
  { path: '/changes/:id', component: Changes },
  { path: '/incidents', component: Incidents },
  { path: '/reviews', component: Reviews },
  { path: '/reviews/:id', component: Reviews },
  { path: '/settings', component: Settings },
  { path: '/admin', component: Admin },
  { path: '/admin/:tab', component: Admin },
]

function buildOrgRoutes(subdomainMode) {
  // On subdomain hosts (e.g. unidoc.isms.sh) the org is implicit: routes are
  // registered at the top level (`/admin`, `/risks/:id`). On apex / localhost
  // the org slug is part of the path (`/:org/admin`, `/:org/risks/:id`).
  return orgScopedRoutes.map(r => ({
    path: subdomainMode ? r.path : '/:org' + r.path,
    component: r.component,
    meta: { orgScoped: true },
  }))
}

const subdomainMode = isSubdomainMode()

const routes = [
  // Public — always at the top level regardless of host
  { path: '/', component: Landing, meta: { public: true } },
  { path: '/login', component: Login, meta: { public: true } },
  { path: '/signup', component: Signup, meta: { public: true } },
  { path: '/forgot-password', component: ForgotPassword, meta: { public: true } },
  { path: '/verify-email', component: VerifyEmail, meta: { public: true } },
  { path: '/verify-email-change', component: VerifyEmailChange, meta: { public: true } },

  // Org picker — also always top level
  { path: '/organizations', component: Organizations },

  // Org-scoped — registered with or without /:org prefix depending on host
  ...buildOrgRoutes(subdomainMode),

  // Path mode only: /<org>/login must be a real (public) route. Otherwise an
  // org-scoped SSO callback or a bookmarked login link matches nothing and
  // dead-ends on the app shell around a blank page. Subdomain mode already has
  // the top-level /login.
  //
  // NB: deliberately NO /:pathMatch(.*)* catch-all here. App.vue's link
  // interceptor navigates NATIVELY for any absolute path vue-router doesn't
  // match (/docs Scalar UI, /api/openapi.yaml, /healthz, /branding/…, /terms).
  // A catch-all makes every such path "match", so the SPA swallows those clicks
  // and the auth guard bounces to /login — it breaks server-served routes. The
  // residual dead-end on a mistyped /<org>/xyz is a far smaller cost.
  ...(subdomainMode ? [] : [{ path: '/:org/login', component: Login, meta: { public: true } }]),
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// Navigation guard: check auth before each route
let sessionValidated = false
// Probe Cloudflare Access SSO at most once per load — if we're not behind CF
// Access it 401s, and we shouldn't pay that latency on every navigation.
let cfTried = false

router.beforeEach(async (to, from) => {
  // SSO (OIDC, …) hands the session token back in the URL hash (#token=…). Store
  // it BEFORE any auth gate runs: otherwise the guard sees no local token and
  // bounces to /login, trapping the token in a ?redirect= param — and in path mode
  // the org-scoped `/<org>/login` isn't even a matched route, so the destination
  // never gets to process the hash (#187; same class as the CF Access fix in #100).
  //
  // A hash token is a fresh, server-minted assertion, so it wins UNCONDITIONALLY —
  // even over an existing localStorage session. Gating this on `!getApiToken()`
  // would silently drop the new token for anyone already logged in (a re-auth, an
  // org switch, or the reporter's step-6 re-login): the `/<org>/login` route is
  // unmatched, so the old session's shell renders around a blank page and the
  // stale identity sticks. Reset sessionValidated so the new token is revalidated
  // via getMe on the redirect rather than riding a prior validation this page load.
  // Then land on the org's overview with the hash stripped from the URL.
  if (to.hash && to.hash.includes('token=')) {
    const token = new URLSearchParams(to.hash.replace(/^#/, '')).get('token')
    if (token) {
      setApiToken(token)
      sessionValidated = false
      // "/login" → "/overview"; "/<org>/login" → "/<org>/overview".
      return { path: to.path.replace(/\/login$/, '/overview') }
    }
  }

  // Cloudflare Access probe — try once per page load, BEFORE any route-type
  // decision (including meta.public) is made. This used to sit after the
  // meta.public check below, which meant it never ran on the routes a fresh
  // visitor actually lands on: the root, /login, and /<org>/login are ALL
  // meta.public, so a CF-Access-authenticated visitor with no local token
  // could land on any entry point, get told "you're not logged in", and never
  // once have the CF identity checked — the only way in was to already know to
  // type a non-public URL like /organizations by hand. Running the probe
  // unconditionally here means getApiToken() is accurate by the time the
  // meta.public branch runs, so it routes a freshly-minted session into the
  // app the same way it already does for a token that was there beforehand.
  // Login.vue's own onMounted separately redirects an already-authenticated
  // visitor away from the form, so landing on /login with a fresh token still
  // resolves correctly without any change needed there.
  if (!getApiToken() && !cfTried) {
    cfTried = true
    // Bounded: cfSession() itself swallows every failure and resolves null, but
    // the underlying fetch has no timeout of its own, so a hung /auth/cf-session
    // (an overloaded origin, a proxy black-holing the request) would otherwise
    // hold up first paint on every public route indefinitely — this probe now
    // runs there too (review finding F5 on #223). 3s is generous for a same-
    // deployment round trip and short enough that a hang degrades to "not
    // behind CF Access" rather than a stalled landing page.
    const cf = await Promise.race([
      api.cfSession(),
      new Promise((resolve) => setTimeout(() => resolve(null), 3000)),
    ])
    if (cf?.token) {
      setApiToken(cf.token)
      if (cf.email) localStorage.setItem('isms_user_email', cf.email)
      if (cf.name) localStorage.setItem('isms_user_name', cf.name)
    }
  }

  // A tenant subdomain (e.g. acme.isms.sh) IS the org context — the org picker
  // should never be reachable from there. Stale-token refreshes would
  // otherwise leak the user's other org memberships into the tenant's UI.
  if (to.path === '/organizations' && orgFromSubdomain()) {
    return getApiToken() ? { path: '/overview' } : { path: '/login' }
  }

  if (to.meta.public) {
    if (to.path === '/' && getApiToken()) {
      // Already logged in landing on the root — go straight into the org
      // implied by the subdomain. Only fall back to the org picker if we
      // genuinely have no subdomain context (apex / localhost).
      const slug = orgFromSubdomain()
      if (slug) return { path: '/overview' }
      return { path: '/organizations' }
    }
    return true
  }

  if (!getApiToken()) {
    return { path: '/login', query: to.fullPath !== '/' ? { redirect: to.fullPath } : undefined }
  }

  if (!sessionValidated) {
    try {
      const me = await api.getMe()
      if (me?.email) {
        sessionValidated = true
        return true
      }
    } catch (e) {
      // Only treat 401 as invalid session. Network errors (abort, timeout) keep the token.
      if (e?.status !== 401 && e?.isNetwork) {
        return true // let the page load, it'll retry on next navigation
      }
    }
    clearApiToken()
    sessionValidated = false
    return { path: '/login', query: to.fullPath !== '/' ? { redirect: to.fullPath } : undefined }
  }

  return true
})

export default router
