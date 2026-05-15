// Lightweight dirty-detection for edit forms in detail modals.
//
// Pattern: each detail modal has an `editForm` ref that holds the working
// copy of the entity being edited. After `startEdit()` initialises that
// form, call `capture()` to take a baseline snapshot. `isDirty()` then
// compares the current form to that snapshot via JSON serialisation.
//
// This lets `switchDetailTab` / `closeDetail` warn only when the user
// has actually changed something — not just because edit mode is open
// (e.g. when we auto-open edit mode after create).

import { ref } from 'vue'

export function useDirtyEdit(formRef) {
  const snapshot = ref('')
  const capture = () => { snapshot.value = JSON.stringify(formRef.value) }
  const isDirty = () => JSON.stringify(formRef.value) !== snapshot.value
  return { capture, isDirty }
}
