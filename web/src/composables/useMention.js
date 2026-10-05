import { currentOrgPath } from './useCurrentOrg.js'
import { programKeys, loadProgramKeys } from './usePrograms.js'
import { linkEntityReferences } from './entityReferenceLinks.js'

// linkEntityRefs links #RISK-1 / #KEY-1 / #KEY references in escaped HTML.
// For a surface that renders its own @-mentions (Documents.vue); everything
// else should call renderMention.
export function linkEntityRefs(escaped) {
  loadProgramKeys() // fire-and-forget; no-op after the first load
  return linkEntityReferences(escaped, { programKeys: programKeys.value, orgPath: currentOrgPath })
}

export function renderMention(body) {
  if (!body) return ''

  let out = body.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  out = linkEntityRefs(out)

  // User mentions: @email / @handle.
  out = out.replace(/@([\w.+-]+@[\w.-]+)/g, '<span class="text-blue-400 font-medium">@$1</span>')
    .replace(/@(\w+)/g, '<span class="text-blue-400 font-medium">@$1</span>')

  return out
}
