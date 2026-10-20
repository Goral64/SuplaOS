import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// npm run dev talks to a panel at SUPLAOS_PANEL (e.g. http://192.168.1.20).
export default defineConfig({
  plugins: [vue()],
  define: {
    __VUE_I18N_LEGACY_API__: false,
    __VUE_I18N_FULL_INSTALL__: false,
    __INTLIFY_PROD_DEVTOOLS__: false,
  },
  server: {
    proxy: { '/api': { target: process.env.SUPLAOS_PANEL || 'http://localhost:8000', changeOrigin: true } },
  },
  build: { outDir: 'dist', emptyOutDir: true },
})
