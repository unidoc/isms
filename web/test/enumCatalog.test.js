// The `common.enum.*` catalogue is the shared vocabulary: StatusBadge, the
// `<option>` lists extracted per view, and the notification renderer all read
// the same keys. These tests pin the parts of that contract a typo would
// otherwise break silently — a missing key degrades to a de-slugged label,
// which reads plausibly in English and is invisible in review.
import test from 'node:test'
import assert from 'node:assert/strict'
import { enumLabel, enumLabelInline } from '../src/composables/useEnumLabel.js'
import { i18n } from '../src/i18n.js'
import en from '../src/locales/en/index.js'
import id from '../src/locales/id-ID/index.js'

// Every status value any register can store, per the CHECK constraints in
// migrations/ and the enum slices in internal/isms/db. One flat group, because
// the values are distinct across registers and StatusBadge receives a bare
// value with no family attached.
const STATUS = [
  'draft', 'open', 'in_review', 'approved', 'changes_requested', 'proposed_revision',
  'merged', 'retired', 'pending', 'accepted', 'applied', 'rejected', 'withdrawn',
  'resolved', 'closed', 'investigating', 'contained', 'todo', 'in_progress', 'done',
  'cancelled', 'proposed', 'not_started', 'implemented', 'verified', 'planned',
  'completed', 'active', 'at_risk', 'paused', 'complete', 'assessment',
  'awaiting_approval', 'implementation', 'monitoring', 'under_review', 'suspended',
  'terminated', 'decommissioned', 'archived',
]

// The translatable notification params (plan 82's closed set) and the groups the
// four non-status StatusBadge call sites name explicitly.
const GROUPS = {
  status: STATUS,
  severity: ['critical', 'high', 'medium', 'low'],
  criticality: ['critical', 'high', 'medium', 'low'],
  classification: ['public', 'internal', 'confidential', 'restricted'],
  audit_result: ['not_assessed', 'conforming', 'minor_nc', 'major_nc', 'observation', 'opportunity'],
  // corrective_actions.severity and audit_findings.finding_type share this value
  // set. It is NOT `severity`: that group is the incident/priority scale, and
  // two tables happen to have named their column `severity`.
  finding_type: ['major_nc', 'minor_nc', 'observation', 'opportunity'],
  action: ['applied', 'rejected'],
  suggestion_type: ['create', 'update', 'reassess', 'link', 'review', 'reading'],
}

// The suggestion `entity` param, resolved through common.entity.* rather than
// common.enum.* — an entity name is a noun the whole app reuses, not an enum
// member. Matches the entity_type CHECK on `suggestions`.
const ENTITIES = [
  'risk', 'supplier', 'incident', 'legal_requirement', 'change_request',
  'corrective_action', 'objective', 'task', 'system', 'asset', 'audit',
  'audit_finding', 'program', 'checkin', 'access_review',
]

test('every enum member a register can store has a label', () => {
  for (const [group, values] of Object.entries(GROUPS)) {
    for (const value of values) {
      assert.ok(
        en.common.enum[group]?.[value],
        `common.enum.${group}.${value} is missing from locales/en`,
      )
    }
  }
})

test('every suggestion entity type has an entity name', () => {
  for (const e of ENTITIES) {
    assert.ok(en.common.entity[e], `common.entity.${e} is missing from locales/en`)
  }
})

test('id-ID mirrors the catalogue, so no user sees a de-slugged English value', () => {
  for (const [group, values] of Object.entries(GROUPS)) {
    for (const value of values) {
      assert.ok(
        id.common.enum[group]?.[value],
        `common.enum.${group}.${value} is missing from locales/id-ID`,
      )
    }
  }
  for (const e of ENTITIES) {
    assert.ok(id.common.entity[e], `common.entity.${e} is missing from locales/id-ID`)
  }
})

test('labels are looked up per group, so one value can differ by family', async () => {
  assert.equal(enumLabel('status', 'open'), 'Open')
  assert.equal(enumLabel('audit_result', 'major_nc'), 'Major non-conformity')
  // Same member, two families: the de-slugging this seam replaces could not
  // have told these apart.
  assert.equal(enumLabel('suggestion_type', 'review'), 'Review')
  assert.equal(enumLabel('status', 'in_review'), 'In review')
  // A column name is not enough to pick a group: `severity` is the incident
  // scale, while corrective_actions.severity holds the finding taxonomy.
  assert.equal(enumLabel('severity', 'critical'), 'Critical')
  assert.equal(enumLabel('finding_type', 'major_nc'), 'Major non-conformity')
})

test('a lagging locale falls back to the English label, not to de-slugging', () => {
  // te() consults a single locale, so enumLabel probes the fallback catalogue.
  // Registering a locale with an empty catalogue stands in for one that has not
  // caught up with a newly added enum member.
  i18n.global.setLocaleMessage('xx-XX', { common: { enum: {}, entity: {} } })
  const previous = i18n.global.locale.value
  i18n.global.locale.value = 'xx-XX'
  try {
    assert.equal(enumLabel('audit_result', 'major_nc'), 'Major non-conformity')
  } finally {
    i18n.global.locale.value = previous
  }
})

// ─────────────────────────────────────────────────────────────────────────
// Inline forms. `common.enum.*` is the standalone label (badge, <option>);
// a value interpolated into a sentence frame needs the mid-sentence form, or
// notifications read "New Critical incident". The fallback in
// enumLabelInline() is deliberately silent, so only a test can catch a new
// enum member that reintroduces the bug.
// ─────────────────────────────────────────────────────────────────────────

// The closed set a notification param can actually carry. `status` is the six
// incident values only: incident_status is the sole frame interpolating one.
const INLINE_REQUIRED = {
  severity: ['critical', 'high', 'medium', 'low'],
  status: ['draft', 'open', 'investigating', 'contained', 'resolved', 'closed'],
  suggestion_type: ['create', 'update', 'reassess', 'link', 'review', 'reading'],
  action: ['applied', 'rejected'],
}

test('every notification-interpolated enum value has an inline form in en', () => {
  for (const [group, values] of Object.entries(INLINE_REQUIRED)) {
    for (const value of values) {
      const inline = en.common.enum_inline?.[group]?.[value]
      assert.equal(typeof inline, 'string', `common.enum_inline.${group}.${value} is missing in en`)
      assert.ok(inline.length > 0, `common.enum_inline.${group}.${value} is empty in en`)
    }
  }
})

test('every entity name has an inline form in en', () => {
  for (const name of Object.keys(en.common.entity)) {
    const inline = en.common.entity_inline?.[name]
    assert.equal(typeof inline, 'string', `common.entity_inline.${name} is missing in en`)
  }
})

test('id-ID mirrors every inline form it needs', () => {
  for (const [group, values] of Object.entries(INLINE_REQUIRED)) {
    for (const value of values) {
      assert.equal(typeof id.common.enum_inline?.[group]?.[value], 'string',
        `common.enum_inline.${group}.${value} is missing in id-ID`)
    }
  }
  for (const name of Object.keys(en.common.entity_inline)) {
    assert.equal(typeof id.common.entity_inline?.[name], 'string',
      `common.entity_inline.${name} is missing in id-ID`)
  }
})

test('inline forms are used mid-sentence, so they do not lead with a capital', () => {
  // en and id-ID both lowercase mid-sentence. A locale where that is false
  // (German nouns) authors its own strings and this assertion moves with it.
  for (const [group, values] of Object.entries(INLINE_REQUIRED)) {
    for (const value of values) {
      const inline = en.common.enum_inline[group][value]
      assert.equal(inline, inline.toLowerCase(),
        `en common.enum_inline.${group}.${value} should be lowercase mid-sentence`)
    }
  }
})

test('enumLabelInline falls back to the standalone label, not a raw key', () => {
  // `criticality` has no inline group at all.
  assert.equal(enumLabelInline('criticality', 'high'), enumLabel('criticality', 'high'))
  // an unknown member still degrades through enumLabel's humanize()
  assert.equal(enumLabelInline('severity', 'nonexistent'), 'Nonexistent')
})
