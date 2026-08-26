# Locale files

Every user-facing string in the web app lives here. A component never holds a
literal, never derives a label from a database value by string munging, and
never formats a date with a hardcoded locale.

## Layout

```
locales/
├── en/                  ← the fallback; always bundled, always complete
│   ├── index.js         ← imports and merges every area file
│   ├── common.json      ← shared copy: actions, enums, entities, fields, errors
│   ├── risks.json       ← one file per register / workflow area, added by the
│   └── …                  PR that extracts that area
└── <tag>/               ← same filenames, mirrored
```

Only `common.json` exists up front — the shared vocabulary every area draws on.
Area files arrive with the PR that extracts their view, because extraction is one
PR per view and a single `en.json` would make them all collide on one file.

Adding an **area**: one JSON file plus one import line in `index.js`.
Adding a **locale**: register the tag server-side in
`internal/isms/i18n/locale.go`, copy `en/`, translate the values, register the
loader — see [`docs/i18n.md`](../../../docs/i18n.md).

## Key convention

```
<area>.<group>.<key>
```

| Rule | Example |
|---|---|
| The area is the filename | `risks.*` lives in `risks.json` |
| Copy used in more than one area goes in `common.*` | `common.action.save` |
| Enum values | `common.enum.<enum>.<db_value>` → `common.enum.status.changes_requested` |
| Validation messages | `common.validation.required` |
| Entity names | `common.entity.risk` |
| Field names | `common.field.title` |
| API error codes | `common.error.not_found` |
| Group by UI structure, not by phrasing | `risks.table.header.likelihood` |
| Keys are `snake_case`, and enum keys mirror the DB value verbatim | `changes_requested`, never `changesRequested` |

## Hard rules

**Never concatenate a key at runtime.** `t('common.enum.' + name)` defeats keyset
extraction and unused-key detection. The composables `useEnumLabel` and the API
error renderer are the only sanctioned dynamic lookups; they exist precisely so
nothing else needs to do it.

**Never build a sentence from fragments.** Word order is language-specific.

```js
t('risks.detail.owner_assigned', { name })      // ✅ one key, interpolated
t('risks.detail.owner') + ' ' + name            // ❌ unreachable in most languages
```

**Interpolated entity and field names must themselves be translated.** An API
error arrives as `{code: 'not_found', params: {entity: 'risk'}}`. Splicing the
English `"risk"` into an Indonesian sentence produces neither language. The
render helper resolves `entity` and `field` params through `common.entity.*` and
`common.field.*` first — which is why those two groups exist and why every
`entity` value the server emits must have a key here.

**Use vue-i18n pluralization, not a `count === 1` branch.**

```json
{ "count": "no risks | one risk | {count} risks" }
```

Indonesian has no grammatical plural and Portuguese shares English's two-form
rule, so neither exercises this — which is exactly why it has to be built in
now, rather than retrofitted when a locale with real plural rules arrives.

## Enum groups

`common.enum.<group>.<db_value>`, and the group name is not free-form:

- **A group a backend notification interpolates is named after the param.** The
  notification renderer resolves translatable params as
  `common.enum.<param>.<value>` — so `severity`, `status`, `action` and
  `suggestion_type` are group names because those are the param names emitted by
  `internal/isms/api`. The `entity` param is the one exception: it resolves
  through `common.entity.*`, because an entity name is a noun the whole app
  reuses rather than a member of an enum.
- **`status` is one flat group across every register**, not one group per table.
  Status values are distinct app-wide (`draft`, `investigating`, `awaiting_approval`),
  and `StatusBadge` receives a bare value with no family attached — the same
  reason its colour table has always been flat. Its `group` prop names the family
  for the few callers rendering something else (`classification`, `criticality`,
  `audit_result`).
- **Everything else is named after its column**: `suggestion_type` for
  `suggestions.suggestion_type`, `audit_result` for `audit_items.result`.

If a language ever needs two different translations for one status value —
Portuguese gender agreement on *aberto* / *aberta*, for instance — splitting the
flat group is purely additive: add `risk_status.*`, pass `group="risk_status"` at
those call sites, and leave `status.*` for everyone else. No key is renamed, so
no in-flight translation is invalidated.

**Key renames are breaking.** They invalidate in-flight translation work across
every locale. Adding a key is cheap; renaming one is not.

## What belongs in `common.json`

Copy reused across two or more areas, plus the four cross-cutting groups
(`enum`, `entity`, `field`, `error`). Everything else belongs to its area, even
if the English happens to read the same in two places — the translations may
diverge.
