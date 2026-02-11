import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [vue()],
  server: {
    // 开发服务器代理配置，将 /api 请求转发到 Go 后端
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true
      }
    }
  },
  build: {
    // 构建输出目录，Go 后端从 web/dist/ 提供静态文件
    outDir: 'dist'
  }
})
