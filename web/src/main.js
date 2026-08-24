import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { i18n, resolveInitialLocale, setLocale } from './i18n.js'
import mermaidDirective from './directives/mermaid.js'
import './style.css'

const app = createApp(App)
app.directive('mermaid', mermaidDirective)
app.use(i18n)

// Apply the best locale we can know pre-login (localStorage, then the browser's
// languages). The authenticated value from GET /me and the org default from
// GET /config both arrive later and re-apply through the same seam.
setLocale(resolveInitialLocale())

app.use(router).mount('#app')
