// Whether a user may act on an item assigned to `assignee`: managers and admins
// always, a contributor only on an item assigned to them (#203). Mirrors
// canActOnAssignment in internal/isms/api/assignment.go, including the
// case-insensitive email match, so the UI offers exactly what the API allows.
export function canActOnAssignment(role, email, assignee) {
  if (role === 'admin' || role === 'manager') return true
  if (role !== 'contributor' || !assignee || !email) return false
  return assignee.toLowerCase() === email.toLowerCase()
}
