// #360: a heat map shows current exposure, so a closed risk or legal
// requirement must not be plotted. The rule lives in one module shared by
// HeatMap.vue (Risks, Dashboard and Legal maps) and the Dashboard's
// high/critical count, so it is tested once here rather than through either.
import test from 'node:test'
import assert from 'node:assert/strict'
import { isLiveExposure, plottableItems } from '../src/utils/heatMap.js'

const at = (status, likelihood = 4, impact = 4) =>
  ({ status, current_likelihood: likelihood, current_impact: impact })

test('closed items are not live exposure; draft and open are', () => {
  assert.equal(isLiveExposure(at('closed')), false)
  assert.equal(isLiveExposure(at('open')), true)
  assert.equal(isLiveExposure(at('draft')), true)
  // A payload without a status is plotted, as before, rather than hidden.
  assert.equal(isLiveExposure({ current_likelihood: 3, current_impact: 3 }), true)
})

test('plottableItems drops closed items and keeps the rest in order', () => {
  const open = at('open', 4, 4)
  const draft = at('draft', 2, 5)
  const items = [open, at('closed', 4, 4), draft, at('closed', 1, 1)]
  assert.deepEqual(plottableItems(items), [open, draft])
})

test('plottableItems still drops items the 5×5 grid cannot place', () => {
  const items = [
    at('open', 0, 3), at('open', 3, 6), at('open', null, 3),
    { status: 'open', current_likelihood: 3 }, at('open', 5, 1),
  ]
  assert.deepEqual(plottableItems(items), [at('open', 5, 1)])
})

test('plottableItems tolerates a missing or non-array list', () => {
  assert.deepEqual(plottableItems(undefined), [])
  assert.deepEqual(plottableItems(null), [])
  assert.deepEqual(plottableItems({}), [])
})
