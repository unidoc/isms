// Generator for the jurisdiction picker's region list, and for the one-shot SQL
// that converted `legal_requirements.jurisdiction` from English display names to
// the codes the picker now writes.
//
// Both artefacts come from this one file on purpose. The migration's CASE arms
// and `src/data/countries.js` have to agree exactly: a name the migration
// converts to a code the picker does not offer becomes a row nobody can edit
// back to a valid value, and a code in the picker that the migration never
// produced silently splits old and new rows. `regionCodes.test.js` asserts the
// data file still matches `codeList()`, so an edit to one without the other
// fails CI rather than surfacing as a stale row months later.
//
// The English names in LEGACY_NAMES are frozen history, NOT display copy. They
// are the exact strings the old `countries.js` shipped, and they exist only so
// the migration can recognise what is already in the column. Display text now
// comes from `Intl.DisplayNames` via `regionLabel()` in `useFormat.js` — never
// from here. Do not "fix" a spelling in this table: it would stop matching the
// rows it is here to convert.
//
// Usage:
//   node scripts/i18nRegionCodes.mjs --codes   # the alpha-2 list, one per line
//   node scripts/i18nRegionCodes.mjs --sql     # the migration's CASE expression

// The four non-country values. They are not ISO 3166 and must survive as
// sentinels: `EU` in particular IS an ISO exceptional reservation, so
// `Intl.DisplayNames` renders it "European Union" — `common.region.eu` has to
// win the lookup, which is why regionLabel() checks these first.
export const SENTINELS = ['Global', 'EU', 'EEA', 'APAC']

// Names `Intl.DisplayNames(['en'])` spells differently to the old data file.
// The other 181 matched its output exactly and are resolved by reverse lookup
// below rather than being restated here.
const HAND_MAPPED = {
  'Antigua and Barbuda': 'AG',
  'Bosnia and Herzegovina': 'BA',
  'Cabo Verde': 'CV',
  Congo: 'CG',
  'Czech Republic': 'CZ',
  'Democratic Republic of the Congo': 'CD',
  'East Timor': 'TL',
  'Ivory Coast': 'CI',
  Myanmar: 'MM',
  Palestine: 'PS',
  'Saint Kitts and Nevis': 'KN',
  'Saint Lucia': 'LC',
  'Saint Vincent and the Grenadines': 'VC',
  'Sao Tome and Principe': 'ST',
  'Trinidad and Tobago': 'TT',
  Turkey: 'TR',
}

// The 197 country names the old picker offered, verbatim.
const LEGACY_NAMES = [
  'Afghanistan', 'Albania', 'Algeria', 'Andorra', 'Angola', 'Antigua and Barbuda',
  'Argentina', 'Armenia', 'Australia', 'Austria', 'Azerbaijan', 'Bahamas',
  'Bahrain', 'Bangladesh', 'Barbados', 'Belarus', 'Belgium', 'Belize', 'Benin',
  'Bhutan', 'Bolivia', 'Bosnia and Herzegovina', 'Botswana', 'Brazil', 'Brunei',
  'Bulgaria', 'Burkina Faso', 'Burundi', 'Cabo Verde', 'Cambodia', 'Cameroon',
  'Canada', 'Central African Republic', 'Chad', 'Chile', 'China', 'Colombia',
  'Comoros', 'Congo', 'Costa Rica', 'Croatia', 'Cuba', 'Cyprus',
  'Czech Republic', 'Democratic Republic of the Congo', 'Denmark', 'Djibouti',
  'Dominica', 'Dominican Republic', 'East Timor', 'Ecuador', 'Egypt',
  'El Salvador', 'Equatorial Guinea', 'Eritrea', 'Estonia', 'Eswatini',
  'Ethiopia', 'Fiji', 'Finland', 'France', 'Gabon', 'Gambia', 'Georgia',
  'Germany', 'Ghana', 'Greece', 'Grenada', 'Guatemala', 'Guinea',
  'Guinea-Bissau', 'Guyana', 'Haiti', 'Honduras', 'Hungary', 'Iceland', 'India',
  'Indonesia', 'Iran', 'Iraq', 'Ireland', 'Israel', 'Italy', 'Ivory Coast',
  'Jamaica', 'Japan', 'Jordan', 'Kazakhstan', 'Kenya', 'Kiribati', 'Kosovo',
  'Kuwait', 'Kyrgyzstan', 'Laos', 'Latvia', 'Lebanon', 'Lesotho', 'Liberia',
  'Libya', 'Liechtenstein', 'Lithuania', 'Luxembourg', 'Madagascar', 'Malawi',
  'Malaysia', 'Maldives', 'Mali', 'Malta', 'Marshall Islands', 'Mauritania',
  'Mauritius', 'Mexico', 'Micronesia', 'Moldova', 'Monaco', 'Mongolia',
  'Montenegro', 'Morocco', 'Mozambique', 'Myanmar', 'Namibia', 'Nauru', 'Nepal',
  'Netherlands', 'New Zealand', 'Nicaragua', 'Niger', 'Nigeria', 'North Korea',
  'North Macedonia', 'Norway', 'Oman', 'Pakistan', 'Palau', 'Palestine',
  'Panama', 'Papua New Guinea', 'Paraguay', 'Peru', 'Philippines', 'Poland',
  'Portugal', 'Qatar', 'Romania', 'Russia', 'Rwanda', 'Saint Kitts and Nevis',
  'Saint Lucia', 'Saint Vincent and the Grenadines', 'Samoa', 'San Marino',
  'Sao Tome and Principe', 'Saudi Arabia', 'Senegal', 'Serbia', 'Seychelles',
  'Sierra Leone', 'Singapore', 'Slovakia', 'Slovenia', 'Solomon Islands',
  'Somalia', 'South Africa', 'South Korea', 'South Sudan', 'Spain',
  'Sri Lanka', 'Sudan', 'Suriname', 'Sweden', 'Switzerland', 'Syria', 'Taiwan',
  'Tajikistan', 'Tanzania', 'Thailand', 'Togo', 'Tonga',
  'Trinidad and Tobago', 'Tunisia', 'Turkey', 'Turkmenistan', 'Tuvalu',
  'Uganda', 'Ukraine', 'United Arab Emirates', 'United Kingdom',
  'United States', 'Uruguay', 'Uzbekistan', 'Vanuatu', 'Vatican City',
  'Venezuela', 'Vietnam', 'Yemen', 'Zambia', 'Zimbabwe',
]

// Codes Intl still answers a CURRENT region name for, but which no longer are
// one: ISO 3166-3 withdrawn codes, plus `UK` (an exceptional reservation for
// `GB`) and `FX` (Metropolitan France).
//
// These have to be excluded explicitly, and getting it wrong is not cosmetic —
// these codes go in the database. `Intl.DisplayNames` maps both `SU` and `RU` to
// "Russia", both `YU` and `RS` to "Serbia", both `HV` and `BF` to "Burkina
// Faso"; a reverse index that lets the later code win stores Russia as the
// Soviet Union and Serbia as Yugoslavia. Neither "first wins" nor "last wins" is
// correct across the 15 affected names, which is why the exclusion is a list
// rather than a rule.
const WITHDRAWN_CODES = new Set([
  'AN', // Netherlands Antilles -> CW et al.
  'BU', // Burma -> MM
  'CS', // Serbia and Montenegro -> RS
  'DD', // East Germany -> DE
  'DY', // Dahomey -> BJ
  'FX', // Metropolitan France -> FR
  'HV', // Upper Volta -> BF
  'NH', // New Hebrides -> VU
  'RH', // Southern Rhodesia -> ZW
  'SU', // Soviet Union -> RU
  'TP', // East Timor -> TL
  'UK', // exceptional reservation for GB
  'VD', // North Vietnam -> VN
  'YD', // South Yemen -> YE
  'YU', // Yugoslavia -> RS
  'ZR', // Zaire -> CD
])

// Reverse index of every current alpha-2 code Intl knows a region name for.
// Built by enumerating AA..ZZ because there is no way to ask Intl for its
// supported set; `of()` echoes the input back for a code it does not recognise,
// which is the discriminator used here.
//
// Two names claiming one code, or one name claiming two, are both hard errors.
// The second was a silent overwrite in the first draft of this file and is
// exactly how the withdrawn codes above got in.
function englishNameToCode() {
  const dn = new Intl.DisplayNames(['en'], { type: 'region' })
  const out = {}
  for (let a = 65; a < 91; a++) {
    for (let b = 65; b < 91; b++) {
      const code = String.fromCharCode(a, b)
      if (WITHDRAWN_CODES.has(code)) continue
      let name
      try {
        name = dn.of(code)
      } catch {
        continue
      }
      if (!name || name === code) continue
      if (out[name]) {
        throw new Error(`"${name}" resolves to both ${out[name]} and ${code} — one is withdrawn and belongs in WITHDRAWN_CODES`)
      }
      out[name] = code
    }
  }
  return out
}

/**
 * Legacy English name -> ISO 3166-1 alpha-2, for all 197 country names.
 * Throws if any name resolves to nothing or two names claim one code — either
 * would mean the migration below silently drops or merges rows.
 */
export function nameToCode() {
  const intl = englishNameToCode()
  const out = {}
  const unmapped = []
  for (const name of LEGACY_NAMES) {
    const code = HAND_MAPPED[name] ?? intl[name]
    if (!code) {
      unmapped.push(name)
      continue
    }
    out[name] = code
  }
  if (unmapped.length) {
    throw new Error(`no alpha-2 code for: ${unmapped.join(', ')}`)
  }
  const seen = new Map()
  for (const [name, code] of Object.entries(out)) {
    if (seen.has(code)) {
      throw new Error(`${code} claimed by both ${seen.get(code)} and ${name}`)
    }
    seen.set(code, name)
  }
  // A hand-mapped code Intl does not recognise would render as the bare code
  // forever, which looks like a working picker and is not one.
  const dn = new Intl.DisplayNames(['en'], { type: 'region' })
  for (const [name, code] of Object.entries(HAND_MAPPED)) {
    let rendered
    try {
      rendered = dn.of(code)
    } catch {
      throw new Error(`${name}: ${code} is not a valid region code`)
    }
    if (rendered === code) {
      throw new Error(`${name}: ${code} has no region name`)
    }
  }
  return out
}

/** Sentinels first, then the 197 codes sorted alphabetically by code. */
export function codeList() {
  return [...SENTINELS, ...Object.values(nameToCode()).sort()]
}

/**
 * The shared data file Go embeds: the offered code set, and for each country the
 * frozen legacy name plus the current English label.
 *
 * Go needs all three. It gates region display on the offered set (the same gate
 * `regionLabel()` applies, so the search index can never resolve a name the UI
 * renders verbatim), and it folds both names into the search blob so a query
 * that worked before the conversion still works after it.
 *
 * `label` is carried here rather than looked up server-side on purpose.
 * `golang.org/x/text` ships its own CLDR snapshot, and it does not agree with
 * the browser's: it calls `SZ` "Swaziland" and `MK` "Macedonia" where the
 * browser says "Eswatini" and "North Macedonia". Indexing x/text's answer would
 * put a name in the index that no reader can see on screen.
 */
export function dataFile() {
  const dn = new Intl.DisplayNames(['en'], { type: 'region' })
  return {
    _generated: 'node web/scripts/i18nRegionCodes.mjs --json > internal/isms/api/regions.json',
    sentinels: SENTINELS,
    entries: Object.entries(nameToCode()).map(([legacy, code]) => ({
      code,
      legacy,
      label: dn.of(code),
    })),
  }
}

/** The migration's CASE arms, exact-match on the stored English name. */
function sql() {
  const arms = Object.entries(nameToCode())
    .map(([name, code]) => `        WHEN ${quote(name)} THEN '${code}'`)
    .join('\n')
  return `UPDATE legal_requirements\n   SET jurisdiction = CASE jurisdiction\n${arms}\n        ELSE jurisdiction\n    END\n WHERE jurisdiction IN (\n${
    Object.keys(nameToCode()).map((n) => `        ${quote(n)}`).join(',\n')
  }\n );`
}

function quote(s) {
  return `'${s.replace(/'/g, "''")}'`
}

if (process.argv.includes('--codes')) console.log(codeList().join('\n'))
else if (process.argv.includes('--sql')) console.log(sql())
else if (process.argv.includes('--json')) console.log(`${JSON.stringify(dataFile(), null, 2)}\n`)
