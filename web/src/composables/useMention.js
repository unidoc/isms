export function renderMention(body) {
  if (!body) return ''
  const escaped = body.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
  return escaped.replace(/@([\w.+-]+@[\w.-]+)/g, '<span class="text-blue-400 font-medium">@$1</span>')
    .replace(/@(\w+)/g, '<span class="text-blue-400 font-medium">@$1</span>')
}
