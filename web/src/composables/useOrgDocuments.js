import { ref } from 'vue'
import { api } from '../api.js'

// Every document of the organization as a flat list {document_id, title,
// status}, from the folder tree GET /documents/all returns.
export function flattenDocuments(folders) {
  const out = []
  const walk = (list) => {
    for (const f of list || []) {
      for (const d of f.files || []) {
        if (d.document_id) out.push({ document_id: d.document_id, title: d.title || d.document_id, status: d.status || '' })
      }
      walk(f.subfolders)
    }
  }
  walk(folders)
  return out
}

export function useOrgDocuments() {
  const documents = ref([])
  const loading = ref(false)
  const error = ref(null)
  async function load() {
    loading.value = true
    error.value = null
    try {
      documents.value = flattenDocuments(await api.getAllDocuments())
    } catch (e) {
      documents.value = []
      error.value = e
    } finally {
      loading.value = false
    }
  }
  return { documents, loading, error, load }
}
