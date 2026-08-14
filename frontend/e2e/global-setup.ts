import { request } from '@playwright/test'
import { cleanupE2EData } from './global-cleanup'
import { DEV_URL, BACKEND_URL } from '../ports.ts'

// Must match playwright.config.ts's default, or setup seeds users into one
// server while the tests run against another.
const BASE_URL = process.env.BASE_URL || DEV_URL
const DB_URL = process.env.TEST_DATABASE_URL || 'postgres://whatomate:whatomate@127.0.0.1:5432/whatomate'

interface CreateUser {
  email: string
  password: string
  full_name: string
  role_name: string
}

/**
 * Extract the whm_csrf cookie value from Set-Cookie response headers.
 */
function extractCSRFToken(response: { headersArray: () => Array<{ name: string; value: string }> }): string | null {
  const cookieHeaders = response.headersArray().filter(h => h.name.toLowerCase() === 'set-cookie')
  for (const header of cookieHeaders) {
    const match = header.value.match(/whm_csrf=([^;]+)/)
    if (match) return match[1]
  }
  return null
}

/**
 * Fail before the suite runs, with the reason, rather than letting every spec
 * fail on a login form that says "Invalid credentials" because a 502 body
 * wasn't a valid envelope. Distinguishes the three ways this goes wrong:
 * nothing listening, the proxy is up but the backend behind it isn't, and the
 * backend is up but hasn't been migrated.
 */
async function preflight() {
  const viaProxy = BASE_URL === DEV_URL
  const ctx = await request.newContext({ baseURL: BASE_URL })

  let status: number
  let body: string
  try {
    const resp = await ctx.post('/api/auth/login', {
      data: { email: 'admin@admin.com', password: 'admin' },
    })
    status = resp.status()
    body = await resp.text()
  } catch (err) {
    throw new Error(
      `Nothing is answering at ${BASE_URL}.\n` +
        (viaProxy
          ? '  Start the dev stack with `make dev` (backend + frontend).'
          : `  Start the backend with \`make run-migrate\`, or use the dev server at ${DEV_URL}.`) +
        `\n  Underlying error: ${err instanceof Error ? err.message : String(err)}`,
      { cause: err }
    )
  } finally {
    await ctx.dispose()
  }

  if (status === 502 || status === 503 || status === 504) {
    throw new Error(
      `The dev server at ${BASE_URL} is running, but it can't reach the backend at ${BACKEND_URL} (got ${status}).\n` +
        '  Start the backend with `make run-migrate`, or run the whole stack with `make dev`.'
    )
  }

  if (status === 401 || status === 403) {
    throw new Error(
      `Backend at ${BASE_URL} rejected the default super admin (admin@admin.com).\n` +
        '  Migrations create that account — start the backend with `-migrate` (`make run-migrate`).'
    )
  }

  if (status >= 400) {
    throw new Error(`Unexpected ${status} from ${BASE_URL}/api/auth/login: ${body.slice(0, 300)}`)
  }

  console.log(`  ✅ Preflight: backend reachable via ${BASE_URL}`)
}

async function globalSetup() {
  console.log(`\n🔧 Global Setup: Preflight against ${BASE_URL} ...`)
  await preflight()

  console.log('🔧 Global Setup: Cleaning leftover E2E data...')
  await cleanupE2EData(DB_URL)

  console.log('🔧 Global Setup: Creating test users...')

  const context = await request.newContext({
    baseURL: BASE_URL,
  })

  // Step 1: Login as the default superadmin (created by migrations)
  // This user has IsSuperAdmin=true and can create users in any org
  const defaultAdmin = {
    email: 'admin@admin.com',
    password: 'admin',
  }

  let csrfToken: string | null = null

  try {
    const loginResponse = await context.post('/api/auth/login', {
      data: defaultAdmin,
    })

    if (loginResponse.ok()) {
      // Auth cookies are auto-persisted by Playwright's APIRequestContext
      csrfToken = extractCSRFToken(loginResponse)
      console.log(`  ✅ Logged in as superadmin: ${defaultAdmin.email}`)
    } else {
      console.log(`  ❌ Failed to login as superadmin: ${await loginResponse.text()}`)
      console.log(`  ℹ️  Make sure migrations have run (./whatomate server -migrate)`)
    }
  } catch (error) {
    console.log(`  ❌ Error logging in as superadmin:`, error)
  }

  // Step 2: Get the roles to find admin, manager and agent role IDs
  const roleIds: Record<string, string> = {}

  try {
    // GET requests don't need CSRF token — cookies auto-sent by Playwright
    const rolesResponse = await context.get('/api/roles')

    if (rolesResponse.ok()) {
      const data = await rolesResponse.json()
      const roles = data.data?.roles || []
      for (const role of roles) {
        roleIds[role.name] = role.id
      }
      console.log(`  ✅ Found roles: ${Object.keys(roleIds).join(', ')}`)
    } else {
      console.log(`  ⚠️  Could not fetch roles: ${rolesResponse.status()}`)
    }
  } catch (error) {
    console.log(`  ⚠️  Error fetching roles:`, error)
  }

  // Step 3: Create test users in the default organization
  const usersToCreate: CreateUser[] = [
    { email: 'admin@test.com', password: 'password', full_name: 'Test Admin', role_name: 'admin' },
    { email: 'manager@test.com', password: 'password', full_name: 'Test Manager', role_name: 'manager' },
    { email: 'agent@test.com', password: 'password', full_name: 'Test Agent', role_name: 'agent' },
  ]

  // Get existing users to check for duplicates
  let existingEmails: Set<string> = new Set()
  try {
    const listResponse = await context.get('/api/users')
    if (listResponse.ok()) {
      const data = await listResponse.json()
      const users = data.data?.users || []
      existingEmails = new Set(users.map((u: { email: string }) => u.email))
    }
  } catch (error) {
    console.log(`  ⚠️  Error fetching existing users:`, error)
  }

  const csrfHeaders: Record<string, string> = csrfToken ? { 'X-CSRF-Token': csrfToken } : {}

  for (const user of usersToCreate) {
    if (existingEmails.has(user.email)) {
      console.log(`  ⏭️  User already exists: ${user.email}`)
      continue
    }

    try {
      const roleId = roleIds[user.role_name] || null

      const createResponse = await context.post('/api/users', {
        headers: csrfHeaders,
        data: {
          email: user.email,
          password: user.password,
          full_name: user.full_name,
          role_id: roleId,
          is_active: true,
        },
      })

      if (createResponse.ok()) {
        console.log(`  ✅ Created user: ${user.email} (${user.role_name})`)
      } else {
        const body = await createResponse.text()
        if (body.includes('already') || createResponse.status() === 409) {
          console.log(`  ⏭️  User already exists: ${user.email}`)
        } else {
          console.log(`  ⚠️  Could not create ${user.email}: ${createResponse.status()} - ${body}`)
        }
      }
    } catch (error) {
      console.log(`  ❌ Error creating ${user.email}:`, error)
    }
  }

  await context.dispose()
  console.log('🔧 Global Setup: Complete\n')
}

export default globalSetup
