// Shared risk-category helpers (#213). Kept out of the views so the label
// resolution rules are unit-testable — see test/riskCategories.test.js.

// Built-in categories, mirroring db.DefaultRiskCategories(). Used as the fallback
// so a picker is never empty when GET /risks/categories cannot be reached.
export const DEFAULT_RISK_CATEGORIES = [
  { key: 'people_process', label: 'People & Process' },
  { key: 'technology', label: 'Technology' },
  { key: 'third_party', label: 'Third Party' },
  { key: 'legal_regulatory', label: 'Legal & Regulatory' },
  { key: 'physical_environmental', label: 'Physical & Environmental' },
  { key: 'business_continuity', label: 'Business Continuity' },
  { key: 'governance', label: 'Governance' },
  { key: 'quality_operations', label: 'Quality & Operations' },
]

// De-slug a key for display: the fallback when no definition is available.
export function deslugCategory(key) {
  return (key || '').replace(/_/g, ' ')
}

// Display name for a stored category key.
//
// Resolves against the org's configured list so an admin-authored label is what
// the user sees — including punctuation the slug cannot carry ("Cloud / SaaS"
// stores as `cloud_saas`). Falls back to the de-slugged key for ORPHANS:
// categories the org has since removed, which existing risks still legitimately
// hold and must keep displaying.
export function categoryLabel(key, categories) {
  if (!key) return ''
  const hit = (categories || []).find(c => c && c.key === key)
  return hit && hit.label ? hit.label : deslugCategory(key)
}

// Slug for a new category, matching the server's ^[a-z0-9]+(_[a-z0-9]+)*$ rule.
export function slugifyCategory(label) {
  return (label || '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '_')
    .replace(/^_+|_+$/g, '')
    .slice(0, 64)
    .replace(/^_+|_+$/g, '')
}
