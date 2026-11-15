import { ref } from 'vue'

export const CONFIRM_CODE = 'trust_classification_confirm'

// Runs a request that may need the classification confirmation. `send(confirm)`
// performs it; it is first called with false. When the server answers 409
// trust_classification_confirm, `ask(classification)` shows the dialog; on yes
// the SAME request is sent again with true, on no nothing more is sent.
// Resolves {cancelled, result}; any other error is rethrown.
export async function withClassificationConfirm(send, ask) {
  try {
    return { cancelled: false, result: await send(false) }
  } catch (e) {
    if (e?.code !== CONFIRM_CODE) throw e
    if (!(await ask(e.params?.value || ''))) return { cancelled: true, result: undefined }
    return { cancelled: false, result: await send(true) }
  }
}

// State for the shared confirm dialog (TrustClassificationConfirm.vue): `ask`
// opens it and resolves with the answer, `answer` closes it.
export function useClassificationConfirm() {
  const pending = ref(null)
  let resolver = null
  const ask = (classification) => new Promise((resolve) => {
    if (resolver) resolver(false) // never leave an earlier question unanswered
    resolver = resolve
    pending.value = { classification }
  })
  const answer = (yes) => {
    pending.value = null
    const r = resolver
    resolver = null
    if (r) r(!!yes)
  }
  const run = (send) => withClassificationConfirm(send, ask)
  return { pending, ask, answer, run }
}
