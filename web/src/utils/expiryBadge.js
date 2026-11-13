// Badge colours for a validity state ("expired", "expiring", "current"), shared by
// every list that shows one: supplier external links and trust center
// certifications read the same way.
export function stateClass(state) {
  if (state === 'expired') return 'bg-red-900/40 text-red-300'
  if (state === 'expiring') return 'bg-amber-900/40 text-amber-300'
  return 'bg-emerald-900/40 text-emerald-300'
}
