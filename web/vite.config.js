import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      // 显式 127.0.0.1：localhost 在本机可能被解析到 [::1] 上的其他代理（如桌面端预览），
      // 会导致 /api 请求打回开发服务器自身形成死循环。
      '/api': 'http://127.0.0.1:8787',
      '/feed': 'http://127.0.0.1:8787',
      '/llms.txt': 'http://127.0.0.1:8787'
    }
  },
  build: {
    outDir: 'dist',
    assetsInlineLimit: 4096
  }
})
