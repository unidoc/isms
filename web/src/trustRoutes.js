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
    { path: `${prefix}/trust`, component: TrustCenter, meta: { public: true } },
    { path: `${prefix}/trust/documents/:id`, component: TrustCenter, meta: { public: true } },
  ]
}
