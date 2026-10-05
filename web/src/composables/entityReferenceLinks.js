// Turns #RISK-1 / #KEY-1 / #KEY in a comment body into links (#171). Every
// comment surface goes through this one function so a reference renders the
// same everywhere (#194). It takes already-escaped HTML and has no imports, so
// it is unit-testable; useMention.js supplies the live program keys and org
// path.

// Fixed system identifier prefixes → register route (Identifier = "TASK-6" etc.
// from NextIdentifier). Programs and objectives are NOT here: a program is keyed
// by an arbitrary per-org string and an objective's display_id is "<key>-<seq>",
// so they're matched against the live program-key cache instead (#171 review).
export const ENTITY_ROUTES = {
  RISK: 'risks', INC: 'incidents', TASK: 'tasks', CA: 'corrective-actions',
  SUPPLIER: 'suppliers', SYSTEM: 'systems', LEGAL: 'legal', CR: 'changes',
  ASSET: 'assets', AST: 'assets',
}

// linkEntityReferences rewrites references in escaped HTML. programKeys is a
// Set of the org's program keys; orgPath maps a route to its org-scoped URL.
export function linkEntityReferences(escaped, { programKeys, orgPath }) {
  const anchor = (route, label) =>
    `<a href="${orgPath(route)}" class="text-blue-400 font-medium hover:underline">#${label}</a>`
  const span = (label) => `<span class="text-blue-400 font-medium">#${label}</span>`

  // Entity references (single pass so nothing double-matches):
  //   #TASK-6  → fixed-prefix register        (link)
  //   #ISMS-1  → objective (display_id KEY-seq, KEY is a known program key) (link)
  //   #ISMS    → the program itself (bare known key)                       (link)
  //   #other / #other-1 (unknown) → highlighted / untouched
  return escaped.replace(/#([A-Z][A-Z0-9]*)(?:-(\d+))?\b/g, (m, prefix, num) => {
    if (num !== undefined) {
      const ident = `${prefix}-${num}`
      if (ENTITY_ROUTES[prefix]) return anchor(`/${ENTITY_ROUTES[prefix]}/${ident}`, ident)
      if (programKeys.has(prefix)) return anchor(`/objectives/${ident}`, ident)
      return span(ident)
    }
    if (programKeys.has(prefix)) return anchor(`/programs/${prefix}`, prefix)
    return m // arbitrary #word — leave as typed
  })
}
