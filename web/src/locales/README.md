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
Adding a **locale**: copy `en/`, translate the values, register the loader — see
[`docs/i18n.md`](../../../docs/i18n.md).

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
t('risks.owner_assigned', { name })   // ✅ one key, interpolated
t('risks.owner') + ' ' + name          // ❌ unreachable in most languages
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

**Key renames are breaking.** They invalidate in-flight translation work across
every locale. Adding a key is cheap; renaming one is not.

## What belongs in `common.json`

Copy reused across two or more areas, plus the four cross-cutting groups
(`enum`, `entity`, `field`, `error`). Everything else belongs to its area, even
if the English happens to read the same in two places — the translations may
diverge.
