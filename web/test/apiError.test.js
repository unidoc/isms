// Task 1.3 of plan 81 §1: the server's stable `code` becomes translated text.
//
// The contract runs across two languages — Go composes the code and params,
// the locale files carry the sentence — and every mismatch is silent, because
// every failure path falls back to the English `message` the server already
// sent. That is the right behaviour and it is also why this needs tests: a
// renderer wired to nothing looks identical to a renderer working perfectly,
// in English.
import test from 'node:test'
import assert from 'node:assert/strict'
import { renderApiError } from '../src/composables/useApiError.js'
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

const err = (code, params, message = 'server english') => ({ code, params, message, status: 400 })

test('a catalogued code renders from the catalogue, not the server message', () => {
  assert.equal(renderApiError(err('not_found', { entity: 'risk' })), 'risk not found')
  assert.equal(renderApiError(err('invalid_entity_id', { entity: 'review' })), 'invalid review id')
  assert.equal(renderApiError(err('required', { field: 'title' })), 'title is required')
})

// The point of the whole seam. An English-only suite passes with the renderer
// disconnected from the locale entirely — the failure #235's review caught in
// the useFormat tests, and the same one applies here.
test('the active locale drives the sentence', () => {
  withLocale('id-ID', () => {
    const out = renderApiError(err('not_found', { entity: 'risk' }))
    assert.notEqual(out, 'risk not found')
    assert.match(out, /tidak ditemukan/)
  })
})

// Splicing the English identifier into a translated frame is the specific bug
// this ordering exists to prevent: the sentence translates, one noun does not,
// and fallbackLocale cannot rescue it because nothing looks missing.
test('params are translated before interpolation, not spliced raw', () => {
  withLocale('id-ID', () => {
    const out = renderApiError(err('not_found', { entity: 'risk' }))
    assert.ok(!out.includes('risk'), `English identifier leaked into: ${out}`)
  })
})

test('every param kind resolves through its own catalogue path', () => {
  // entity -> common.entity_inline, field -> common.field,
  // status -> common.enum_inline.status, count/value -> raw.
  assert.equal(renderApiError(err('review_wrong_status', { status: 'changes_requested' })),
    'review is changes requested')
  assert.equal(renderApiError(err('password_too_short', { count: '7' })),
    'password must be at least 7 characters')
  assert.equal(renderApiError(err('invalid_status', { value: 'draught' })),
    'invalid status: draught')
})

test('value is echoed verbatim even when it collides with an enum member', () => {
  // `value` is the caller's own input. It is not looked up, so a caller who
  // typed a real enum member still sees exactly what they sent.
  withLocale('id-ID', () => {
    assert.match(renderApiError(err('invalid_status', { value: 'approved' })), /approved/)
  })
})

test('an unconverted route renders the server message', () => {
  // No code: the route is still on echo.NewHTTPError. ~191 render sites read
  // err.message directly and must keep working through the whole migration.
  assert.equal(renderApiError({ message: 'something specific', status: 500 }), 'something specific')
})

test('a code this bundle has no key for falls back to the message', () => {
  assert.equal(renderApiError(err('invented_code', { entity: 'risk' })), 'server english')
})

test('an untranslated param value degrades to the raw identifier, not a key path', () => {
  const out = renderApiError(err('not_found', { entity: 'flux_capacitor' }))
  assert.equal(out, 'flux_capacitor not found')
  assert.ok(!out.includes('common.'), `key path leaked into: ${out}`)
})

test('missing params and empty errors degrade instead of throwing', () => {
  assert.equal(renderApiError(null), '')
  assert.equal(renderApiError({ status: 500 }), '')
  // A code whose template has slots, called with no params at all: vue-i18n
  // renders the slot empty rather than throwing, and the sentence survives.
  assert.doesNotThrow(() => renderApiError({ code: 'not_found', message: 'x' }))
})
