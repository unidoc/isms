import { ref } from 'vue'
import { api } from '../api'
import { isSubdomainMode } from './useCurrentOrg'

const notifications = ref([])
const unreadCount = ref(0)
const showNotifications = ref(false)

export function useNotifications() {
  async function loadNotifications(userEmail) {
    try {
      const data = await api.getNotifications(userEmail, true)
      notifications.value = Array.isArray(data) ? data.filter(n => n && n.id) : []
    } catch {
      notifications.value = []
    }
  }

  async function loadUnreadCount() {
    try {
      const result = await api.getUnreadCount()
      unreadCount.value = result?.count || 0
    } catch { /* ignore */ }
  }

  async function markRead(n, orgSlug, router) {
    try {
      if (!n.read) {
        await api.markRead(n.id)
        n.read = true
        unreadCount.value = Math.max(0, unreadCount.value - 1)
      }
      // Navigate to linked resource if available. On subdomain hosts the org
      // slug is implicit in the hostname — paths are top-level. On apex/dev
      // the slug must be prepended.
      if (n.link) {
        showNotifications.value = false
        const orgPrefix = isSubdomainMode() ? '' : (orgSlug ? `/${orgSlug}` : '')
        router.push(orgPrefix + n.link)
      }
    } catch { /* ignore */ }
  }

  async function markAllRead(userEmail) {
    try {
      await api.markAllRead(userEmail)
      notifications.value.forEach(n => { n.read = true })
      unreadCount.value = 0
    } catch { /* ignore */ }
  }

  function formatNotifDate(dateStr) {
    if (!dateStr && dateStr !== 0) return ''
    const d = typeof dateStr === 'number' ? new Date(dateStr * 1000) : new Date(dateStr)
    return d.toLocaleDateString('en-GB', { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' })
  }

  // Listen for external notification changes (e.g. Inbox page marking all read)
  if (typeof window !== 'undefined') {
    window.addEventListener('isms:notifications-changed', () => {
      loadUnreadCount()
    })
  }

  return {
    notifications,
    unreadCount,
    showNotifications,
    loadNotifications,
    loadUnreadCount,
    markRead,
    markAllRead,
    formatNotifDate,
  }
}
