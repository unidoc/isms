// Step 6 of plan 82: the stored key+params become translated text.
//
// The contract these pin is split across two repos' worth of code — Go writes
// the wire keys, the locale files carry the frames — and a mismatch is silent:
// an unmapped key falls back to the stored English, which looks correct in
// review and never translates. So every key Go can emit is rendered here.
import test from 'node:test'
import assert from 'node:assert/strict'
import { notificationTitle, notificationBody } from '../src/composables/useNotificationRender.js'
import { i18n } from '../src/i18n.js'
import id from '../src/locales/id-ID/index.js'

i18n.global.setLocaleMessage('id-ID', id)

function withLocale(tag, fn) {
  const previous = i18n.global.locale.value
  i18n.global.locale.value = tag
  try {
    return fn()
  } finally {
    i18n.global.locale.value = previous
  }
}

// One sample row per title key Go emits, with the params that site actually
// passes. Mirrors the call sites in api_collab.go, api_corrective.go,
// api_incidents.go and api_suggestions.go.
const ROWS = {
  'notifications.mention_comment': { actor: 'ana@x.io' },
  'notifications.mention_review_comment': { actor: 'ana@x.io' },
  'notifications.mention_change_request': { actor: 'ana@x.io' },
  'notifications.review_forwarded': { actor: 'ana@x.io', doc_id: 'iso27001-4-1', title: 'Context', version: '2' },
  'notifications.review_requested': { actor: 'ana@x.io', title: 'Context', version: '2' },
  'notifications.review_resubmitted': { actor: 'ana@x.io', doc_id: 'iso27001-4-1', title: 'Context', version: '2' },
  'notifications.ai_review_escalated': { doc_id: 'iso27001-4-1', round: 3 },
  'notifications.ca_assigned': { title: 'Patch the gateway' },
  'notifications.ca_resolved': { title: 'Patch the gateway', id: 12, actor: 'ana@x.io' },
  'notifications.suggestion_new': { actor: 'ana@x.io', title: 'Raise the score', suggestion_type: 'update', entity: 'risk' },
  'notifications.suggestion_resolved': { action: 'applied' },
  'notifications.incident_new': { severity: 'critical', title: 'Phishing wave' },
  'notifications.incident_status': { status: 'resolved', title: 'Phishing wave', id: 7 },
}

const BODY_ROWS = {
  'notifications.review_forwarded.body': ROWS['notifications.review_forwarded'],
  'notifications.review_forwarded.body_with_note': { ...ROWS['notifications.review_forwarded'], note: 'please look' },
  'notifications.review_requested.body': ROWS['notifications.review_requested'],
  'notifications.review_requested.body_with_note': { ...ROWS['notifications.review_requested'], note: 'please look' },
  'notifications.review_resubmitted.body': ROWS['notifications.review_resubmitted'],
  'notifications.review_resubmitted.body_with_note': { ...ROWS['notifications.review_resubmitted'], note: 'please look' },
  'notifications.ai_review_escalated.body': ROWS['notifications.ai_review_escalated'],
  'notifications.ca_resolved.body': ROWS['notifications.ca_resolved'],
  'notifications.suggestion_new.body': ROWS['notifications.suggestion_new'],
  'notifications.suggestion_applied.body': { title: 'Raise the score', actor: 'ana@x.io', entity: 'risk', id: 'R-4' },
  'notifications.suggestion_rejected.body': { title: 'Raise the score', actor: 'ana@x.io', reason: 'out of scope' },
  'notifications.incident_status.body': ROWS['notifications.incident_status'],
}

const STORED = 'STORED ENGLISH'

test('every title key Go emits renders from the catalogue', () => {
  for (const [key, params] of Object.entries(ROWS)) {
    const out = notificationTitle({ title_key: key, title: STORED, params })
    assert.notEqual(out, STORED, `${key} fell back to the stored title — no message for it`)
    assert.ok(out.length > 0, `${key} rendered empty`)
    assert.ok(!out.includes('{'), `${key} left an uninterpolated placeholder: ${out}`)
  }
})

test('every body key Go emits renders from the catalogue', () => {
  for (const [key, params] of Object.entries(BODY_ROWS)) {
    const out = notificationBody({ body_key: key, body: STORED, params })
    assert.notEqual(out, STORED, `${key} fell back to the stored body — no message for it`)
    assert.ok(!out.includes('{'), `${key} left an uninterpolated placeholder: ${out}`)
  }
})

// vue-i18n falls back to `en` for a missing id-ID message, so "not the stored
// English" is not enough here — an untranslated key would render the English
// frame and pass. Comparing against the en render is what actually catches it.
test('the same keys all render in id-ID', () => {
  const cases = [
    ...Object.entries(ROWS).map(([key, params]) => [key, params, (p) => notificationTitle({ title_key: key, title: STORED, params: p })]),
    ...Object.entries(BODY_ROWS).map(([key, params]) => [key, params, (p) => notificationBody({ body_key: key, body: STORED, params: p })]),
  ]
  for (const [key, params, render] of cases) {
    const english = render(params)
    const out = withLocale('id-ID', () => render(params))
    assert.notEqual(out, STORED, `${key} has no id-ID message`)
    assert.notEqual(out, english, `${key} rendered the English frame in id-ID — the message is missing`)
    assert.ok(!out.includes('{'), `${key} left a placeholder in id-ID: ${out}`)
  }
})

// The half-translated trap plan 82 was written to avoid: an enum param spliced
// in raw yields "Insiden resolved: …".
test('enum params are translated before interpolation, not spliced raw', () => {
  const row = { title_key: 'notifications.incident_status', title: STORED, params: ROWS['notifications.incident_status'] }
  assert.equal(notificationTitle(row), 'Incident resolved: Phishing wave')
  withLocale('id-ID', () => {
    const out = notificationTitle(row)
    assert.ok(!out.includes('resolved'), `raw enum value survived into id-ID: ${out}`)
    assert.ok(out.includes('selesai ditangani'), `expected the id-ID status label, got: ${out}`)
  })
})

test('the entity param resolves through common.entity.*', () => {
  withLocale('id-ID', () => {
    const out = notificationBody({
      body_key: 'notifications.suggestion_applied.body',
      body: STORED,
      params: BODY_ROWS['notifications.suggestion_applied.body'],
    })
    assert.ok(out.includes('risiko'), `entity was not translated: ${out}`)
    assert.ok(!out.includes('risk'), `raw entity value survived: ${out}`)
  })
})

test('verbatim params pass through untouched', () => {
  const note = 'don’t ship — "as-is" per §4'
  const out = notificationBody({
    body_key: 'notifications.review_forwarded.body_with_note',
    body: STORED,
    params: { ...ROWS['notifications.review_forwarded'], note },
  })
  assert.ok(out.includes(note), `user text was altered: ${out}`)
})

test('a row with no key renders the stored English', () => {
  assert.equal(notificationTitle({ title: 'AI review escalated' }), 'AI review escalated')
  assert.equal(notificationBody({ body: 'raw description' }), 'raw description')
})

test('an unmapped key renders the stored English rather than the key', () => {
  const out = notificationTitle({ title_key: 'notifications.invented_later', title: STORED, params: {} })
  assert.equal(out, STORED)
})

test('a body that is org content keeps its stored text', () => {
  // api_incidents.go passes inc.Description with no BodyKey, on purpose.
  const out = notificationBody({ title_key: 'notifications.incident_new', body: 'attacker mailed staff', params: {} })
  assert.equal(out, 'attacker mailed staff')
})

test('missing params and null rows degrade instead of throwing', () => {
  assert.equal(notificationTitle(null), '')
  assert.equal(notificationBody(undefined), '')
  const out = notificationTitle({ title_key: 'notifications.ca_assigned', title: STORED })
  assert.ok(typeof out === 'string')
})
