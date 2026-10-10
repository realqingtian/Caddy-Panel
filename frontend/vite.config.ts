import { fileURLToPath, URL } from 'node:url'

import react from '@vitejs/plugin-react'
import { defineConfig } from 'vite'

// 构建产物直接输出到后端 embed 目录(backend/internal/web/dist),
// 支撑"单二进制"交付形态;.gitkeep 由 build 脚本补回以保证 embed 可编译。
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url)),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
  build: {
    outDir: '../backend/internal/web/dist',
    emptyOutDir: true,
  },
})
