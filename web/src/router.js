import { createRouter, createWebHistory } from 'vue-router'
import { getApiToken, clearApiToken } from './api'
import api from './api'
import Login from './views/Login.vue'
import Signup from './views/Signup.vue'
import ForgotPassword from './views/ForgotPassword.vue'
import VerifyEmail from './views/VerifyEmail.vue'
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
  { path: '/objectives/:tab', component: Objectives },
  { path: '/objectives/:tab/:objId', component: Objectives },
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

  // Org picker — also always top level
  { path: '/organizations', component: Organizations },

  // Org-scoped — registered with or without /:org prefix depending on host
  ...buildOrgRoutes(subdomainMode),
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// Navigation guard: check auth before each route
let sessionValidated = false

router.beforeEach(async (to, from) => {
  // A tenant subdomain (e.g. verkis.commandvector.net) IS the org context —
  // the org picker should never be reachable from there. Stale-token refreshes
  // would otherwise leak the user's other org memberships into the verkis UI.
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
