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
- **One value may need two different renderings**, and that is an area-file job,
  not a second enum group. `Audit.vue` and `CorrectiveActions.vue` show these
  same four values as compact chips reading "Major NC" and "OFI", while a table
  column has room for "Major non-conformity". The catalogue holds the full form;
  an abbreviation is copy belonging to the view that needs it, and goes in that
  view's area file when it is extracted.

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
