import { currentOrgPath } from './useCurrentOrg.js'

// Identifier prefix → register route, so an entity reference in a comment links to
// the entity's deep-link page (#166 / #171). Objectives (display_id is program-key
// based, no fixed prefix) and audit items aren't mapped — they render as a
// highlighted reference rather than a link.
const ENTITY_ROUTES = {
  RISK: 'risks', INC: 'incidents', TASK: 'tasks', CA: 'corrective-actions',
  SUPPLIER: 'suppliers', SYSTEM: 'systems', LEGAL: 'legal', CR: 'changes',
  ASSET: 'assets', AST: 'assets', PROG: 'programs',
}

export function renderMention(body) {
  if (!body) return ''
  let out = body.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')

  // Entity references: #TASK-6 → deep-link to the entity (#171). Known prefixes
  // become links; anything else stays a highlighted reference.
  out = out.replace(/#([A-Z][A-Z0-9]*)-(\d+)\b/g, (_m, prefix, num) => {
    const ident = `${prefix}-${num}`
    const route = ENTITY_ROUTES[prefix]
    if (route) {
      const href = currentOrgPath(`/${route}/${ident}`)
      return `<a href="${href}" class="text-blue-400 font-medium hover:underline">#${ident}</a>`
    }
    return `<span class="text-blue-400 font-medium">#${ident}</span>`
  })

  // User mentions: @email / @handle.
  out = out.replace(/@([\w.+-]+@[\w.-]+)/g, '<span class="text-blue-400 font-medium">@$1</span>')
    .replace(/@(\w+)/g, '<span class="text-blue-400 font-medium">@$1</span>')

  return out
}
