import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

// 后端 API 端口，可通过环境变量 MOTE_PORT 覆盖
const API_PORT = process.env.MOTE_PORT || '18788';

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  base: './',
  resolve: {
    alias: {
      // 直接指向 shared/ui 的源码目录，支持 HMR 热更新
      '@mote/shared-ui': path.resolve(__dirname, '../../shared/ui/src'),
    },
  },
  // 优化配置：确保 shared-ui 源码的更改能触发 HMR
  optimizeDeps: {
    // 排除 shared-ui，让 vite 直接处理源码
    exclude: ['@mote/shared-ui'],
  },
  build: {
    outDir: '../../internal/ui/ui',
    emptyOutDir: true,
    assetsDir: 'assets',
    chunkSizeWarningLimit: 1500,
    rollupOptions: {
      output: {
        manualChunks: {
          vendor: ['react', 'react-dom'],
          antd: ['antd'],
        },
      },
    },
  },
  server: {
    port: 5173,
    // 监听 shared/ui 目录的文件更改
    watch: {
      // 包含 shared/ui 目录
      ignored: ['!**/shared/ui/**'],
    },
    proxy: {
      '/api': {
        target: `http://127.0.0.1:${API_PORT}`,
        changeOrigin: true,
      },
      '/ws': {
        target: `ws://127.0.0.1:${API_PORT}`,
        ws: true,
      },
    },
  },
});
