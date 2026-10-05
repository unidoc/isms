import { ref } from 'vue'
import { api } from '../api.js'

// Shared cache of program keys. Programs are referenced by their per-org key (an
// arbitrary user-chosen string like "ISMS"/"SEC"), and an objective's display_id
// is "<programKey>-<seq>". Neither fits a static prefix map, so #KEY / #KEY-N
// linkification needs the live key set. Loaded once, reused across renders.
const programKeys = ref(new Set())
// Plain variables, not refs: renderMention calls loadProgramKeys from inside a
// component's render, so a ref written here would re-run that render. After a
// failed request the re-run would request again, and again, for as long as the
// endpoint keeps failing. A failure now waits for the next render to retry.
let loaded = false
let loading = false

export async function loadProgramKeys() {
  if (loaded || loading) return
  loading = true
  try {
    const res = await api.getPrograms()
    const list = (res && (res.data || res)) || []
    programKeys.value = new Set(list.map(p => p.key).filter(Boolean))
    loaded = true
  } catch {
    /* leave empty — references degrade to a highlighted (non-linked) span */
  }
  loading = false
}

export { programKeys }
