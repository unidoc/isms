// Pure helpers for the trust center manager screen.

// Which status label a manager row shows, with the params its text needs.
export function rowStatus(row) {
  const d = row.document || {}
  if (d.status === 'hidden') {
    return { key: row.hidden_reason === 'deleted' ? 'hidden_deleted' : 'hidden_retired', params: {} }
  }
  if (d.status === 'draft_in_progress') {
    return { key: 'draft_in_progress', params: { draft: d.document_version, version: d.served_version?.version || '' } }
  }
  if (d.status === 'cannot_publish' || d.status === 'error' || d.status === 'published') {
    return { key: d.status, params: {} }
  }
  return { key: 'published', params: {} }
}

// What a row offers. publishAgain is null (not offered), 'enabled', or
// 'disabled_retired' (offered but the document is still retired). A hidden row
// can always be removed.
export function rowActions(row) {
  if (!row.hidden_reason) return { unpublish: true, publishAgain: null, remove: false }
  if (row.hidden_reason === 'deleted') return { unpublish: false, publishAgain: null, remove: true }
  const stillRetired = row.document?.document_status === 'retired'
  return { unpublish: false, publishAgain: stillRetired ? 'disabled_retired' : 'enabled', remove: true }
}

export function publicAddress(origin, slug, subdomainMode) {
  return subdomainMode ? `${origin}/trust` : `${origin}/${slug}/trust`
}

// The blocked-domains textarea: one domain per line (commas and spaces also
// separate). The server normalises and validates; this only splits.
export function parseDomains(text) {
  return String(text || '').split(/[\s,]+/).filter(Boolean)
}

// Mirrors the server's normalisation (trim, lower-case, leading @ dropped,
// de-duplicated, sorted) so "did the list change" ignores spelling and order.
export function normalizeDomains(list) {
  return [...new Set((list || []).map((d) => String(d).trim().toLowerCase().replace(/^@/, '')).filter(Boolean))].sort()
}

// The fields of a settings form that differ from what the server last sent.
// Only these are sent, so a stale form cannot restore a value the server has
// since changed (for instance an overview document it cleared on retire).
export function changedSettings(orig, form) {
  const patch = {}
  for (const k of ['enabled', 'security_contact', 'default_access_level']) {
    if (form[k] !== orig[k]) patch[k] = form[k]
  }
  if ((form.overview_document_id || null) !== (orig.overview_document_id || null)) {
    patch.overview_document_id = form.overview_document_id || null
  }
  if (form.privacy_notice_url !== undefined && (form.privacy_notice_url || '').trim() !== (orig.privacy_notice_url || '')) {
    patch.privacy_notice_url = (form.privacy_notice_url || '').trim()
  }
  // The NDA document (#525) and its skip list. The list is compared like the
  // blocked domains: spelling and order are not a change.
  if (form.nda_document_id !== undefined && (form.nda_document_id || null) !== (orig.nda_document_id || null)) {
    patch.nda_document_id = form.nda_document_id || null
  }
  if (form.ndaSkipText !== undefined) {
    const list = parseDomains(form.ndaSkipText)
    if (JSON.stringify(normalizeDomains(list)) !== JSON.stringify(normalizeDomains(orig.nda_skip_domains))) patch.nda_skip_domains = list
  }
  // Blocked domains: mode 'default' is null (the built-in list), mode 'custom'
  // is the list in the text, which may be empty (allow every domain).
  if (form.domainsMode !== undefined) {
    if (form.domainsMode === 'default') {
      if (orig.blocked_email_domains_default === false) patch.blocked_email_domains = null
    } else {
      const list = parseDomains(form.domainsText)
      // The same list as the one in effect is no change, even when the original
      // is the built-in default: touching the textarea must not freeze the list.
      const same = JSON.stringify(normalizeDomains(list)) === JSON.stringify(normalizeDomains(orig.blocked_email_domains))
      if (!same) patch.blocked_email_domains = list
    }
  }
  return patch
}

// The row to swap with when moving row i by delta (-1 up, +1 down): the nearest
// visible (not hidden) row in that direction, or -1. Hidden rows keep their slots.
export function visibleNeighbour(rows, i, delta) {
  for (let j = i + delta; j >= 0 && j < rows.length; j += delta) {
    if (!rows[j].hidden_reason) return j
  }
  return -1
}

// The warning for a public row whose document is classified confidential or
// restricted (HEAD or the served version), else null. When HEAD itself is not
// the sensitive one only the served version is, and the word is "confidential".
export function sensitiveBadge(row) {
  if (row.access_level !== 'public' || !row.document?.classification_sensitive) return null
  const cls = row.document.classification || ''
  const headSensitive = ['confidential', 'restricted'].includes(cls.trim().toLowerCase())
  return { classification: headSensitive ? cls : 'confidential' }
}

export function previewAddress(origin, slug, subdomainMode) {
  return subdomainMode ? `${origin}/trust-center/preview` : `${origin}/${slug}/trust-center/preview`
}
