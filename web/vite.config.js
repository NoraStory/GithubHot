import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8787',
      '/feed': 'http://localhost:8787',
      '/llms.txt': 'http://localhost:8787'
    }
  },
  build: {
    outDir: 'dist',
    assetsInlineLimit: 4096
  }
})
