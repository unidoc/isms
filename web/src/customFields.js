// Shared risk-custom-field helpers (#213 / #216). Kept out of the views so
// these rules stay unit-testable — see test/customFields.test.js. Mirrors
// riskCategories.js, which exists for the same reason.

import { slugifyCategory } from './riskCategories.js'

// Slug for a new custom field key, matching the server's
// ^[a-z0-9]+(_[a-z0-9]+)*$ rule. Reuses the category slugifier — same shape.
export function slugifyFieldKey(label) {
  return slugifyCategory(label)
}

// Seed an empty values object for the create form: one key per def, so
// v-model bindings always have a defined (blank) target.
export function emptyCustomFieldValues(defs) {
  const out = {}
  for (const def of defs || []) {
    if (!def || !def.key) continue
    out[def.key] = def.type === 'number' ? null : ''
  }
  return out
}

// The entries in values that have no matching definition — i.e. left behind
// by a deleted custom field definition. These must remain visible: that is
// the whole point of the orphan-not-cascade rule.
export function orphanCustomValues(defs, values) {
  const keys = new Set((defs || []).filter(d => d && d.key).map(d => d.key))
  const out = {}
  for (const [k, v] of Object.entries(values || {})) {
    if (!keys.has(k) && v !== null && v !== undefined && v !== '') {
      out[k] = v
    }
  }
  return out
}

// De-slug an orphaned key for display, mirroring deslugCategory.
export function deslugFieldKey(key) {
  return (key || '').replace(/_/g, ' ')
}

// Maps a custom field type to an <input type>. select is handled separately
// by the caller with a <select> element.
export function customFieldInputType(type) {
  switch (type) {
    case 'number':
      return 'number'
    case 'date':
      return 'date'
    default:
      return 'text'
  }
}

// True when every required def in defs has a non-empty value in values.
// Mirrors the server's ValidateCustomFieldValues(checkRequired=true) rule for
// what counts as "set": non-nil, and non-empty after trim for strings.
export function requiredCustomFieldsSatisfied(defs, values) {
  for (const def of defs || []) {
    if (!def || !def.required) continue
    const v = (values || {})[def.key]
    if (v === null || v === undefined) return false
    if (typeof v === 'string' && v.trim() === '') return false
  }
  return true
}
