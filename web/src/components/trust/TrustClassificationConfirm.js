import { h } from 'vue'
import { useI18n } from 'vue-i18n'

// The confirmation the server asks for (409 trust_classification_confirm) when a
// confidential or restricted document would become readable by anyone. Shared by
// the document-page control and the trust center manager screen; the state lives
// in useClassificationConfirm(). A render function rather than a .vue file so
// the node test harness, which cannot import .vue children, can mount the
// components that use it.
export default {
  name: 'TrustClassificationConfirm',
  props: { pending: { type: Object, default: null } },
  emits: ['answer'],
  setup(props, { emit }) {
    const { t } = useI18n()
    return () => {
      if (!props.pending) return null
      return h('div', { class: 'fixed inset-0 z-[60] flex items-center justify-center', 'data-trust': 'classification-dialog' }, [
        h('div', { class: 'absolute inset-0 bg-black/60', onClick: () => emit('answer', false) }),
        h('div', { class: 'relative bg-slate-900 border border-slate-700 rounded-2xl shadow-2xl w-full max-w-md p-6 space-y-4 text-sm', role: 'alertdialog' }, [
          h('h2', { class: 'text-lg font-bold text-white' }, t('components.trust_publish.classification_title')),
          h('p', { class: 'text-slate-300' }, t('components.trust_publish.classification_body', { classification: props.pending.classification })),
          h('div', { class: 'flex justify-end gap-2' }, [
            h('button', { type: 'button', 'data-trust': 'classification-cancel', class: 'px-3 py-1.5 text-xs text-slate-400 hover:text-slate-200', onClick: () => emit('answer', false) }, t('common.action.cancel')),
            h('button', { type: 'button', 'data-trust': 'classification-confirm', class: 'px-3 py-1.5 text-xs font-medium rounded-lg bg-red-600 hover:bg-red-500 text-white', onClick: () => emit('answer', true) }, t('components.trust_publish.classification_confirm')),
          ]),
        ]),
      ])
    }
  },
}
