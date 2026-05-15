// Shared slash-command definitions used by DocumentEditor (tiptap) and
// MarkdownField (textarea). Single source of truth for command list, labels,
// shorthands, icons, and per-command behaviour.
//
// Two flavours of command:
//   - markdown commands  → { markdown: '# ', cursorOffset?: N }
//                          textarea inserts the literal markdown; tiptap maps
//                          the same id to a tiptap chain action.
//   - picker commands    → { picker: { type, label, fetchKey, linkPath } }
//                          opens an inline entity picker; both editors call
//                          the named api method, then insert a link.

import { api } from '../api.js'

export const slashCommands = [
  { id: 'h1',       label: 'Heading 1',     shorthand: 'h1',       desc: 'Large section heading',     icon: 'h1',      markdown: '# ' },
  { id: 'h2',       label: 'Heading 2',     shorthand: 'h2',       desc: 'Medium section heading',    icon: 'h2',      markdown: '## ' },
  { id: 'h3',       label: 'Heading 3',     shorthand: 'h3',       desc: 'Small section heading',     icon: 'h3',      markdown: '### ' },
  { id: 'bullet',   label: 'Bullet List',   shorthand: 'ul',       desc: 'Unordered list',            icon: 'list',    markdown: '- ' },
  { id: 'ordered',  label: 'Numbered List', shorthand: 'ol',       desc: 'Ordered list',              icon: 'list-ol', markdown: '1. ' },
  { id: 'quote',    label: 'Blockquote',    shorthand: null,       desc: 'Indented quote block',      icon: 'quote',   markdown: '> ' },
  { id: 'code',     label: 'Code Block',    shorthand: null,       desc: 'Fenced code block',         icon: 'code',    markdown: '```\n\n```\n', cursorOffset: 4 },
  { id: 'hr',       label: 'Divider',       shorthand: 'hr',       desc: 'Horizontal rule',           icon: 'hr',      markdown: '\n---\n' },
  // Entity pickers
  { id: 'risk',     label: 'Risk',     shorthand: 'risk',     desc: 'Link to risk register',     icon: 'link', picker: { type: 'risk',     label: 'Risk',              fetchKey: 'getRisks',        linkPath: '/risks/'     } },
  { id: 'legal',    label: 'Legal',    shorthand: 'legal',    desc: 'Link to legal requirement', icon: 'link', picker: { type: 'legal',    label: 'Legal Requirement', fetchKey: 'getLegal',        linkPath: '/legal/'     } },
  { id: 'asset',    label: 'Asset',    shorthand: 'asset',    desc: 'Link to asset register',    icon: 'link', picker: { type: 'asset',    label: 'Asset',             fetchKey: 'getAssets',       linkPath: '/assets/'    } },
  { id: 'supplier', label: 'Supplier', shorthand: 'supplier', desc: 'Link to supplier',          icon: 'link', picker: { type: 'supplier', label: 'Supplier',          fetchKey: 'getSuppliers',    linkPath: '/suppliers/' } },
  { id: 'system',   label: 'System',   shorthand: 'system',   desc: 'Link to IT system',         icon: 'link', picker: { type: 'system',   label: 'System',            fetchKey: 'getSystems',      linkPath: '/systems/'   } },
  { id: 'doc',      label: 'Document', shorthand: 'doc',      desc: 'Link to document',          icon: 'link', picker: { type: 'document', label: 'Document',          fetchKey: 'getAllDocuments', linkPath: '/documents/', usesFolders: true } },
  { id: 'incident', label: 'Incident', shorthand: 'incident', desc: 'Link to incident',          icon: 'link', picker: { type: 'incident', label: 'Incident',          fetchKey: 'getIncidents',    linkPath: '/incidents/' } },
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

// Filter slashCommands by user query (matches label/id/desc/shorthand).
export function filterSlashCommands(query) {
  const q = (query || '').toLowerCase()
  if (!q) return slashCommands
  return slashCommands.filter(c =>
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
