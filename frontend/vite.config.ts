import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath, URL } from 'node:url'
import { createRequire } from 'node:module'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import compression from 'vite-plugin-compression'
import { FRONTEND_PORT, BACKEND_PORT, BACKEND_URL } from './ports.ts'

const _require = createRequire(import.meta.url)
try {
  const ts = _require('typescript')
  if (!ts.sys) {
    ts.sys = {
      fileExists: (p) => fs.existsSync(p),
      readFile: (p) => fs.readFileSync(p, 'utf-8'),
      useCaseSensitiveFileNames: true
    }
  }
  if (!ts.findConfigFile) ts.findConfigFile = () => undefined
  if (!ts.resolveModuleName) {
    ts.resolveModuleName = (source, containingFile) => {
      try {
        let resolved = _require.resolve(source, { paths: [path.dirname(containingFile)] })
        if (resolved.endsWith('.js') || resolved.endsWith('.cjs') || resolved.endsWith('.mjs')) {
          resolved = resolved.replace(/\.[cm]?js$/, '.d.ts')
          if (!fs.existsSync(resolved) && source === 'reka-ui') {
            resolved = _require.resolve('reka-ui/dist/index.d.ts')
          }
        }
        return { resolvedModule: { resolvedFileName: resolved } }
      } catch (e) {
        return { resolvedModule: undefined }
      }
    }
  }
} catch (e) {
  // typescript not found or couldn't be patched, ignore
}

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
    vue({
      script: {
        fs: {
          fileExists(file: string): boolean {
            return fs.existsSync(file)
          },
          readFile(file: string): string | undefined {
            return fs.readFileSync(file, 'utf-8')
          }
        }
      }
    }),
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

          // d3 is here only because @unovis depends on it — 34 d3-* packages
          // plus three that don't carry the prefix. It is the bulk of the
          // charting weight, so it belongs in the charts chunk; matched by
          // prefix because listing every package would go stale on the next
          // @unovis bump.
          if (/[\\/]node_modules[\\/](d3-|internmap[\\/]|delaunator[\\/]|robust-predicates[\\/])/.test(id)) {
            return 'charts'
          }

          const chunks: Record<string, string[]> = {
            'vue-vendor': ['vue', 'vue-router', 'pinia'],
            'reka-ui': ['reka-ui'],
            'charts': ['@unovis/vue', '@unovis/ts', '@unovis/dagre-layout', '@unovis/graphlibrary'],
            'grid-layout': ['grid-layout-plus'],
            'emoji-picker': ['vue3-emoji-picker'],
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
    host: '0.0.0.0',
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
        target: BACKEND_URL.replace('http://', 'ws://'),
        ws: true,
        configure: reportBackendDown
      }
    }
  }
})
