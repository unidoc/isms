import { ref, computed } from 'vue'
import { api, getCurrentUser, clearApiToken, setApiToken } from '../api'
import { orgFromSubdomain, isSubdomainMode, orgEntryURL } from './useCurrentOrg'

const user = ref(getCurrentUser())
const currentUserData = ref(null)
const userOrgs = ref([])
const orgName = ref('')
const logoUrl = ref(null)
const logoError = ref(false)
const termsUrl = ref('')
const privacyUrl = ref('')
const showPoweredBy = ref(true)
const openReviewCount = ref(0)

let refreshTimer = null

export function useSession() {
  const orgSlug = computed(() => currentUserData.value?.organization_slug || '')
  const orgID = computed(() => currentUserData.value?.organization_id || 0)
  // True when the user has an active org context — either via the JWT's
  // organization_id or by virtue of being on a tenant subdomain (e.g.
  // unidoc.isms.sh). The latter handles refreshes that hit before /me has
  // resolved, and DB-reset scenarios where the JWT predates the new org row.
  const hasOrg = computed(() =>
    (currentUserData.value?.organization_id > 0) || !!orgFromSubdomain()
  )
  const otherOrgs = computed(() => userOrgs.value.filter(o => o.slug !== currentUserData.value?.organization_slug))

  const displayName = computed(() => {
    if (currentUserData.value?.name) return currentUserData.value.name
    return user.value.split('@')[0]
  })

  function startRefreshTimer(router) {
    if (refreshTimer) clearInterval(refreshTimer)
    refreshTimer = setInterval(async () => {
      try {
        await api.refresh()
      } catch (e) {
        if (e?.status === 401) {
          clearApiToken()
          router.push('/login')
        }
      }
    }, 12 * 60 * 60 * 1000) // 12 hours
  }

  function stopRefreshTimer() {
    if (refreshTimer) {
      clearInterval(refreshTimer)
      refreshTimer = null
    }
  }

  async function loadUserData(route, router) {
    try {
      const me = await api.getMe()
      currentUserData.value = me
      if (currentUserData.value?.email) {
        user.value = currentUserData.value.email
      }
      // Set org name from /me if config didn't have it
      if (!orgName.value && currentUserData.value?.organization_name) {
        orgName.value = currentUserData.value.organization_name
      }

      // If on an org-scoped route but no org_id from server, redirect to org picker.
      // Org context can come from either the :org path param OR the host subdomain.
      const onOrgScopedRoute = !!route.params.org || !!orgFromSubdomain()
      if (onOrgScopedRoute && (!currentUserData.value?.organization_id || currentUserData.value.organization_id === 0)) {
        router.push('/organizations')
        return false
      }

      // Load all user orgs for switcher
      try {
        const orgs = await api.getMyOrgs()
        userOrgs.value = Array.isArray(orgs) ? orgs : []
      } catch { /* ignore */ }

      return true
    } catch (e) {
      // Only clear token and redirect on genuine auth failure (401).
      // Network errors (abort from F5, connection refused) keep the token.
      if (e?.status === 401) {
        clearApiToken()
        // Don't kick the user away from a public auth page they're trying
        // to complete (signup / forgot-password / verify-email / login).
        const publicAuthPaths = ['/login', '/signup', '/forgot-password', '/verify-email', '/']
        if (!publicAuthPaths.includes(route.path)) {
          router.push('/login')
        }
      }
      return false
    }
  }

  async function loadBranding() {
    try {
      const cfg = await api.getConfig()
      // Branding settings override org defaults
      if (cfg.branding?.branding_name) {
        orgName.value = cfg.branding.branding_name
      } else if (cfg.organization_name) {
        orgName.value = cfg.organization_name
      } else if (cfg.organization?.name) {
        orgName.value = cfg.organization.name
      }
      if (cfg.branding?.branding_logo) {
        logoUrl.value = cfg.branding.branding_logo + '?t=' + Date.now()
      } else if (cfg.organization?.logo) {
        logoUrl.value = '/branding/logo?t=' + Date.now()
      } else {
        logoUrl.value = null
      }
      if (cfg.branding?.branding_favicon) {
        const link = document.querySelector('link[rel="icon"]') || document.createElement('link')
        link.rel = 'icon'
        link.href = cfg.branding.branding_favicon + '?t=' + Date.now()
        if (!link.parentNode) document.head.appendChild(link)
      }
      if (cfg.branding?.branding_color) {
        document.documentElement.style.setProperty('--brand-color', cfg.branding.branding_color)
      }
      if (cfg.terms_url) termsUrl.value = cfg.terms_url
      else if (cfg.has_terms) termsUrl.value = '/terms'
      if (cfg.privacy_url) privacyUrl.value = cfg.privacy_url
      else if (cfg.has_privacy) privacyUrl.value = '/privacy'
      showPoweredBy.value = cfg.show_powered_by !== false
    } catch (e) {
      // Fallback: use org name from /me response
    }
  }

  async function loadReviewCount() {
    try {
      const reviews = await api.getReviews('open')
      openReviewCount.value = Array.isArray(reviews) ? reviews.length : 0
    } catch { /* ignore */ }
  }

  async function logout(router) {
    stopRefreshTimer()
    try {
      await api.postJSON('/api/v1/auth/logout', {})
    } catch { /* ignore — still clear locally */ }
    clearApiToken()
    return router.push('/login')
  }

  async function switchOrg(org) {
    try {
      const result = await api.postJSON('/api/v1/auth/switch-org', { slug: org.slug })
      if (result.token) {
        setApiToken(result.token)
        // Canonical URL for the org: subdomain hop on hosts that support it,
        // path-based on localhost / single-label hosts. Same helper used by
        // the /organizations picker so apex → subdomain works uniformly.
        window.location.href = orgEntryURL(org.slug, '/overview')
      }
    } catch {
      // Fallback to login redirect
      window.location.href = '/login?org=' + org.slug
    }
  }

  function refreshUser() {
    user.value = getCurrentUser()
  }

  return {
    // Refs (shared state)
    user,
    currentUserData,
    userOrgs,
    orgName,
    logoUrl,
    logoError,
    termsUrl,
    privacyUrl,
    showPoweredBy,
    openReviewCount,

    // Computed
    orgSlug,
    orgID,
    hasOrg,
    otherOrgs,
    displayName,

    // Methods
    startRefreshTimer,
    stopRefreshTimer,
    loadUserData,
    loadBranding,
    loadReviewCount,
    logout,
    switchOrg,
    refreshUser,
  }
}
