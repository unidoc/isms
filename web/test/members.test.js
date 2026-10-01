import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'

const dom = new JSDOM('<!doctype html><html><body></body></html>', { url: 'https://acme.example/' })
globalThis.window = dom.window
globalThis.document = dom.window.document
globalThis.localStorage = dom.window.localStorage

globalThis.fetch = async () => ({
  ok: true,
  status: 200,
  json: async () => ({
    data: [
      { id: '1', email: 'marlin@acme-corp.com', name: 'Marlin Monroe', role: 'manager' },
      { id: '2', email: 'nameless@acme-corp.com', name: '', role: 'reader' },
    ],
  }),
})

const { useMembers, nameFor, labelFor } = await import('../src/composables/useMembers.js')

test('nameFor and labelFor resolve emails to names and fall back to the full email', async () => {
  const { loadMembers, members } = useMembers()
  // useMembers() starts the load on first use; wait for it to land.
  await loadMembers()
  for (let i = 0; i < 50 && members.value.length === 0; i++) await new Promise(r => setTimeout(r, 10))
  assert.equal(members.value.length, 2)

  assert.equal(nameFor('marlin@acme-corp.com'), 'Marlin Monroe')
  assert.equal(nameFor('nameless@acme-corp.com'), 'nameless@acme-corp.com')
  assert.equal(nameFor('stranger@elsewhere.io'), 'stranger@elsewhere.io')
  assert.equal(nameFor(''), '')

  assert.equal(labelFor('marlin@acme-corp.com'), 'Marlin Monroe (marlin@acme-corp.com)')
  assert.equal(labelFor('nameless@acme-corp.com'), 'nameless@acme-corp.com')
  assert.equal(labelFor('stranger@elsewhere.io'), 'stranger@elsewhere.io')
  assert.equal(labelFor(''), '')
})
