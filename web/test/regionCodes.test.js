// The jurisdiction seam: the generated code list, and the three-tier renderer
// that turns a stored value into something a reader recognises.
//
// `legal_requirements.jurisdiction` used to store English display names, so it
// could never be translated. It stores ISO 3166-1 alpha-2 codes now. Two things
// have to hold for that to be safe, and both are asserted here: the picker
// offers exactly the codes the migration produced, and every value the column
// can legally hold renders as something rather than blank.
import test from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { codeList, nameToCode, SENTINELS } from '../scripts/i18nRegionCodes.mjs'
import countries from '../src/data/countries.js'
import { REGION_SENTINELS, regionLabel } from '../src/composables/useFormat.js'
import { i18n } from '../src/i18n.js'

test('the generator agrees with itself: every name maps, uniquely, to a real region', () => {
  // nameToCode throws on an unmapped name, a duplicate code, or a hand-mapped
  // code x/Intl has no region for. Calling it IS the assertion; the counts below
  // just pin the shape so a silently emptied table cannot pass.
  const map = nameToCode()
  assert.equal(Object.keys(map).length, 197)
  assert.equal(new Set(Object.values(map)).size, 197)
  assert.equal(codeList().length, 201) // 197 countries + 4 sentinels
  assert.deepEqual(codeList().slice(0, 4), SENTINELS)
})

test('the committed data file matches the generator', () => {
  // This is the anti-drift check. A code in `countries.js` the migration never
  // produced splits old rows from new ones; a code the migration produced that
  // the picker does not offer leaves a row nobody can edit to a valid value.
  // Regenerate with `node scripts/i18nRegionCodes.mjs --codes` rather than
  // editing the data file by hand.
  assert.deepEqual(countries, codeList())
})

test('withdrawn ISO codes never win a name', () => {
  // The bug this pins, caught by a picker test before it shipped. Intl answers a
  // CURRENT name for withdrawn ISO 3166-3 codes, so a reverse index that lets
  // the later code win stored Russia as `SU` (Soviet Union), Serbia as `YU`
  // (Yugoslavia), Burkina Faso as `HV` (Upper Volta), Benin as `DY` (Dahomey),
  // France as `FX` and the UK as `UK`. These codes go in the DATABASE, so this
  // is not a display nit.
  //
  // Neither "first code wins" nor "last code wins" is right across the 15
  // affected names, which is why the generator carries an explicit exclusion
  // list — and why the canonical answer for each is asserted here rather than
  // derived. x/text is NOT a cross-check for this: `language.ParseRegion` returns
  // `SU` and `YU` unchanged rather than canonicalising them, so it would confirm
  // a wrong list just as happily.
  const map = nameToCode()
  const canonical = {
    Benin: 'BJ',
    'Burkina Faso': 'BF',
    France: 'FR',
    Germany: 'DE',
    Russia: 'RU',
    Serbia: 'RS',
    'United Kingdom': 'GB',
    Vanuatu: 'VU',
    Vietnam: 'VN',
    Yemen: 'YE',
    Zimbabwe: 'ZW',
  }
  for (const [name, code] of Object.entries(canonical)) {
    assert.equal(map[name], code, `${name} must be ${code}`)
  }
  const withdrawn = ['AN', 'BU', 'CS', 'DD', 'DY', 'FX', 'HV', 'NH', 'RH', 'SU', 'TP', 'UK', 'VD', 'YD', 'YU', 'ZR']
  const offered = new Set(codeList())
  for (const code of withdrawn) {
    assert.ok(!offered.has(code), `${code} is withdrawn and must not be offered`)
  }
})

test('the generator and the runtime agree on which values are sentinels', () => {
  // Two copies exist on purpose: the generator produces the migration and must
  // not import from `src/`. They must not drift — a value that is a sentinel to
  // one and a country to the other either gets converted by the migration or
  // loses its translated label.
  assert.deepEqual([...REGION_SENTINELS].sort(), [...SENTINELS].sort())
})

test('the legacy names table is frozen history, not display copy', () => {
  // The names exist only so the migration recognises what is already in the
  // column. "Fixing" a spelling here would stop it matching those rows — so
  // assert a few of the deliberately non-Intl spellings stay put.
  const map = nameToCode()
  assert.equal(map['Czech Republic'], 'CZ') // Intl says "Czechia"
  assert.equal(map['Turkey'], 'TR') //          Intl says "Türkiye"
  assert.equal(map['Ivory Coast'], 'CI') //     Intl says "Côte d'Ivoire"
  assert.equal(map['East Timor'], 'TL') //      Intl says "Timor-Leste"
})

test('the migration converts every legacy name and no sentinel', () => {
  // Globbed, not hardcoded. Appending to an unreleased release file requires
  // renaming it (the runner records applied migrations by filename), so a
  // hardcoded path would make the next append fail this test on the filename
  // rather than on anything about its content.
  const dir = new URL('../../migrations/', import.meta.url)
  const file = readdirSync(dir).find((f) => f.endsWith('_v0.8.0.sql'))
  assert.ok(file, 'no 0.8.0 migration found')
  const sql = readFileSync(new URL(file, dir), 'utf8')
  const jurisdiction = sql.slice(sql.indexOf('UPDATE legal_requirements'))
  assert.ok(jurisdiction.includes('UPDATE legal_requirements'), 'migration has no jurisdiction conversion')
  for (const [name, code] of Object.entries(nameToCode())) {
    const arm = `WHEN '${name.replace(/'/g, "''")}' THEN '${code}'`
    assert.ok(jurisdiction.includes(arm), `migration is missing: ${arm}`)
  }
  // A sentinel in the CASE would be a live bug, not a cosmetic one: 'EU' is a
  // real ISO exceptional reservation, so converting it turns the sentinel into a
  // country code and loses its translated endonym.
  for (const sentinel of SENTINELS) {
    assert.ok(!jurisdiction.includes(`WHEN '${sentinel}' THEN`), `sentinel ${sentinel} must not be converted`)
  }
})

test('regionLabel resolves sentinels before ISO, so EU is not "European Union"', () => {
  i18n.global.mergeLocaleMessage('en', {
    common: { region: { global: 'Global', eu: 'EU', eea: 'EEA', apac: 'APAC' } },
  })
  // The ordering that matters. `Intl.DisplayNames` renders EU as "European
  // Union", which is not what the picker means and would drop the translated
  // endonym in every other locale.
  assert.equal(regionLabel('EU'), 'EU')
  assert.equal(regionLabel('EEA'), 'EEA')
  assert.equal(regionLabel('APAC'), 'APAC')
  assert.equal(regionLabel('Global'), 'Global')
})

test('regionLabel renders codes, and falls back rather than throwing', () => {
  assert.equal(regionLabel('IS'), 'Iceland')
  assert.equal(regionLabel('GB'), 'United Kingdom')
  // Tier 3. The column is free text: the CLI and agent suggestions write
  // anything, historical changelog rows and suggestion payloads keep the
  // pre-migration English name permanently, and a row the migration did not
  // recognise must stay readable. `DisplayNames.of('Iceland')` throws
  // RangeError, which is why the guard is a regex and not a try/catch alone.
  assert.equal(regionLabel('Iceland'), 'Iceland')
  assert.equal(regionLabel('Germany/France'), 'Germany/France')
  assert.equal(regionLabel('is'), 'is') // lowercase is not a code we wrote
  assert.equal(regionLabel(''), '')
  assert.equal(regionLabel(null), '')
  assert.equal(regionLabel(undefined), '')
})

test('every code the picker offers renders as a name, not as itself', () => {
  // A code that rendered as the bare code would look like a working picker and
  // would not be one — the whole list has to be legible.
  for (const code of countries) {
    const label = regionLabel(code)
    assert.ok(label, `${code} renders blank`)
    if (SENTINELS.includes(code)) continue
    assert.notEqual(label, code, `${code} has no region name`)
  }
})

test('labels follow the active locale', () => {
  i18n.global.mergeLocaleMessage('id-ID', {
    common: { region: { global: 'Global', eu: 'UE', eea: 'EEA', apac: 'APAC' } },
  })
  const previous = i18n.global.locale.value
  try {
    i18n.global.locale.value = 'id-ID'
    // Both tiers have to move: the sentinel through the catalogue, the code
    // through Intl. If the Intl formatter were cached without the locale in its
    // key, this would still read "Iceland".
    assert.equal(regionLabel('EU'), 'UE')
    assert.equal(regionLabel('IS'), 'Islandia')
  } finally {
    i18n.global.locale.value = previous
  }
})
