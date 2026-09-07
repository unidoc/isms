// Shared slash-command definitions used by DocumentEditor (tiptap) and
// MarkdownField (textarea). Single source of truth for command list, labels,
// shorthands, icons, and per-command behaviour.
//
// The definitions hold message KEYS, not display strings, and
// `translatedSlashCommands()` resolves them. Two reasons the labels cannot
// live here as literals: this is a .js module, so the raw-text scanner never
// reads it and English here is invisible to the ratchet; and the array is
// built once at module evaluation, before the locale resolves, so literals
// would stay English after a locale switch even in a component that is
// otherwise fully translated. `t` comes from the global scope because
// `useI18n()` requires a component instance and there is none here.
//
// Two flavours of command:
//   - markdown commands  → { markdown: '# ', cursorOffset?: N }
//                          textarea inserts the literal markdown; tiptap maps
//                          the same id to a tiptap chain action.
//   - picker commands    → { picker: { type, label, fetchKey, linkPath } }
//                          opens an inline entity picker; both editors call
//                          the named api method, then insert a link.

import { api } from '../api.js'
import { t } from '../i18n.js'

export const slashCommands = [
  { id: 'h1',       labelKey: 'components.slash.h1.label',       shorthand: 'h1',       descKey: 'components.slash.h1.desc',       icon: 'h1',      markdown: '# ' },
  { id: 'h2',       labelKey: 'components.slash.h2.label',       shorthand: 'h2',       descKey: 'components.slash.h2.desc',       icon: 'h2',      markdown: '## ' },
  { id: 'h3',       labelKey: 'components.slash.h3.label',       shorthand: 'h3',       descKey: 'components.slash.h3.desc',       icon: 'h3',      markdown: '### ' },
  { id: 'bullet',   labelKey: 'components.slash.bullet.label',   shorthand: 'ul',       descKey: 'components.slash.bullet.desc',   icon: 'list',    markdown: '- ' },
  { id: 'ordered',  labelKey: 'components.slash.ordered.label',  shorthand: 'ol',       descKey: 'components.slash.ordered.desc',  icon: 'list-ol', markdown: '1. ' },
  { id: 'quote',    labelKey: 'components.slash.quote.label',    shorthand: null,       descKey: 'components.slash.quote.desc',    icon: 'quote',   markdown: '> ' },
  { id: 'code',     labelKey: 'components.slash.code.label',     shorthand: null,       descKey: 'components.slash.code.desc',     icon: 'code',    markdown: '```\n\n```\n', cursorOffset: 4 },
  { id: 'hr',       labelKey: 'components.slash.hr.label',       shorthand: 'hr',       descKey: 'components.slash.hr.desc',       icon: 'hr',      markdown: '\n---\n' },
  // Entity pickers
  { id: 'risk',     labelKey: 'components.slash.risk.label',     shorthand: 'risk',     descKey: 'components.slash.risk.desc',     icon: 'link', picker: { type: 'risk',     labelKey: 'common.entity.risk',              fetchKey: 'getRisks',        linkPath: '/risks/'     } },
  { id: 'legal',    labelKey: 'components.slash.legal.label',    shorthand: 'legal',    descKey: 'components.slash.legal.desc',    icon: 'link', picker: { type: 'legal',    labelKey: 'common.entity.legal_requirement', fetchKey: 'getLegal',        linkPath: '/legal/'     } },
  { id: 'asset',    labelKey: 'components.slash.asset.label',    shorthand: 'asset',    descKey: 'components.slash.asset.desc',    icon: 'link', picker: { type: 'asset',    labelKey: 'common.entity.asset',             fetchKey: 'getAssets',       linkPath: '/assets/'    } },
  { id: 'supplier', labelKey: 'components.slash.supplier.label', shorthand: 'supplier', descKey: 'components.slash.supplier.desc', icon: 'link', picker: { type: 'supplier', labelKey: 'common.entity.supplier',          fetchKey: 'getSuppliers',    linkPath: '/suppliers/' } },
  { id: 'system',   labelKey: 'components.slash.system.label',   shorthand: 'system',   descKey: 'components.slash.system.desc',   icon: 'link', picker: { type: 'system',   labelKey: 'common.entity.system',            fetchKey: 'getSystems',      linkPath: '/systems/'   } },
  { id: 'doc',      labelKey: 'components.slash.doc.label',      shorthand: 'doc',      descKey: 'components.slash.doc.desc',      icon: 'link', picker: { type: 'document', labelKey: 'common.entity.document',          fetchKey: 'getAllDocuments', linkPath: '/documents/', usesFolders: true } },
  { id: 'incident', labelKey: 'components.slash.incident.label', shorthand: 'incident', descKey: 'components.slash.incident.desc', icon: 'link', picker: { type: 'incident', labelKey: 'common.entity.incident',          fetchKey: 'getIncidents',    linkPath: '/incidents/' } },
]

// Field map used by both editors when a picker command resolves a result item
// to (id, name) for link generation.
export const entityFieldMap = {
  risk:     { idField: 'identifier',  nameField: 'title' },
  legal:    { idField: 'identifier',  nameField: 'title' },
  asset:    { idField: 'identifier',  nameField: 'name'  },
  supplier: { idField: 'identifier',  nameField: 'name'  },
  system:   { idField: 'identifier',  nameField: 'name'  },
  document: { idField: 'document_id', nameField: 'title' },
  incident: { idField: 'id',          nameField: 'title' },
}

// Resolve the definitions to display form for the CURRENT locale. Call this
// inside a computed() — the result is a snapshot, and a cached one would be the
// same staleness bug as holding the literals in the array.
export function translatedSlashCommands() {
  return slashCommands.map((c) => ({
    ...c,
    label: t(c.labelKey),
    desc: t(c.descKey),
    picker: c.picker ? { ...c.picker, label: t(c.picker.labelKey) } : undefined,
  }))
}

// Filter by user query (matches label/id/desc/shorthand). Filtering runs over
// the TRANSLATED labels — a reader typing in their own language must match
// what the menu actually shows them.
export function filterSlashCommands(query) {
  const commands = translatedSlashCommands()
  const q = (query || '').toLowerCase()
  if (!q) return commands
  return commands.filter(c =>
    c.label.toLowerCase().includes(q) ||
    c.id.includes(q) ||
    c.desc.toLowerCase().includes(q) ||
    (c.shorthand && c.shorthand.includes(q))
  )
}

// Fetch picker results via the shared api object. Document picker flattens the
// nested folder tree into a flat list of files.
export async function fetchPickerItems(picker) {
  const fn = api[picker.fetchKey]
  if (typeof fn !== 'function') return []
  try {
    const data = await fn()
    if (picker.usesFolders) {
      const folders = Array.isArray(data) ? data : (data?.data || [])
      const flatten = (fs) => (fs || []).flatMap(f => [...(f.files || []), ...flatten(f.subfolders)])
      return flatten(folders)
    }
    return Array.isArray(data) ? data : (data?.data || data?.items || [])
  } catch {
    return []
  }
}

// Resolve an item to (id, name) using entityFieldMap with sensible fallbacks.
export function resolveEntity(type, item) {
  const map = entityFieldMap[type] || {}
  const id   = String(item[map.idField]   || item.identifier || item.document_id || item.id   || '')
  const name = String(item[map.nameField] || item.title      || item.name        || id)
  return { id, name }
}
