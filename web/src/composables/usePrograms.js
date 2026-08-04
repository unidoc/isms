import { ref } from 'vue'
import { api } from '../api'

// Shared cache of program keys. Programs are referenced by their per-org key (an
// arbitrary user-chosen string like "ISMS"/"SEC"), and an objective's display_id
// is "<programKey>-<seq>". Neither fits a static prefix map, so #KEY / #KEY-N
// linkification needs the live key set. Loaded once, reused across renders.
const programKeys = ref(new Set())
const loaded = ref(false)
const loading = ref(false)

export async function loadProgramKeys() {
  if (loaded.value || loading.value) return
  loading.value = true
  try {
    const res = await api.getPrograms()
    const list = (res && (res.data || res)) || []
    programKeys.value = new Set(list.map(p => p.key).filter(Boolean))
    loaded.value = true
  } catch {
    /* leave empty — references degrade to a highlighted (non-linked) span */
  }
  loading.value = false
}

export { programKeys }
