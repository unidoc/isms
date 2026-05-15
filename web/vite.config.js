import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
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
