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
`internal/isms/i18n/locale.go`, settle the ISO vocabulary (see "ISO terminology"
below), then add area files one at a time as you finish them — starting from an
empty directory, **not** from a copy of `en/`, which reports itself as fully
translated while it is still English. Then register the loader. Full procedure —
seven steps, ending at the release gates and a layout pass — and the reasoning:
[`docs/i18n.md`](../../../docs/i18n.md).

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

### Worked example — Brazilian Portuguese (`pt-BR`)

Verified against the Brazilian adoption of ISO/IEC 27001:2022 and the shared Annex SL vocabulary in ABNT NBR ISO 9001:2015.

| English | Brazilian Portuguese | Where it comes from |
|---|---|---|
| improvement | melhoria | clause 10 heading |
| continual improvement | melhoria contínua | clause 10.3 (ISO 9001); clause 10.1 (ISO/IEC 27001) |
| opportunity for improvement | oportunidade de melhoria | Brazilian certification/audit practice |
| nonconformity | não conformidade | clause 10.2 heading |
| corrective action | ação corretiva | clause 10.2 heading |
| conformity | conformidade | throughout |
| objective | objetivo | clause 6.2 |
| monitoring | monitoramento | clause 9.1 heading |
| review (management) | análise crítica (pela Direção) | clause 9.3 heading |
| internal audit | auditoria interna | clause 9.2 heading |

The Brazilian adoption uses **análise crítica** for the management-system meaning of **review**. This terminology is also used in ABNT NBR ISO 9001:2015 and ABNT NBR ISO/IEC 27001:2022.

**Correction is not improvement.** The Brazilian terminology distinguishes **correção** from **melhoria** and **ação corretiva**. ABNT NBR ISO 9001:2015 treats correction, corrective action, and continual improvement as distinct concepts, while clause 10.2 uses **não conformidade** and **ação corretiva**.

**Opportunity wording.** ABNT NBR ISO 9001:2015 uses the expression **oportunidades para melhoria**, while ABNT NBR ISO/IEC 27001:2022 uses **oportunidades para a melhoria contínua**. However, **oportunidade de melhoria** is the established wording used for the audit finding/result category in Brazilian certification practice. The locale therefore uses **oportunidade de melhoria** for the catalogue value, preserving the distinction between the ISO improvement concept and the audit-result label.

**Nonconformity grading is not defined by ISO/IEC 27001.** The application uses **major nonconformity**, **minor nonconformity**, and **observation** as audit-result terminology. These are certification/audit practice terms rather than levels defined by ISO/IEC 27001 itself. The Brazilian Portuguese translations are **não conformidade maior**, **não conformidade menor**, and **observação**.

**Opportunity for improvement is kept distinct from nonconformity.** It is translated as **oportunidade de melhoria**, preserving the distinction between a potential improvement and a nonconformity requiring corrective action.

**Classification terms.** The locale uses **público**, **interno**, **confidencial**, and **restrito** for the application's existing `public`, `internal`, `confidential`, and `restricted` classification levels. These labels are application classification levels, not a four-level classification scheme prescribed by ISO/IEC 27001 itself, so the locale preserves the ordering and semantics of the existing application values.

**Security-management vocabulary.** The locale uses the Brazilian Portuguese terminology established by the Brazilian adoption of ABNT NBR ISO/IEC 27001:2022, including **segurança da informação**, **sistema de gestão da segurança da informação**, **política de segurança da informação**, **objetivos da segurança da informação**, **avaliação de riscos de segurança da informação**, **tratamento de riscos da segurança da informação**, **informação documentada**, **partes interessadas**, **Alta Direção**, and **controles de segurança da informação**. Terms are kept aligned with the wording used in the Brazilian standard rather than translated independently from the English.

**CIA terms.** For confidentiality, integrity, and availability, the Brazilian Portuguese terms are **confidencialidade**, **integridade**, and **disponibilidade**. In Brazil, these principles are universally referred to as the **CID triad** (*tríade CID*), using the Portuguese initials. The `pt-BR` locale therefore localizes badge codes and table headers consistently to the **CID** form:

| Key | en | pt-BR | Rationale |
|---|---|---|---|
| `common.cia_abbr.a` | `A` | `D` | Initial of *disponibilidade* |
| `components.readings.table.availability` | `A` | `D` | Same chip, same rule |
| `*.table.cia` (risks, assets, systems, suppliers) | `C/I/A` | `C/I/D` | Header follows the triad |

Unlike Indonesian (K/I/K collision), the CID triad has no initial collision, so localization is both possible and the established practice in Brazilian security literature.

**Likelihood column code.** The readings table column for likelihood (`components.readings.table.likelihood`) is localized to `"P"` (*Probabilidade*), following the same rationale as the CID codes: "P" is the single-character form Brazilian risk practitioners expect in a narrow column header, and it does not collide with any other column initial in the same table (C, I, D, Imp).

**Finding-type abbreviations.** `common.enum_abbr.finding_type` and `common.enum_abbr.audit_result` are fully localized:

| Key | en | pt-BR | Rationale |
|---|---|---|---|
| `finding_type.opportunity` | `OFI` | `OM` | *Oportunidade de Melhoria* — the established Brazilian audit abbreviation |
| `finding_type.major_nc` | `Major NC` | `NC Maior` | Word-order follows Brazilian usage (*Não Conformidade Maior*) |
| `finding_type.minor_nc` | `Minor NC` | `NC Menor` | Same rule |

**"N/A" format.** The `common.cia.na` field uses `"N/A"` to represent *Não Aplicável* (Not Applicable) in Brazilian Portuguese. The most common alternative in translations is "N/D" (*Não Disponível*) which was considered but discarded. In compliance contexts, *Não Aplicável* indicates that the control does not apply to the record, whereas *Não Disponível* indicates that the expected evidence is unavailable or was not found. The latter situation may, therefore, indicate a control failure and result in the recording of a non-conformity.


Privacy and data protection terms follow Brazil's Lei Geral de Proteção de Dados (LGPD — Lei nº 13.709/2018), rather than an ISO adoption:

| English | Brazilian Portuguese | Basis |
|---|---|---|
| controller | **controlador** | LGPD, art. 5º, VI |
| processor | **operador** | LGPD, art. 5º, VII — legal wording; avoid literal "processador" |
| data protection officer (DPO) | **encarregado** | LGPD, art. 5º, VIII |
| data protection (legal category) | **proteção de dados** | LGPD statutory vocabulary |

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

Three more terms were settled when the bundle was completed. They are **not**
in the table above, deliberately: no adopted standard arbitrates them, so
citing one would be inventing authority the choice does not have. Recording
them separately is the honest form, and it is the form to copy when your
language hits the same situation — a decision with a reason beats a decision
that looks sourced.

| English | Indonesian | Basis |
|---|---|---|
| control (document type) | **kendali** | maintainer decision; common usage also says *kontrol* |
| controller / processor | **pengendali** / **prosesor** | wording of UU 27/2022 (personal data protection), not an ISO adoption |
| data protection (legal category) | **pelindungan data** | UU 27/2022 spelling — the law says *pelindungan*, general usage says *perlindungan* |

The middle two matter beyond vocabulary: those are the words a data-protection
regulator uses, so a reader who works with the law recognises them and a reader
who does not is not misled. Where your jurisdiction has its own privacy statute,
prefer its wording over a literal rendering of the English for the same reason.

### Worked example — Icelandic (`is-IS`)

Iceland's national standards body, Staðlaráð Íslands, publishes an official
Icelandic translation of the standard itself — ÍST EN ISO/IEC 27001 ("Upplýsingatækni
– Öryggisaðferðir – Stjórnunarkerfi um upplýsingaöryggi – Kröfur"), most recently
as ÍST EN ISO/IEC 27001:2023+AC:2025. That text is sold through Staðlabúðin and was
not read clause-by-clause for this bundle — the general management-system and
audit vocabulary below follows established Icelandic ISO/9001-adjacent and
infosec-industry usage rather than a clause citation, and this paragraph is that
disclosure, not a claim of having checked the standard directly.

**The CIA triad is not translated letter-for-letter.** Confidentiality,
integrity and availability are **leynd**, **heilleiki** and **tiltækileiki** in
Icelandic information-security practice (consistent usage across Háskóli
Íslands's and 112.is's information-security policies, and multiple corporate
security policies published in Icelandic) — so the triad itself, and every
abbreviated C/I/A badge, table header and single-letter code in the app, is
**L/H/T** here, not CIA. This is the same category of finding as Brazilian
Portuguese's CID: a literal transliteration of the initials would have been
wrong, not just non-idiomatic, because L/H/T is what a Icelandic-reading
practitioner actually expects to see labeled on a risk or asset assessment.

| English | Icelandic |
|---|---|
| confidentiality | leynd |
| integrity | heilleiki |
| availability | tiltækileiki |
| C / I / A | L / H / T |

**Privacy and data-protection terms follow Icelandic law directly, not an ISO
adoption.** Iceland is in the EEA and GDPR applies without a local equivalent
of Brazil's LGPD; the operative statute is *Lög um persónuvernd og vinnslu
persónuupplýsinga, nr. 90/2018*, Iceland's GDPR implementation.

| English | Icelandic | Basis |
|---|---|---|
| controller | **ábyrgðaraðili** | nr. 90/2018 |
| processor | **vinnsluaðili** | nr. 90/2018 |
| data protection officer (DPO) | **persónuverndarfulltrúi** | nr. 90/2018; GDPR arts. 37–39 |
| personal data | **persónuupplýsingar** | nr. 90/2018 |

**Nonconformity/audit-result grading is certification-body practice, not
standard text** — same caveat as the Indonesian section above. *Frávik*
(nonconformity), *ábending* (observation) and *tækifæri til umbóta*
(opportunity for improvement) follow general Icelandic audit usage; *meiri
háttar* / *minni háttar* (major/minor) are the ordinary Icelandic adjectives
for grading severity, not standard-defined terms.

**A known, undisclosed-elsewhere limitation of the bundle's pluralization,
not specific to this locale:** every `{count} X | {count} Y` message in this
app resolves through vue-i18n's default two-way plural rule — index 0 when
the count is exactly 1, index 1 otherwise. Icelandic grammar instead wants the
singular form after any count ending in 1 except 11 (21, 31, 101 mánuður, not
mánuðir), which this bundle cannot express without a locale-specific plural
rule the i18n setup does not register for any locale today. The two forms
provided here are therefore the closest available fit, not a claim of
grammatical correctness at every count — the same simplification English
itself is not tested against, but Icelandic's plural rule genuinely differs
from English's where English's does not (English "21 months" is idiomatic;
"21 mánuðir" is not).

### Worked example — Polish (`pl-PL`)

Poland's national standards body, Polski Komitet Normalizacyjny (PKN), publishes
an official Polish translation of the standard itself — PN-EN ISO/IEC 27001
("Bezpieczeństwo informacji, cyberbezpieczeństwo i ochrona prywatności — Systemy
zarządzania bezpieczeństwem informacji — Wymagania"), maintained by technical
committee KT nr 182 (Ochrona Informacji w Systemach Teleinformatycznych). The
2017-06 Polish edition is the most recent one confirmed publicly available at
the time this bundle was written; PKN's Polish translation of the 2023-08
English revision was still in progress. That text is sold through PKN's own
store and was not read clause-by-clause for this bundle — the general
management-system and audit vocabulary below follows established Polish
ISO-9001-adjacent and infosec-industry usage rather than a clause citation,
and this paragraph is that disclosure, not a claim of having checked the
standard directly.

**The CIA triad is a genuine judgment call in Polish, not a settled one.**
Polish information-security writing overwhelmingly still says "triada CIA",
borrowing the English initials, even in text that fully translates the three
underlying words (poufność, integralność, dostępność). A second acronym,
**PID**, is also independently attested — it is the form used as the base of
the extended "PIDPAU" (Parker Hexad) acronym in Polish security literature.
Both circulate; neither is wrong. This bundle uses **PID**, on the reasoning
that every other CIA-adjacent string here — the three full words, the
per-level labels — is already translated, so a lone untranslated "CIA"
abbreviation would be the one Polish-reading inconsistency left in an
otherwise fully localized set of badges and table headers. A reviewer who
disagrees and prefers the borrowed "CIA" form is not wrong to; this is
recorded as a decision made, not a fact established.

| English | Polish |
|---|---|
| confidentiality | poufność |
| integrity | integralność |
| availability | dostępność |
| C / I / A | P / I / D |

**Privacy and data-protection terms follow Polish/EU law directly, not an ISO
adoption.** Poland is an EU member state; GDPR applies directly, under its
Polish name **RODO** (Rozporządzenie o Ochronie Danych Osobowych — RODO *is*
the GDPR regulation's Polish name, not a separate national law, the same
relationship Iceland's section describes for its own EEA implementation),
alongside the national implementing act, *Ustawa z dnia 10 maja 2018 r. o
ochronie danych osobowych*.

| English | Polish | Basis |
|---|---|---|
| controller | **administrator danych** | RODO / ustawa z 10.05.2018 |
| processor | **podmiot przetwarzający** | RODO / ustawa z 10.05.2018 |
| data protection officer (DPO) | **inspektor ochrony danych (IOD)** | RODO / GDPR arts. 37–39 |
| personal data | **dane osobowe** | RODO / ustawa z 10.05.2018 |

**Nonconformity/audit-result grading is certification-body practice, not
standard text** — same caveat as the Indonesian and Icelandic sections above.
*Niezgodność większa* / *niezgodność mniejsza* (major/minor nonconformity),
*obserwacja* (observation) and *możliwość doskonalenia* (opportunity for
improvement) follow general Polish audit usage, not standard-defined wording.

**"Overdue" is deliberately never translated as anything related to
overflow.** A literal-minded pass could reach for a cognate of "excess" or
"flooding" the way an earlier locale in this bundle initially did before a
review round caught it; Polish has the same trap available and this bundle
avoids it on purpose. Every occurrence — badges, dashboard stats, review and
document and task language — uses **po terminie** ("past the deadline"), a
prepositional phrase that needs no gender or number agreement, so it composes
safely with any noun it follows.

**The objective check-in is not a hotel check-in.** "Check-in" here means a
periodic measurement recorded against an objective's target, not an arrival.
This bundle uses **pomiar** ("measurement") throughout — check-in cycle is
*częstotliwość pomiarów*, last check-in is *ostatni pomiar*, and so on —
rather than a word that reads as checking into a hotel or a flight.

**A known, undisclosed-elsewhere limitation of the bundle's pluralization,
more severe for Polish than for the locales above:** every `{count} X | {count}
Y` message in this app resolves through vue-i18n's default plural rule —
two-way (index 0 at exactly 1, index 1 otherwise) for a two-form message, or
zero/one/two-or-more for a three-form message where the English source
happens to provide one (e.g. `suppliers.linked_systems.heading`). Polish
grammar has neither shape: nouns take one form after 1, a second ("few") form
after 2–4, and a third ("many") form after 0, 5–21, and most higher counts
depending on the last digit — a genuine three-way split that does not align
with vue-i18n's built-in zero/one/many rule at the 2–4 boundary. Where this
bundle had two forms available, the second slot uses the **many/genitive**
form (correct for 0 and 5+, the more common range in this app's counts, and
merely non-idiomatic rather than wrong at 2–4). Where the English source
already provides three forms, this bundle uses them as zero/one/many, which
is a closer fit but still not Polish's real "few" case. Neither is a claim of
grammatical correctness at every count — the same simplification English
itself is not tested against, but Polish's plural system genuinely has more
cases than the infrastructure can express today.

### Worked example — German (`de-DE`)

Germany's national standards body, Deutsches Institut für Normung (DIN),
publishes an official German edition of the standard itself — DIN EN ISO/IEC
27001, currently "DIN EN ISO/IEC 27001:2024-01" (the German edition of
ISO/IEC 27001:2022), maintained by committee DIN NIA-01-27
"IT-Sicherheitsverfahren", which participates in the international committee
ISO/IEC JTC 1/SC 27. That text is sold through Beuth Verlag and was not read
clause-by-clause for this bundle — general management-system and audit
vocabulary below follows established German ISO/quality-management and
infosec-industry usage instead, and this paragraph is that disclosure, not a
claim of having checked the standard directly.

**The CIA triad is kept as "CIA" here, not translated — the opposite call
from this bundle's Polish and Icelandic sections.** A search across German
infosec writing (vendor blogs, security consultancies, university material)
found "CIA-Triade" used essentially universally, with the three underlying
words translated (Vertraulichkeit, Integrität, Verfügbarkeit) but the letters
themselves kept as a borrowed English acronym — no German-native abbreviation
turned up anywhere. So every full label is German, and every abbreviated
badge or table header (`common.cia_abbr.*` and everywhere it is consumed)
stays **C/I/A**. This is a finding, not a guess: it would have been easy to
default to inventing a German-letter form the way the Polish section does
with PID, and that would have been wrong here.

| English | German |
|---|---|
| confidentiality | Vertraulichkeit |
| integrity | Integrität |
| availability | Verfügbarkeit |
| C / I / A | C / I / A (kept, not translated — see above) |

**Privacy and data-protection terms follow German/EU law directly, not an
ISO adoption.** Germany is an EU member state; GDPR applies directly, under
its German name **DSGVO** (Datenschutz-Grundverordnung — this *is* the GDPR
regulation's German name, not a separate law, the same relationship the
Icelandic and Polish sections describe for their own EEA/EU implementations),
alongside the national implementing act, the **BDSG**
(Bundesdatenschutzgesetz). One genuine German-specific wrinkle worth
recording: Germany sets its own, lower mandatory-DPO-appointment threshold in
§ 38 BDSG, on top of the baseline in GDPR Art. 37 — a difference from the
bare regulation text, not an inconsistency in this bundle.

| English | German | Basis |
|---|---|---|
| controller | **Verantwortlicher** | DSGVO Art. 4, Art. 24 |
| processor | **Auftragsverarbeiter** | DSGVO Art. 4, Art. 28 |
| data protection officer (DPO) | **Datenschutzbeauftragter (DSB)** | DSGVO Art. 37; § 38 BDSG (Germany's own lower threshold) |
| personal data | **personenbezogene Daten** | DSGVO |

**Nonconformity/audit-result grading is certification-body practice, not
standard text** — same caveat as the sections above. *Abweichung*
(nonconformity), *Beobachtung* (observation) and *Verbesserungspotenzial*
(opportunity for improvement) follow general German audit usage, not
standard-defined wording.

**"Overdue" is deliberately never translated toward anything related to
overflow.** This bundle uses **überfällig** throughout — badges, dashboard
stats, review/document/task language, contract-expiry language — the
ordinary German word for a bill, a task or a library book that has passed
its due date, and unrelated in root to *Überlauf* ("overflow"), the
false-friend trap an earlier locale in this bundle fell into before a review
caught it. Swept every English "overdue" occurrence after the first draft
and confirmed the German is consistent throughout: no drift.

**The objective check-in is not a hotel check-in.** "Check-in" here means a
periodic measurement recorded against an objective's target, not an arrival
— and German borrows "Check-in" itself as an unambiguously hotel/flight word
(*Einchecken*), so a literal pass would land on exactly the wrong sense. This
bundle uses **Messung** ("measurement") throughout instead: check-in cycle is
*Messintervall*, last check-in is *letzte Messung*, recording one is
*Messung erfassen*.

**Notification-interpolation grammar was traced through the actual code, not
assumed correct**, the same way the Polish section's `enum_inline.action`
finding was made. `useNotificationRender.js` resolves `severity`, `status`,
and `action` through `common.enum_inline.*` before splicing them into a
sentence frame; tracing the Go call sites in `api_incidents.go` and
`api_collab.go` found that `{severity}` is interpolated exactly once, always
attributively before the masculine noun *Vorfall* ("incident") — unlike a
predicative German participle, an attributive German adjective inflects for
case and gender, so `enum_inline.severity.*` carries the declined forms
(*kritischer*, *hoher*, *mittlerer*, *niedriger*), not the bare stems a
literal pass would produce. `{status}` and `{action}`, by contrast, land in
predicative/headline constructions ("Vorfall {status}", "Vorschlag
{action}") where German participles do not inflect at all regardless of the
subject's gender, so the bare forms already used there are correct as
drafted.

**A shared frame issue, found here and confirmed present verbatim in other
locales in this bundle, fixed only in `de-DE/notifications.json`:**
`suggestion_new.body`'s `"{suggestion_type} {entity}"` juxtaposes two bare
nouns with nothing between them. Rendered with real German values (e.g.
*Erstellung* + *Risiko*, *Verknüpfung* + *Lieferant*), the pair reads as two
stacked nouns rather than natural German — not as broken as the genitive
case Polish requires there, but not idiomatic either. Fixed the same way the
Polish PR fixed it: an en dash inside this one frame
(`"{suggestion_type} – {entity}"`), leaving `enum_inline.suggestion_type.*`
and `entity_inline.*` themselves untouched, since those values are shared
correctly-nominative with other frames. `en/notifications.json` itself was
not changed — this is a per-locale fix, and the same juxtaposition in
`is-IS` and `pt-BR` is a pre-existing, shared issue this PR does not touch.

**Pluralization is the simplest of this bundle's non-English locales.**
German's plural system is close to English's — singular at exactly 1,
plural everywhere else including 0 — so vue-i18n's default two-way rule
fits cleanly with no compromise, unlike the Polish and (to a lesser extent)
Icelandic sections above. No irregularity was found while translating.

### Abbreviations are not always translatable, and that is a decision to record

`common.enum.entity_abbr.*` and `common.cia_abbr.*` are badge codes, not copy:
they render in fixed-width chips beside a title, so they are constrained by
space in a way the full label is not. `id-ID` keeps them in their English form,
for two reasons worth reusing rather than rediscovering:

- **A translated abbreviation can collide where the full term does not.**
  Confidentiality / integrity / availability are *kerahasiaan* / *integritas* /
  *ketersediaan*, which abbreviate to K / I / K — the C and the A become the
  same letter. `document_type` has the same problem: *kendali*, *kebijakan* and
  *klausul* all start with K.
- **An abbreviation that is not shorter is not an abbreviation.** *RISK* →
  *RISIKO* costs two characters in a chip sized for four.

So: translate the full label, keep the code. If your language has established
short forms, use them — `enum_abbr.finding_type.*` does exactly that in
`id-ID`, where *KTS Mayor* / *KTS Minor* are the forms Indonesian auditors
write (*KTS* = *ketidaksesuaian*). The point is not that codes stay English;
it is that the choice is deliberate and written down, because the next
contributor will otherwise read an untranslated value as an oversight.

### Two keys may share a translation

That is fine, and sometimes it is the honest encoding.
`suggestion_type.reassess` and `suggestion_type.reading` are both *Nilai ulang*
in `id-ID` because they are the same operation — `api_suggestions.go` registers
one handler for both. Keep them as separate keys anyway: the English distinction
may matter to a future locale, and a key removed is a key renamed.
