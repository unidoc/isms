// toDate converts an API date value (epoch seconds or date string) to a JS Date.
// Returns null for falsy values.
export function toDate(v) {
  if (!v && v !== 0) return null
  if (typeof v === 'number') return new Date(v * 1000)
  return new Date(v)
}
