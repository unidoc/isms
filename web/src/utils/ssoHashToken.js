// The SSO hash handoff (router.js): a server-minted session token arrives in the
// URL fragment as `#token=...` (OIDC and Cloudflare Access redirects) and the
// router's global guard stores it as the member API token.
//
// That guard runs on EVERY route before any component mounts, so two rules keep
// it from ever swallowing a credential that is not a member session token:
//   - the key must be exactly `token` (parsed, not a substring match), so
//     `#visitor_token=...` is never taken; and
//   - a trust center route (meta.trust) never hands off at all. The trust
//     center's visitor sign-in link carries its own credential in the fragment,
//     which must reach the page untouched and must never become a member session.
//
// Returns the token to store, or null when the route must not hand off.
export function ssoHashToken(to) {
  if (!to || !to.hash || (to.meta && to.meta.trust)) return null
  const token = new URLSearchParams(to.hash.replace(/^#/, '')).get('token')
  return token || null
}
