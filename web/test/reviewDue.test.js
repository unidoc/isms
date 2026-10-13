// #448: a closed risk must not be flagged as overdue for review. The rule is
// one pure predicate shared by Risks.vue and the Dashboard, tested here.
import test from 'node:test'
import assert from 'node:assert/strict'
import { isRiskReviewOverdue } from '../src/utils/reviewDue.js'

const NOW = new Date('2026-10-13T12:00:00Z')
const PAST = '2026-04-24'
const FUTURE = '2027-04-24'

test('a closed risk with a past date is not overdue', () => {
  assert.equal(isRiskReviewOverdue({ status: 'closed', next_review: PAST }, NOW), false)
})

test('open, draft and accepted risks with a past date are overdue', () => {
  for (const status of ['open', 'draft', 'accepted']) {
    assert.equal(isRiskReviewOverdue({ status, next_review: PAST }, NOW), true, status)
  }
})

test('a future date is never overdue, whatever the status', () => {
  for (const status of ['open', 'draft', 'accepted', 'closed']) {
    assert.equal(isRiskReviewOverdue({ status, next_review: FUTURE }, NOW), false, status)
  }
})

test('a missing next_review is not overdue', () => {
  assert.equal(isRiskReviewOverdue({ status: 'open' }, NOW), false)
  assert.equal(isRiskReviewOverdue({ status: 'open', next_review: null }, NOW), false)
})

test('epoch seconds and ISO strings are both parsed', () => {
  const past = Date.parse('2026-04-24T00:00:00Z') / 1000
  const future = Date.parse('2027-04-24T00:00:00Z') / 1000
  assert.equal(isRiskReviewOverdue({ status: 'open', next_review: past }, NOW), true)
  assert.equal(isRiskReviewOverdue({ status: 'open', next_review: future }, NOW), false)
  assert.equal(isRiskReviewOverdue({ status: 'open', next_review: '2026-04-24T00:00:00Z' }, NOW), true)
})

test('the now argument is honoured', () => {
  const risk = { status: 'open', next_review: '2026-06-01' }
  assert.equal(isRiskReviewOverdue(risk, new Date('2026-05-01')), false)
  assert.equal(isRiskReviewOverdue(risk, new Date('2026-07-01')), true)
})
