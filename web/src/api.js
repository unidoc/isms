const API = '/api/v1'

// Get the API token from localStorage
export function getApiToken() {
  return localStorage.getItem('isms_api_token') || ''
}

export function setApiToken(token) {
  localStorage.setItem('isms_api_token', token)
}

export function clearApiToken() {
  localStorage.removeItem('isms_api_token')
  localStorage.removeItem('isms_user_email')
  localStorage.removeItem('isms_user_name')
}

// Get the current user email from stored session or token
export function getCurrentUser() {
  return localStorage.getItem('isms_user_email') || ''
}

function setCurrentUser(email, name) {
  if (email) localStorage.setItem('isms_user_email', email)
  if (name) localStorage.setItem('isms_user_name', name)
}

function getHeaders() {
  const headers = { 'Content-Type': 'application/json' }
  const token = getApiToken()
  if (token) {
    headers['Authorization'] = 'Bearer ' + token
  }
  return headers
}

async function fetchRaw(url) {
  let res
  try {
    res = await fetch(url, { headers: getHeaders() })
  } catch (e) {
    const err = new Error(e.message || 'Network error')
    err.isNetwork = true
    throw err
  }
  if (res.status === 401) {
    clearApiToken()
    window.dispatchEvent(new Event('isms:unauthorized'))
    const err = new Error('Authentication required')
    err.status = 401
    throw err
  }
  if (!res.ok) {
    const err = new Error(`${res.status} ${res.statusText}`)
    err.status = res.status
    throw err
  }
  return await res.json()
}

async function fetchJSON(url) {
  const json = await fetchRaw(url)
  // Unwrap {"data": [...]} wrapper from simple list endpoints (used for unpaginated lists)
  if (json && typeof json === 'object' && !Array.isArray(json) && 'data' in json) {
    if (Array.isArray(json.data)) return json.data
    if (json.data === null && Object.keys(json).length === 1) return []
  }
  return json
}

// Wrap fetch to catch abort/network errors separately from HTTP errors
async function safeFetch(url, opts) {
  try {
    return await fetch(url, opts)
  } catch (e) {
    const err = new Error(e.message || 'Network error')
    err.isNetwork = true
    throw err
  }
}

function checkAuth(res) {
  if (res.status === 401) {
    clearApiToken()
    window.dispatchEvent(new Event('isms:unauthorized'))
    const err = new Error('Authentication required')
    err.status = 401
    throw err
  }
}

async function postJSON(url, data) {
  const res = await safeFetch(url, { method: 'POST', headers: getHeaders(), body: JSON.stringify(data) })
  checkAuth(res)
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    const err = new Error(body.message || body.error || `${res.status} ${res.statusText}`)
    err.status = res.status
    throw err
  }
  return res.json()
}

async function putJSON(url, data) {
  const res = await safeFetch(url, { method: 'PUT', headers: getHeaders(), body: JSON.stringify(data) })
  checkAuth(res)
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    const err = new Error(body.message || body.error || `${res.status} ${res.statusText}`)
    err.status = res.status
    throw err
  }
  return res.json()
}

async function deleteJSON(url, data) {
  const opts = { method: 'DELETE', headers: getHeaders() }
  if (data !== undefined) opts.body = JSON.stringify(data)
  const res = await safeFetch(url, opts)
  checkAuth(res)
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    const err = new Error(body.message || body.error || `${res.status} ${res.statusText}`)
    err.status = res.status
    throw err
  }
  return res.json()
}

async function uploadFile(url, file, extraFields = {}) {
  const form = new FormData()
  form.append('file', file)
  for (const [k, v] of Object.entries(extraFields)) form.append(k, v)
  const token = getApiToken()
  const headers = {}
  if (token) headers['Authorization'] = 'Bearer ' + token
  const res = await safeFetch(url, { method: 'POST', headers, body: form })
  checkAuth(res)
  if (!res.ok) {
    const body = await res.json().catch(() => null)
    const err = new Error(body?.message || `${res.status} ${res.statusText}`)
    err.status = res.status
    throw err
  }
  return res.json()
}

// Login: authenticate and store token + user info. When the user has TOTP
// enabled, the first call (without `otp`) returns `{otp_required: true}` with
// an empty token — the caller must prompt for the code and call login again
// with the otp argument.
async function login(email, password, otp) {
  const body = { email, password }
  if (otp) body.otp = otp
  const res = await fetch(`${API}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  if (!res.ok) {
    const err = await res.json().catch(() => null)
    throw new Error(err?.error || err?.message || `Login failed (${res.status})`)
  }
  const data = await res.json()
  if (data.token) {
    setApiToken(data.token)
    setCurrentUser(data.email, data.name)
  }
  return data
}

export { login }

export const api = {
  // Raw helpers for custom calls
  postJSON,
  putJSON,
  deleteJSON,
  fetchJSON,
  fetchRaw,

  // Config
  getConfig: () => fetchJSON(`${API}/config`),

  // Cloudflare Access SSO: when behind CF Access, the proxy adds identity
  // headers and the server mints a session. Returns the login payload, or null
  // when not behind CF Access / not provisioned (a probe — never throws/redirects).
  cfSession: async () => {
    try {
      const res = await safeFetch(`${API}/auth/cf-session`, { headers: getHeaders() })
      if (!res.ok) return null
      return await res.json()
    } catch {
      return null
    }
  },

  // Users
  getMe: () => fetchJSON(`${API}/me`),
  getMyOrgs: () => fetchJSON(`${API}/me/organizations`),
  getUsers: () => fetchJSON(`${API}/users`),
  upsertUser: (user) => postJSON(`${API}/users`, user),
  getAvailableTemplates: () => fetchJSON(`${API}/templates/available`),
  addTemplate: (template) => postJSON(`${API}/templates`, { template }),
  removeTemplate: (name) => deleteJSON(`${API}/templates/${encodeURIComponent(name)}`),

  // Dynamic documents (git-backed, all folders)
  validateDocuments: () => fetchJSON(`${API}/documents/validate`),
  getAllDocuments: () => fetchJSON(`${API}/documents/all`),
  searchDocuments: (q, limit) => fetchJSON(`${API}/documents/search?q=${encodeURIComponent(q)}${limit ? '&limit=' + limit : ''}`),
  getChangedDocuments: (commits) => fetchJSON(`${API}/documents/changed${commits ? '?commits=' + commits : ''}`),
  getNeedsReview: () => fetchJSON(`${API}/documents/needs-review`),
  getDocument: (folder, id) => fetchJSON(`${API}/documents/file/${encodeURIComponent(folder)}/${encodeURIComponent(id)}`),
  getDocumentBlame: (docId) => fetchJSON(`${API}/documents/${encodeURIComponent(docId)}/blame`),
  updateDocumentMetadata: (docId, fields) => putJSON(`${API}/documents/${encodeURIComponent(docId)}/metadata`, { fields }),
  updateDocumentContent: (docId, content, version, author, owner) => putJSON(`${API}/documents/${encodeURIComponent(docId)}/content`, { content, ...(version && { version }), ...(author && { author }), ...(owner !== undefined && { owner }) }),
  createDocument: (doc) => postJSON(`${API}/documents`, doc),
  createFolder: (path, title) => postJSON(`${API}/documents/folders`, { path, title }),
  deleteDocument: (docId) => deleteJSON(`${API}/documents/${encodeURIComponent(docId)}`),

  // Risks
  getRisks: () => fetchJSON(`${API}/risks`),
  addRisk: (risk) => postJSON(`${API}/risks`, risk),
  getRiskTaxonomy: () => fetchJSON(`${API}/risks/taxonomy`),
  getRiskAdvisories: (id) => fetchJSON(`${API}/risks/${id}/advisories`),
  getRiskReadings: (id) => fetchJSON(`${API}/risks/${id}/readings`),
  createRiskReading: (id, data) => postJSON(`${API}/risks/${id}/readings`, data),

  // Assets
  getAssets: () => fetchJSON(`${API}/assets`),
  addAsset: (asset) => postJSON(`${API}/assets`, asset),
  updateAsset: (id, data) => putJSON(`${API}/assets/${id}`, data),

  // Suppliers
  getSuppliers: () => fetchJSON(`${API}/suppliers`),
  addSupplier: (supplier) => postJSON(`${API}/suppliers`, supplier),
  getSupplierReadings: (id) => fetchJSON(`${API}/suppliers/${id}/readings`),
  createSupplierReading: (id, data) => postJSON(`${API}/suppliers/${id}/readings`, data),

  // Reviews
  getReviews: (status) => fetchJSON(`${API}/reviews${status ? '?status=' + status : ''}`),
  // Paginated endpoint — use fetchRaw to keep the full {data, total, page, page_size}
  // shape. fetchJSON unwraps `data` into a bare array, which loses pagination metadata.
  getReviewsPaginated: (params) => fetchRaw(`${API}/reviews?${new URLSearchParams(params || {})}`),
  getReviewStats: () => fetchJSON(`${API}/reviews/stats`),
  createReview: (review) => postJSON(`${API}/reviews`, review),
  getReview: (id) => fetchJSON(`${API}/reviews/${id}`),
  updateReviewStatus: (id, status) => putJSON(`${API}/reviews/${id}/status`, { status }),
  forwardReview: (id, data) => postJSON(`${API}/reviews/${id}/forward`, data),
  getReviewTimeline: (id) => fetchJSON(`${API}/reviews/${id}/timeline`),
  getReviewDiff: (id, from) => fetchJSON(`${API}/reviews/${id}/diff${from ? '?from=' + encodeURIComponent(from) : ''}`),
  addReviewComment: (id, data) => postJSON(`${API}/reviews/${id}/comment`, typeof data === 'string' ? { body: data } : data),
  approveReview: (id, decision, comment) => postJSON(`${API}/reviews/${id}/approve`, { decision, comment }),
  mergeReview: (id) => postJSON(`${API}/reviews/${id}/merge`, {}),
  acceptAndMerge: (id) => postJSON(`${API}/reviews/${id}/accept-and-merge`, {}),
  sendForReview: (docId, data) => postJSON(`${API}/documents/${encodeURIComponent(docId)}/reviews`, data),
  getReviewAssignments: (id) => fetchJSON(`${API}/reviews/${id}/assignments`),
  getReviewContent: (id) => fetchJSON(`${API}/reviews/${id}/content`),
  updateReviewContent: (id, content) => putJSON(`${API}/reviews/${id}/content`, { content }),

  // Comments
  getAllOpenComments: () => fetchJSON(`${API}/comments/open`),
  getDocComments: (docId) => fetchJSON(`${API}/documents/${encodeURIComponent(docId)}/comments`),
  addComment: (comment) => postJSON(`${API}/comments`, comment),
  resolveComment: (id) => postJSON(`${API}/comments/${id}/resolve`),
  acceptSuggestion: (id) => postJSON(`${API}/comments/${id}/accept`),
  rejectSuggestion: (id) => postJSON(`${API}/comments/${id}/reject`),
  getReviewSuggestions: (reviewId) => fetchJSON(`${API}/reviews/${reviewId}/suggestions`),

  // Entity Comments (generic comments on any entity)
  getEntityComments: (type, id) => fetchJSON(`${API}/entity-comments/${encodeURIComponent(type)}/${encodeURIComponent(id)}`),
  createEntityComment: (comment) => postJSON(`${API}/entity-comments`, comment),
  resolveEntityComment: (id) => postJSON(`${API}/entity-comments/${id}/resolve`),
  deleteEntityComment: (id) => deleteJSON(`${API}/entity-comments/${id}`),

  // Reactions
  toggleReaction: (targetType, targetId, emoji) => postJSON(`${API}/reactions`, { target_type: targetType, target_id: targetId, emoji }),
  getReactions: (targetType, targetId) => fetchJSON(`${API}/reactions/${encodeURIComponent(targetType)}/${targetId}`),

  // Approvals
  getDocApprovals: (docId) => fetchJSON(`${API}/documents/${encodeURIComponent(docId)}/approvals`),

  // Decision log
  getDocDecisions: (docId) => fetchJSON(`${API}/documents/${encodeURIComponent(docId)}/decisions`),
  getReviewDecisions: (reviewId) => fetchJSON(`${API}/reviews/${reviewId}/decisions`),

  // Tasks
  getTasks: (assignee, status) => fetchJSON(`${API}/tasks?${new URLSearchParams({ ...(assignee && { assignee }), ...(status && { status }) })}`),
  listTasksLinked: (params) => fetchJSON(`${API}/tasks?${new URLSearchParams(params || {})}`),
  getTask: (id) => fetchJSON(`${API}/tasks/${id}`),
  createTask: (task) => postJSON(`${API}/tasks`, task),
  updateTask: (id, task) => putJSON(`${API}/tasks/${id}`, task),
  updateTaskStatus: (id, status) => putJSON(`${API}/tasks/${id}/status`, { status }),
  deleteTask: (id) => deleteJSON(`${API}/tasks/${id}`),

  // Entity Suggestions
  getSuggestions: (params) => fetchJSON(`${API}/suggestions?${new URLSearchParams(params || {})}`),
  getSuggestion: (id) => fetchJSON(`${API}/suggestions/${id}`),
  createSuggestion: (sg) => postJSON(`${API}/suggestions`, sg),
  updateSuggestion: (id, sg) => putJSON(`${API}/suggestions/${id}`, sg),
  deleteSuggestion: (id) => deleteJSON(`${API}/suggestions/${id}`),
  claimSuggestion: (id) => postJSON(`${API}/suggestions/${id}/claim`),
  applySuggestion: (id, opts) => postJSON(`${API}/suggestions/${id}/apply`, opts || {}),
  rejectEntitySuggestion: (id, reason) => postJSON(`${API}/suggestions/${id}/reject`, { reason }),
  withdrawSuggestion: (id) => postJSON(`${API}/suggestions/${id}/withdraw`),

  // Approval Policies
  listPolicies: () => fetchJSON(`${API}/admin/policies`),
  createPolicy: (policy) => postJSON(`${API}/admin/policies`, policy),
  updatePolicy: (id, policy) => putJSON(`${API}/admin/policies/${id}`, policy),
  deletePolicy: (id) => deleteJSON(`${API}/admin/policies/${id}`),
  getReviewPolicyStatus: (reviewId) => fetchJSON(`${API}/reviews/${reviewId}/policy-status`),

  // Changes
  getChanges: (status) => fetchJSON(`${API}/changes${status ? '?status=' + status : ''}`),
  getChange: (id) => fetchJSON(`${API}/changes/${id}`),
  createChange: (change) => postJSON(`${API}/changes`, change),
  updateChange: (id, data) => putJSON(`${API}/changes/${id}`, data),
  updateChangeStatus: (id, status) => putJSON(`${API}/changes/${id}/status`, { status }),
  deleteChange: (id) => deleteJSON(`${API}/changes/${id}`),

  // Implementation
  getImplementation: (type, status) => fetchJSON(`${API}/implementation?${new URLSearchParams({ ...(type && { item_type: type }), ...(status && { status }) })}`),
  updateImplementation: (itemId, data) => putJSON(`${API}/implementation/${encodeURIComponent(itemId)}`, data),
  getProgress: () => fetchJSON(`${API}/implementation/progress`),

  // Notifications
  getNotifications: (unreadOnly) => fetchJSON(`${API}/notifications${unreadOnly ? '?unread=true' : ''}`),
  markRead: (id) => postJSON(`${API}/notifications/${id}/read`),
  markAllRead: () => postJSON(`${API}/notifications/read-all`),
  getUnreadCount: () => fetchJSON(`${API}/notifications/count`),

  // Activity
  getActivity: (limit) => fetchJSON(`${API}/activity?limit=${limit || 20}`),
  getEntityChangelog: (type, id) => fetchJSON(`${API}/changelog/${encodeURIComponent(type)}/${encodeURIComponent(id)}`),

  // Branding
  uploadBranding: (file, type = 'logo') => uploadFile(`${API}/admin/branding/upload`, file, { type }),
  deleteBranding: (name) => deleteJSON(`${API}/admin/branding/${encodeURIComponent(name)}`),
  getDocActivity: (docId) => fetchJSON(`${API}/documents/${encodeURIComponent(docId)}/activity`),

  // Systems
  getSystems: () => fetchJSON(`${API}/systems`),
  listSystemsLinked: (params) => fetchJSON(`${API}/systems?${new URLSearchParams(params || {})}`),
  createSystem: (system) => postJSON(`${API}/systems`, system),
  updateSystem: (id, data) => putJSON(`${API}/systems/${id}`, data),
  getSystemReadings: (id) => fetchJSON(`${API}/systems/${id}/readings`),
  createSystemReading: (id, data) => postJSON(`${API}/systems/${id}/readings`, data),
  getAccessReviews: (systemId) => fetchJSON(`${API}/systems/${systemId}/access-reviews`),
  createAccessReview: (systemId, review) => postJSON(`${API}/systems/${systemId}/access-reviews`, review),
  deleteAccessReview: (id) => deleteJSON(`${API}/access-reviews/${id}`),

  // Versions & Diff
  getDocVersions: (docId) => fetchJSON(`${API}/documents/${encodeURIComponent(docId)}/versions`),
  getDocDiff: (docId, from, to) => fetchJSON(`${API}/documents/${encodeURIComponent(docId)}/diff?from=${from}&to=${to || 'HEAD'}`),

  // Audit
  getAuditProgrammes: () => fetchJSON(`${API}/audit/programmes`),
  getAuditProgramme: (id) => fetchJSON(`${API}/audit/programmes/${id}`),
  createAuditProgramme: (prog) => postJSON(`${API}/audit/programmes`, prog),
  updateAuditProgramme: (id, data) => putJSON(`${API}/audit/programmes/${id}`, data),
  deleteAuditProgramme: (id) => deleteJSON(`${API}/audit/programmes/${id}`),
  getAuditCalendar: (year) => fetchJSON(`${API}/audit/calendar?year=${year || new Date().getFullYear()}`),
  getAuditFindingsPaginated: (params) => fetchJSON(`${API}/audit/findings?${new URLSearchParams(params || {})}`),
  listAuditFindings: (params) => fetchJSON(`${API}/audit/findings?${new URLSearchParams(params || {})}`),
  getAudits: (programmeId) => fetchJSON(`${API}/audits${programmeId ? '?programme_id=' + programmeId : ''}`),
  listAudits: (params) => fetchJSON(`${API}/audits?${new URLSearchParams(params || {})}`),
  createAudit: (audit) => postJSON(`${API}/audits`, audit),
  getAudit: (id) => fetchJSON(`${API}/audits/${id}`),
  updateAudit: (id, data) => putJSON(`${API}/audits/${id}`, data),
  updateAuditStatus: (id, status) => putJSON(`${API}/audits/${id}/status`, { status }),
  getAuditItems: (auditId) => fetchJSON(`${API}/audits/${auditId}/items`),
  createAuditItem: (auditId, data) => postJSON(`${API}/audits/${auditId}/items`, data),
  updateAuditItem: (id, data) => putJSON(`${API}/audit-items/${id}`, data),
  deleteAuditItem: (id) => deleteJSON(`${API}/audit-items/${id}`),
  getAuditFindings: (auditId) => fetchJSON(`${API}/audits/${auditId}/findings`),
  getAuditFinding: (id) => fetchJSON(`${API}/audit-findings/${id}`),
  addAuditFinding: (finding) => postJSON(`${API}/audit-findings`, finding),
  updateAuditFinding: (id, data) => putJSON(`${API}/audit-findings/${id}`, data),
  deleteAuditFinding: (id) => deleteJSON(`${API}/audit-findings/${id}`),

  // Legal Register
  getLegal: (status) => fetchJSON(`${API}/legal${status ? '?status=' + status : ''}`),
  createLegal: (item) => postJSON(`${API}/legal`, item),
  getLegalTaxonomy: () => fetchJSON(`${API}/legal/taxonomy`),
  getLegalItem: (id) => fetchJSON(`${API}/legal/${id}`),
  updateLegal: (id, data) => putJSON(`${API}/legal/${id}`, data),
  deleteLegal: (id) => deleteJSON(`${API}/legal/${id}`),
  getLegalReadings: (id) => fetchJSON(`${API}/legal/${id}/readings`),
  createLegalReading: (id, data) => postJSON(`${API}/legal/${id}/readings`, data),
  getAssetReadings: (id) => fetchJSON(`${API}/assets/${id}/readings`),
  createAssetReading: (id, data) => postJSON(`${API}/assets/${id}/readings`, data),
  getSupplierReadings: (id) => fetchJSON(`${API}/suppliers/${id}/readings`),
  createSupplierReading: (id, data) => postJSON(`${API}/suppliers/${id}/readings`, data),

  // Incidents
  getIncidents: (status, severity, assignee) => fetchJSON(`${API}/incidents?${new URLSearchParams({ ...(status && { status }), ...(severity && { severity }), ...(assignee && { assignee }) })}`),
  createIncident: (incident) => postJSON(`${API}/incidents`, incident),
  getIncident: (id) => fetchJSON(`${API}/incidents/${id}`),
  updateIncident: (id, data) => putJSON(`${API}/incidents/${id}`, data),
  updateIncidentStatus: (id, status) => putJSON(`${API}/incidents/${id}/status`, { status }),
  deleteIncident: (id) => deleteJSON(`${API}/incidents/${id}`),
  getIncidentStats: () => fetchJSON(`${API}/incidents/stats`),

  // Corrective Actions
  getCorrectiveActions: (status, severity, assignee) => fetchJSON(`${API}/corrective-actions?${new URLSearchParams({ ...(status && { status }), ...(severity && { severity }), ...(assignee && { assignee }) })}`),
  listCorrectiveActionsLinked: (params) => fetchJSON(`${API}/corrective-actions?${new URLSearchParams(params || {})}`),
  createCorrectiveAction: (ca) => postJSON(`${API}/corrective-actions`, ca),
  getCorrectiveAction: (id) => fetchJSON(`${API}/corrective-actions/${id}`),
  updateCorrectiveAction: (id, data) => putJSON(`${API}/corrective-actions/${id}`, data),
  updateCorrectiveActionStatus: (id, status) => putJSON(`${API}/corrective-actions/${id}/status`, { status }),
  deleteCorrectiveAction: (id) => deleteJSON(`${API}/corrective-actions/${id}`),
  getCorrectiveActionStats: () => fetchJSON(`${API}/corrective-actions/stats`),

  // Programs
  getPrograms: () => fetchJSON(`${API}/programs`),
  createProgram: (program) => postJSON(`${API}/programs`, program),
  getProgram: (id) => fetchJSON(`${API}/programs/${id}`),
  updateProgram: (id, data) => putJSON(`${API}/programs/${id}`, data),
  deleteProgram: (id) => deleteJSON(`${API}/programs/${id}`),

  // Objectives
  getObjectives: (programId, status) => fetchJSON(`${API}/objectives?${new URLSearchParams({ ...(programId && { program_id: programId }), ...(status && { status }) })}`),
  createObjective: (objective) => postJSON(`${API}/objectives`, objective),
  getObjective: (id) => fetchJSON(`${API}/objectives/${id}`),
  updateObjective: (id, data) => putJSON(`${API}/objectives/${id}`, data),
  deleteObjective: (id) => deleteJSON(`${API}/objectives/${id}`),
  archiveObjective: (id) => postJSON(`${API}/objectives/${id}/archive`),
  unarchiveObjective: (id) => postJSON(`${API}/objectives/${id}/unarchive`),

  // Checkins
  getCheckins: (objectiveId, limit, offset) => fetchJSON(`${API}/objectives/${objectiveId}/checkins?${new URLSearchParams({ ...(limit && { limit }), ...(offset && { offset }) })}`),
  createCheckin: (objectiveId, checkin) => postJSON(`${API}/objectives/${objectiveId}/checkins`, checkin),
  updateCheckin: (id, data) => putJSON(`${API}/checkins/${id}`, data),
  deleteCheckin: (id) => deleteJSON(`${API}/checkins/${id}`),

  // Evidence
  uploadEvidence: async (checkinId, file, title) => {
    const form = new FormData()
    form.append('file', file)
    if (title) form.append('title', title)
    const headers = {}
    const token = getApiToken()
    if (token) headers['Authorization'] = 'Bearer ' + token
    const res = await fetch(`${API}/checkins/${checkinId}/evidence`, {
      method: 'POST',
      headers,
      body: form,
    })
    if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
    return res.json()
  },
  getEvidence: (checkinId) => fetchJSON(`${API}/checkins/${checkinId}/evidence`),
  // Evidence download has two backends: S3 returns JSON {url} to redirect to;
  // the local (file) backend streams the file directly. Branch on Content-Type
  // so the local backend works instead of failing to parse bytes as JSON (#31).
  downloadEvidence: async (id) => {
    const res = await safeFetch(`${API}/evidence/${id}/download`, { headers: getHeaders() })
    checkAuth(res)
    if (!res.ok) {
      const e = new Error(`${res.status} ${res.statusText}`)
      e.status = res.status
      throw e
    }
    if ((res.headers.get('content-type') || '').includes('application/json')) {
      return await res.json() // S3 backend: { url, title, content_type }
    }
    // Local backend: the file itself — hand back a blob + filename to save.
    const cd = res.headers.get('content-disposition') || ''
    const m = /filename="?([^"]+)"?/.exec(cd)
    return { blob: await res.blob(), filename: m ? m[1] : `evidence-${id}` }
  },
  deleteEvidence: (id) => deleteJSON(`${API}/evidence/${id}`),

  // Overdue
  getOverdue: () => fetchJSON(`${API}/overdue`),
  createOverdueTasks: () => postJSON(`${API}/overdue/tasks`, {}),

  // Readings (periodic assessments)
  getRiskReadings: (id) => fetchJSON(`${API}/risks/${id}/readings`),
  createRiskReading: (id, data) => postJSON(`${API}/risks/${id}/readings`, data),
  getLegalReadings: (id) => fetchJSON(`${API}/legal/${id}/readings`),
  createLegalReading: (id, data) => postJSON(`${API}/legal/${id}/readings`, data),
  getAssetReadings: (id) => fetchJSON(`${API}/assets/${id}/readings`),
  createAssetReading: (id, data) => postJSON(`${API}/assets/${id}/readings`, data),
  getSupplierReadings: (id) => fetchJSON(`${API}/suppliers/${id}/readings`),
  createSupplierReading: (id, data) => postJSON(`${API}/suppliers/${id}/readings`, data),
  getSystemReadings: (id) => fetchJSON(`${API}/systems/${id}/readings`),
  createSystemReading: (id, data) => postJSON(`${API}/systems/${id}/readings`, data),

  // Supplier reviews (periodic supplier assessments)
  getSupplierReviews: (id) => fetchJSON(`${API}/suppliers/${id}/reviews`),
  createSupplierReview: (id, data) => postJSON(`${API}/suppliers/${id}/reviews`, data),

  // Asset reviews (periodic asset assessments)
  getAssetReviews: (id) => fetchJSON(`${API}/assets/${id}/reviews`),
  createAssetReview: (id, data) => postJSON(`${API}/assets/${id}/reviews`, data),

  // Universal search (for linking)
  search: (q) => fetchJSON(`${API}/search?q=${encodeURIComponent(q || '')}`),

  // Entity cross-references
  getReferences: (type, id) => fetchJSON(`${API}/references?type=${encodeURIComponent(type)}&id=${encodeURIComponent(id)}`),
  createReference: (ref) => postJSON(`${API}/references`, ref),
  deleteReference: (id) => deleteJSON(`${API}/references/${id}`),

  // Personal Access Tokens (self-service)
  getMyAPIKeys: () => fetchJSON(`${API}/auth/api-keys`),
  createMyAPIKey: (data) => postJSON(`${API}/auth/api-keys`, data),
  revokeMyAPIKey: (id) => deleteJSON(`${API}/auth/api-keys/${id}`),

  // Auth
  login,
  logout: () => postJSON(`${API}/auth/logout`, {}),
  fetchJSON: (url) => fetchJSON(url),
  refresh: async () => {
    const res = await fetch(`${API}/auth/refresh`, {
      method: 'POST',
      headers: getHeaders(),
    })
    if (!res.ok) throw new Error('refresh failed')
    const data = await res.json()
    setApiToken(data.token)
    return data
  },

  // OIDC
  getOIDCProviders: (orgSlug) => fetchJSON(`${API}/auth/oidc/providers?org=${encodeURIComponent(orgSlug)}`),

  // Passkeys
  passkeyLoginBegin: (email) => postJSON(`${API}/auth/passkey/login/begin`, { email }),
  passkeyLoginComplete: (email, data) => {
    return postJSON(`${API}/auth/passkey/login/complete?email=${encodeURIComponent(email)}`, data)
  },
  passkeyRegisterBegin: () => postJSON(`${API}/auth/passkey/register/begin`, {}),
  passkeyRegisterComplete: (data) => postJSON(`${API}/auth/passkey/register/complete`, data),
  listPasskeys: () => fetchJSON(`${API}/auth/passkeys`),
  deletePasskey: (id) => deleteJSON(`${API}/auth/passkeys/${id}`),
  renamePasskey: (id, name) => putJSON(`${API}/auth/passkeys/${id}`, { name }),
}

export default api
