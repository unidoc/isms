// Highlights @-mentions in already-escaped comment HTML for the Documents
// page. That page's own mention dropdown inserts a member's display name, which
// can be two words ("@Jane Doe"), while the other comment boxes insert
// "@email" (#404). One pass with the email form tried first, so an address is
// highlighted whole and never split into a name plus a stray ".com", and no
// highlight is nested inside another.
const MENTION = /@([\w.+-]+@[\w.-]+|\w+(?:\s\w+)?)/g

export function highlightMentions(escaped) {
  return escaped.replace(MENTION, '<span class="text-blue-400 font-medium">@$1</span>')
}
