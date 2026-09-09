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

## The keyset is frozen

`en` is pinned to `../../test/keyset.snapshot.json` — the exact set of keys, as
of the freeze. **This is a translator's contract, not a restriction on features.**

If you are **translating**: the snapshot is the set to translate against, and it
is the same set both release gates measure your bundle by. You need 90% coverage
before the locale can be enabled — a lower number renders the rest in English,
which is worse for you than for anyone.

If you are **adding a feature** and the keyset test fails, that is expected. Run
`npm run i18n:keyset` to record the new keys; `npm run i18n:keyset-diff` shows
what changed first, removals before additions.

**One rule beyond that.** Adding a key costs a translator nothing — an
untranslated key renders in English. **Removing or renaming one costs them work
they already did**, and nothing in CI will stop you: a key missing from a
translation is only a warning. So before recording a removal, check whether a
translation in flight carries it, and prefer add-then-remove over a single rename.
[`docs/i18n.md`](../../../docs/i18n.md) has the reasoning under "The frozen
keyset".

## Key convention

```
<area>.<group>.<key>
```

| Rule | Example |
|---|---|
| The area is the filename | `risks.*` lives in `risks.json` |
| Copy used in more than one area goes in `common.*` | `common.action.save` |
| Enum values | `common.enum.<enum>.<db_value>` → `common.enum.status.changes_requested` |
| Enum values, mid-sentence (authored, not lowercased) | `common.enum_inline.status.changes_requested` |
| Enum values, abbreviated for a narrow control (authored, not truncated) | `common.enum_abbr.finding_type.opportunity` → "OFI" |
| Entity names, mid-sentence | `common.entity_inline.risk` → "risk not found" |
| Validation messages | `common.validation.required` |
| Entity names | `common.entity.risk` |
| Field names, lowercase, for splicing into a sentence | `common.field.title` → "title is required" |
| Shared standalone labels, capitalised | `common.label.version` |
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
render helper resolves the params *before* interpolating, and each param name has
exactly one lookup path:

| Param | Resolved through |
|---|---|
| `entity` | `common.entity_inline.*` |
| `field` | `common.field.*` |
| `status` | `common.enum_inline.status.*` |
| `count`, `value` | spliced raw — a number, and the caller's own echoed input |

**The `_inline` groups, not the standalone ones.** An error sentence puts the
noun mid-sentence ("risk not found"), and which words a language capitalises
mid-sentence is a property of that language — so the inline form is separately
authored rather than a lowercasing of `common.entity.*`, which is for the badge
and the page heading. Every `entity` and `status` value the server can emit needs
a key in the inline group, and CI checks exactly that.

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
  `common.enum_inline.<param>.<value>` — so `severity`, `status`, `action` and
  `suggestion_type` are group names because those are the param names emitted by
  `internal/isms/api`. The `entity` param is the one exception: it resolves
  through `common.entity_inline.*`, because an entity name is a noun the whole
  app reuses rather than a member of an enum.
  **The inline group, for the same reason as the error seam above**: a
  notification title interpolates the value mid-sentence. So a group a
  notification touches needs entries in *both* `common.enum.*` (the standalone
  label a badge or table cell shows) and `common.enum_inline.*` — they are
  separate authored strings, and `enumCatalog.test.js` requires the inline form
  for every value a notification can interpolate.
- **`status` is one flat group across every register**, not one group per table.
  Status values are distinct app-wide (`draft`, `investigating`, `awaiting_approval`),
  and `StatusBadge` receives a bare value with no family attached — the same
  reason its colour table has always been flat. Its `group` prop names the family
  for the few callers rendering something else (`classification`, `criticality`,
  `audit_result`).
- **Everything else is named after its column**: `suggestion_type` for
  `suggestions.suggestion_type`, `audit_result` for `audit_items.result`.
- **A group is really named after its value set, and the column name is just the
  usual way to say that.** Where two columns share a name but not a value set,
  the column name belongs to one of them and the other is named after its
  taxonomy. There is one such collision today, and it is the trap to check for
  before picking a group:

  | Column | Value set | Group |
  |---|---|---|
  | `incidents.severity` | critical … low | `severity` |
  | `corrective_actions.severity` | major_nc … opportunity | **`finding_type`** |
  | `audit_findings.finding_type` | major_nc … opportunity | `finding_type` |
  | `audit_items.result` | the four above plus `not_assessed`, `conforming` | `audit_result` |

  Reaching for `severity` because the column says `severity` would render a
  corrective action's `major_nc` against the incident scale, which has no such
  member — so it would de-slug to "Major nc" in every language. `finding_type`
  and `audit_result` deliberately overlap: a finding is always one of the four,
  while an audit item may also be unassessed or conforming.
- **One value may need two or three different renderings, and all of them live in
  the catalogue.** `common.enum.*` is the standalone label, `common.enum_inline.*`
  the mid-sentence form, and `common.enum_abbr.*` the short form for a control too
  narrow to hold the full label — an `<option>` in a filter bar, a chip in a dense
  table. `useEnumLabel()` exposes one function per form: `enumLabel`,
  `enumLabelInline`, `enumLabelAbbr`. The abbreviated and inline forms fall back to
  the standalone label, so a group with neither authored still renders.

  All three are **authored per language, never derived**. "OFI" is an English
  initialism and truncating "Major non-conformity" to fit a chip is a decision only
  a speaker of the language can make.

  **Do not put an abbreviation in an area file.** An earlier version of this
  document said to, and following it produced per-view duplicates of catalogue
  entries — six copies of one string in one case, deleted again later. If two
  views abbreviate the same value set, that is one `common.enum_abbr.*` group;
  if one view needs wording no other view wants, that is area copy and it is not
  an abbreviation of an enum.

If a language ever needs two different translations for one status value —
Portuguese gender agreement on *aberto* / *aberta*, for instance — splitting the
flat group is purely additive: add `risk_status.*`, pass `group="risk_status"` at
those call sites, and leave `status.*` for everyone else. No key is renamed, so
no in-flight translation is invalidated.

**Key renames are breaking.** They invalidate in-flight translation work across
every locale. Adding a key is cheap; renaming one is not.

## What belongs in `common.json`

Copy reused across two or more areas, plus the cross-cutting groups — the ones
that exist because a *seam* reads them rather than because two views happened to
share a word:

- `enum`, `enum_inline`, `enum_abbr` — read by `useEnumLabel`
- `entity`, `entity_inline` — the same, for entity nouns
- `field` — read by `renderApiError`
- `error` — the API error catalogue, one key per code

Everything else belongs to its area, even if the English happens to read the same
in two places — the translations may diverge. `common.json` holds 30-odd
top-level groups today; the four-group version of this paragraph predated the
inline and abbreviated forms.

### `field.*` vs `label.*` vs `<area>.table.header.*`

Three groups hold what looks like the same word, and the difference is where the
word is used, not what it says.

- **`common.field.*` is lowercase** because it is never rendered alone.
  `renderApiError` splices it into a frame — `common.error.required` is
  `"{field} is required"` — so the value has to read as a noun mid-sentence. A
  language that capitalises differently mid-sentence changes the value here, and
  the frame stays untouched.
- **`common.label.*` is capitalised** and stands on its own: a form label above
  an input, a badge, an inline caption. It earns its place in `common.json` only
  when two or more areas show the same label; `version`, `round` and `reviewers`
  are there because the document, review and inbox surfaces all show them.
- **`<area>.table.header.*` stays in the area** even when the English matches
  another area's, which it very often does — "Status", "Type", "Owner". A column
  header is abbreviated to fit a column, and how far a language can abbreviate a
  word differs per table. Do not lift these into `common.label.*`.

The test for a new `common.label.*` key is the second area, not the second call
site: the same label twice inside one view belongs to that view.

## ISO terminology

The audit vocabulary is the part of this app most easily mistranslated, because
**the wrong choice is always fluent**. ISO separates *correction* from
*improvement*, *conformity* from *compliance*, and a nonconformity from an
observation. A translator who has not sat an audit will reasonably collapse
those pairs, and nothing about the result looks wrong — which is why this cannot
be caught by proofreading, and why the procedure below is not optional.

The rule for every language: **where a national adoption of the standard exists,
it decides.** Not general fluency, not a dictionary, and not this file.

### Procedure — do this before translating anything else

1. **Find the national adoption of ISO/IEC 27001** in your language. If there is
   none, ISO 9001 works just as well for this vocabulary: clauses 4–10 are the
   Annex SL structure the two standards share verbatim, and every term in the
   checklist below lives in those clauses. Many adoptions are published
   bilingually, which gives you the term pairs directly.
2. **Fill in the checklist** from that text, quoting the clause you took each
   term from. Do not translate the English; copy what the standard prints.
3. **Watch for the recurring traps** — the three below have bitten us or are
   known to bite in more than one language.
4. **Record your table** as a subsection here, with citations. That is what makes
   the next contributor's job smaller than yours, and what lets a reviewer who
   does not speak your language still check your work.

### The checklist

These are the terms the enum catalogue actually uses. The clause reference is
the same in every adoption, so this table is language-independent — copy it into
your subsection and fill the right-hand column.

| English | Defined in | Used by |
|---|---|---|
| improvement | clause 10 heading | — |
| continual improvement | clause 10.3 heading | — |
| opportunity for improvement | clause 9.3.2 f, 10.1 a | `enum.{audit_result,finding_type}.opportunity` |
| nonconformity | clause 10.2 heading | `enum.{audit_result,finding_type}.{minor_nc,major_nc}` |
| corrective action | clause 10.2 heading | `entity.corrective_action` |
| conformity | throughout | `enum.audit_result.conforming` |
| objective | clause 6.2 | `entity.objective`, `enum.status.*` |
| monitoring | clause 9.1 heading | `enum.status.monitoring` |
| review (management) | clause 9.3 heading | `enum.status.{in_review,under_review}` |
| internal audit | clause 9.2 heading | `entity.audit` |

### Recurring traps

**Correction is not improvement.** Most languages have distinct words, and ISO
relies on the distinction: an opportunity for improvement is explicitly *not* a
nonconformity requiring correction. Picking the correction word collapses the
only thing that separates the two categories. Indonesian *perbaikan* vs
*peningkatan*, Portuguese *correção* vs *melhoria*, Spanish *corrección* vs
*mejora*, French *correction* vs *amélioration* — check which one your adoption
uses in clause 10.1 before writing `enum.audit_result.opportunity`.

**Classification levels follow your national ladder, not the English words.**
`public` → `internal` → `confidential` → `restricted` is ordered by increasing
sensitivity, and the badge colours say so. Many countries have a legally defined
classification ladder whose terms do not line up word-for-word with the English.
Translate the *position in the ladder*, not the label: a literal rendering that
lands a reader on the wrong rung is worse than a loose one that preserves the
order.

**Nonconformity grading is not in the standard.** Major/minor/observation come
from certification-body practice under ISO/IEC 17021, not from ISO 27001 or
9001, so no adopted text can arbitrate them. Follow the usage of certification
bodies operating in your language and say in your subsection that you did.

### Worked example — Indonesian (`id-ID`)

Verified against the bilingual SNI ISO 9001:2015, whose clauses 4–10 are the
Annex SL structure SNI ISO/IEC 27001:2022 shares verbatim:

| English | Indonesian | Where it comes from |
|---|---|---|
| improvement | peningkatan | clause 10 heading |
| continual improvement | peningkatan berkelanjutan | clause 10.3 heading |
| opportunity for improvement | **peluang peningkatan** | clause 9.3.2 f, 10.1 a |
| nonconformity | ketidaksesuaian | clause 10.2 heading |
| corrective action | tindakan korektif | clause 10.2 heading |
| conformity | kesesuaian | throughout |
| objective | sasaran | clause 6.2 (*sasaran mutu*) |
| monitoring | pemantauan | clause 9.1 heading |
| review (management) | tinjauan | clause 9.3 heading |
| internal audit | audit internal | clause 9.2 heading |

Note what the standard does **not** contain: the word *perbaikan* appears
nowhere in it. *Perbaikan* is correction/repair; improvement is *peningkatan*.
"Peluang perbaikan" therefore inverted the distinction the category exists to
draw, and was corrected on that basis — trap 1, found only by checking.

Classification follows ANRI Perka 7/2016 (and the ministry regulations adopting
it), which orders the national ladder *Biasa/Terbuka → Terbatas → Rahasia →
Sangat Rahasia* by increasing sensitivity. `restricted` is our most sensitive
level, so it is *Sangat Rahasia*; the literal *Terbatas* put it below *Rahasia*
in a reader's mind while the badge beside it was red — trap 2.

*Mayor* / *minor* / *observasi* follow Indonesian certification-body usage —
trap 3, not sourced from the standard.

### Two keys may share a translation

That is fine, and sometimes it is the honest encoding.
`suggestion_type.reassess` and `suggestion_type.reading` are both *Nilai ulang*
in `id-ID` because they are the same operation — `api_suggestions.go` registers
one handler for both. Keep them as separate keys anyway: the English distinction
may matter to a future locale, and a key removed is a key renamed.
