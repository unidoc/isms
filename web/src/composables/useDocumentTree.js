import { ref, reactive, computed } from 'vue'

// Natural sort: splits on digits/non-digits and compares segments numerically where possible
function naturalCompare(a, b) {
  const re = /(\d+)|(\D+)/g
  const aParts = (a || '').match(re) || []
  const bParts = (b || '').match(re) || []
  for (let i = 0; i < Math.max(aParts.length, bParts.length); i++) {
    if (i >= aParts.length) return -1
    if (i >= bParts.length) return 1
    const aNum = Number(aParts[i])
    const bNum = Number(bParts[i])
    if (!isNaN(aNum) && !isNaN(bNum)) {
      if (aNum !== bNum) return aNum - bNum
    } else {
      const cmp = aParts[i].localeCompare(bParts[i])
      if (cmp !== 0) return cmp
    }
  }
  return 0
}

export function useDocumentTree(api) {
  const folders = ref([])
  const activeFolder = ref('')
  const loadingTree = ref(true)
  const expandedNodes = reactive(new Set())
  const showTreePanel = ref(true)

  function formatFileName(file) {
    const parts = (file.path || '').split('/')
    return parts[parts.length - 1].replace(/\.md$/, '')
  }

  function formatFileTitle(file) {
    return file.title || formatFileName(file)
  }

  function formatFolderName(node) {
    return node.title || node.name
  }

  // Build nested tree from flat folder data
  const fileTree = computed(() => {
    const tree = []
    // Build folder title lookup from API subfolders (recursive)
    const folderTitles = {}
    function collectTitles(f, pathPrefix) {
      const p = pathPrefix ? pathPrefix + '/' + f.name : f.name
      if (f.title) folderTitles[p] = f.title
      for (const sub of (f.subfolders || [])) collectTitles(sub, p)
    }
    for (const folder of folders.value) {
      collectTitles(folder, '')
    }

    for (const folder of folders.value) {
      const folderNode = { name: folder.name, title: folder.title || '', type: 'folder', path: folder.name, depth: 0, children: [], fileCount: 0 }

      // Collect files from nested subfolders too
      const allFiles = [...(folder.files || [])]
      function collectFiles(f) {
        for (const sub of (f.subfolders || [])) {
          allFiles.push(...(sub.files || []))
          collectFiles(sub)
        }
      }
      collectFiles(folder)

      for (const file of allFiles) {
        const parts = file.path.split('/')
        let current = folderNode
        for (let i = 1; i < parts.length - 1; i++) {
          const segName = parts[i]
          const segPath = parts.slice(0, i + 1).join('/')
          let existing = current.children.find(c => c.type === 'folder' && c.name === segName)
          if (!existing) {
            existing = { name: segName, title: folderTitles[segPath] || '', type: 'folder', path: segPath, depth: i, children: [], fileCount: 0 }
            current.children.push(existing)
          }
          current = existing
        }
        // Add file node
        current.children.push({
          name: formatFileTitle(file),
          type: 'file',
          file: file,
          folder: folder.name,
          depth: parts.length - 1,
          path: file.path,
        })
      }

      // Sort children recursively and compute file counts
      function sortAndCount(node) {
        if (node.type !== 'folder') return 0
        let count = 0
        for (const child of node.children) {
          if (child.type === 'folder') {
            count += sortAndCount(child)
          } else {
            count++
          }
        }
        node.fileCount = count
        // Sort: folders first (sorted naturally), then files (sorted naturally)
        const folderChildren = node.children.filter(c => c.type === 'folder').sort((a, b) => naturalCompare(a.name, b.name))
        const fileChildren = node.children.filter(c => c.type === 'file').sort((a, b) => naturalCompare(a.file.document_id || a.file.path, b.file.document_id || b.file.path))
        node.children = [...folderChildren, ...fileChildren]
        return count
      }
      sortAndCount(folderNode)
      tree.push(folderNode)
    }
    return tree.sort((a, b) => naturalCompare(a.name, b.name))
  })

  function toggleTreeNode(path) {
    if (expandedNodes.has(path)) {
      expandedNodes.delete(path)
    } else {
      expandedNodes.add(path)
    }
  }

  // Expand tree path to a file (for auto-select on mount/route change)
  function expandPathToFile(filePath) {
    if (!filePath) return
    const parts = filePath.split('/')
    // Expand each parent segment
    for (let i = 1; i < parts.length; i++) {
      expandedNodes.add(parts.slice(0, i).join('/'))
    }
  }

  // Find a file by document_id across all folders
  function findFileInFolder(folderName, docId) {
    const folder = folders.value.find(f => f.name === folderName)
    if (!folder) return null
    // Search top-level files and all nested subfolders
    function searchFiles(node) {
      const match = (node.files || []).find(f => f.document_id === docId)
      if (match) return match
      for (const sub of (node.subfolders || [])) {
        const found = searchFiles(sub)
        if (found) return found
      }
      return null
    }
    return searchFiles(folder)
  }

  // Collect all folder paths from the tree
  const allFolderPaths = computed(() => {
    const paths = []
    function walk(folder, prefix) {
      const p = prefix ? prefix + '/' + folder.name : folder.name
      paths.push(p)
      for (const sub of (folder.subfolders || [])) walk(sub, p)
    }
    for (const f of folders.value) walk(f, '')
    return paths
  })

  // Load the full document tree from the API
  async function loadTree() {
    const data = await api.getAllDocuments()
    folders.value = Array.isArray(data) ? data : []
    return folders.value
  }

  return {
    folders,
    fileTree,
    expandedNodes,
    activeFolder,
    showTreePanel,
    loadingTree,
    toggleTreeNode,
    expandPathToFile,
    formatFolderName,
    formatFileTitle,
    formatFileName,
    findFileInFolder,
    allFolderPaths,
    loadTree,
  }
}
