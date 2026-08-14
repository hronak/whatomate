import { defineConfig, devices } from '@playwright/test'
import { DEV_URL, FRONTEND_PORT } from './ports.ts'

// Default to the Vite dev server (:3000) so a local run always exercises the
// frontend source you just edited. CI overrides BASE_URL to the backend
// (:8080) on purpose — there the binary was just built from the current tree,
// so its embedded frontend is current and is the artifact worth testing.
// See ports.ts for why the two URLs are not interchangeable.
const baseURL = process.env.BASE_URL || DEV_URL

// Skip webServer if CI or BASE_URL is set (user has their own server running)
const skipWebServer = !!process.env.CI || !!process.env.BASE_URL

export default defineConfig({
  testDir: './e2e/tests',
  globalSetup: './e2e/global-setup.ts',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  workers: process.env.CI ? 4 : undefined,
  reporter: [
    ['html', { open: 'never' }],
    ['list']
  ],
  use: {
    baseURL,
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  // Starts the frontend only. The backend is not managed here: it needs
  // Postgres and Redis, and killing `go run` leaks the server it spawned.
  // global-setup preflights it and tells you what to run if it's missing.
  webServer: skipWebServer ? undefined : {
    command: 'npm run dev',
    url: `http://localhost:${FRONTEND_PORT}`,
    reuseExistingServer: true,
    timeout: 120000,
  },
})
