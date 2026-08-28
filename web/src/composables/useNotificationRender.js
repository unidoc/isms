// The render seam for stored notifications.
//
// `notifications.title` / `body` are written pre-rendered in English and can
// never be retranslated, so the backend also stores `title_key`, `body_key` and
// a flat `params` object (plan 82 / PR #220). The stored English text stays the
// fallback — for rows written before the keys existed, for agent-actionable
// rows that are deliberately English, and for any key this bundle has no
// message for.
//
// Wire keys are frozen: they are in DB rows already, so the catalogue bends to
// them and not the other way round. A title wire key (`notifications.ca_resolved`)
// is also the parent of its body wire key (`notifications.ca_resolved.body`),
// which no JSON node can be at once — so the catalogue nests uniformly and the
// wire key is mapped onto it here:
//
//   notifications.ca_resolved                     -> notifications.ca_resolved.title
//   notifications.ca_resolved.body                -> unchanged
//   notifications.review_forwarded.body_with_note -> unchanged
import { FALLBACK, i18n } from '../i18n.js'
import { enumLabel } from './useEnumLabel.js'

// The closed set of params carrying an enum value, per plan 82. Each name is
// also its `common.enum.*` group — that is why the groups were named after the
// params in #229, so there is no second mapping to keep in sync.
const ENUM_PARAMS = ['status', 'severity', 'action', 'suggestion_type']

// `entity` is the one translatable param that is a noun the whole app reuses
// rather than an enum member, so it resolves through common.entity.* instead.
const ENTITY_PARAM = 'entity'

// Body wire keys already name a leaf; title wire keys name the group.
function catalogKey(wireKey) {
  return /\.body(_with_note)?$/.test(wireKey) ? wireKey : `${wireKey}.title`
}

// Translate the params that are enum values and pass everything else through
// untouched: actor, title, doc_id, version, round, id, note and reason are
// proper nouns, numbers or the user's own words.
function resolveParams(params) {
  const out = {}
  for (const [name, value] of Object.entries(params ?? {})) {
    if (ENUM_PARAMS.includes(name)) {
      out[name] = enumLabel(name, value)
    } else if (name === ENTITY_PARAM) {
      const key = `common.entity.${value}`
      out[name] = i18n.global.te(key, FALLBACK) ? i18n.global.t(key) : value
    } else {
      out[name] = value
    }
  }
  return out
}

// Renders one field of a notification. `wireKey` is n.title_key or n.body_key;
// `fallback` is the stored English n.title or n.body.
//
// te() is probed against FALLBACK rather than the active locale: the fallback
// catalogue is complete by contract, so a locale that lags renders English
// through fallbackLocale instead of dropping to the stored string — the same
// reasoning as enumLabel().
function render(wireKey, fallback, params) {
  if (!wireKey) return fallback ?? ''
  const key = catalogKey(wireKey)
  if (!i18n.global.te(key, FALLBACK)) return fallback ?? ''
  return i18n.global.t(key, resolveParams(params))
}

export function notificationTitle(n) {
  if (!n) return ''
  return render(n.title_key, n.title, n.params)
}

export function notificationBody(n) {
  if (!n) return ''
  return render(n.body_key, n.body, n.params)
}

export function useNotificationRender() {
  return { notificationTitle, notificationBody }
}
