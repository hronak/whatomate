import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import compression from 'vite-plugin-compression'
import { FRONTEND_PORT, BACKEND_PORT, BACKEND_URL } from './ports.ts'

/**
 * Without this, a backend that isn't running surfaces as a bare 502 whose body
 * isn't an API envelope — which the login page renders as "Invalid
 * credentials". Return a real envelope so the actual cause reaches the screen,
 * and say it once in the terminal too.
 */
let lastReported = 0
function reportBackendDown(proxy: { on: (ev: string, cb: (...args: any[]) => void) => void }) {
  proxy.on('error', (err: NodeJS.ErrnoException, _req: unknown, res: any) => {
    const hint =
      `Backend not reachable at ${BACKEND_URL} (${err.code || err.message}). ` +
      'Start the whole dev stack with `make dev`, or just the backend with `make run-migrate`.'

    if (Date.now() - lastReported > 5000) {
      lastReported = Date.now()
      console.error(`\n  ⚠  ${hint}\n`)
    }

    // A failed websocket upgrade hands us a raw socket, not a ServerResponse.
    if (!res || typeof res.writeHead !== 'function') {
      res?.destroy?.()
      return
    }
    if (res.headersSent) return
    res.writeHead(503, { 'Content-Type': 'application/json' })
    res.end(JSON.stringify({ status: 'error', message: hint, data: null }))
  })
}

export default defineConfig({
  base: './',
  plugins: [
    vue(),
    // Gzip compression
    compression({
      algorithm: 'gzip',
      ext: '.gz',
      threshold: 1024 // Only compress files > 1KB
    }),
    // Brotli compression (better ratio)
    compression({
      algorithm: 'brotliCompress',
      ext: '.br',
      threshold: 1024
    })
  ],
  resolve: {
    alias: {
      '@': fileURLToPath(new URL('./src', import.meta.url))
    }
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes('node_modules')) return
          const chunks: Record<string, string[]> = {
            'vue-vendor': ['vue', 'vue-router', 'pinia'],
            'reka-ui': ['reka-ui'],
            'charts': ['chart.js', 'vue-chartjs'],
            'grid-layout': ['grid-layout-plus'],
            'emoji-picker': ['vue3-emoji-picker'],
            'validation': ['vee-validate', '@vee-validate/zod', 'zod'],
            'utils': ['@vueuse/core', 'axios', 'clsx', 'tailwind-merge', 'class-variance-authority']
          }
          for (const [chunk, pkgs] of Object.entries(chunks)) {
            for (const pkg of pkgs) {
              if (id.includes(`/node_modules/${pkg}/`) || id.includes(`\\node_modules\\${pkg}\\`)) {
                return chunk
              }
            }
          }
        }
      }
    },
    // Increase chunk size warning limit (optional)
    chunkSizeWarningLimit: 500
  },
  // Drop console and debugger in production builds
  esbuild: {
    drop: ['console', 'debugger']
  },
  server: {
    port: FRONTEND_PORT,
    // Fail rather than silently drifting to :3001 — a second frontend port is
    // how you end up testing one app while looking at another.
    strictPort: true,
    allowedHosts: [],
    proxy: {
      '/api': {
        target: BACKEND_URL,
        changeOrigin: true,
        configure: reportBackendDown
      },
      '/ws': {
        target: `ws://localhost:${BACKEND_PORT}`,
        ws: true,
        configure: reportBackendDown
      }
    }
  }
})
