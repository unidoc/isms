// The org brand color is admin-chosen, so it can be any hex, including ones
// that vanish on the dark theme (#408: #1a365d is 1.2:1 on slate-800). Link and
// button text therefore uses a derived --brand-text-color: the brand color
// mixed toward white by the smallest amount that reaches 4.5:1. Fills and
// accents keep the raw --brand-color. Every writer goes through
// applyBrandColor so the two variables never drift apart.

export const TEXT_BACKGROUND = '#1e293b' // Tailwind slate-800, the card/sidebar surface
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

// The brand color as readable text on the dark theme: unchanged when it already
// passes, otherwise mixed toward white in 1% steps. null for an unparseable hex.
export function readableTextColor(hex) {
  const rgb = parseHex(hex)
  if (!rgb) return null
  for (let pct = 0; pct <= 100; pct++) {
    const mixed = toHex(rgb.map((v) => Math.round(v + (255 - v) * pct / 100)))
    if (contrastRatio(mixed, TEXT_BACKGROUND) >= MIN_TEXT_CONTRAST) return mixed
  }
  return '#ffffff'
}

// Sets --brand-color (raw) and --brand-text-color (derived). Does nothing for
// an unparseable value so a bad stored color cannot blank the theme.
export function applyBrandColor(hex) {
  if (!parseHex(hex)) return
  const root = document.documentElement.style
  root.setProperty('--brand-color', hex)
  root.setProperty('--brand-text-color', readableTextColor(hex))
}
