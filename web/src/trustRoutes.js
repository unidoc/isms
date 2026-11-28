// The public trust center routes (#519). Kept apart from router.js so a test can
// load the real table: router.js reaches the whole app and cannot load under
// node. Always public: a visitor has no session.
//
// Path mode keeps the organization in the path (/:org/trust); on a tenant host
// the organization is implicit and the routes sit at the top level.
const TrustCenter = () => import('./views/TrustCenter.vue')

export function buildTrustRoutes(subdomainMode) {
  const prefix = subdomainMode ? '' : '/:org'
  return [
    { path: `${prefix}/trust`, component: TrustCenter, meta: { public: true, trust: true } },
    // Where the emailed sign-in link lands (#524); the token is in the fragment.
    { path: `${prefix}/trust/sign-in`, component: TrustCenter, meta: { public: true, trust: true, signIn: true } },
    { path: `${prefix}/trust/documents/:id`, component: TrustCenter, meta: { public: true, trust: true } },
  ]
}

// The routes of a trust domain (#528): the host serves only the trust center, at
// the root. These are exactly the three page shapes the server's gate answers
// (trustHostPage in internal/isms/api/trust_host.go): "/", "/sign-in" and
// "/documents/<one segment>". Anything else is redirected to "/" in the SPA
// (the server already answers 404 to what it does not serve).
export function buildTrustHostRoutes() {
  const meta = { public: true, trust: true, trustHost: true }
  return [
    { path: '/', component: TrustCenter, meta },
    { path: '/sign-in', component: TrustCenter, meta: { ...meta, signIn: true } },
    { path: '/documents/:id', component: TrustCenter, meta },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ]
}
