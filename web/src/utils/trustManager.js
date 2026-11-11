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
  return patch
}
