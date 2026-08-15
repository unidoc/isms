import test from 'node:test'
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'

const css = await readFile(new URL('../src/style.css', import.meta.url), 'utf8')

test('inverts the dark Mermaid palette for readable print output', () => {
  const printStyles = css.slice(css.indexOf('@media print'))

  assert.match(
    printStyles,
    /\.mermaid-diagram svg\s*{[^}]*filter:\s*invert\(1\) hue-rotate\(180deg\)/s,
  )
})
