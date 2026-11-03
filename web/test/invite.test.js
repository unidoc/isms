import test from 'node:test'
import assert from 'node:assert/strict'
import { inviteButtonTypes } from '../src/utils/invite.js'

test('exactly one invite button submits the form, and it is never the disabled one', () => {
  const on = inviteButtonTypes(true)
  assert.deepEqual(on, { email: 'submit', link: 'button' })
  const off = inviteButtonTypes(false)
  assert.deepEqual(off, { email: 'button', link: 'submit' })
})
