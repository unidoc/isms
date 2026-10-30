// The org brand color is admin-chosen, so it can be any hex, including ones
// that vanish on the dark theme (#408: #1a365d is 1.2:1 on slate-800). Link and
// button text therefore uses a derived --brand-text-color: the brand color
// mixed toward white by the smallest amount that reaches 4.5:1. Fills that
// carry white labels use --brand-fill-color, the brand color darkened until
// white reaches 4.5:1. Other fills and accents keep the raw --brand-color.
// Every writer goes through applyBrandColor so the variables never drift apart.

// Tailwind v4 slate-800 as rendered, oklch(27.9% .041 260.031); v3 was #1e293b.
export const TEXT_BACKGROUND = '#1d293d' // the card/sidebar surface
export const MIN_TEXT_CONTRAST = 4.5 // WCAG AA for normal text

// Returns [r, g, b] for #rgb, #rgba, #rrggbb and #rrggbbaa (alpha dropped),
// or null when the value is not one of those.
export function parseHex(hex) {
  if (typeof hex !== 'string') return null
  const m = /^#([0-9a-f]{3,4}|[0-9a-f]{6}|[0-9a-f]{8})$/i.exec(hex)
  if (!m) return null
  let h = m[1]
  if (h.length <= 4) h = h.split('').map((c) => c + c).join('')
  return [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16))
}

function luminance([r, g, b]) {
  const lin = (v) => {
    const s = v / 255
    return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4
  }
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b)
}

// WCAG 2 contrast ratio between two hex colors (1 to 21).
export function contrastRatio(a, b) {
  const la = luminance(parseHex(a))
  const lb = luminance(parseHex(b))
  return (Math.max(la, lb) + 0.05) / (Math.min(la, lb) + 0.05)
}

const toHex = (rgb) => '#' + rgb.map((v) => v.toString(16).padStart(2, '0')).join('')

// Mixes rgb toward `target` (255 = white, 0 = black) in 1% steps and returns the
// first mix whose contrast(mix) reaches the threshold; rgb itself when it
// already passes.
function mixUntilReadable(rgb, target, contrast) {
  for (let pct = 0; pct <= 100; pct++) {
    const mixed = toHex(rgb.map((v) => Math.round(v + (target - v) * pct / 100)))
    if (contrast(mixed) >= MIN_TEXT_CONTRAST) return mixed
  }
  return toHex([target, target, target])
}

// The brand color as readable text on the dark theme: unchanged when it already
// passes, otherwise mixed toward white. null for an unparseable hex.
export function readableTextColor(hex) {
  const rgb = parseHex(hex)
  if (!rgb) return null
  return mixUntilReadable(rgb, 255, (c) => contrastRatio(c, TEXT_BACKGROUND))
}

// The brand color as a fill that carries a white label: unchanged when white
// already reaches 4.5:1, otherwise mixed toward black. null for an unparseable
// hex.
export function readableFillColor(hex) {
  const rgb = parseHex(hex)
  if (!rgb) return null
  return mixUntilReadable(rgb, 0, (c) => contrastRatio('#ffffff', c))
}

// The Admin > Branding form starts its color at this value and saves every key,
// so orgs that saved branding for any reason store it (#408). It renders
// exactly like an unbranded org.
export const DEFAULT_BRAND = '#3b82f6'

const OVERRIDES = ['--brand-color', '--brand-text-color', '--brand-fill-color']

// Sets --brand-color (raw), --brand-text-color and --brand-fill-color
// (derived). A missing, empty or unparseable value, or the default brand,
// removes all three so the :root defaults in App.vue apply; this also clears
// the previous org's colors when the new one has none.
export function applyBrandColor(hex) {
  const root = document.documentElement.style
  const rgb = parseHex(hex)
  if (!rgb || toHex(rgb) === DEFAULT_BRAND) {
    OVERRIDES.forEach((name) => root.removeProperty(name))
    return
  }
  root.setProperty('--brand-color', hex)
  root.setProperty('--brand-text-color', readableTextColor(hex))
  root.setProperty('--brand-fill-color', readableFillColor(hex))
}
