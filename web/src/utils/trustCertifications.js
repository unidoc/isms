// Pure helpers for the trust center certifications section (#521) and the public
// page's certifications table. Dates are YYYY-MM-DD text everywhere: they are
// never turned into a Date in the visitor's time zone, which shifts the day west
// of UTC.
import { compareTrustItems } from './trustOrder.js'

// No "contract": that type belongs to supplier external links only.
export const CERT_TYPES = ['certificate', 'audit_report', 'penetration_test', 'attestation', 'other']
export const CERT_LEVELS = ['public', 'gated', 'restricted']
export const MAX_FILE_BYTES = 50 * 1024 * 1024
export const FILE_ACCEPT = '.pdf,.png,.jpg,.jpeg'
const FILE_EXT = /\.(pdf|png|jpe?g)$/i
const FILE_MIME = new Set(['application/pdf', 'image/png', 'image/jpeg'])

const DAY = /^(\d{4})-(\d{2})-(\d{2})$/

// "2026-11-13" -> "Nov 13, 2026" in `locale`, as that calendar day whatever the
// browser's time zone is: the parts are built into a UTC date and formatted with
// timeZone 'UTC'. A value that is not a YYYY-MM-DD day is returned unchanged.
export function formatDayUTC(ymd, locale) {
  const m = DAY.exec(ymd || '')
  if (!m) return ymd || ''
  const d = new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3])))
  try {
    return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeZone: 'UTC' }).format(d)
  } catch {
    return ymd
  }
}

export function formatBytes(n) {
  if (!Number.isFinite(n) || n < 0) return ''
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(n < 10 * 1024 ? 1 : 0)} KB`
  return `${(n / (1024 * 1024)).toFixed(n < 10 * 1024 * 1024 ? 1 : 0)} MB`
}

// Defence in depth: only an http(s) address is ever put in an href.
export function safeHref(url) {
  return typeof url === 'string' && /^https?:\/\//i.test(url.trim()) ? url.trim() : null
}

export function hostOf(url) {
  try { return new URL(url).host } catch { return '' }
}

// The filename a Content-Disposition header offers (RFC 5987 form first).
export function dispositionFilename(header) {
  const h = header || ''
  const star = /filename\*\s*=\s*(?:UTF-8|utf-8)''([^;]+)/.exec(h)
  if (star) {
    try { return decodeURIComponent(star[1].trim()) } catch { /* fall through */ }
  }
  const plain = /filename\s*=\s*"((?:[^"\\]|\\.)*)"/.exec(h) || /filename\s*=\s*([^;]+)/.exec(h)
  return plain ? plain[1].replace(/\\(.)/g, '$1').trim() : ''
}

// Manager rows in display order: published ones by their item's sort_order (the
// public page's order), then the unpublished ones by label. `sortOf` gives the
// sort_order of a certification's item (from GET /trust-center/items, which is
// refreshed by every reorder).
export function orderCertifications(certs, sortOf = (c) => c.item?.sort_order ?? 0) {
  const published = certs.filter((c) => c.item)
    .map((c) => ({ c, sort_order: sortOf(c), title: c.label }))
    .sort((a, b) => compareTrustItems(a, b))
    .map((x) => x.c)
  const unpublished = certs.filter((c) => !c.item)
    .sort((a, b) => String(a.label).localeCompare(String(b.label)))
  return { published, unpublished }
}

// The id list for PUT /trust-center/items/order after moving certification
// `itemId` by delta (-1 up, +1 down). The server needs EVERY item of that kind in
// the new order (409 trust_order_stale otherwise), so all kind=certification
// items are listed, including any whose certification row is missing. The move
// swaps with the nearest row in that direction that is a known certification
// (`known` is the set of item ids that have one), so an item without one keeps
// its slot. Returns null when there is nowhere to move.
export function moveIds(items, known, itemId, delta) {
  const ordered = items.filter((i) => i.kind === 'certification')
    .sort((a, b) => compareTrustItems(a, b, (x) => x.certification?.label || ''))
  const ids = ordered.map((i) => i.id)
  const i = ids.indexOf(itemId)
  if (i < 0) return null
  let j = i + delta
  while (j >= 0 && j < ids.length && !known.has(ids[j])) j += delta
  if (j < 0 || j >= ids.length) return null
  ;[ids[i], ids[j]] = [ids[j], ids[i]]
  return ids
}

export function blankForm(defaults = {}) {
  return {
    source: 'file', label: '', link_type: '', issuer: '', valid_from: '', valid_until: '', verified_at: '',
    url: '', publish: false, access_level: 'gated', ...defaults,
  }
}

export function formFromCert(c) {
  return {
    label: c.label, link_type: c.link_type, issuer: c.issuer || '',
    valid_from: c.valid_from || '', valid_until: c.valid_until || '', verified_at: c.verified_at || '',
    url: c.url || '',
  }
}

// The fields of an edit form that differ from the certification as the server
// sent it. Only these go in the PUT, so a stale form cannot restore a value
// another manager changed; an emptied date is null (the server answers 400 for
// ""), and url is sent for link certifications only.
export function changedFields(cert, form) {
  const patch = {}
  const orig = formFromCert(cert)
  for (const k of ['label', 'link_type', 'issuer']) {
    const v = typeof form[k] === 'string' ? form[k].trim() : form[k]
    if (v !== orig[k]) patch[k] = v
  }
  for (const k of ['valid_from', 'valid_until', 'verified_at']) {
    if ((form[k] || '') !== orig[k]) patch[k] = form[k] || null
  }
  if (cert.source === 'link' && (form.url || '').trim() !== orig.url) patch.url = form.url.trim()
  return patch
}

function urlOk(url) {
  try {
    const u = new URL(url)
    return (u.protocol === 'http:' || u.protocol === 'https:') && !!u.host
  } catch { return false }
}

// Checks a file against what the server accepts. The server stays authoritative
// (it reads the bytes); this only avoids sending an obviously wrong file.
export function fileProblem(file) {
  if (!file) return 'file_required'
  if (file.size > MAX_FILE_BYTES) return 'file_too_large'
  if (!FILE_EXT.test(file.name || '') && !FILE_MIME.has(file.type)) return 'file_type'
  return ''
}

// Errors of an add or edit form as { field: i18n key suffix }. `mode` is 'add'
// or 'edit'; `cert` is the certification being edited.
export function validateForm(form, { mode = 'add', file = null, cert = null } = {}) {
  const e = {}
  const label = (form.label || '').trim()
  if (!label) e.label = 'label_required'
  else if ([...label].length > 200) e.label = 'label_too_long'
  if (!CERT_TYPES.includes(form.link_type)) e.link_type = 'type_required'
  const isLink = mode === 'add' ? form.source === 'link' : cert?.source === 'link'
  if (isLink) {
    const url = (form.url || '').trim()
    if (!url) e.url = 'url_required'
    else if (!urlOk(url)) e.url = 'url_invalid'
  } else if (mode === 'add') {
    const p = fileProblem(file)
    if (p) e.file = p
  }
  if (form.valid_from && form.valid_until && form.valid_from > form.valid_until) e.valid_until = 'dates_order'
  return e
}

// The multipart fields of a file upload: empty dates are left out, publish is the
// text "true" only when on, and access_level only accompanies a publish.
export function uploadFields(form) {
  const f = { label: form.label.trim(), link_type: form.link_type }
  if (form.issuer.trim()) f.issuer = form.issuer.trim()
  for (const k of ['valid_from', 'valid_until', 'verified_at']) if (form[k]) f[k] = form[k]
  if (form.publish) { f.publish = 'true'; f.access_level = form.access_level }
  return f
}

// The JSON body of a link certification.
export function linkBody(form) {
  const b = { label: form.label.trim(), link_type: form.link_type, url: form.url.trim() }
  if (form.issuer.trim()) b.issuer = form.issuer.trim()
  for (const k of ['valid_from', 'valid_until', 'verified_at']) if (form[k]) b[k] = form[k]
  if (form.publish) { b.publish = true; b.access_level = form.access_level }
  return b
}
