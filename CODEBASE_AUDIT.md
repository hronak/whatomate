# Codebase Audit — Inconsistencies & Discrepancies

> **Date**: 2026-08-17  
> **Scope**: Full-stack analysis (Go backend + Vue/TypeScript frontend)  
> **Method**: Automated deep-dive by 4 specialized analysis agents across handlers, models, database layer, types, services, components, and views.

---

## Summary

| Severity | Count | Key Themes |
|----------|-------|------------|
| 🔴 Critical | 4 | Missing RBAC on campaigns/chatbot/messages; cross-tenant data leak |
| 🟠 High | 7 | Missing tenant isolation; non-atomic DB operations; legacy error handling; shadcn-vue accessibility |
| 🟡 Medium | 11 | Silent validation failures; type mismatches; styling anti-patterns; monolithic views |
| 🔵 Low | 10 | Naming inconsistencies; dead code; minor anti-patterns |

---

## 🔴 Critical

### 1. Missing RBAC Authorization on Campaign Endpoints

**Backend · Security**

Every endpoint in `internal/handlers/campaigns.go` — `ListCampaigns`, `CreateCampaign`, `UpdateCampaign`, `StartCampaign`, `DeleteCampaign` — lacks `a.HasPermission` or `a.requireAuth` checks. Any authenticated user within an organization can create or delete campaigns regardless of their assigned role.

- `CreateCampaign` — `campaigns.go:147`
- `DeleteCampaign` — `campaigns.go:362`

### 2. Missing RBAC on Chatbot Write Operations

**Backend · Security**

Multiple chatbot write operations skip permission checks:

- `UpdateChatbotSettings` — `chatbot.go:268`
- `CreateKeywordRule` — `chatbot.go:596`
- `UpdateKeywordRule` — `chatbot.go:704`
- `DeleteKeywordRule` — `chatbot.go:777`

### 3. Missing RBAC on SendTemplateMessage

**Backend · Security**

`SendTemplateMessage` (`messages.go:666`) fetches user context but never checks if they're authorized to send messages.

### 4. Cross-Tenant Data Leak in Agent Transfers

**Backend · Security / Data Leak**

`agent_transfers.go:212` queries team memberships by `user_id` alone — without `organization_id`. Since a user can belong to multiple organizations, this leaks team visibility across tenants:

```go
a.DB.Where("user_id = ?", userID).Find(&memberships)
```

---

## 🟠 High

### 5. Missing Tenant Isolation in Agent Analytics

**Backend · Security**

`agent_analytics.go:360` — `calculateBreakTime` omits `organization_id` when querying `user_availability_logs`. Break time aggregates across all tenants an agent belongs to.

### 6. User Creation Without Transaction

**Backend · Logic Bug**

`users.go:349-361` — `User` and `UserOrganization` records are created as two separate DB calls without a transaction. If the second fails, the user is permanently orphaned from their org and role.

### 7. Non-Atomic Agent Transfer Completion

**Backend · Logic Bug**

`agent_transfers.go:779-791` — Transfer completion updates `AgentTransfer` and `Contact.assigned_user` in separate DB operations with no transaction. Partial failure leaves inconsistent state.

### 8. Inconsistent Error Handling Patterns (Legacy vs Modern)

**Backend · Consistency**

The codebase has a modern `a.sendError` API but a large swath of handlers still use the legacy `r.SendErrorEnvelope` directly, creating inconsistent client responses:

- `accounts.go:74, 138, 155`
- `auth.go:93, 180`
- `campaigns.go:111, 187`
- `contacts.go:142, 356`
- `messages.go:727, 990`

### 9. Missing `SelectGroup` in shadcn-vue Select Components

**Frontend · Component Composition**

`SelectItem` is used directly inside `SelectContent` without a wrapping `SelectGroup`, violating the shadcn-vue composition rule across:

- `src/components/LanguageSwitcher.vue:34-40`
- `src/components/calling/IVRNodeProperties.vue` (lines 552, 647, 713, 838)
- `src/components/chatbot/ChatNodeProperties.vue` (lines 315-321, 545-548, 752-759)
- `src/views/dashboard/DashboardView.vue`

### 10. Missing `DialogTitle` in CommandDialog

**Frontend · Accessibility**

`src/components/ui/command/CommandDialog.vue:15-21` — uses `<DialogContent>` without a `<DialogTitle>`, violating accessibility requirements (screen readers can't announce the dialog purpose).

### 11. Raw Hex Colors Instead of Semantic Tokens

**Frontend · Styling**

The chatbot flow preview components use hardcoded hex colors extensively instead of semantic CSS variables:

- Files in `src/components/chatbot/flow-preview/` (e.g., `FlowPreviewPanel.vue`, `InteractivePreview.vue`, `PreviewInputBar.vue`)
- Evidence: `bg-[#00a884]`, `bg-[#efeae2]`, `dark:bg-[#0b141a]`, `text-[#00a884]`, `bg-[#075e54]`

These will not respond to theme changes and break dark mode theming.

---

## 🟡 Medium

### 12. Silent JSON Decode Error Swallowing

**Backend · Validation**

Several handlers use `_ = r.Decode(&req, "json")`, silently ignoring malformed input:

- `accounts.go:837`
- `auth.go:274`
- `templates.go:472`

### 13. Bypassing `decodeRequest` API

**Backend · Validation**

Some handlers manually call `json.Unmarshal(r.RequestCtx.PostBody(), &req)` instead of using the standard `a.decodeRequest`:

- `chatbot.go:611` (`CreateKeywordRule`)
- `chatbot.go:732` (`UpdateKeywordRule`)
- `contacts.go:555` (`SendContactMessage`)

### 14. Inconsistent UUID Parsing from Path Params

**Backend · Consistency**

Some handlers manually extract and parse UUIDs from paths instead of using the `parsePathUUID` helper:

- `contacts.go:942` — `messageIDStr := r.RequestCtx.UserValue("id").(string)`
- `users.go:117`

### 15. Unchecked `First()` Before Delete

**Backend · Logic Bug**

`campaigns.go:788` — `DeleteCampaignRecipient` runs `First(&recipient)` without checking error or `RowsAffected`. If the record doesn't exist, it proceeds to delete a zero-value struct.

### 16. WhatsAppAccount Field Used as String Instead of FK

**Backend · Naming / Design**

Across `Contact`, `Template`, `WhatsAppFlow`, and `Message`, `WhatsAppAccount` is a plain `string` referencing `WhatsAppAccount.Name` rather than a proper foreign key (`WhatsAppAccountID uuid`):

```go
WhatsAppAccount string `gorm:"size:100;index" json:"whatsapp_account"` // References .Name
```

This makes renames destructive and prevents referential integrity.

### 17. Frontend Type Mismatches vs Backend Models

**Frontend · Types**

- `stores/auth.ts:26-37` — `User` interface is missing `is_active`, `sso_provider`, `sso_provider_id`, `created_at`, `updated_at` from the backend model.
- `services/api.ts:916-931` — `Team` interface is missing `organization_id`.

### 18. Defensive Coding Against Inconsistent API Response Shape

**Frontend · API Contract**

`stores/teams.ts:45, 66, 82` — The frontend has to work around inconsistent backend response wrapping:

```typescript
const data = (response.data as any).data || response.data;
```

This means the backend sometimes returns `{ data: { teams: [] } }` and sometimes `{ teams: [] }`.

### 19. Missing `data-icon` Attribute on Button Icons

**Frontend · shadcn-vue**

Icons placed inside `<Button>` components never use the `data-icon` attribute. This attribute is absent from the **entire** codebase, causing incorrect icon spacing/sizing inside buttons.

### 20. `space-x-*` / `space-y-*` Usage Instead of `gap-*`

**Frontend · Styling**

Widespread use of forbidden spacing utilities across:

- `src/components/calling/IVRNodeProperties.vue:284`
- `src/components/chatbot/ChatNodeProperties.vue:261, 281, 292`
- `src/components/ContactInfoPanel.vue:278, 380, 424`
- `src/views/settings/AgentTransfersView.vue:516`
- `src/views/settings/CustomActionsView.vue:572`

### 21. Giant View Files Mixing Concerns

**Frontend · Architecture**

- `src/views/chat/ChatView.vue` — **3400+ lines / 123KB**. Mixes WebSocket handling, infinite scroll, file upload state, and multi-account logic all in one `<script setup>`.
- `src/views/dashboard/DashboardView.vue` — **~2000 lines**. Mixes grid layout calculations, Chart.js init, widget CRUD, and permission evaluations.

### 22. `useCrudState` Lacks Error Tracking

**Frontend · Composables**

`src/composables/useCrudState.ts` tracks `isLoading` and `isSubmitting` but has no `error` ref, forcing each consumer to independently manage error state.

---

## 🔵 Low

### 23. Inconsistent `WhatsAppMessageID` Column Sizes

**Backend** — `Message` uses `size:255`, `BulkMessageRecipient` uses `size:100` for the same logical field.

### 24. GORM Column Name Inconsistency for WhatsApp IDs

**Backend** — `CallLog.WhatsAppCallID` maps to `whatsapp_call_id` while `Message.WhatsAppMessageID` maps to `whats_app_message_id` (GORM default camelCase split).

### 25. `ChatbotSessionMessage` Duplicates `Message` Fields

**Backend** — Redundantly stores `Message`, `Direction`, and metadata without a FK reference to `Message.ID`.

### 26. Unused Constants

**Backend** — `internal/models/constants.go` defines `TemplateStatusRejected`, `SSOProviderGitHub`, `SSOProviderFacebook` but these are never referenced.

### 27. Inconsistent Error Wrapping in DB Layer

**Backend** — `internal/database/postgres.go:125-133` returns bare `err`, while `RunMigrationWithProgress` wraps with `fmt.Errorf`.

### 28. Overlapping Button Types in WhatsApp Client

**Backend** — `pkg/whatsapp/types.go` has both `Button` (messaging) and `TemplateButton` (templates) with divergent field names for the same concept (`Title` vs `Text`).

### 29. Token Refresh Bypasses Vue Router

**Frontend** — `services/api.ts:167` does a hard `window.location.href` redirect to `/login` on refresh failure, losing query params and bypassing router guards.

### 30. Parallel Route Navigation Structure

**Frontend** — `router/index.ts:344-391` maintains a separate `navigationOrder` array that must be manually kept in sync with actual route definitions.

### 31. `cn()` Not Used for Conditional Classes

**Frontend** — The codebase uses Vue array/object class syntax (`:class="['...', condition ? '' : '-rotate-90']"`) instead of the `cn()` utility throughout.

### 32. Hardcoded i18n Strings

**Frontend** — e.g. `src/views/chat/ChatView.vue:2543` — `<p class="font-medium">Location</p>` should use `{{ t('...') }}`.
