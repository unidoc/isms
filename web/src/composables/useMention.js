import { currentOrgPath } from './useCurrentOrg.js'
import { programKeys, loadProgramKeys } from './usePrograms.js'

// Fixed system identifier prefixes → register route (Identifier = "TASK-6" etc.
// from NextIdentifier). Programs and objectives are NOT here: a program is keyed
// by an arbitrary per-org string and an objective's display_id is "<key>-<seq>",
// so they're matched against the live program-key cache instead (#171 review).
const ENTITY_ROUTES = {
  RISK: 'risks', INC: 'incidents', TASK: 'tasks', CA: 'corrective-actions',
  SUPPLIER: 'suppliers', SYSTEM: 'systems', LEGAL: 'legal', CR: 'changes',
  ASSET: 'assets', AST: 'assets',
}

export function renderMention(body) {
  if (!body) return ''
  loadProgramKeys() // fire-and-forget; no-op after the first load
  const keys = programKeys.value

  let out = body.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')

  const anchor = (route, label) =>
    `<a href="${currentOrgPath(route)}" class="text-blue-400 font-medium hover:underline">#${label}</a>`
  const span = (label) => `<span class="text-blue-400 font-medium">#${label}</span>`

  // Entity references (single pass so nothing double-matches):
  //   #TASK-6  → fixed-prefix register        (link)
  //   #ISMS-1  → objective (display_id KEY-seq, KEY is a known program key) (link)
  //   #ISMS    → the program itself (bare known key)                       (link)
  //   #other / #other-1 (unknown) → highlighted / untouched
  out = out.replace(/#([A-Z][A-Z0-9]*)(?:-(\d+))?\b/g, (m, prefix, num) => {
    if (num !== undefined) {
      const ident = `${prefix}-${num}`
      if (ENTITY_ROUTES[prefix]) return anchor(`/${ENTITY_ROUTES[prefix]}/${ident}`, ident)
      if (keys.has(prefix)) return anchor(`/objectives/${ident}`, ident)
      return span(ident)
    }
    if (keys.has(prefix)) return anchor(`/programs/${prefix}`, prefix)
    return m // arbitrary #word — leave as typed
  })

  // User mentions: @email / @handle.
  out = out.replace(/@([\w.+-]+@[\w.-]+)/g, '<span class="text-blue-400 font-medium">@$1</span>')
    .replace(/@(\w+)/g, '<span class="text-blue-400 font-medium">@$1</span>')

  return out
}
