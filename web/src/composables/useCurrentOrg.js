import { computed } from 'vue'
import { useRoute } from 'vue-router'

// Internal router handle for use OUTSIDE Vue component setup (e.g. global
// DOM event listeners in App.vue's onMounted). Vue's `useRouter()` is only
// available inside a setup context, so we accept the router instance via
// `registerRouter()` once at app boot and read it from module scope.
//
// Stored as a function-returning-router to keep the seam testable and to
// dodge accidental circular-import shenanigans (router.js already imports
// from this module).
let _routerRef = null
export function registerRouter(router) {
  _routerRef = router
}

// Apex hosts — hostnames that are themselves an apex (not a tenant subdomain).
// The server reports its own apex via /api/v1/config (`apex_host`); the
// frontend extends this set at runtime via setApexHost(). The seeds below
// cover the public deployments and are useful before /config has loaded.
const _apexHosts = new Set([
  'isms.sh',
  'www.isms.sh',
])

export function setApexHost(host) {
  if (host) _apexHosts.add(host.toLowerCase())
}

// Hostname patterns that are always path-based (dev / local / container).
const LOCAL_HOSTS = new Set([
  'localhost',
  '127.0.0.1',
  '0.0.0.0',
  '::1',
  'host.containers.internal',
  'host.docker.internal',
])

/**
 * Returns true when the current host is a tenant subdomain (e.g. `unidoc.isms.sh`).
 * Returns false on apex (`isms.sh`, `www.isms.sh`), localhost, container hosts,
 * and any single-label / two-label hostnames.
 */
export function isSubdomainHost(hostname) {
  const host = (hostname || '').toLowerCase()
  if (!host) return false
  if (LOCAL_HOSTS.has(host)) return false
  if (_apexHosts.has(host)) return false
  // Strip an optional port, just in case.
  const hostNoPort = host.split(':')[0]
  if (LOCAL_HOSTS.has(hostNoPort)) return false
  if (_apexHosts.has(hostNoPort)) return false

  const parts = hostNoPort.split('.')
  if (parts.length < 3) return false
  if (parts[0] === 'www') return false
  // Last two labels must look like a public domain (e.g. "isms.sh"). We accept
  // anything where the final label is at least 2 chars and contains only
  // letters/digits; this filters out things like "10.0.0.5" (IP literals).
  const tld = parts[parts.length - 1]
  if (!/^[a-z0-9-]{2,}$/.test(tld)) return false
  // IPv4 literal check: all parts numeric.
  if (parts.every(p => /^\d+$/.test(p))) return false
  return true
}

/**
 * Read the org slug from the subdomain, or '' if not on a tenant subdomain.
 * Safe to call before/outside Vue setup (used in router boot logic).
 */
export function orgFromSubdomain() {
  if (typeof window === 'undefined') return ''
  const host = window.location.hostname
  if (!isSubdomainHost(host)) return ''
  return host.split('.')[0].toLowerCase()
}

/**
 * Boot-time flag: are we running on a tenant subdomain? Routes are registered
 * differently depending on this.
 */
export function isSubdomainMode() {
  if (typeof window === 'undefined') return false
  return isSubdomainHost(window.location.hostname)
}

/**
 * Can the current host serve tenants on subdomains? True for the apex domain
 * (e.g. `isms.sh`) and existing tenant subdomains. False for localhost, IP
 * literals, container hosts, and any single-label hostname.
 *
 * Use this to decide whether to redirect to `<slug>.<apex>/...` or stay on
 * path-based routing `/<slug>/...` when navigating into an org.
 */
export function canHostSubdomain(hostname) {
  const host = ((hostname || '').toLowerCase()).split(':')[0]
  if (!host) return false
  if (LOCAL_HOSTS.has(host)) return false
  if (_apexHosts.has(host)) return true
  if (isSubdomainHost(host)) return true
  return false
}

/**
 * The apex domain portion of the current host, for building tenant subdomain
 * URLs. Returns '' when the host can't serve subdomains.
 *
 *   apexDomainFromHost('isms.sh')          → 'isms.sh'
 *   apexDomainFromHost('unidoc.isms.sh')   → 'isms.sh'
 *   apexDomainFromHost('www.isms.sh')      → 'isms.sh'
 *   apexDomainFromHost('localhost')        → ''
 */
export function apexDomainFromHost(hostname) {
  const host = ((hostname || '').toLowerCase()).split(':')[0]
  if (!canHostSubdomain(host)) return ''
  // If we're already on a subdomain, strip the first label.
  if (isSubdomainHost(host)) return host.split('.').slice(1).join('.')
  // www.isms.sh → isms.sh
  if (host.startsWith('www.')) return host.slice(4)
  return host
}

/**
 * Server-reported flag (from /api/v1/config) — whether this deployment
 * serves tenant orgs on wildcard subdomains. Set by setSubdomainRouting().
 * Default null (unknown) — callers that need a deterministic value should
 * await ensureConfigLoaded() before navigating, otherwise orgEntryURL
 * conservatively falls back to path-based.
 */
let _subdomainRoutingEnabled = null

let _configPromise = null

/**
 * Block until /api/v1/config has resolved (or rejected). Safe to call many
 * times — the underlying fetch only happens once per page load. Use this
 * before any code path that calls orgEntryURL on a fresh navigation
 * (org picker, switchOrg, deep-link entry).
 */
export function ensureConfigLoaded(loader) {
  if (_configPromise) return _configPromise
  _configPromise = Promise.resolve().then(() => loader && loader()).catch(() => null)
  return _configPromise
}

export function setSubdomainRouting(enabled) {
  _subdomainRoutingEnabled = !!enabled
}

/**
 * Build the canonical URL for navigating into an org.
 *
 * Rules:
 *   - If subdomain routing is disabled for this deployment (demo / dev)
 *     → always path-based: `/<slug>/<path>`.
 *   - If user is already on a tenant subdomain → hop to `<slug>.<apex>/<path>`.
 *   - If user is on the apex (path mode) → stay on apex, path-based.
 *
 * The point of the last rule is that a session's routing mode is determined
 * by how the user entered — landing on `isms.sh` stays on `isms.sh/<slug>`,
 * landing on `<existing>.isms.sh` stays on subdomain. Predictable for users.
 */
export function orgEntryURL(slug, suffix) {
  const path = suffix ? (suffix.startsWith('/') ? suffix : '/' + suffix) : '/overview'
  if (typeof window === 'undefined') return '/' + slug + path
  // Safe-fallback: if config hasn't resolved yet, assume path-based. Demo /
  // dev deployments break with the wrong default; production stays on path
  // until config arrives, then switches behaviour for subsequent navigations.
  if (_subdomainRoutingEnabled !== true) return '/' + slug + path
  const host = window.location.hostname
  const apex = apexDomainFromHost(host)
  if (!apex) return '/' + slug + path // localhost / IP / single-label
  // On apex (not yet on a subdomain) — stay path-based.
  if (!isSubdomainHost(host)) return '/' + slug + path
  // Already on a tenant subdomain — hop to the new org's subdomain.
  const port = window.location.port ? ':' + window.location.port : ''
  return window.location.protocol + '//' + slug + '.' + apex + port + path
}

/**
 * Build an org-scoped path from OUTSIDE Vue setup (e.g. a global DOM
 * listener). Subdomain mode returns the suffix as-is; path-based mode
 * prepends the current `:org` route param read from the registered router.
 *
 *   currentOrgPath('/risks/RISK-5')
 *     → on subdomain: '/risks/RISK-5'
 *     → on path mode: '/<slug>/risks/RISK-5'
 *
 * Falls back to the bare suffix if no router is registered yet or no org
 * slug is resolvable.
 */
export function currentOrgPath(suffix) {
  const s = suffix || ''
  const clean = s.startsWith('/') ? s : '/' + s
  if (isSubdomainMode()) return clean
  const sub = orgFromSubdomain()
  if (sub) return clean
  const slug = _routerRef?.currentRoute?.value?.params?.org || ''
  if (!slug) return clean
  // Idempotent: don't double-prefix if the path is already org-scoped.
  if (clean === '/' + slug || clean.startsWith('/' + slug + '/')) return clean
  return '/' + slug + clean
}

/**
 * Composable: resolves the active org slug from either the subdomain or the
 * `:org` route param. Returns reactive refs plus an `orgPath` helper for
 * building canonical in-app URLs.
 *
 *   const { orgSlug, orgPath, subdomainMode } = useCurrentOrg()
 *   router.push(orgPath('/risks/RISK-5'))
 *     → on subdomain: /risks/RISK-5
 *     → on apex     : /<slug>/risks/RISK-5
 */
export function useCurrentOrg() {
  const route = useRoute()
  const subdomainSlug = orgFromSubdomain()
  const subdomainMode = isSubdomainMode()

  const orgSlug = computed(() => {
    if (subdomainSlug) return subdomainSlug
    return route?.params?.org || ''
  })

  /**
   * Build an org-scoped path. Input is the path SUFFIX with leading slash,
   * e.g. `/risks/RISK-5`. On subdomain mode the suffix is returned as-is.
   * On path mode the org slug is prepended.
   */
  function orgPath(suffix) {
    const s = suffix || ''
    const clean = s.startsWith('/') ? s : '/' + s
    if (subdomainMode) return clean
    const slug = orgSlug.value
    if (!slug) return clean
    return '/' + slug + clean
  }

  return {
    orgSlug,
    orgPath,
    subdomainMode,
  }
}

export default useCurrentOrg
