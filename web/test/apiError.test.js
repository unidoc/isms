// The server's stable `code` becomes translated text.
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
import { apiErrorFrom } from '../src/api.js'
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
  //
  // `path_pattern`, not `title`: common.field.title is the string "title", so a
  // field lookup that did nothing at all would still pass on it. Every
  // assertion here uses a value whose label differs from its identifier.
  assert.equal(renderApiError(err('required', { field: 'path_pattern' })),
    'path pattern is required')
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

// The two keys this PR adds. Nothing else in the suite renders them, so without
// this they could be absent from id-ID and every test would still pass.
test('the field vocabulary this module needs renders in id-ID', () => {
  withLocale('id-ID', () => {
    assert.match(renderApiError(err('required', { field: 'entity_type' })), /jenis entitas/)
    assert.match(renderApiError(err('required', { field: 'suggestion_type' })), /jenis saran/)
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

// ---- the wire join -------------------------------------------------------
//
// The Go side pins the response shape (TestErrorBodyWireShape) and
// renderApiError() is covered above; apiErrorFrom is the six lines between
// them, and it is also where fetchRaw's behaviour changed.

const response = (body, status = 404, statusText = 'Not Found') =>
  new Response(body === null ? null : JSON.stringify(body), { status, statusText })

test('a converted route carries code and params onto the thrown error', async () => {
  const res = response({ message: 'suggestion not found', code: 'not_found', params: { entity: 'suggestion' } })
  const err = apiErrorFrom(res, await res.json())
  assert.equal(err.message, 'suggestion not found')
  assert.equal(err.status, 404)
  assert.equal(err.code, 'not_found')
  assert.deepEqual(err.params, { entity: 'suggestion' })
  // …and the whole chain: wire -> Error -> translated sentence.
  assert.equal(renderApiError(err), 'suggestion not found')
  withLocale('id-ID', () => assert.match(renderApiError(err), /saran tidak ditemukan/))
})

test('an unconverted route yields an error with no code at all', async () => {
  const res = response({ message: 'readers cannot edit suggestions' }, 403, 'Forbidden')
  const err = apiErrorFrom(res, await res.json())
  assert.equal(err.message, 'readers cannot edit suggestions')
  assert.equal(err.status, 403)
  // Absent, not undefined-valued: renderApiError branches on presence.
  assert.ok(!('code' in err))
  assert.equal(renderApiError(err), 'readers cannot edit suggestions')
})

test('an unparseable error body falls back to the status line', () => {
  const err = apiErrorFrom(response(null, 502, 'Bad Gateway'), null)
  assert.equal(err.message, '502 Bad Gateway')
  assert.equal(err.status, 502)
  assert.ok(!('code' in err))
})

test('the fallback parameter wins over the status line but not over the server', async () => {
  // login's "Login failed (401)" is friendlier than "401 Unauthorized"…
  assert.equal(apiErrorFrom(response(null, 401, 'Unauthorized'), null, 'Login failed (401)').message,
    'Login failed (401)')
  // …but the server's own sentence is better than either.
  const res = response({ message: 'too many attempts, try again later' }, 429, 'Too Many Requests')
  assert.equal(apiErrorFrom(res, await res.json(), 'Login failed (429)').message,
    'too many attempts, try again later')
})
