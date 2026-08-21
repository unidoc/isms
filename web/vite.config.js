import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: {
    // '@' -> src. Existing relative imports keep working; this is for modules
    // that get imported from many depths (locale files, i18n setup) where
    // '../../..' chains are noise.
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  build: {
    chunkSizeWarningLimit: 1000,
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/git': 'http://localhost:8080',
      '/healthz': 'http://localhost:8080',
      '/branding': 'http://localhost:8080',
    }
  }
})
