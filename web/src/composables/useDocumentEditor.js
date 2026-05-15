import { ref, onBeforeUnmount } from 'vue'

/**
 * Composable for document editing state and draft autosave.
 *
 * @param {Object} deps
 * @param {import('vue').Ref} deps.activeId - current document ID
 * @param {import('vue').Ref} deps.activeDoc - current document object
 * @param {import('vue').Ref} deps.activeType - current folder/type
 * @param {import('vue').Ref} deps.rawContent - raw markdown content
 * @param {Object} deps.api - API module
 * @param {Function} deps.loadContent - function(folder, id) to reload content
 * @param {Function} deps.loadChangedDocs - function to reload changed docs list
 * @param {Function} deps.loadNeedsReview - function to reload needs-review list
 */
export function useDocumentEditor({ activeId, activeDoc, activeType, rawContent, api, loadContent, loadChangedDocs, loadNeedsReview }) {
  const editMode = ref(false)
  const editContent = ref('')
  const savingEdit = ref(false)
  const editSaveMsg = ref('')
  const editSaveError = ref(false)
  const editVersion = ref('')
  const editAuthor = ref('')
  const editOwner = ref('')

  // --- Draft autosave (localStorage) ---
  // Key includes org slug from URL to prevent cross-org draft leaks
  function draftKey(docId) {
    const org = window.location.pathname.split('/')[1] || 'default'
    return `isms_draft_${org}_${docId}`
  }

  function saveDraft() {
    if (!activeId.value || !editContent.value) return
    localStorage.setItem(draftKey(activeId.value), JSON.stringify({
      content: editContent.value,
      version: editVersion.value,
      savedAt: new Date().toISOString(),
    }))
  }

  const DRAFT_MAX_AGE_MS = 7 * 24 * 60 * 60 * 1000 // 7 days

  function loadDraft(docId) {
    try {
      const raw = localStorage.getItem(draftKey(docId))
      if (!raw) return null
      const draft = JSON.parse(raw)
      // Expire old drafts
      if (draft.savedAt && (Date.now() - new Date(draft.savedAt).getTime()) > DRAFT_MAX_AGE_MS) {
        localStorage.removeItem(draftKey(docId))
        return null
      }
      return draft
    } catch { return null }
  }

  function clearDraft(docId) {
    localStorage.removeItem(draftKey(docId || activeId.value))
  }

  let autosaveTimer = null
  function startAutosave() {
    stopAutosave()
    autosaveTimer = setInterval(saveDraft, 5000) // save every 5 seconds
  }
  function stopAutosave() {
    if (autosaveTimer) { clearInterval(autosaveTimer); autosaveTimer = null }
  }

  const hasDraft = ref(false)
  const draftSavedAt = ref('')

  function startEdit() {
    editMode.value = true
    editVersion.value = activeDoc.value?.version || '0.1'
    editAuthor.value = activeDoc.value?.author || ''
    editOwner.value = activeDoc.value?.owner || ''
    editSaveMsg.value = ''
    editSaveError.value = false

    // Check for unsaved draft — only recover if content genuinely differs
    const draft = loadDraft(activeId.value)
    if (draft && draft.content && draft.content.trim() && draft.content !== rawContent.value) {
      hasDraft.value = true
      draftSavedAt.value = draft.savedAt
      editContent.value = draft.content
      if (draft.version) editVersion.value = draft.version
    } else {
      // No valid draft or content matches — clear stale draft
      clearDraft(activeId.value)
      hasDraft.value = false
      editContent.value = rawContent.value
    }

    startAutosave()
  }

  function discardDraft() {
    clearDraft()
    editContent.value = rawContent.value
    editVersion.value = activeDoc.value?.version || '0.1'
    hasDraft.value = false
  }

  function cancelEdit() {
    stopAutosave()
    editMode.value = false
    editContent.value = ''
    editOwner.value = ''
    editSaveMsg.value = ''
    hasDraft.value = false
    // Keep draft in localStorage -- user might come back
  }

  async function saveEdit() {
    if (!activeId.value || savingEdit.value) return
    savingEdit.value = true
    editSaveMsg.value = ''
    editSaveError.value = false
    try {
      // Save content + metadata in one commit
      const payload = { content: editContent.value }
      const newVersion = editVersion.value.trim()
      if (newVersion && newVersion !== activeDoc.value?.version) {
        payload.version = newVersion
      }
      if (editAuthor.value && editAuthor.value !== activeDoc.value?.author) {
        payload.author = editAuthor.value
      }
      if (editOwner.value !== (activeDoc.value?.owner || '')) {
        payload.owner = editOwner.value
      }
      await api.updateDocumentContent(activeId.value, payload.content, payload.version, payload.author, payload.owner)
      if (payload.version && activeDoc.value) activeDoc.value.version = payload.version
      if (payload.author && activeDoc.value) activeDoc.value.author = payload.author
      if (payload.owner !== undefined && activeDoc.value) activeDoc.value.owner = payload.owner
      editSaveMsg.value = 'Saved'
      clearDraft()
      stopAutosave()
      editMode.value = false
      hasDraft.value = false
      // Reload content + refresh data
      if (activeType.value && activeId.value) {
        await loadContent(activeType.value, activeId.value)
      }
      loadChangedDocs()
      loadNeedsReview()
      setTimeout(() => { editSaveMsg.value = '' }, 3000)
    } catch (e) {
      editSaveMsg.value = 'Failed to save: ' + e.message
      editSaveError.value = true
    } finally {
      savingEdit.value = false
    }
  }

  onBeforeUnmount(() => {
    stopAutosave()
  })

  return {
    editMode,
    editContent,
    savingEdit,
    editVersion,
    editAuthor,
    editOwner,
    editSaveMsg,
    editSaveError,
    draftKey,
    saveDraft,
    loadDraft,
    clearDraft,
    startAutosave,
    stopAutosave,
    hasDraft,
    draftSavedAt,
    startEdit,
    cancelEdit,
    saveEdit,
    discardDraft,
  }
}
