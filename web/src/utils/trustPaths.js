// Where the public trust center lives, for the page and its links.
//
// root is the page itself (the back link, "sign in" returns there); prefix is
// what sub-paths hang off (`${prefix}/documents/<id>`, `${prefix}/sign-in`):
//   trust domain:  root "/"            prefix ""
//   tenant host:   root "/trust"       prefix "/trust"
//   path mode:     root "/<slug>/trust" prefix "/<slug>/trust"
export function trustPaths({ trustHost, subdomain, slug }) {
  if (trustHost) return { root: '/', prefix: '' }
  const base = subdomain ? '/trust' : `/${encodeURIComponent(slug)}/trust`
  return { root: base, prefix: base }
}
