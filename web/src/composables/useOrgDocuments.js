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

// Every folder path of the tree ("policies", "policies/it"), for a picker.
export function flattenFolderPaths(folders) {
  const out = []
  const walk = (list, prefix) => {
    for (const f of list || []) {
      const p = prefix ? `${prefix}/${f.name}` : f.name
      out.push(p)
      walk(f.subfolders, p)
    }
  }
  walk(folders, '')
  return out
}

export function useOrgDocuments() {
  const documents = ref([])
  const folderPaths = ref([])
  const loading = ref(false)
  const error = ref(null)
  async function load() {
    loading.value = true
    error.value = null
    try {
      const tree = await api.getAllDocuments()
      documents.value = flattenDocuments(tree)
      folderPaths.value = flattenFolderPaths(tree)
    } catch (e) {
      documents.value = []
      folderPaths.value = []
      error.value = e
    } finally {
      loading.value = false
    }
  }
  return { documents, folderPaths, loading, error, load }
}
