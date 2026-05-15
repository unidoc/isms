import { ref, computed, nextTick } from 'vue'
import { getCurrentUser } from '../api'

/**
 * Composable for document inline/block comments.
 *
 * @param {Object} deps
 * @param {import('vue').Ref} deps.activeId - current document ID
 * @param {import('vue').ComputedRef} deps.contentBlocks - computed content blocks
 * @param {Object} deps.api - API module
 * @param {import('vue').Ref} deps.commentAuthor - comment author name/email ref
 */
export function useDocumentComments({ activeId, contentBlocks, api, commentAuthor }) {
  const comments = ref([])
  const loadingComments = ref(false)

  const inlineCommentCount = computed(() =>
    comments.value.filter(c => c.paragraph_index != null && c.status !== 'resolved').length
  )

  async function loadComments(docId) {
    loadingComments.value = true
    comments.value = []
    try {
      const data = await api.getDocComments(docId)
      comments.value = Array.isArray(data) ? data : []
    } catch {
      comments.value = []
    } finally {
      loadingComments.value = false
    }
  }

  // --- Inline comment helpers ---
  function blockHash(index) {
    const block = contentBlocks.value[index]
    if (!block) return ''
    const text = block.raw || block.html || ''
    let h = 5381
    for (let i = 0; i < text.length; i++) {
      h = ((h << 5) + h + text.charCodeAt(i)) & 0xffffffff
    }
    return h.toString(36)
  }

  function commentsForBlock(index) {
    const h = blockHash(index)
    return comments.value.filter(c => {
      if (c.paragraph_hash && h) {
        if (c.paragraph_hash !== h) return false
        if (c.paragraph_index != null) return c.paragraph_index === index
        return true
      }
      return c.paragraph_index === index
    })
  }

  function hasOpenComments(index) {
    return commentsForBlock(index).some(c => c.status !== 'resolved')
  }

  function commentCountForBlock(index) {
    return commentsForBlock(index).filter(c => c.status !== 'resolved').length
  }

  // --- Inline comment UI state ---
  const expandedBlock = ref(null)
  const inlineCommentText = ref('')
  const submittingInline = ref(false)

  function toggleBlockComments(index) {
    if (expandedBlock.value === index) {
      expandedBlock.value = null
      inlineCommentText.value = ''
    } else {
      expandedBlock.value = index
      inlineCommentText.value = ''
      focusInlineTextarea()
    }
  }

  function focusInlineTextarea() {
    nextTick(() => {
      const textareas = document.querySelectorAll('.comment-block textarea')
      if (textareas.length > 0) {
        // Focus the last (newly created) textarea
        textareas[textareas.length - 1].focus()
      }
    })
  }

  function startInlineComment(index) {
    expandedBlock.value = index
    inlineCommentText.value = ''
    focusInlineTextarea()
  }

  async function submitInlineComment(blockIndex) {
    if (!inlineCommentText.value.trim() || submittingInline.value) return
    submittingInline.value = true
    try {
      let body = inlineCommentText.value.trim()
      const author = (commentAuthor && commentAuthor.value ? commentAuthor.value.trim() : '') || getCurrentUser()
      await api.addComment({
        document_id: activeId.value,
        author,
        body: body,
        paragraph_index: blockIndex,
        paragraph_hash: blockHash(blockIndex),
        quote: extractQuote(blockIndex),
      })
      inlineCommentText.value = ''
      await loadComments(activeId.value)
    } catch (e) {
      console.error('Failed to submit inline comment:', e)
    } finally {
      submittingInline.value = false
    }
  }

  function extractQuote(blockIndex) {
    // For table row composite indices, find the block and row
    const block = contentBlocks.value[blockIndex] || contentBlocks.value[Math.floor(blockIndex / 1000)]
    if (!block) return ''
    // Strip HTML and take first 100 chars of the paragraph text
    const text = block.text.trim()
    return text.length > 100 ? text.substring(0, 100) + '...' : text
  }

  return {
    comments,
    loadingComments,
    inlineCommentCount,
    loadComments,
    commentsForBlock,
    hasOpenComments,
    commentCountForBlock,
    blockHash,
    expandedBlock,
    inlineCommentText,
    submittingInline,
    toggleBlockComments,
    startInlineComment,
    submitInlineComment,
    extractQuote,
  }
}
