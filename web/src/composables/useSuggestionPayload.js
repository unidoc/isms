// Shared preview for a suggestion's proposed values.
//
// Both the Inbox and the entity Suggestions panel preview a payload, and
// `update` payloads nest their values under `fields` (#198). This seam turns
// a raw payload (string or object, `fields`-wrapped or flat) into a flat list
// of labelled rows so both call sites render identically.
import { i18n } from '../i18n.js'
import { regionLabel } from './useFormat.js'

// A module-scope label map is invisible to both scanners, which is exactly why
// it survived this long. Keys are spelled out rather than built from the field
// name so the keyset walk can see them.
const PAYLOAD_FIELD_KEYS = {
  title: 'inbox.suggestions.payload_field.title',
  name: 'inbox.suggestions.payload_field.name',
  description: 'inbox.suggestions.payload_field.description',
  category: 'inbox.suggestions.payload_field.category',
  risk_type: 'inbox.suggestions.payload_field.risk_type',
  origin: 'inbox.suggestions.payload_field.origin',
  severity: 'inbox.suggestions.payload_field.severity',
  priority: 'inbox.suggestions.payload_field.priority',
  status: 'inbox.suggestions.payload_field.status',
  type: 'inbox.suggestions.payload_field.type',
  criticality: 'inbox.suggestions.payload_field.criticality',
  jurisdiction: 'inbox.suggestions.payload_field.jurisdiction',
  classification: 'inbox.suggestions.payload_field.classification',
  finding_type: 'inbox.suggestions.payload_field.finding_type',
  risk_level: 'inbox.suggestions.payload_field.risk_level',
  source: 'inbox.suggestions.payload_field.source',
  incident_type: 'inbox.suggestions.payload_field.incident_type',
  current_likelihood: 'inbox.suggestions.payload_field.current_likelihood',
  current_impact: 'inbox.suggestions.payload_field.current_impact',
}

function isPlainObject(v) {
  return v !== null && typeof v === 'object' && !Array.isArray(v)
}

function labelFor(key) {
  return PAYLOAD_FIELD_KEYS[key] ? i18n.global.t(PAYLOAD_FIELD_KEYS[key]) : key.replace(/_/g, ' ')
}

function valueFor(key, v) {
  // `jurisdiction` holds a region code or sentinel, so it renders through the
  // same seam the legal register uses, at any nesting depth.
  return key === 'jurisdiction' ? regionLabel(v) : v
}

// Renders an array element (used by arrays of plain objects) as the
// space-joined non-empty values of that element.
// A `{type, id}` link reads as "risk RISK-3" whatever order its keys arrive
// in: Postgres JSONB re-sorts object keys, so the stored order is `id, type`.
function renderArrayObject(obj) {
  if (obj.type && obj.id) return `${obj.type} ${obj.id}`
  return Object.values(obj)
    .filter((v) => v !== null && v !== '' && v !== undefined)
    .join(' ')
}

function isEmpty(v) {
  if (v === null || v === undefined || v === '') return true
  if (Array.isArray(v)) return v.length === 0
  if (isPlainObject(v)) return Object.keys(v).length === 0
  return false
}

// Expands one level of a plain object into rows. `depth` tracks how many
// plain-object levels have already been unwrapped, so nesting deeper than one
// level (R5) renders as a single JSON-stringified row instead of recursing
// further.
function expand(entries, depth, parentKey, parentLabel) {
  const rows = []
  for (const [k, v] of entries) {
    if (isEmpty(v)) continue

    const key = parentKey ? `${parentKey}.${k}` : k
    const label = parentLabel ? `${parentLabel} · ${labelFor(k)}` : labelFor(k)

    if (Array.isArray(v)) {
      const value = v.every((el) => isPlainObject(el))
        ? v.map(renderArrayObject).join(', ')
        : v.join(', ')
      rows.push({ key, label, value })
      continue
    }

    if (isPlainObject(v)) {
      if (depth === 0 && k === 'fields') {
        // R3: `fields` unwraps without a "fields" prefix in the label.
        rows.push(...expand(Object.entries(v), depth + 1, key, ''))
      } else if (depth === 0) {
        // R4: any other top-level object expands with a parent-label prefix.
        rows.push(...expand(Object.entries(v), depth + 1, key, labelFor(k)))
      } else {
        // R5: nested deeper than one level — stop recursing, stringify.
        rows.push({ key, label, value: JSON.stringify(v) })
      }
      continue
    }

    rows.push({ key, label, value: valueFor(k, v) })
  }
  return rows
}

// Takes the raw payload (not the suggestion) and returns
// Array<{ key: string, label: string, value: string | number | boolean }>.
export function payloadFields(payload) {
  if (!payload) return []
  try {
    const obj = typeof payload === 'string' ? JSON.parse(payload) : payload
    if (!isPlainObject(obj)) return []
    return expand(Object.entries(obj), 0, '', '')
  } catch {
    return []
  }
}
