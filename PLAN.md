# Whatomate Go Refactor Plan

*August 2026 — a phased plan to fix the correctness bugs, break up the 62k-line handlers package, and bring the backend in line with idiomatic Go, grounded in five parallel code audits (package structure, error handling, concurrency, `pkg/whatsapp` + tests, idiom sweep).*

**Key numbers:** 90k lines of Go · 62k of them in `internal/handlers` · ~330 methods on one `App` struct · 7 P0 races/panic paths · 131/224 handlers with no permission check · 1/~636 GORM calls carrying context.

**Agreed scope:** full domain split of `internal/handlers`; all bug fixes included (RBAC gap too, via staged rollout); `pkg/whatsapp`'s exported API is free to break (no external consumers).

---

## Where the code stands

What's **good** is worth protecting: error wrapping discipline is real (66% of `fmt.Errorf` uses `%w`), the logger is dependency-injected with zero globals, the inbound webhook types in `pkg/whatsapp` are fully and correctly modeled, lock ordering in the calling `Manager` is consistent, `go vet` is clean, the `errEnvelopeSent` contract is honored at all ~295 call sites, and no deprecated stdlib survives anywhere.

The debt is concentrated in five places:

- **One mega-package.** `internal/handlers` holds two-thirds of the codebase: 224 HTTP handlers plus, hidden among them, thirteen files of pure business logic (the chatbot engine, SLA processor, webhook dispatcher, template engine, RBAC) that never touch HTTP. Four internal dependency cycles hold it together.
- **Live concurrency bugs.** `internal/calling` has seven distinct data races and panic paths (an unsafe `safeClose`, unguarded channel-field reassignment, send-on-closed-channel windows), the WebSocket client races two fields between goroutines, and graceful shutdown in `main.go` abandons the hub, the workers, background tasks, and active calls.
- **No error taxonomy, no context.** Zero exported sentinel errors; a DB outage returns 404 to every client; exactly one of ~636 GORM queries carries a context, so nothing in the request path is cancellable.
- **An authorization gap.** 131 of 224 handlers perform org scoping but no `resource:action` permission check — route registration in `main.go` explicitly delegates the check to handlers, and most never took the handoff.
- **A public library that isn't one.** `pkg/whatsapp` flattens Meta's structured errors into strings, builds every outbound payload as `map[string]any` (108 sites), has three constructors instead of options, no retry/backoff in a bulk-messaging product, and imports `internal/templateutil`.

---

## Phase 0 — Mechanical groundwork & dead code

*3–4 small PRs · a few days · behavior-preserving except one flagged JSON fix*

Cheap, low-risk wins that shrink the codebase before the churn starts, so later diffs are cleaner.

### 0.1 Delete dead code (−1,068 LOC)

- `test/testutil/mocks.go` (385 LOC) — `MockWhatsAppClient`, `MockQueue`, `MockJobHandler` have zero references and can't even be wired in (`App.WhatsApp` is a concrete type). Their signatures have already drifted from the real client.
- `test/fixtures/models/factories.go` (683 LOC) — zero imports; duplicates the live `test/testutil/fixtures.go`.
- `test/testutil/db.go:63` `SetupTestDBWithCleanup` — a no-op with zero callers.
- Unify the drifted twin table lists `cleanupTables` / `TruncateTables` (`db.go:137` vs `:189` — six tables already missing from one copy) into a single list derived next to `runMigrations`.

### 0.2 Modern-idiom sweep

| Change | Scope |
| --- | --- |
| **Fix `omitempty` on a struct field (BUG)** | `pkg/whatsapp/analytics.go:193` — `omitempty` never omits struct values; needs `omitzero`. *This changes emitted JSON* — verify no consumer depends on the always-present empty `cursors`. |
| Delete hand-rolled `strings.Repeat` | `internal/database/postgres.go:223` (`repeatChar`, an O(n²) loop), `internal/utils/phone.go:9` |
| Adopt `slices`/`maps` | Delete `contains` (`widgets.go:653`, 6 call sites) and `containsEvent` (`webhook_dispatch.go:111`); convert 6 `sort.*` sites (incl. the `Atoi`-per-comparison sort in `pkg/whatsapp/message.go:308`); collapse `message.go:293` key-collection to `slices.Sorted(maps.Keys(m))` |
| Adopt `cmp.Or` | 96 default-fallback sites; chiefly the config-defaulting run in `internal/config/config.go:230–272`, plus `internal/calling/ivr.go` |
| `interface{}` → `any` | 22 stragglers, 21 of them in `handlers/embedded_signup_test.go` |
| Misc | `max()` builtin at `templates.go:745`; ~18 `fmt.Sprintf("%s%s")` cache keys in `cache.go` → `+`; ~10 `for i := range n` conversions; `errors.New` for `fmt.Errorf("%s", …)` at `pkg/whatsapp/types.go:61` |

### 0.3 Naming and hygiene

- Rename `internal/utils` → `internal/privacy` (all three functions are PII masking; 10 importers). In Phase 5 the org-level masking policy (`ShouldMaskPhoneNumbers` / `MaskContactFields` from `organization.go`) joins it.
- Add a package doc (`doc.go`) to `pkg/whatsapp` — the module's only public library has no package comment.
- Commit a pinned `.golangci.yml` (matching CI's v2.11.4) enabling at minimum `errcheck`, `staticcheck`, `govet`, `usetesting`, and gopls's `modernize` analyzer — this locks in the sweep so the old idioms can't creep back.

---

## Phase 1 — Concurrency & lifecycle correctness

*~6 PRs · 1–2 weeks · fixes live panics, races, and data loss — lands before any package moves so the diffs stay reviewable and cherry-pickable*

### 1.1 `internal/calling` races (P0)

- `safeClose` (`session.go:500`) is check-then-act with no lock — two goroutines can both close the same channel and panic. Replace with a `sync.Once` per signal channel, or close only under the owning mutex, applied at every call site (`audio.go:100`, `bridge.go:225`, `outgoing.go:565`, and the seven transfer paths).
- `session.DTMFBuffer`, `session.BridgeStarted`, `session.TransferAccepted` are read unlocked from consumer goroutines while other paths reassign or close them under (or without) `session.mu` — a send-on-closed-channel panic window and a stale-channel goroutine leak. Fix by making them accessible only through mutex-guarded accessor methods on `CallSession`; never close `DTMFBuffer`, nil it under the lock.
- `outgoing.go:217–222` writes six session fields after pion callbacks reading them are already registered — move the writes under `session.mu` (or set fields before registering callbacks).
- `transfer.go:605` uses a 25 ms `time.Sleep` to "ensure" the hold-music goroutine exited before reusing its track — replace with the done-channel pattern already used at `ivr.go:303`.
- RTP consumer goroutines (`consumeAudioTrack`, `consumeAudioWithDTMF`, `handleDTMFTrack`) block forever on half-open connections — add `SetReadDeadline` + a ctx-aware exit at all 8 launch sites.

### 1.2 WebSocket hub (P0)

- `client.currentContact` and `client.authenticated` are raced between `ReadPump`, `WritePump`, and the hub goroutine — guard with a small `client.mu` (or `atomic.Bool` + mutex for the contact).
- Add `Hub.Stop()` — `Run()` is currently unstoppable (needed by Phase 1.4's shutdown).
- Lower-priority: move `countClients` out from under the write lock; disconnect clients after N consecutive full-buffer drops instead of silently losing messages.

### 1.3 Fire-and-forget goroutines

- `audit.LogAudit` spawns one untracked goroutine per audit event — unbounded under load, silently dropped on shutdown. Replace with a buffered channel + one drain worker owned by `App` and tracked by its WaitGroup.
- Webhook ingest (`webhook.go`) launches up to 7 untracked goroutines *per Meta POST* — batches spawn hundreds, and shutdown drops messages already 200-ACKed to Meta. Bound with a semaphore (or `errgroup.SetLimit`) and track in `a.wg`.
- Introduce one `App.spawn(fn)` helper that wraps: `wg.Add/Done`, a detached-but-bounded context, and a `recover()` that logs — today 23 raw `go func()` launches have no recover, so any panic in them kills the process. Migrate the stragglers (`contacts.go:1038`, `templates.go:344`, `middleware.go:222`, the calling callbacks, recording finalizers).
- `agent_transfers.go:882` silently swallows a panic after `tx.Rollback()` — log and re-panic.

### 1.4 Graceful shutdown, rewritten

Current state in `main.go:317–345`: the hub is never stopped, the campaign-stats subscriber's Redis connection leaks, workers are cancelled but never awaited, `App.WaitForBackgroundTasks()` is called only by tests, active calls just die, and the HTTP server shuts down *after* the workers it feeds. Target order:

```
server.Shutdown()            // stop accepting; drain in-flight requests
slaProcessor.Stop()          // stop producers
workers: cancel + g.Wait()   // errgroup.WithContext replaces the bare spawn loops
app.WaitForBackgroundTasks() // webhook dispatch, async sends, audit drain
subscriber.Close()           // keep it on App so it's reachable
callManager.Shutdown(ctx)    // end calls cleanly, finalize recordings
hub.Stop()
```

Also: remove the `lo.Fatal` inside the listener goroutine (`main.go:275` — `os.Exit` bypasses all of the above) and give `runWorker` the same await treatment.

### 1.5 Queue semantics (data loss)

- **The retry policy is inverted.** A permanently-missing campaign returns an error → redelivered every 5 minutes forever; a transient send failure returns `nil` → never retried. Fix the return-value convention, add an `XPENDING` delivery-count check and a dead-letter stream for poison messages.
- Make the error backoff ctx-aware (`redis.go:175` blocks shutdown up to 1s), run `claimPendingMessages` on a ticker rather than once at startup, and let an in-flight job finish before the consumer exits.

### 1.6 Unchecked writes

- 24 GORM `Save/Create/Updates/Exec` statements never read `.Error` — contact assignment, call-log transitions, campaign counters silently fail. Check them all (the new `errcheck` lint keeps them checked).
- Redis `Del` results on cache *invalidation* paths (6 sites in `cache.go`, plus refresh-token revocation at `auth.go:507`) must be checked and logged — a failed invalidation serves stale permissions or a revoked session.
- `redirectTokens` (`custom_actions.go:69`) is the package's only mutable global: an in-memory token store that leaks unredeemed tokens forever and breaks under multi-instance deployment. Move it to Redis with TTL.

---

## Phase 2 — Error model & context propagation

*3–4 PRs · ~1 week · establishes the conventions every later phase builds on*

### 2.1 A real error taxonomy

- **`pkg/whatsapp`:** make `*MetaAPIError` implement `error` (plus `Unwrap` and a `StatusCode` field) instead of flattening it to a string at `types.go:51–64`. Add sentinels mapped from Meta codes — `ErrRateLimited`, `ErrInvalidToken`, `ErrReengagementRequired`, `ErrTemplateNotFound` — so the worker and handlers can branch with `errors.Is/As`. Today `errors.As` appears zero times in the repo because there has been nothing to assert against.
- **Handlers:** introduce a small set of app sentinels (`errNotFound`, `errForbidden`, typed validation errors) and one `a.sendError(r, err)` mapper. `findByIDAndOrg` (64 call sites) currently returns 404 for *any* DB error — a Postgres outage masquerades as "not found"; distinguish `gorm.ErrRecordNotFound` from everything else.
- Stop leaking `err.Error()` into API responses (32 sites) — the mapper returns generic client text and logs the detail.
- Compare `redis.Nil` with `errors.Is`, not `==` (`queue/redis.go:167`).

### 2.2 Context through the stack

The repo has the pattern exactly inverted: `pkg/whatsapp` is ctx-first on 55 of 67 methods (exemplary), but its callers feed it `context.Background()` — 96 non-test occurrences — and 635 of ~636 GORM queries carry no context at all. Nothing in the request path is cancellable.

- `fasthttp.RequestCtx` implements `context.Context` — handlers derive a ctx from the request once and pass it down. Services take `ctx` as the first parameter.
- Do the centralized fixes now: the 20 `context.Background()` sites in `cache.go`, the LLM calls in `chatbot_processor.go` (the longest-latency requests in the codebase, currently built with `http.NewRequest` and no deadline), the OAuth calls in `sso.go`, and `messages.go:866`'s bare `http.Get` on a *user-supplied URL* with no timeout (route it through the shared client + the existing SSRF guard).
- The long tail of `.WithContext` on GORM chains is threaded per-domain *during* Phase 5 extraction, so each signature is touched once, not twice.

### 2.3 Log once, at the boundary

`pkg/whatsapp` logs 79 times and also returns the error — every failed send is logged twice. Libraries return errors; remove the logging (or keep `Debug`-only behind an option) and let handlers log at the terminal `SendErrorEnvelope` site, which the codebase already does correctly 273 times. Also: replace the `fmt.Printf` migration progress bar in `internal/database/postgres.go:151–218` — the only non-main code writing to stdout — with the injected logger, and rebalance the 666-Error/68-Warn skew so alerts mean something.

---

## Phase 3 — `pkg/whatsapp` redesign

*4–5 PRs · 1–1.5 weeks · self-contained, can run in parallel with Phases 1–2 · API breakage approved*

### 3.1 Construction & transport

- Collapse the three constructors into `New(opts ...Option)` with `WithTimeout`, `WithBaseURL`, `WithHTTPClient`, `WithLogger` — today there is no way to set timeout *and* base URL together, which is why `messages_test.go:131` silently talks to real `graph.facebook.com`. Unexport `HTTPClient` and `Log` (mutable exported fields on a shared client are racy by construction).
- Add retry with exponential backoff + jitter honoring `Retry-After` and Meta's rate-limit codes. This is a bulk-messaging product with zero backoff anywhere.
- Route the five bypass methods (`UploadMedia`, `DownloadMedia`, `ResumableUpload`, `ExchangeCodeForToken`, `UpdateFlowJSON`) through `doRequest` so all Meta errors surface uniformly; accept 2xx rather than exactly 200.
- Fixes riding along: hardcoded multipart boundary + unescaped filename header (`client.go:312–325` → `mime/multipart`); `Handle[:20]` slice panic (`client.go:518`); silent `("", nil)` success (`call.go:130`); `client_secret` in the URL query string (`client.go:602` → POST form body); raw `"POST"` literals → `http.MethodPost`.

### 3.2 Types over maps

- Replace the 108 `map[string]any` outbound payload sites with typed request structs mirroring the (already excellent) inbound `WebhookPayload` tree. Typos in wire keys become compile errors; payloads become unit-testable.
- Turn the ~15 stringly-typed enums (template status/category, message type, granularity, OTP type…) into named string types with constants, following the `AnalyticsType` model already in the package.
- Collapse duplication with the generics the module already uses: one `listResponse[T]` for the 8+ `{Data []T}` envelopes, one ID-response type for the 10 redeclared `{ID string}` structs, and adopt `doJSON[T]` (currently used 3 times) at the ~30 hand-rolled unmarshal sites.
- Kill the variadic-as-optional signatures: `SendTextMessage(…, replyToMsgID ...string)` and `ButtonURLParamsToComponents(…, templateButtons ...[]any)` → options structs; `TemplateSubmission.Buttons []any` → `[]TemplateButton`.

### 3.3 Make it actually standalone, and actually used

- Move `templateutil.ExtParamNames` into the package — `pkg/` importing `internal/` is the one thing preventing genuine reuse. Its tests stop importing `test/testutil` for a nop logger.
- Fix the bypasses in `internal/`: `flows.go` (4 sites) and `worker.go:49` call `whatsapp.New()`, which hardcodes the production base URL and silently ignores the `whatsapp.base_url` config — inject the configured client instead. `contacts.go:1056` builds a Graph API reaction request inline → becomes `client.SendReaction`. `accounts.go`'s private `fetchMetaJSON` → the typed client methods that already exist.
- Split the 470-line `analytics.go` into its four sub-domains; fold the 16-line `profile_extras.go` away.

---

## Phase 4 — Declarative authorization & HTTP boilerplate

*2–3 PRs + a shadow-enforcement release · closes the RBAC gap in one artifact (SECURITY)*

### 4.1 A route table that owns permissions

The 131-handler permission gap should not be fixed with 131 individual edits. Replace the 233 imperative registrations in `main.go` with a declarative table:

```go
type route struct {
    method, path string
    handler      fastglue.FastRequestHandler
    permission   string // "campaigns:read"; "" = authenticated-only
    public       bool   // replaces the hand-maintained allowlist
}
```

A single wrapper enforces the permission before invoking the handler; the public-path allowlist in the auth `Before` hook derives from the same table, so adding a public endpoint stops being a two-place edit. The table survives Phase 5 unchanged (it references handler funcs wherever they end up), and route permission strings can be asserted against the frontend's `meta.permission` strings in a test.

> **This is a visible behavior change.** Under-privileged callers who today succeed will start receiving 403s. Roll out in two steps: a release where the wrapper only *logs* would-deny decisions (shadow enforcement) while each route is assigned its correct `resource:action`, then flip to enforcing. Coordinate with the frontend permission map.

### 4.2 Collapse the handler boilerplate

- With the table enforcing permissions, the 41 handlers that hand-roll `requireAuth`'s exact 12-line body and the 149 doing manual org extraction collapse to a single `getOrgAndUserID` call each.
- Adopt `listEnvelope` at the ~22 list endpoints still building pagination maps inline; adopt `decodeRequest` at the 7 raw `r.Decode` sites; the Phase 2 error mapper absorbs most of the 874 hand-written `SendErrorEnvelope` branches as files get touched.
- Extract shared, exported context-key constants (`organization_id`, `user_id`, …) used by `middleware`, `handlers`, and `testutil` — today they're string literals on both sides, and a typo in a test key silently skips authorization instead of failing.

---

## Phase 5 — Domain package split

*10–12 PRs · 3–4 weeks · the structural core of the refactor · each step keeps the build green and tests passing*

### 5.1 Target layout

Domain-named packages, one level deep, no layer names — `main.go` stays the wiring point. The existing well-factored packages (`calling`, `assignment`, `queue`, `websocket`, `audit`, `crypto`…) already follow this shape; the split brings `handlers` in line rather than inventing a new architecture:

```
internal/
  httpd/       thin HTTP layer: route table, envelope/decode helpers,
               error mapper, per-domain handler structs
  authn/       login, JWT/refresh, SSO, API keys, cookies
  rbac/        permissions + roles + the permission cache (out of cache.go)
  orgs/        organizations, users, teams, audit-log queries
  privacy/     PII masking policy (Phase 0 rename + org masking rules)
  accounts/    WhatsApp account config, embedded signup, templates,
               catalog, WhatsApp Flows, business profile
  messaging/   outbound send core: SendOutgoingMessage, media,
               broadcasts, template rendering (absorbs template_engine)
  inbox/       contacts, conversations, notes, tags, canned responses
  chatbot/     processor + graph runner + graph types + AI providers
               + flow migration (the cycle disappears: one domain)
  transfers/   agent transfers + SLA processor (same lifecycle domain)
  inbound/     Meta webhook ingest + fan-out (message/status/call routing)
  campaigns/
  webhookout/  outbound webhook CRUD + dispatch engine
  analytics/   widgets, agent analytics, meta analytics
  dataio/      import/export + custom actions
```

### 5.2 Breaking the four cycles

| Cycle | Resolution |
| --- | --- |
| `chatbot_processor ↔ chatbot_graph_runner` | Same domain — they merge into `internal/chatbot` and the cycle ceases to exist. |
| `chatbot_processor ↔ agent_transfers` | Both directions are service calls, not HTTP. `chatbot` depends on a small consumer-defined `TransferService` interface (create-to-queue/team, has-active); `transfers`' reverse needs (`isWithinBusinessHours` → `orgs`, `sendAndSaveTextMessage` → `messaging`) move to packages both may import. |
| `agent_transfers ↔ sla_processor` | SLA deadlines are part of the transfer lifecycle — one `transfers` package. |
| `contacts ↔ messages` | Extract the outbound-send core into `messaging`; both `inbox` handlers and everything else (10 domains call `SendOutgoingMessage` today) depend on it downward. |

Misplaced helpers get correct homes on the way: `resolveWhatsAppAccountByID` → `accounts` (and loses its `*fastglue.Request` parameter — it's account resolution, not a handler); `getMimeTypeFromExtension` → stdlib `mime.TypeByExtension`; `sanitizeFilename` → `storage`; `generateSlug`, `splitPermission`/`splitPermissionKey` dedup → their owning domains.

### 5.3 Dissolving the god object

`App` (13 dependencies, ~330 methods) becomes per-domain structs with narrow constructors — `chatbot.Engine` needs the DB, cache, WhatsApp client, and a `TransferService`; `analytics.Handler` needs the DB and nothing else. `main.go` constructs each and hands them to the route table. The unexported `App.wg` becomes the Phase 1 `spawn` helper's runner, owned by `main` and shared by injection. `cache.go` splits along its two identities: the permission engine goes to `rbac`; each cached entity's getter/invalidator pair moves to its domain (the cache-invalidation-on-write coupling documented in CLAUDE.md becomes package-local instead of package-global).

### 5.4 Migration order

Leaf-first, so each extraction only depends on already-extracted packages. Each step is one PR: move files, add ctx parameters (finishing Phase 2.2's long tail), move the tests alongside, keep `make test` and the e2e suite green.

1. `privacy`, then `rbac` (splitting `cache.go`) — everything depends on these
2. `messaging` (breaks `contacts ↔ messages`)
3. `webhookout`, `analytics`, `campaigns`, `dataio` — near-leaf, cheap wins to validate the pattern
4. `chatbot` (absorbs the processor/runner cycle), then `transfers` (+SLA), wiring the interface between them in `main`
5. `inbound` (webhook ingest fan-out over the now-extracted domains), `accounts`, `inbox`, `authn`/`orgs`
6. Finally the thin `httpd` layer and the handler-struct split; `main.go` shrinks to config, construction, table, shutdown

---

## Phase 6 — Testing hardening

*~1 week focused + ongoing · runs interleaved with Phases 1 and 5*

- **Test `internal/calling`.** 4,907 LOC of WebRTC/IVR/transfer state machines — the most concurrency-heavy code in the repo — has zero tests. Write them *with* the Phase 1 race fixes (the fixes make the package testable; `testing/synctest` makes the timer/timeout logic deterministic), and run the package under `-race` in CI.
- **Kill the sleeps.** Of 14 `time.Sleep` sites in tests, 12 are synchronization-by-luck: the `embedded_signup_test.go` pair waits 50 ms for an async audit write (use `app.WaitForBackgroundTasks()` — it exists for exactly this); the negative-assertion sleeps in `hub_gaps_test.go` and `pubsub_test.go` can only produce false confidence (rewrite with `synctest`); the queue teardown sleeps become real synchronization.
- **Make skipping loud.** Nearly all 1,283 tests silently skip without `TEST_DATABASE_URL`/`TEST_REDIS_URL`, so a bare `go test ./...` prints PASS while running almost nothing. Add a `TestMain` that prints a prominent skip summary; make the Redis helper skip like the DB helper does instead of returning nil.
- **Cover the wiring.** Handler tests invoke methods directly, so no test ever exercises route registration, middleware chains, or the Phase 4 permission table. Add a small routed integration suite that boots the real fastglue router and asserts auth/permission behavior per route — this is the regression test for the RBAC fix.
- Adopt `t.Context()` (currently used once) and delete the homegrown `testutil.TestContext`; convert the near-duplicate linear tests to tables opportunistically as files are touched — a wholesale rewrite of 941 test funcs is not worth its diff.

---

## Sequencing, verification, non-goals

### Why this order

Bugs before structure: the Phase 1 fixes are small, reviewable, cherry-pickable diffs — burying them inside package moves would make them unreviewable and unbisectable. Phase 2 sets the error/ctx conventions so extracted packages are born correct rather than migrated twice. Phase 3 is independent and parallelizable. Phase 4 lands before Phase 5 because the route table makes the handler layer thin enough to move, and the shadow-enforcement window needs calendar time anyway. The ctx long tail deliberately rides along with Phase 5 so every signature is touched exactly once.

### Verification, every phase

- `make lint` (golangci-lint v2, now with the pinned config) and `go vet ./...`
- Full suite with infrastructure: `TEST_DATABASE_URL=… TEST_REDIS_URL=… gotestsum -- -race -p 1 ./...` — never trust a green run without the env vars
- Playwright e2e against `make dev`; `make test-e2e-embedded` before each release-bound merge
- Per `CONTRIBUTING.md`: issues first for each phase, small single-concern conventional-commit PRs

### Explicitly out of scope

- **No migration off fastglue/fasthttp.** The stdlib `ServeMux` is the modern default for new code, but the middleware, envelope, and WebSocket layers are built on fasthttp — a router swap is a rewrite, not a refactor, and clears no cost bar here.
- **No GORM replacement and no migration-file system** — AutoMigrate stays, per the project's existing convention.
- **Don't flatten `internal/`.** The single-package-flat default is for small tools; a multi-domain service is exactly the case for one-level domain packages, which is what Phase 5 produces.
- **No mocking frameworks or BDD.** The `httptest`-server style (99 sites) is the right pattern and stays; the functional-options fixture builders in `testutil` are already exemplary.
- **Preserve what works:** the `errEnvelopeSent` contract (extend the mapper around it, don't replace it), the inbound webhook type tree, logger injection, the calling package's lock-ordering discipline, and `import_export.go`'s declarative config registry — which is the in-repo model the route table imitates.

### Rough footprint

| Phase | PRs | Elapsed | Risk |
| --- | --- | --- | --- |
| 0 Groundwork | 3–4 | days | low |
| 1 Concurrency & lifecycle | ~6 | 1–2 wk | medium — touches live call paths; race tests gate it |
| 2 Errors & context | 3–4 | ~1 wk | low |
| 3 pkg/whatsapp | 4–5 | 1–1.5 wk | low — httptest coverage is strong |
| 4 Authorization | 2–3 + shadow release | ~1 wk + soak | visible — 403 behavior change, staged rollout |
| 5 Package split | 10–12 | 3–4 wk | medium — mechanical but wide; green-per-PR discipline |
| 6 Testing | interleaved | ~1 wk focused | low |

Roughly two months of focused effort end-to-end, comfortably splittable across contributors after Phase 2 lands (Phases 3, 4, and the early Phase 5 extractions are independent).
