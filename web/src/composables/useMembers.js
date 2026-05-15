import { ref } from 'vue'
import { api } from '../api'

// Shared cache - loaded once, reused across components
const members = ref([])
const loaded = ref(false)
const loading = ref(false)

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
      }))
      loaded.value = true
    } catch {
      members.value = []
    }
    loading.value = false
  }

  // Auto-load on first use
  if (!loaded.value && !loading.value) loadMembers()

  return { members, loadMembers }
}
