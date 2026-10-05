import test from 'node:test'
import assert from 'node:assert/strict'
import { canActOnAssignment } from '../src/utils/assignment.js'

test('admin and manager may act on any item, assigned or not', () => {
  for (const role of ['admin', 'manager']) {
    assert.equal(canActOnAssignment(role, 'a@x.test', 'b@x.test'), true)
    assert.equal(canActOnAssignment(role, 'a@x.test', ''), true)
  }
})

test('contributor only on an item assigned to them, ignoring case', () => {
  assert.equal(canActOnAssignment('contributor', 'a@x.test', 'a@x.test'), true)
  assert.equal(canActOnAssignment('contributor', 'A@X.test', 'a@x.test'), true)
  assert.equal(canActOnAssignment('contributor', 'a@x.test', 'b@x.test'), false)
})

test('contributor with no assignee or no email may not act', () => {
  assert.equal(canActOnAssignment('contributor', 'a@x.test', ''), false)
  assert.equal(canActOnAssignment('contributor', '', 'a@x.test'), false)
})

test('reader may not act, even on their own item', () => {
  assert.equal(canActOnAssignment('reader', 'a@x.test', 'a@x.test'), false)
})
