/**
 * Single source of truth for local dev ports, shared by vite.config.ts,
 * playwright.config.ts and e2e/global-setup.ts.
 *
 * There are two URLs on purpose, and they serve *different frontends*:
 *
 *   :3000  Vite dev server — the frontend from live source, proxying /api and
 *          /ws through to :8080. This is the app. Develop and test here.
 *
 *   :8080  the Go backend — the API, plus a *built* copy of the frontend.
 *          Which copy depends on app.frontend_dir: dev/config.toml sets it to
 *          frontend/dist, so :8080 reflects the last `make frontend-build` and
 *          a plain rebuild is enough to refresh it. Shipped binaries leave it
 *          empty and serve the copy embedded by `make build-prod` instead —
 *          that one is a snapshot frozen at build time, which is why CI points
 *          at :8080: there the binary was just built from the current tree and
 *          is the artifact that ships.
 *
 * Rule of thumb: use :3000 unless you are specifically verifying a built
 * artifact. `make frontend-build` refreshes :8080 in dev; `make
 * test-e2e-embedded` is what checks the embedded production binary.
 *
 * Override either port via the FRONTEND_PORT / BACKEND_PORT env vars — `make
 * dev` forwards both, and also passes BACKEND_PORT to the server as
 * WHATOMATE_SERVER__PORT so the two stay in agreement.
 */
export const FRONTEND_PORT = Number(process.env.FRONTEND_PORT ?? 3000)
export const BACKEND_PORT = Number(process.env.BACKEND_PORT ?? 8080)

export const BACKEND_HOST = process.env.BACKEND_HOST ?? 'localhost'

/** Live frontend + proxied API. The default target for `npm run dev` and e2e. */
export const DEV_URL = `http://${BACKEND_HOST}:${FRONTEND_PORT}`

/** Backend API, and the embedded frontend snapshot. What CI tests against. */
export const BACKEND_URL = `http://${BACKEND_HOST}:${BACKEND_PORT}`
