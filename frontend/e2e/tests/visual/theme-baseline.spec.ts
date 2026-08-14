import { test, expect } from '@playwright/test'
import { loginAsAdmin } from '../../helpers/auth'

/**
 * Visual regression baseline for the shadcn-vue migration.
 *
 * Not part of the default `npm run test:e2e` run — vue-tsc/eslint can't catch
 * a wrong color, and every migration phase changes colors, so this is the
 * actual verification gate for those phases. Skipped unless RUN_VISUAL=1
 * because there is no committed baseline yet: run once on `main` before the
 * migration starts to generate one (`--update-snapshots`), then re-run after
 * each phase and treat every diff as something to justify, not rubber-stamp.
 *
 * Usage:
 *   RUN_VISUAL=1 BASE_URL=http://localhost:8080 npx playwright test e2e/tests/visual --update-snapshots
 *   RUN_VISUAL=1 BASE_URL=http://localhost:8080 npx playwright test e2e/tests/visual
 */

test.skip(!process.env.RUN_VISUAL, 'Visual baselines require RUN_VISUAL=1 (see file header)')

const VIEWS = [
  { name: 'login', path: '/login', requiresAuth: false },
  { name: 'dashboard', path: '/', requiresAuth: true },
  { name: 'settings', path: '/settings', requiresAuth: true },
  { name: 'meta-insights', path: '/analytics/meta-insights', requiresAuth: true },
  { name: 'chat', path: '/chat', requiresAuth: true },
  { name: 'chatbot', path: '/chatbot', requiresAuth: true },
] as const

const THEMES = ['dark', 'light'] as const

for (const view of VIEWS) {
  for (const theme of THEMES) {
    test(`${view.name} — ${theme}`, async ({ page }) => {
      // Set before any app script runs, so useColorMode picks it up on first paint.
      await page.addInitScript((mode) => {
        window.localStorage.setItem('color-mode', mode)
      }, theme)

      if (view.requiresAuth) {
        await loginAsAdmin(page)
      }

      await page.goto(view.path, { waitUntil: 'domcontentloaded' })
      await page.waitForLoadState('networkidle')

      // Confirm the theme actually applied before trusting the screenshot.
      const themeClass = await page.evaluate(() => document.documentElement.className)
      expect(themeClass).toContain(theme)

      await expect(page).toHaveScreenshot(`${view.name}-${theme}.png`, {
        fullPage: true,
        animations: 'disabled',
        maxDiffPixelRatio: 0.02,
      })
    })
  }
}
