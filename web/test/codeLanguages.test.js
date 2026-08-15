import test from 'node:test'
import assert from 'node:assert/strict'
import { codeLanguages } from '../src/components/codeLanguages.js'

test('offers Mermaid as an editable source language', () => {
  assert.deepEqual(
    codeLanguages.find(language => language.id === 'mermaid'),
    { id: 'mermaid', label: 'Mermaid' },
  )
})
