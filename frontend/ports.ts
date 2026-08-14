/**
 * Single source of truth for local dev ports, shared by vite.config.ts,
 * playwright.config.ts and e2e/global-setup.ts.
 *
 * There are two URLs on purpose, and they serve *different frontends*:
 *
 *   :3000  Vite dev server — the frontend from live source, proxying /api and
 *          /ws through to :8080. This is the app. Develop and test here.
 *
 *   :8080  the Go backend — the API, and (only when built by `make build-prod`)
 *          the frontend snapshot embedded into the binary at build time. That
 *          snapshot is whatever was last embedded, so browsing or testing here
 *          after editing frontend source silently exercises the *previous*
 *          build. CI points at :8080 deliberately, because there the binary was
 *          just built from the current tree and that is the shipped artifact.
 *
 * Rule of thumb: use :3000 unless you are specifically verifying the embedded
 * production binary, and run `make build-prod` first when you are.
 *
 * Override either port via the FRONTEND_PORT / BACKEND_PORT env vars — `make
 * dev` forwards both, and also passes BACKEND_PORT to the server as
 * WHATOMATE_SERVER__PORT so the two stay in agreement.
 */
export const FRONTEND_PORT = Number(process.env.FRONTEND_PORT ?? 3000)
export const BACKEND_PORT = Number(process.env.BACKEND_PORT ?? 8080)

/** Live frontend + proxied API. The default target for `npm run dev` and e2e. */
export const DEV_URL = `http://localhost:${FRONTEND_PORT}`

/** Backend API, and the embedded frontend snapshot. What CI tests against. */
export const BACKEND_URL = `http://localhost:${BACKEND_PORT}`
