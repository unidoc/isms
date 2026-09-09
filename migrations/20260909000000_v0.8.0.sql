-- The single migration for the 0.8.0 release. One migration per minor release,
-- named for the release (not for any one change). ALL schema changes shipping in
-- 0.8.0 accumulate in this file, appended in the order they land on master.
--
-- NB: this file has been renamed twice — from 20260823000000_v0.8.0.sql, which
-- briefly shipped #213 on master, and then from 20260824000000_v0.8.0.sql when
-- the jurisdiction conversion at the bottom was appended. Appending under an
-- existing name would be SKIPPED (the runner tracks applied migrations by
-- filename, see internal/isms/db/postgres.go) on any DB that already ran it, so
-- each append renames. That is safe only because 0.8.0 is unreleased and every
-- statement here is idempotent — re-running the whole file under the new name is
-- the expected consequence, not an accident. Keep both properties when you
-- append: bump the timestamp, and write statements that survive a second run.

-- Per-org custom risk categories (#213, merged first via #215): a JSON array in
-- organization_settings under 'risk_categories', e.g.
-- [{"key":"people_process","label":"People & Process"}]. Only the registry row is
-- seeded — GetOrgSetting queries `settings`, so without it every lookup errors
-- with "no rows". default_value stays NULL because defaults are virtual: unset
-- falls back to db.DefaultRiskCategories(), which leaves existing orgs unchanged
-- and lets the default list improve in later releases. risks.category is
-- deliberately untouched plain TEXT — a removed category orphans, it does not
-- cascade.
INSERT INTO settings (key, description, category, default_value, sensitive)
VALUES (
    'risk_categories',
    'Risk categories available to this organization, as a JSON array of {key, label}. Empty falls back to the built-in defaults.',
    'risk',
    NULL,
    false
)
ON CONFLICT (key) DO NOTHING;

-- Per-user language choice (#212): a column rather than a setting because
-- settings are per-organization and a user in two orgs has one language.
-- Nullable with no default on purpose — NULL means "never chose", which is what
-- lets the org default (and, pre-login, Accept-Language) apply; DEFAULT 'en'
-- would make every existing user look like they had picked English and shadow the
-- org default permanently. No CHECK on the value: the supported set lives in
-- internal/isms/i18n and changes with the binary, so a dropped locale must
-- degrade to the fallback, not wedge writes to users.
ALTER TABLE users ADD COLUMN IF NOT EXISTS locale TEXT;

-- The org-wide default locale (#212), a setting rather than an organizations
-- column: per-org config already lives in the settings registry, which brings the
-- admin settings API, UI and audit path with it. The description says "users who
-- have not chosen one" rather than "new users" because resolution applies it to
-- EVERY user whose locale is NULL — which right after this migration is all of
-- them — and this copy is what an admin reads in the settings UI.
INSERT INTO settings (key, description, category, default_value, sensitive) VALUES
    ('default_locale', 'Default language for users who have not chosen one, and for org-wide notifications (BCP 47 tag, e.g. en, id-ID)', 'localization', 'en', false)
ON CONFLICT (key) DO NOTHING;

-- Keyed notifications (#212). `notifications.title` / `body` are written
-- pre-rendered in English at 13 call sites, so a row can never be retranslated
-- once written — there is no key to re-render from. These three columns are the
-- key, and they are added BEFORE a non-English UI ships rather than after,
-- because after means real rows exist with nothing to recover from.
--
-- title/body deliberately stay populated in English. Three independent reasons,
-- any one sufficient: historical rows have no keys and must still render;
-- CreateAgentNotification rows are deliberately English (the MCP
-- get_pending_actions surface is LLM-facing and translation there is a
-- non-goal); and it is the fallback whenever a key exists but the client has no
-- message for it. Same shape as the API-error decision — additive key, the
-- server keeps a readable English original.
--
-- No CHECK tying the keys to a catalog: the catalog lives in the web locale
-- files and changes with the frontend, not with the schema. A key the client
-- cannot resolve falls back to `title`, which is exactly the desired failure.
--
-- body_key is nullable and often NULL on purpose: several bodies are org
-- authored content (an incident or corrective-action description, a mention
-- snippet), not product copy, and must never be translated.
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS title_key TEXT;
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS body_key  TEXT;

-- Bodies with an optional trailing note ("Note: <message>") use two keys, not
-- one frame with an optional `note` param: the "Note:" label is product copy
-- the frame owns, so a single frame either renders a dangling label when there
-- is no note or loses the label's translation. The with-note variant is the
-- base key plus "_with_note" (notifications.review_requested.body and
-- notifications.review_requested.body_with_note), and `note` appears in params
-- only for that variant.
--
-- Flat JSON object of interpolation values. Two categories, told apart by key
-- name so the renderer knows which to translate first: translatable enum-ish
-- params (status, severity, action, entity, suggestion_type) resolve through
-- common.enum.* / common.entity.* BEFORE interpolation, and verbatim params
-- (actor, title, doc_id, version, round, id, note, reason) interpolate as-is
-- because they are proper nouns, numbers, or user-authored text. Splicing an
-- untranslated enum into a translated frame yields half-translated output, so
-- the distinction is load-bearing rather than cosmetic.
ALTER TABLE notifications ADD COLUMN IF NOT EXISTS params JSONB;

-- Jurisdictions from English display names to ISO 3166-1 alpha-2 (#212). The
-- legal register's picker used to offer 201 English strings and store the string
-- it was given, so the column held display text — which can never be translated
-- for a reader in another language. It now offers codes and stores codes;
-- `regionLabel()` in web/src/composables/useFormat.js renders them per locale.
--
-- EXACT match only, and deliberately so. The column is free text with no CHECK:
-- the CLI (`isms legal add --jurisdiction`) and agent suggestions can write
-- anything, and real jurisdictions exist that are not countries. A LOWER()/TRIM()
-- match would convert a few more rows and would also risk rewriting a value a
-- manager meant literally. Whatever does not match is left alone and renders
-- verbatim, which is the third tier of regionLabel() and the reason it exists.
--
-- The four region sentinels ('Global', 'EU', 'EEA', 'APAC') are NOT in the CASE:
-- they are not ISO 3166 and keep their own `common.region.*` keys. 'EU' would
-- otherwise be a live hazard — it is a real ISO exceptional reservation, so
-- converting it would silently turn the sentinel into a country code.
--
-- Idempotent by construction: after one run no row holds a name the CASE
-- recognises, so a second run matches nothing. Generated, not hand-written —
-- `node web/scripts/i18nRegionCodes.mjs --sql` emits exactly this, from the same
-- table that generates web/src/data/countries.js, so the codes this produces and
-- the codes the picker offers cannot drift apart.
UPDATE legal_requirements
   SET jurisdiction = CASE jurisdiction
        WHEN 'Afghanistan' THEN 'AF'
        WHEN 'Albania' THEN 'AL'
        WHEN 'Algeria' THEN 'DZ'
        WHEN 'Andorra' THEN 'AD'
        WHEN 'Angola' THEN 'AO'
        WHEN 'Antigua and Barbuda' THEN 'AG'
        WHEN 'Argentina' THEN 'AR'
        WHEN 'Armenia' THEN 'AM'
        WHEN 'Australia' THEN 'AU'
        WHEN 'Austria' THEN 'AT'
        WHEN 'Azerbaijan' THEN 'AZ'
        WHEN 'Bahamas' THEN 'BS'
        WHEN 'Bahrain' THEN 'BH'
        WHEN 'Bangladesh' THEN 'BD'
        WHEN 'Barbados' THEN 'BB'
        WHEN 'Belarus' THEN 'BY'
        WHEN 'Belgium' THEN 'BE'
        WHEN 'Belize' THEN 'BZ'
        WHEN 'Benin' THEN 'BJ'
        WHEN 'Bhutan' THEN 'BT'
        WHEN 'Bolivia' THEN 'BO'
        WHEN 'Bosnia and Herzegovina' THEN 'BA'
        WHEN 'Botswana' THEN 'BW'
        WHEN 'Brazil' THEN 'BR'
        WHEN 'Brunei' THEN 'BN'
        WHEN 'Bulgaria' THEN 'BG'
        WHEN 'Burkina Faso' THEN 'BF'
        WHEN 'Burundi' THEN 'BI'
        WHEN 'Cabo Verde' THEN 'CV'
        WHEN 'Cambodia' THEN 'KH'
        WHEN 'Cameroon' THEN 'CM'
        WHEN 'Canada' THEN 'CA'
        WHEN 'Central African Republic' THEN 'CF'
        WHEN 'Chad' THEN 'TD'
        WHEN 'Chile' THEN 'CL'
        WHEN 'China' THEN 'CN'
        WHEN 'Colombia' THEN 'CO'
        WHEN 'Comoros' THEN 'KM'
        WHEN 'Congo' THEN 'CG'
        WHEN 'Costa Rica' THEN 'CR'
        WHEN 'Croatia' THEN 'HR'
        WHEN 'Cuba' THEN 'CU'
        WHEN 'Cyprus' THEN 'CY'
        WHEN 'Czech Republic' THEN 'CZ'
        WHEN 'Democratic Republic of the Congo' THEN 'CD'
        WHEN 'Denmark' THEN 'DK'
        WHEN 'Djibouti' THEN 'DJ'
        WHEN 'Dominica' THEN 'DM'
        WHEN 'Dominican Republic' THEN 'DO'
        WHEN 'East Timor' THEN 'TL'
        WHEN 'Ecuador' THEN 'EC'
        WHEN 'Egypt' THEN 'EG'
        WHEN 'El Salvador' THEN 'SV'
        WHEN 'Equatorial Guinea' THEN 'GQ'
        WHEN 'Eritrea' THEN 'ER'
        WHEN 'Estonia' THEN 'EE'
        WHEN 'Eswatini' THEN 'SZ'
        WHEN 'Ethiopia' THEN 'ET'
        WHEN 'Fiji' THEN 'FJ'
        WHEN 'Finland' THEN 'FI'
        WHEN 'France' THEN 'FR'
        WHEN 'Gabon' THEN 'GA'
        WHEN 'Gambia' THEN 'GM'
        WHEN 'Georgia' THEN 'GE'
        WHEN 'Germany' THEN 'DE'
        WHEN 'Ghana' THEN 'GH'
        WHEN 'Greece' THEN 'GR'
        WHEN 'Grenada' THEN 'GD'
        WHEN 'Guatemala' THEN 'GT'
        WHEN 'Guinea' THEN 'GN'
        WHEN 'Guinea-Bissau' THEN 'GW'
        WHEN 'Guyana' THEN 'GY'
        WHEN 'Haiti' THEN 'HT'
        WHEN 'Honduras' THEN 'HN'
        WHEN 'Hungary' THEN 'HU'
        WHEN 'Iceland' THEN 'IS'
        WHEN 'India' THEN 'IN'
        WHEN 'Indonesia' THEN 'ID'
        WHEN 'Iran' THEN 'IR'
        WHEN 'Iraq' THEN 'IQ'
        WHEN 'Ireland' THEN 'IE'
        WHEN 'Israel' THEN 'IL'
        WHEN 'Italy' THEN 'IT'
        WHEN 'Ivory Coast' THEN 'CI'
        WHEN 'Jamaica' THEN 'JM'
        WHEN 'Japan' THEN 'JP'
        WHEN 'Jordan' THEN 'JO'
        WHEN 'Kazakhstan' THEN 'KZ'
        WHEN 'Kenya' THEN 'KE'
        WHEN 'Kiribati' THEN 'KI'
        WHEN 'Kosovo' THEN 'XK'
        WHEN 'Kuwait' THEN 'KW'
        WHEN 'Kyrgyzstan' THEN 'KG'
        WHEN 'Laos' THEN 'LA'
        WHEN 'Latvia' THEN 'LV'
        WHEN 'Lebanon' THEN 'LB'
        WHEN 'Lesotho' THEN 'LS'
        WHEN 'Liberia' THEN 'LR'
        WHEN 'Libya' THEN 'LY'
        WHEN 'Liechtenstein' THEN 'LI'
        WHEN 'Lithuania' THEN 'LT'
        WHEN 'Luxembourg' THEN 'LU'
        WHEN 'Madagascar' THEN 'MG'
        WHEN 'Malawi' THEN 'MW'
        WHEN 'Malaysia' THEN 'MY'
        WHEN 'Maldives' THEN 'MV'
        WHEN 'Mali' THEN 'ML'
        WHEN 'Malta' THEN 'MT'
        WHEN 'Marshall Islands' THEN 'MH'
        WHEN 'Mauritania' THEN 'MR'
        WHEN 'Mauritius' THEN 'MU'
        WHEN 'Mexico' THEN 'MX'
        WHEN 'Micronesia' THEN 'FM'
        WHEN 'Moldova' THEN 'MD'
        WHEN 'Monaco' THEN 'MC'
        WHEN 'Mongolia' THEN 'MN'
        WHEN 'Montenegro' THEN 'ME'
        WHEN 'Morocco' THEN 'MA'
        WHEN 'Mozambique' THEN 'MZ'
        WHEN 'Myanmar' THEN 'MM'
        WHEN 'Namibia' THEN 'NA'
        WHEN 'Nauru' THEN 'NR'
        WHEN 'Nepal' THEN 'NP'
        WHEN 'Netherlands' THEN 'NL'
        WHEN 'New Zealand' THEN 'NZ'
        WHEN 'Nicaragua' THEN 'NI'
        WHEN 'Niger' THEN 'NE'
        WHEN 'Nigeria' THEN 'NG'
        WHEN 'North Korea' THEN 'KP'
        WHEN 'North Macedonia' THEN 'MK'
        WHEN 'Norway' THEN 'NO'
        WHEN 'Oman' THEN 'OM'
        WHEN 'Pakistan' THEN 'PK'
        WHEN 'Palau' THEN 'PW'
        WHEN 'Palestine' THEN 'PS'
        WHEN 'Panama' THEN 'PA'
        WHEN 'Papua New Guinea' THEN 'PG'
        WHEN 'Paraguay' THEN 'PY'
        WHEN 'Peru' THEN 'PE'
        WHEN 'Philippines' THEN 'PH'
        WHEN 'Poland' THEN 'PL'
        WHEN 'Portugal' THEN 'PT'
        WHEN 'Qatar' THEN 'QA'
        WHEN 'Romania' THEN 'RO'
        WHEN 'Russia' THEN 'RU'
        WHEN 'Rwanda' THEN 'RW'
        WHEN 'Saint Kitts and Nevis' THEN 'KN'
        WHEN 'Saint Lucia' THEN 'LC'
        WHEN 'Saint Vincent and the Grenadines' THEN 'VC'
        WHEN 'Samoa' THEN 'WS'
        WHEN 'San Marino' THEN 'SM'
        WHEN 'Sao Tome and Principe' THEN 'ST'
        WHEN 'Saudi Arabia' THEN 'SA'
        WHEN 'Senegal' THEN 'SN'
        WHEN 'Serbia' THEN 'RS'
        WHEN 'Seychelles' THEN 'SC'
        WHEN 'Sierra Leone' THEN 'SL'
        WHEN 'Singapore' THEN 'SG'
        WHEN 'Slovakia' THEN 'SK'
        WHEN 'Slovenia' THEN 'SI'
        WHEN 'Solomon Islands' THEN 'SB'
        WHEN 'Somalia' THEN 'SO'
        WHEN 'South Africa' THEN 'ZA'
        WHEN 'South Korea' THEN 'KR'
        WHEN 'South Sudan' THEN 'SS'
        WHEN 'Spain' THEN 'ES'
        WHEN 'Sri Lanka' THEN 'LK'
        WHEN 'Sudan' THEN 'SD'
        WHEN 'Suriname' THEN 'SR'
        WHEN 'Sweden' THEN 'SE'
        WHEN 'Switzerland' THEN 'CH'
        WHEN 'Syria' THEN 'SY'
        WHEN 'Taiwan' THEN 'TW'
        WHEN 'Tajikistan' THEN 'TJ'
        WHEN 'Tanzania' THEN 'TZ'
        WHEN 'Thailand' THEN 'TH'
        WHEN 'Togo' THEN 'TG'
        WHEN 'Tonga' THEN 'TO'
        WHEN 'Trinidad and Tobago' THEN 'TT'
        WHEN 'Tunisia' THEN 'TN'
        WHEN 'Turkey' THEN 'TR'
        WHEN 'Turkmenistan' THEN 'TM'
        WHEN 'Tuvalu' THEN 'TV'
        WHEN 'Uganda' THEN 'UG'
        WHEN 'Ukraine' THEN 'UA'
        WHEN 'United Arab Emirates' THEN 'AE'
        WHEN 'United Kingdom' THEN 'GB'
        WHEN 'United States' THEN 'US'
        WHEN 'Uruguay' THEN 'UY'
        WHEN 'Uzbekistan' THEN 'UZ'
        WHEN 'Vanuatu' THEN 'VU'
        WHEN 'Vatican City' THEN 'VA'
        WHEN 'Venezuela' THEN 'VE'
        WHEN 'Vietnam' THEN 'VN'
        WHEN 'Yemen' THEN 'YE'
        WHEN 'Zambia' THEN 'ZM'
        WHEN 'Zimbabwe' THEN 'ZW'
        ELSE jurisdiction
    END
 WHERE jurisdiction IN (
        'Afghanistan',
        'Albania',
        'Algeria',
        'Andorra',
        'Angola',
        'Antigua and Barbuda',
        'Argentina',
        'Armenia',
        'Australia',
        'Austria',
        'Azerbaijan',
        'Bahamas',
        'Bahrain',
        'Bangladesh',
        'Barbados',
        'Belarus',
        'Belgium',
        'Belize',
        'Benin',
        'Bhutan',
        'Bolivia',
        'Bosnia and Herzegovina',
        'Botswana',
        'Brazil',
        'Brunei',
        'Bulgaria',
        'Burkina Faso',
        'Burundi',
        'Cabo Verde',
        'Cambodia',
        'Cameroon',
        'Canada',
        'Central African Republic',
        'Chad',
        'Chile',
        'China',
        'Colombia',
        'Comoros',
        'Congo',
        'Costa Rica',
        'Croatia',
        'Cuba',
        'Cyprus',
        'Czech Republic',
        'Democratic Republic of the Congo',
        'Denmark',
        'Djibouti',
        'Dominica',
        'Dominican Republic',
        'East Timor',
        'Ecuador',
        'Egypt',
        'El Salvador',
        'Equatorial Guinea',
        'Eritrea',
        'Estonia',
        'Eswatini',
        'Ethiopia',
        'Fiji',
        'Finland',
        'France',
        'Gabon',
        'Gambia',
        'Georgia',
        'Germany',
        'Ghana',
        'Greece',
        'Grenada',
        'Guatemala',
        'Guinea',
        'Guinea-Bissau',
        'Guyana',
        'Haiti',
        'Honduras',
        'Hungary',
        'Iceland',
        'India',
        'Indonesia',
        'Iran',
        'Iraq',
        'Ireland',
        'Israel',
        'Italy',
        'Ivory Coast',
        'Jamaica',
        'Japan',
        'Jordan',
        'Kazakhstan',
        'Kenya',
        'Kiribati',
        'Kosovo',
        'Kuwait',
        'Kyrgyzstan',
        'Laos',
        'Latvia',
        'Lebanon',
        'Lesotho',
        'Liberia',
        'Libya',
        'Liechtenstein',
        'Lithuania',
        'Luxembourg',
        'Madagascar',
        'Malawi',
        'Malaysia',
        'Maldives',
        'Mali',
        'Malta',
        'Marshall Islands',
        'Mauritania',
        'Mauritius',
        'Mexico',
        'Micronesia',
        'Moldova',
        'Monaco',
        'Mongolia',
        'Montenegro',
        'Morocco',
        'Mozambique',
        'Myanmar',
        'Namibia',
        'Nauru',
        'Nepal',
        'Netherlands',
        'New Zealand',
        'Nicaragua',
        'Niger',
        'Nigeria',
        'North Korea',
        'North Macedonia',
        'Norway',
        'Oman',
        'Pakistan',
        'Palau',
        'Palestine',
        'Panama',
        'Papua New Guinea',
        'Paraguay',
        'Peru',
        'Philippines',
        'Poland',
        'Portugal',
        'Qatar',
        'Romania',
        'Russia',
        'Rwanda',
        'Saint Kitts and Nevis',
        'Saint Lucia',
        'Saint Vincent and the Grenadines',
        'Samoa',
        'San Marino',
        'Sao Tome and Principe',
        'Saudi Arabia',
        'Senegal',
        'Serbia',
        'Seychelles',
        'Sierra Leone',
        'Singapore',
        'Slovakia',
        'Slovenia',
        'Solomon Islands',
        'Somalia',
        'South Africa',
        'South Korea',
        'South Sudan',
        'Spain',
        'Sri Lanka',
        'Sudan',
        'Suriname',
        'Sweden',
        'Switzerland',
        'Syria',
        'Taiwan',
        'Tajikistan',
        'Tanzania',
        'Thailand',
        'Togo',
        'Tonga',
        'Trinidad and Tobago',
        'Tunisia',
        'Turkey',
        'Turkmenistan',
        'Tuvalu',
        'Uganda',
        'Ukraine',
        'United Arab Emirates',
        'United Kingdom',
        'United States',
        'Uruguay',
        'Uzbekistan',
        'Vanuatu',
        'Vatican City',
        'Venezuela',
        'Vietnam',
        'Yemen',
        'Zambia',
        'Zimbabwe'
 );
