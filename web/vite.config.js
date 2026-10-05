import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// 本地联调时的后端地址（默认线上/本机 8787；沙盒实例可设为如 http://127.0.0.1:8792，
// 避免开发调试的指纹上报写进正式库）
const apiTarget = process.env.VITE_PROXY_TARGET || 'http://127.0.0.1:8787'

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      // 显式 127.0.0.1：localhost 在本机可能被解析到 [::1] 上的其他代理（如桌面端预览），
      // 会导致 /api 请求打回开发服务器自身形成死循环。
      '/api': apiTarget,
      '/feed': apiTarget,
      '/llms.txt': apiTarget
    }
  },
  build: {
    outDir: 'dist',
    assetsInlineLimit: 4096
  }
})
