// A closed risk is finished, so it is never overdue for review. Closing keeps
// the old next_review, which would otherwise turn "overdue" by itself. Mirrors
// the `status NOT IN ('closed')` filter in internal/isms/db/overdue.go.
export function isRiskReviewOverdue(risk, now = new Date()) {
  if (!risk || !risk.next_review || risk.status === 'closed') return false
  const d = typeof risk.next_review === 'number'
    ? new Date(risk.next_review * 1000)
    : new Date(risk.next_review)
  return d < now
}
