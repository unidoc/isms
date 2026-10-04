// Deleting a folder must not leave it as the tree's active folder (#396).
// The New document form prefills its folder from activeFolder, so a stale
// value would offer a path that no longer exists.
import test from 'node:test'
import assert from 'node:assert/strict'
import { useDocumentTree } from '../src/composables/useDocumentTree.js'

test('clears the active folder when that folder is deleted', () => {
  const { activeFolder, forgetActiveFolder } = useDocumentTree({})
  activeFolder.value = 'policies/drafts'
  forgetActiveFolder('policies/drafts')
  assert.equal(activeFolder.value, '')
})

test('clears the active folder when it sits under the deleted folder', () => {
  const { activeFolder, forgetActiveFolder } = useDocumentTree({})
  activeFolder.value = 'policies/drafts/old'
  forgetActiveFolder('policies/drafts')
  assert.equal(activeFolder.value, '')
})

test('keeps an active folder outside the deleted one, including a name prefix match', () => {
  const { activeFolder, forgetActiveFolder } = useDocumentTree({})
  for (const active of ['policies', 'policies/drafts-2024', 'procedures']) {
    activeFolder.value = active
    forgetActiveFolder('policies/drafts')
    assert.equal(activeFolder.value, active)
  }
})
