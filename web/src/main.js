import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import mermaidDirective from './directives/mermaid.js'
import './style.css'

const app = createApp(App)
app.directive('mermaid', mermaidDirective)
app.use(router).mount('#app')
