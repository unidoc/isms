import { ref } from 'vue'
import { api } from '../api.js'

// Shared cache - loaded once, reused across components
const members = ref([])
const loaded = ref(false)
const loading = ref(false)

// Display name for a stored email: the member's full name, else the email itself.
export function nameFor(email) {
  if (!email) return ''
  const m = members.value.find(x => x.email === email)
  return m?.fullName || email
}

// "Name (email)" when a name is known, else the email.
export function labelFor(email) {
  const n = nameFor(email)
  return n && n !== email ? `${n} (${email})` : email || ''
}

export function useMembers() {
  async function loadMembers() {
    if (loaded.value || loading.value) return
    loading.value = true
    try {
      const data = await api.getUsers()
      members.value = (Array.isArray(data) ? data : []).map(u => ({
        id: u.id,
        email: u.email || '',
        name: u.name || u.email?.split('@')[0] || '',
        role: u.role || '',
        fullName: u.name || '',
      }))
      loaded.value = true
    } catch {
      members.value = []
    }
    loading.value = false
  }

  // Auto-load on first use
  if (!loaded.value && !loading.value) loadMembers()

  return { members, loadMembers, nameFor, labelFor }
}
