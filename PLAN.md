# Frontend Refactor Plan

*August 2026 — dependency and component diet for `frontend/`. Principle: lean on Vue 3 language features and the shadcn-vue/reka-ui library already in the tree; a package earns its place only if neither can provide the functionality, and nothing stays installed for a single trivially-writable helper.*

**Key numbers:** 26 runtime deps, of which **7 are completely unused** and 1 more (`@vueuse/core`) is used for two one-liner helpers · 45 `src/components/ui/` directories, of which **17 are never imported** (2,812 lines) · ~530 lines of dead app code (`useApiMocker`, `usePagination`, `PaginationControls`) · 3 near-identical AlertDialog wrappers · `useCrudState` exists but only 3 of ~20 CRUD views use it.

Every claim below was verified by grepping actual imports in `src/` (not just `package.json`). No code changes have been made yet.

---

## Verification harness (run after every phase)

```bash
cd frontend
npm run typecheck        # vue-tsc --noEmit
npm run lint
npm run build            # catches dynamic-import/chunk breakage
npx playwright test      # against `make dev` on :3000
```

Playwright is the only frontend test layer, so it is the safety net for all of this. Phases are ordered so each is independently shippable as a small, single-concern PR (per CONTRIBUTING.md).

---

## Phase 1 — Remove dead packages (zero code risk)

None of these are imported anywhere in `src/`, `e2e/`, or config (verified by full-text grep, not just `import` statements):

| Package | Evidence | Notes |
| --- | --- | --- |
| `@tanstack/vue-query` | Only reference is `app.use(VueQueryPlugin)` in `src/main.ts:3` — **zero** `useQuery`/`useMutation` calls exist | Delete the two lines in `main.ts`, drop the dep. All data fetching already goes through `services/api.ts` + Pinia. |
| `vee-validate` | Only used by the unused `ui/form/` dir (Phase 2) and `vite.config.ts` manualChunks | Forms are hand-rolled with `Input`/`Label`; validation is server-side envelopes. |
| `@vee-validate/zod` | Zero imports | |
| `zod` | Only `vite.config.ts` manualChunks | |
| `dompurify` | Zero references anywhere | |
| `vaul-vue` | Only used by the unused `ui/drawer/` dir (Phase 2) | |
| `vuedraggable` | Zero references (drag needs are served by vue-flow and grid-layout-plus) | |

Dev dependencies to drop — CLAUDE.md already documents there is no vitest config or script; Playwright is the only test runner:

- `vitest`, `@vue/test-utils`, `happy-dom` (zero references in any config or test file)
- Keep `pg` + `@types/pg` — used by `e2e/global-cleanup.ts` and several specs.

Also in this phase:

- `vite.config.ts`: delete the `'validation': ['vee-validate', '@vee-validate/zod', 'zod']` manualChunks entry; remove `@vueuse/core` from the `'utils'` entry once Phase 3 lands.
- `src/main.ts`: remove the VueQueryPlugin import + `app.use`.

## Phase 2 — Delete never-imported code

### 2a. Unused `src/components/ui/` directories (17 dirs, 2,812 lines)

No file outside `components/ui/` imports these (checked app-wide, including relative and cross-`ui` imports):

```
aspect-ratio  calendar  context-menu  drawer  form  hover-card
menubar  navigation-menu  number-field  pagination  pin-input
resizable  sheet  slider  toast  toggle  toggle-group
```

Notes and traps:

- `toggle` is imported **only** by `toggle-group`, which is itself unused — delete both together.
- `calendar` is unused but `range-calendar` **is used** (by `shared/DateRangePicker.vue`) and does not import `calendar` — keep `range-calendar`.
- `toast` (10 files incl. `use-toast.ts`) is fully superseded by `vue-sonner` (58 importing files via `useAppToast`) — delete the dir, keep vue-sonner.
- `drawer` and `form` are what pin `vaul-vue` and `vee-validate` — delete them in the same PR as (or before) Phase 1.
- Any of these can be re-synced from shadcn-vue later if a feature needs them (`components.json` stays); the CLAUDE.md rule about stripping upstream `text-sm`/`text-xs` applies on re-sync.

After deletion the `ui/` surface drops from 45 dirs to 28, all with real consumers.

### 2b. Dead app code (~530 lines)

| File | Lines | Evidence |
| --- | --- | --- |
| `src/composables/useApiMocker.ts` | 214 | Zero consumers |
| `src/composables/usePagination.ts` | 217 | Sole consumer is `PaginationControls.vue`, which is itself unused |
| `src/components/shared/PaginationControls.vue` | 96 | Zero consumers outside `shared/index.ts` (live pagination is `useSearchPagination` + `useInfiniteScroll`) |

Remove their exports from `src/composables/index.ts` / `src/components/shared/index.ts`.

## Phase 3 — Replace tiny-usage packages with short code

### `@vueuse/core` → ~20 lines in `src/lib/utils.ts` (remove the package)

Actual usage across the entire app is exactly two functions:

- `useDebounceFn` — 7 call sites (`useSearchPagination.ts` + 6 views' search boxes)
- `onKeyStroke` — 1 call site (`chat/MediaViewerDialog.vue`, Escape/arrow keys)

Replacements, leveraging plain TS + Vue lifecycle:

```ts
// lib/utils.ts
export function debounce<T extends (...args: any[]) => void>(fn: T, ms = 300) {
  let t: ReturnType<typeof setTimeout>
  return (...args: Parameters<T>) => { clearTimeout(t); t = setTimeout(() => fn(...args), ms) }
}
```

```ts
// composables/useKeydown.ts (~10 lines)
export function useKeydown(handler: (e: KeyboardEvent) => void) {
  onMounted(() => window.addEventListener('keydown', handler))
  onUnmounted(() => window.removeEventListener('keydown', handler))
}
```

Then drop `@vueuse/core` and its manualChunks entry. (If the team would rather keep vueuse as a standing utility belt, that's defensible — but at 2 functions across 8 files it currently fails the "package for a single feature" test this refactor is applying.)

### `vue-chartjs` → one ~40-line wrapper component (optional, low priority)

All chart rendering already funnels through `src/lib/charts.ts`, which re-exports `Line`, `Bar`, `Pie`, `Doughnut` from vue-chartjs to exactly 3 views. vue-chartjs is a thin lifecycle wrapper around chart.js; a single generic `<ChartCanvas type="line" :data :options>` component (canvas ref + `new Chart()` in `onMounted`, `watch` for data with `chart.update()`, `onUnmounted` destroy, `shallowRef` for the instance) replaces it with no consumer churn beyond the import in `charts.ts`. Do this last — it's the only replacement in the plan that trades a working dep for new code that can regress (chart reactivity edge cases), and the win is small.

### Explicit keep list (earn their place)

| Package | Why it stays |
| --- | --- |
| `vue`, `vue-router`, `pinia`, `vue-i18n` | Framework core; i18n is Crowdin-synced across locales |
| `axios` | `services/api.ts` (1,256 lines) depends on its interceptor model for CSRF injection, org header, and the single-use refresh-token mutex + Web Locks flow — CLAUDE.md explicitly says don't touch that. A fetch rewrite is high-risk, zero-feature. |
| `reka-ui` + `clsx` + `tailwind-merge` + `class-variance-authority` | The shadcn-vue foundation; cva/clsx/tw-merge power `cn()` and variants in every `ui/` component |
| `@lucide/vue` | 140 importing files |
| `vue-sonner` | 58 importing files (the app's only toast system after `ui/toast` is deleted) |
| `@vue-flow/*` (4 pkgs) | Chatbot + IVR flow builders; a canvas graph editor is not hand-writable |
| `chart.js` | Real charting engine (analytics, dashboard) |
| `grid-layout-plus` | Dashboard drag/resize grid — real functionality, 1 consumer but not replicable in short code |
| `vue3-emoji-picker` | Chat composer; already lazy-loaded in `ChatView.vue`. An emoji dataset + picker is not "short code" — keep. |
| `@playwright/test`, `pg`, `@types/pg` | E2E layer |

Net result: runtime deps go from 26 → 18 (17 if the vue-chartjs swap happens).

## Phase 4 — Consolidate duplicated shared components

### Merge the three AlertDialog wrappers into one

`ConfirmDialog.vue` (69 lines, 23 consumers), `DeleteConfirmDialog.vue` (73 lines, 17 consumers), and `UnsavedChangesDialog.vue` (38 lines, 12 consumers) are structurally identical AlertDialog shells differing only in defaults:

- `DeleteConfirmDialog` ≡ `ConfirmDialog` with `variant="destructive"`, delete-flavored default strings, and an `itemName` convenience prop. Fold `itemName` into `ConfirmDialog`, delete `DeleteConfirmDialog`, and mechanically update the 17 call sites (or keep a 5-line re-export shim during migration).
- `UnsavedChangesDialog` additionally uses `open` as a plain prop with `stay`/`leave` emits instead of `defineModel` + `confirm`/`cancel`. Either express it as `<ConfirmDialog>` with i18n'd defaults, or keep it as a ~15-line specialization *of* ConfirmDialog. Its i18n usage (`$t(...)`) should be adopted by the merged component — the current ConfirmDialog/DeleteConfirmDialog hardcode English strings, which violates the i18n rule in CLAUDE.md anyway.

One component, one behavior (loading state, escape/cancel semantics), and future fixes land once instead of three times.

### Smaller cleanups

- `IconButton.vue` is a reasonable Tooltip+Button composition (20 consumers) — keep, but it duplicates the `variant`/`size` prop unions from `buttonVariants`; type them with `VariantProps<typeof buttonVariants>` from cva instead of hand-copied literals.
- `Spinner.vue` (37 consumers): fine to keep as the single loading primitive; ensure new code uses it rather than ad-hoc `Loader2` spins.
- `useSearchPagination` (10 consumers) is the surviving pagination pattern — document it in `shared/types.ts` or ARCHITECTURE notes as the default so `usePagination`-style reinvention doesn't come back.

## Phase 5 — Actually use the abstractions that exist (`useCrudState` rollout)

CLAUDE.md says "Reuse `useCrudState` for list/dialog/delete state" — but only **3 of ~20** eligible CRUD list views do (`useCrudState` 131 lines; adopters vs. hand-rolled checked by grep). The settings/chatbot list views (`TagsView`, `WebhooksView`, `TeamsView`, `UsersView`, `RolesView`, `TemplatesView`, `CampaignsView`, `ContactsView`, `APIKeysView`, `CannedResponsesView`, `AccountsView`, `KeywordsView`, `AIContextsView`, `ChatbotFlowsView`, `CustomActionsView`, `FlowsView`, …) each re-derive the same refs: dialog open/close, edit-vs-create, delete-confirm target, submitting flags.

Approach:

1. Audit `useCrudState`'s API against what the hand-rolled views actually need (e.g. does it cover the delete-confirm + `isSubmitting` + toast-on-error shape most views repeat?). Extend it once if needed.
2. Convert views mechanically, one PR per 3–5 views, running the matching Playwright specs per batch (`e2e/tests/settings/*.spec.ts` largely map 1:1 to these views).
3. Pair each conversion with `CrudFormDialog` (currently 3 consumers) and the merged ConfirmDialog where the view's markup allows.

Expected effect: each converted view sheds 40–80 lines of state plumbing; behavior converges (e.g. consistent "dialog stays open on failed submit" semantics).

This phase is the largest by touched-file count but the most mechanical; it is deliberately after the deletions so conversions don't churn files that reference soon-dead code.

## Phase 6 — Vue 3 idiom pass (small, opportunistic)

- `src/components/calling/IVRPathTree.vue` is the **only** non-`<script setup>` component left — convert to `<script setup>`.
- `defineModel` is already the norm (see ConfirmDialog) — sweep for remaining `props.modelValue` + `emit('update:modelValue')` pairs during Phase 4/5 conversions rather than as a standalone pass.
- Chart/flow data objects handed to chart.js or vue-flow should be `shallowRef`/`markRaw` where they aren't already — deep reactivity on chart datasets is wasted work (check `DashboardView.vue`, the two analytics views, `useFlowGraphSimulation.ts`).
- Lazy-loading is already used well (emoji picker, route-level splitting) — no action.

## Not in scope (explicitly considered and rejected)

- **Replacing axios with `fetch`** — the refresh-mutex/CSRF interceptor machinery is load-bearing and documented as do-not-simplify.
- **Splitting `ChatView.vue` (2,490 lines) and other monolith views** — worthwhile, but it's a feature-architecture refactor, not a dependency/component diet; do it separately so these PRs stay single-concern.
- **Replacing `grid-layout-plus` or `vue3-emoji-picker` with hand-written code** — both fail the "short code" test in the other direction.

## Suggested PR sequence

| PR | Content | Risk |
| --- | --- | --- |
| 1 | Phase 2a ui-dir deletions + Phase 1 package removals + `vite.config.ts`/`main.ts` cleanup (one PR — the dirs pin the packages) | None (dead code); `npm run build` + full e2e |
| 2 | Phase 2b dead composables/components | None |
| 3 | Phase 3 `@vueuse/core` → local helpers | Low; e2e covers the 6 search boxes + media viewer keys |
| 4 | Phase 4 dialog merge | Medium (52 call sites total, mechanical); full e2e |
| 5–8 | Phase 5 `useCrudState` rollout in batches | Medium; per-view specs |
| 9 | Phase 6 idiom pass + (optional) vue-chartjs swap | Low/Medium |

After PR 1, also confirm the embedded artifact: `make test-e2e-embedded` (the dev-server e2e run does not exercise the production chunking that `manualChunks` edits affect).
