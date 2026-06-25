// Shared LCS diff primitives for the review diff views (TrackChanges + the
// HTML-table cell differ). Pure functions — no DOM, no Vue — so they can be
// reused and reasoned about in isolation.

// Split into word + whitespace tokens so word boundaries are preserved.
export function tokenize(text) {
  return text.match(/\S+|\s+/g) || []
}

export function escapeHtml(t) {
  return t.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

// Token-level LCS → merged runs of equal/add/remove (each run's text concatenated).
export function diffTokens(oldToks, newToks) {
  const m = oldToks.length, n = newToks.length
  const dp = Array.from({ length: m + 1 }, () => new Uint16Array(n + 1))
  for (let i = 1; i <= m; i++) for (let j = 1; j <= n; j++) dp[i][j] = oldToks[i - 1] === newToks[j - 1] ? dp[i - 1][j - 1] + 1 : Math.max(dp[i - 1][j], dp[i][j - 1])
  const raw = []; let i = m, j = n
  while (i > 0 || j > 0) { if (i > 0 && j > 0 && oldToks[i - 1] === newToks[j - 1]) { raw.push({ type: 'equal', text: oldToks[i - 1] }); i--; j-- } else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) { raw.push({ type: 'add', text: newToks[j - 1] }); j-- } else { raw.push({ type: 'remove', text: oldToks[i - 1] }); i-- } }
  raw.reverse()
  const merged = []; for (const op of raw) { if (merged.length > 0 && merged[merged.length - 1].type === op.type) merged[merged.length - 1].text += op.text; else merged.push({ ...op }) }
  return merged
}

// Line-level LCS → merged runs of equal/add/remove (runs joined by newline).
export function diffLines(oldArr, newArr) {
  const m = oldArr.length, n = newArr.length
  const dp = Array.from({ length: m + 1 }, () => new Uint16Array(n + 1))
  for (let i = 1; i <= m; i++) for (let j = 1; j <= n; j++) dp[i][j] = oldArr[i - 1] === newArr[j - 1] ? dp[i - 1][j - 1] + 1 : Math.max(dp[i - 1][j], dp[i][j - 1])
  const raw = []; let i = m, j = n
  while (i > 0 || j > 0) { if (i > 0 && j > 0 && oldArr[i - 1] === newArr[j - 1]) { raw.push({ type: 'equal', text: oldArr[i - 1] }); i--; j-- } else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) { raw.push({ type: 'add', text: newArr[j - 1] }); j-- } else { raw.push({ type: 'remove', text: oldArr[i - 1] }); i-- } }
  raw.reverse()
  const merged = []; for (const op of raw) { if (merged.length > 0 && merged[merged.length - 1].type === op.type) merged[merged.length - 1].text += '\n' + op.text; else merged.push({ ...op }) }
  return merged
}

// Word-level diff of two PLAIN-TEXT strings → escaped HTML with <del>/<ins>
// spans. Use for content that is not markdown (e.g. a table cell's text).
export function wordDiffPlain(oldText, newText) {
  return diffTokens(tokenize(oldText), tokenize(newText))
    .map(op => op.type === 'equal' ? escapeHtml(op.text)
      : op.type === 'remove' ? `<del class="tc-word-del">${escapeHtml(op.text)}</del>`
        : `<ins class="tc-word-ins">${escapeHtml(op.text)}</ins>`)
    .join('')
}
