package main

import (
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/zerodha/fastglue"
)

// route describes one HTTP endpoint and the permission required to call it.
//
// The 233 imperative g.GET/g.POST registrations this replaces delegated every
// authorization decision to the handler, and 129 of 224 handlers never took the
// handoff: they scoped queries by organization but checked no
// resource:action at all. Fixing that with 129 individual edits would have been
// unreviewable and impossible to audit afterwards. A table can be read top to
// bottom and diffed.
//
// The public-path allowlist in the auth hook derives from this same table, so
// adding a public endpoint is no longer a two-place edit that fails open when
// someone forgets the second place.
type route struct {
	method  string
	path    string
	handler fastglue.FastRequestHandler

	// permission is the "resource:action" pair the caller must hold. Empty
	// means any authenticated user may call it.
	permission [2]string

	// public marks an endpoint reachable without authentication.
	public bool

	// rateLimit names the per-endpoint rate-limit bucket to apply, or "" for
	// none. These were the one class of route that could not be expressed
	// imperatively without a special case in setupRoutes.
	rateLimit string
}

// authenticatedOnly is the empty permission: authentication required, no
// specific grant.
var authenticatedOnly = [2]string{}

// appRoutes returns the complete route table.
//
// Permissions come from two places. Where a handler already enforced one, that
// exact pair is used, so those routes keep their current behavior. The rest are
// derived from the resource the path addresses and the action the method
// implies (GET read, DELETE delete, everything else write) — those are the ones
// the shadow-enforcement window exists to validate before they start denying.
func appRoutes(app *handlers.App) []route {
	return []route{
		// Health check
		{"GET", "/health", app.HealthCheck, authenticatedOnly, true, ""},
		{"GET", "/ready", app.ReadyCheck, authenticatedOnly, true, ""},
		// Auth routes (public, optionally rate-limited)
		{"POST", "/api/auth/login", app.Login, authenticatedOnly, true, "login"},
		{"POST", "/api/auth/register", app.Register, authenticatedOnly, true, "register"},
		{"POST", "/api/auth/refresh", app.RefreshToken, authenticatedOnly, true, "refresh"},
		{"POST", "/api/auth/logout", app.Logout, authenticatedOnly, true, ""},
		{"POST", "/api/auth/switch-org", app.SwitchOrg, authenticatedOnly, false, ""},
		{"GET", "/api/auth/ws-token", app.GetWSToken, authenticatedOnly, false, ""},
		// SSO routes (public, optionally rate-limited)
		{"GET", "/api/auth/sso/providers", app.GetPublicSSOProviders, authenticatedOnly, true, ""},
		{"GET", "/api/auth/sso/{provider}/init", app.InitSSO, authenticatedOnly, true, "sso_init"},
		{"GET", "/api/auth/sso/{provider}/callback", app.CallbackSSO, authenticatedOnly, true, "sso_callback"},
		// Webhook routes (public - for Meta)
		{"GET", "/api/webhook", app.WebhookVerify, authenticatedOnly, true, ""},
		{"POST", "/api/webhook", app.WebhookHandler, authenticatedOnly, true, ""},
		// WebSocket route (auth via message-based flow after upgrade)
		{"GET", "/ws", app.WebSocketHandler, authenticatedOnly, true, ""},
		// Current User (all authenticated users)
		{"GET", "/api/me", app.GetCurrentUser, authenticatedOnly, false, ""},
		{"PUT", "/api/me/settings", app.UpdateCurrentUserSettings, authenticatedOnly, false, ""},
		{"PUT", "/api/me/password", app.ChangePassword, authenticatedOnly, false, ""},
		{"PUT", "/api/me/availability", app.UpdateAvailability, authenticatedOnly, false, ""},
		{"GET", "/api/me/organizations", app.ListMyOrganizations, authenticatedOnly, false, ""},
		// User Management (admin only - enforced by middleware)
		{"GET", "/api/users", app.ListUsers, [2]string{models.ResourceUsers, models.ActionRead}, false, ""},
		{"POST", "/api/users", app.CreateUser, [2]string{models.ResourceUsers, models.ActionWrite}, false, ""},
		{"GET", "/api/users/{id}", app.GetUser, [2]string{models.ResourceUsers, models.ActionRead}, false, ""},
		{"PUT", "/api/users/{id}", app.UpdateUser, [2]string{models.ResourceUsers, models.ActionWrite}, false, ""},
		{"DELETE", "/api/users/{id}", app.DeleteUser, [2]string{models.ResourceUsers, models.ActionDelete}, false, ""},
		// Roles & Permissions (admin only - enforced by middleware)
		{"GET", "/api/roles", app.ListRoles, [2]string{models.ResourceRoles, models.ActionRead}, false, ""},
		{"POST", "/api/roles", app.CreateRole, [2]string{models.ResourceRoles, models.ActionWrite}, false, ""},
		{"GET", "/api/roles/{id}", app.GetRole, [2]string{models.ResourceRoles, models.ActionRead}, false, ""},
		{"PUT", "/api/roles/{id}", app.UpdateRole, [2]string{models.ResourceRoles, models.ActionWrite}, false, ""},
		{"DELETE", "/api/roles/{id}", app.DeleteRole, [2]string{models.ResourceRoles, models.ActionDelete}, false, ""},
		{"GET", "/api/permissions", app.ListPermissions, [2]string{models.ResourceRoles, models.ActionRead}, false, ""},
		// API Keys (admin only - enforced by middleware)
		{"GET", "/api/api-keys", app.ListAPIKeys, [2]string{models.ResourceAPIKeys, models.ActionRead}, false, ""},
		{"GET", "/api/api-keys/{id}", app.GetAPIKey, [2]string{models.ResourceAPIKeys, models.ActionRead}, false, ""},
		{"POST", "/api/api-keys", app.CreateAPIKey, [2]string{models.ResourceAPIKeys, models.ActionWrite}, false, ""},
		{"PUT", "/api/api-keys/{id}", app.UpdateAPIKey, [2]string{models.ResourceAPIKeys, models.ActionWrite}, false, ""},
		{"DELETE", "/api/api-keys/{id}", app.DeleteAPIKey, [2]string{models.ResourceAPIKeys, models.ActionDelete}, false, ""},
		// Accounts
		// The embedded-signup config resolves the caller's org credentials, so
		// it needs the auth middleware to have run: marked public it answers
		// every caller with a 401 from getOrgID, which the SPA reads as a dead
		// session and logs the user out.
		{"GET", "/api/embedded-signup/config", app.GetEmbeddedSignupConfig, [2]string{models.ResourceAccounts, models.ActionRead}, false, ""},
		{"GET", "/api/accounts", app.ListAccounts, [2]string{models.ResourceAccounts, models.ActionRead}, false, ""},
		{"POST", "/api/accounts", app.CreateAccount, [2]string{models.ResourceAccounts, models.ActionWrite}, false, ""},
		{"POST", "/api/accounts/exchange-token", app.ExchangeToken, [2]string{models.ResourceAccounts, models.ActionWrite}, false, ""},
		{"GET", "/api/accounts/{id}", app.GetAccount, [2]string{models.ResourceAccounts, models.ActionRead}, false, ""},
		{"PUT", "/api/accounts/{id}", app.UpdateAccount, [2]string{models.ResourceAccounts, models.ActionWrite}, false, ""},
		{"DELETE", "/api/accounts/{id}", app.DeleteAccount, [2]string{models.ResourceAccounts, models.ActionDelete}, false, ""},
		{"POST", "/api/accounts/{id}/register", app.RegisterPhoneNumber, [2]string{models.ResourceAccounts, models.ActionWrite}, false, ""},
		{"POST", "/api/accounts/{id}/test", app.TestAccountConnection, [2]string{models.ResourceAccounts, models.ActionWrite}, false, ""},
		{"POST", "/api/accounts/{id}/subscribe", app.SubscribeApp, [2]string{models.ResourceAccounts, models.ActionWrite}, false, ""},
		{"GET", "/api/accounts/{id}/business_profile", app.GetBusinessProfile, [2]string{models.ResourceAccounts, models.ActionRead}, false, ""},
		{"PUT", "/api/accounts/{id}/business_profile", app.UpdateBusinessProfile, [2]string{models.ResourceAccounts, models.ActionWrite}, false, ""},
		{"POST", "/api/accounts/{id}/business_profile/photo", app.UpdateProfilePicture, [2]string{models.ResourceAccounts, models.ActionWrite}, false, ""},
		// Contacts
		{"GET", "/api/contacts", app.ListContacts, [2]string{models.ResourceContacts, models.ActionRead}, false, ""},
		{"POST", "/api/contacts", app.CreateContact, [2]string{models.ResourceContacts, models.ActionWrite}, false, ""},
		{"GET", "/api/contacts/{id}", app.GetContact, [2]string{models.ResourceContacts, models.ActionRead}, false, ""},
		{"PUT", "/api/contacts/{id}", app.UpdateContact, [2]string{models.ResourceContacts, models.ActionWrite}, false, ""},
		{"DELETE", "/api/contacts/{id}", app.DeleteContact, [2]string{models.ResourceContacts, models.ActionDelete}, false, ""},
		{"PUT", "/api/contacts/{id}/assign", app.AssignContact, [2]string{models.ResourceContacts, models.ActionWrite}, false, ""},
		{"PUT", "/api/contacts/{id}/tags", app.UpdateContactTags, [2]string{models.ResourceContacts, models.ActionWrite}, false, ""},
		{"GET", "/api/contacts/{id}/session-data", app.GetContactSessionData, [2]string{models.ResourceContacts, models.ActionRead}, false, ""},
		// Generic Import/Export
		{"POST", "/api/export", app.ExportData, [2]string{models.ResourceContacts, models.ActionExport}, false, ""},
		{"POST", "/api/import", app.ImportData, [2]string{models.ResourceContacts, models.ActionImport}, false, ""},
		{"GET", "/api/export/{table}/config", app.GetExportConfig, [2]string{models.ResourceContacts, models.ActionExport}, false, ""},
		{"GET", "/api/import/{table}/config", app.GetImportConfig, [2]string{models.ResourceContacts, models.ActionImport}, false, ""},
		// Tags
		{"GET", "/api/tags", app.ListTags, [2]string{models.ResourceTags, models.ActionRead}, false, ""},
		{"POST", "/api/tags", app.CreateTag, [2]string{models.ResourceTags, models.ActionWrite}, false, ""},
		{"PUT", "/api/tags/{name}", app.UpdateTag, [2]string{models.ResourceTags, models.ActionWrite}, false, ""},
		{"DELETE", "/api/tags/{name}", app.DeleteTag, [2]string{models.ResourceTags, models.ActionDelete}, false, ""},
		// Messages
		{"GET", "/api/contacts/{id}/messages", app.GetMessages, [2]string{models.ResourceContacts, models.ActionRead}, false, ""},
		{"POST", "/api/contacts/{id}/messages", app.SendMessage, [2]string{models.ResourceContacts, models.ActionWrite}, false, ""},
		{"POST", "/api/contacts/{id}/mark-read", app.MarkContactRead, [2]string{models.ResourceContacts, models.ActionWrite}, false, ""},
		{"POST", "/api/contacts/{id}/messages/{message_id}/reaction", app.SendReaction, [2]string{models.ResourceContacts, models.ActionWrite}, false, ""},
		{"POST", "/api/messages", app.SendMessage, [2]string{models.ResourceChat, models.ActionWrite}, false, ""},
		{"POST", "/api/messages/template", app.SendTemplateMessage, [2]string{models.ResourceChat, models.ActionWrite}, false, ""},
		{"POST", "/api/messages/media", app.SendMediaMessage, [2]string{models.ResourceChat, models.ActionWrite}, false, ""},
		{"PUT", "/api/messages/{id}/read", app.MarkMessageRead, [2]string{models.ResourceChat, models.ActionWrite}, false, ""},
		// Conversation Notes
		{"GET", "/api/contacts/{id}/notes", app.ListConversationNotes, [2]string{models.ResourceChat, models.ActionRead}, false, ""},
		{"POST", "/api/contacts/{id}/notes", app.CreateConversationNote, [2]string{models.ResourceChat, models.ActionWrite}, false, ""},
		{"PUT", "/api/contacts/{id}/notes/{note_id}", app.UpdateConversationNote, [2]string{models.ResourceChat, models.ActionWrite}, false, ""},
		{"DELETE", "/api/contacts/{id}/notes/{note_id}", app.DeleteConversationNote, [2]string{models.ResourceChat, models.ActionWrite}, false, ""},
		// Media (serves media files for messages, auth-protected)
		{"GET", "/api/media/{message_id}", app.ServeMedia, [2]string{models.ResourceContacts, models.ActionRead}, false, ""},
		// Templates
		{"GET", "/api/templates", app.ListTemplates, [2]string{models.ResourceTemplates, models.ActionRead}, false, ""},
		{"POST", "/api/templates", app.CreateTemplate, [2]string{models.ResourceTemplates, models.ActionWrite}, false, ""},
		{"GET", "/api/templates/{id}", app.GetTemplate, [2]string{models.ResourceTemplates, models.ActionRead}, false, ""},
		{"PUT", "/api/templates/{id}", app.UpdateTemplate, [2]string{models.ResourceTemplates, models.ActionWrite}, false, ""},
		{"DELETE", "/api/templates/{id}", app.DeleteTemplate, [2]string{models.ResourceTemplates, models.ActionDelete}, false, ""},
		{"POST", "/api/templates/sync", app.SyncTemplates, [2]string{models.ResourceTemplates, models.ActionSync}, false, ""},
		{"POST", "/api/templates/{id}/publish", app.SubmitTemplate, [2]string{models.ResourceTemplates, models.ActionWrite}, false, ""},
		{"POST", "/api/templates/upload-media", app.UploadTemplateMedia, [2]string{models.ResourceTemplates, models.ActionWrite}, false, ""},
		// WhatsApp Flows
		{"GET", "/api/flows", app.ListFlows, [2]string{models.ResourceFlowsWhatsApp, models.ActionRead}, false, ""},
		{"POST", "/api/flows", app.CreateFlow, [2]string{models.ResourceFlowsWhatsApp, models.ActionWrite}, false, ""},
		{"GET", "/api/flows/{id}", app.GetFlow, [2]string{models.ResourceFlowsWhatsApp, models.ActionRead}, false, ""},
		{"PUT", "/api/flows/{id}", app.UpdateFlow, [2]string{models.ResourceFlowsWhatsApp, models.ActionWrite}, false, ""},
		{"DELETE", "/api/flows/{id}", app.DeleteFlow, [2]string{models.ResourceFlowsWhatsApp, models.ActionDelete}, false, ""},
		{"POST", "/api/flows/{id}/save-to-meta", app.SaveFlowToMeta, [2]string{models.ResourceFlowsWhatsApp, models.ActionWrite}, false, ""},
		{"POST", "/api/flows/{id}/publish", app.PublishFlow, [2]string{models.ResourceFlowsWhatsApp, models.ActionWrite}, false, ""},
		{"POST", "/api/flows/{id}/deprecate", app.DeprecateFlow, [2]string{models.ResourceFlowsWhatsApp, models.ActionWrite}, false, ""},
		{"POST", "/api/flows/{id}/duplicate", app.DuplicateFlow, [2]string{models.ResourceFlowsWhatsApp, models.ActionWrite}, false, ""},
		{"POST", "/api/flows/sync", app.SyncFlows, [2]string{models.ResourceFlowsWhatsApp, models.ActionWrite}, false, ""},
		// Bulk Campaigns
		{"GET", "/api/campaigns", app.ListCampaigns, [2]string{models.ResourceCampaigns, models.ActionRead}, false, ""},
		{"POST", "/api/campaigns", app.CreateCampaign, [2]string{models.ResourceCampaigns, models.ActionWrite}, false, ""},
		{"GET", "/api/campaigns/{id}", app.GetCampaign, [2]string{models.ResourceCampaigns, models.ActionRead}, false, ""},
		{"PUT", "/api/campaigns/{id}", app.UpdateCampaign, [2]string{models.ResourceCampaigns, models.ActionWrite}, false, ""},
		{"DELETE", "/api/campaigns/{id}", app.DeleteCampaign, [2]string{models.ResourceCampaigns, models.ActionDelete}, false, ""},
		{"POST", "/api/campaigns/{id}/start", app.StartCampaign, [2]string{models.ResourceCampaigns, models.ActionWrite}, false, ""},
		{"POST", "/api/campaigns/{id}/pause", app.PauseCampaign, [2]string{models.ResourceCampaigns, models.ActionWrite}, false, ""},
		{"POST", "/api/campaigns/{id}/cancel", app.CancelCampaign, [2]string{models.ResourceCampaigns, models.ActionWrite}, false, ""},
		{"POST", "/api/campaigns/{id}/retry-failed", app.RetryFailed, [2]string{models.ResourceCampaigns, models.ActionWrite}, false, ""},
		{"GET", "/api/campaigns/{id}/progress", app.GetCampaign, [2]string{models.ResourceCampaigns, models.ActionRead}, false, ""},
		{"POST", "/api/campaigns/{id}/recipients/import", app.ImportRecipients, [2]string{models.ResourceCampaigns, models.ActionWrite}, false, ""},
		{"GET", "/api/campaigns/{id}/recipients", app.GetCampaignRecipients, [2]string{models.ResourceCampaigns, models.ActionRead}, false, ""},
		{"DELETE", "/api/campaigns/{id}/recipients/{recipientId}", app.DeleteCampaignRecipient, [2]string{models.ResourceCampaigns, models.ActionDelete}, false, ""},
		{"POST", "/api/campaigns/{id}/media", app.UploadCampaignMedia, [2]string{models.ResourceCampaigns, models.ActionWrite}, false, ""},
		{"GET", "/api/campaigns/{id}/media", app.ServeCampaignMedia, [2]string{models.ResourceCampaigns, models.ActionRead}, false, ""},
		// Chatbot Settings
		{"GET", "/api/chatbot/settings", app.GetChatbotSettings, [2]string{models.ResourceSettingsChatbot, models.ActionRead}, false, ""},
		{"PUT", "/api/chatbot/settings", app.UpdateChatbotSettings, [2]string{models.ResourceSettingsChatbot, models.ActionWrite}, false, ""},
		// Keyword Rules
		{"GET", "/api/chatbot/keywords", app.ListKeywordRules, [2]string{models.ResourceChatbotKeywords, models.ActionRead}, false, ""},
		{"POST", "/api/chatbot/keywords", app.CreateKeywordRule, [2]string{models.ResourceChatbotKeywords, models.ActionWrite}, false, ""},
		{"GET", "/api/chatbot/keywords/{id}", app.GetKeywordRule, [2]string{models.ResourceChatbotKeywords, models.ActionRead}, false, ""},
		{"PUT", "/api/chatbot/keywords/{id}", app.UpdateKeywordRule, [2]string{models.ResourceChatbotKeywords, models.ActionWrite}, false, ""},
		{"DELETE", "/api/chatbot/keywords/{id}", app.DeleteKeywordRule, [2]string{models.ResourceChatbotKeywords, models.ActionDelete}, false, ""},
		// Chatbot Flows
		{"GET", "/api/chatbot/flows", app.ListChatbotFlows, [2]string{models.ResourceFlowsChatbot, models.ActionRead}, false, ""},
		{"POST", "/api/chatbot/flows", app.CreateChatbotFlow, [2]string{models.ResourceFlowsChatbot, models.ActionWrite}, false, ""},
		{"GET", "/api/chatbot/flows/{id}", app.GetChatbotFlow, [2]string{models.ResourceFlowsChatbot, models.ActionRead}, false, ""},
		{"PUT", "/api/chatbot/flows/{id}", app.UpdateChatbotFlow, [2]string{models.ResourceFlowsChatbot, models.ActionWrite}, false, ""},
		{"DELETE", "/api/chatbot/flows/{id}", app.DeleteChatbotFlow, [2]string{models.ResourceFlowsChatbot, models.ActionDelete}, false, ""},
		// AI Contexts
		{"GET", "/api/chatbot/ai-contexts", app.ListAIContexts, [2]string{models.ResourceChatbotAI, models.ActionRead}, false, ""},
		{"POST", "/api/chatbot/ai-contexts", app.CreateAIContext, [2]string{models.ResourceChatbotAI, models.ActionWrite}, false, ""},
		{"GET", "/api/chatbot/ai-contexts/{id}", app.GetAIContext, [2]string{models.ResourceChatbotAI, models.ActionRead}, false, ""},
		{"PUT", "/api/chatbot/ai-contexts/{id}", app.UpdateAIContext, [2]string{models.ResourceChatbotAI, models.ActionWrite}, false, ""},
		{"DELETE", "/api/chatbot/ai-contexts/{id}", app.DeleteAIContext, [2]string{models.ResourceChatbotAI, models.ActionDelete}, false, ""},
		// Agent Transfers
		{"GET", "/api/chatbot/transfers", app.ListAgentTransfers, [2]string{models.ResourceTransfers, models.ActionWrite}, false, ""},
		{"POST", "/api/chatbot/transfers", app.CreateAgentTransfer, [2]string{models.ResourceSettingsChatbot, models.ActionWrite}, false, ""},
		{"POST", "/api/chatbot/transfers/pick", app.PickNextTransfer, [2]string{models.ResourceTransfers, models.ActionWrite}, false, ""},
		{"PUT", "/api/chatbot/transfers/{id}/resume", app.ResumeFromTransfer, [2]string{models.ResourceSettingsChatbot, models.ActionWrite}, false, ""},
		{"PUT", "/api/chatbot/transfers/{id}/assign", app.AssignAgentTransfer, [2]string{models.ResourceTransfers, models.ActionWrite}, false, ""},
		// Teams (admin/manager - access control in handler)
		{"GET", "/api/teams", app.ListTeams, [2]string{models.ResourceTeams, models.ActionRead}, false, ""},
		{"POST", "/api/teams", app.CreateTeam, [2]string{models.ResourceTeams, models.ActionWrite}, false, ""},
		{"GET", "/api/teams/{id}", app.GetTeam, [2]string{models.ResourceTeams, models.ActionRead}, false, ""},
		{"PUT", "/api/teams/{id}", app.UpdateTeam, [2]string{models.ResourceTeams, models.ActionWrite}, false, ""},
		{"DELETE", "/api/teams/{id}", app.DeleteTeam, [2]string{models.ResourceTeams, models.ActionDelete}, false, ""},
		{"GET", "/api/teams/{id}/members", app.ListTeamMembers, [2]string{models.ResourceTeams, models.ActionRead}, false, ""},
		{"POST", "/api/teams/{id}/members", app.AddTeamMember, [2]string{models.ResourceTeams, models.ActionWrite}, false, ""},
		{"DELETE", "/api/teams/{id}/members/{member_user_id}", app.RemoveTeamMember, [2]string{models.ResourceTeams, models.ActionWrite}, false, ""},
		// Audit Logs
		{"GET", "/api/audit-logs", app.ListAuditLogs, [2]string{models.ResourceAuditLogs, models.ActionRead}, false, ""},
		{"GET", "/api/audit-logs/{id}", app.GetAuditLog, [2]string{models.ResourceAuditLogs, models.ActionRead}, false, ""},
		// Canned Responses
		{"GET", "/api/canned-responses", app.ListCannedResponses, [2]string{models.ResourceCannedResponses, models.ActionRead}, false, ""},
		{"POST", "/api/canned-responses", app.CreateCannedResponse, [2]string{models.ResourceCannedResponses, models.ActionWrite}, false, ""},
		{"GET", "/api/canned-responses/{id}", app.GetCannedResponse, [2]string{models.ResourceCannedResponses, models.ActionRead}, false, ""},
		{"PUT", "/api/canned-responses/{id}", app.UpdateCannedResponse, [2]string{models.ResourceCannedResponses, models.ActionWrite}, false, ""},
		{"DELETE", "/api/canned-responses/{id}", app.DeleteCannedResponse, [2]string{models.ResourceCannedResponses, models.ActionDelete}, false, ""},
		{"POST", "/api/canned-responses/{id}/use", app.IncrementCannedResponseUsage, [2]string{models.ResourceCannedResponses, models.ActionWrite}, false, ""},
		// Sessions (admin/debug)
		{"GET", "/api/chatbot/sessions", app.ListChatbotSessions, [2]string{models.ResourceSettingsChatbot, models.ActionRead}, false, ""},
		{"GET", "/api/chatbot/sessions/{id}", app.GetChatbotSession, [2]string{models.ResourceSettingsChatbot, models.ActionRead}, false, ""},
		// Analytics
		{"GET", "/api/analytics/dashboard", app.GetDashboardStats, [2]string{models.ResourceAnalytics, models.ActionRead}, false, ""},
		{"GET", "/api/analytics/messages", app.GetMessageAnalytics, [2]string{models.ResourceAnalytics, models.ActionRead}, false, ""},
		{"GET", "/api/analytics/chatbot", app.GetChatbotAnalytics, [2]string{models.ResourceAnalytics, models.ActionRead}, false, ""},
		{"GET", "/api/analytics/agents", app.GetAgentAnalytics, [2]string{models.ResourceAnalyticsAgents, models.ActionRead}, false, ""},
		{"GET", "/api/analytics/agents/{id}", app.GetAgentDetails, [2]string{models.ResourceAnalyticsAgents, models.ActionRead}, false, ""},
		{"GET", "/api/analytics/agents/comparison", app.GetAgentComparison, [2]string{models.ResourceAnalyticsAgents, models.ActionRead}, false, ""},
		// Meta WhatsApp Analytics
		{"GET", "/api/analytics/meta", app.GetMetaAnalytics, [2]string{models.ResourceAnalytics, models.ActionRead}, false, ""},
		{"GET", "/api/analytics/meta/accounts", app.ListMetaAccountsForAnalytics, [2]string{models.ResourceAnalytics, models.ActionRead}, false, ""},
		{"POST", "/api/analytics/meta/refresh", app.RefreshMetaAnalyticsCache, [2]string{models.ResourceAnalytics, models.ActionWrite}, false, ""},
		// Widgets (customizable analytics)
		{"GET", "/api/widgets", app.ListWidgets, [2]string{models.ResourceAnalytics, models.ActionRead}, false, ""},
		{"POST", "/api/widgets", app.CreateWidget, [2]string{models.ResourceAnalytics, models.ActionWrite}, false, ""},
		{"GET", "/api/widgets/data-sources", app.GetWidgetDataSources, [2]string{models.ResourceAnalytics, models.ActionRead}, false, ""},
		{"GET", "/api/widgets/data", app.GetAllWidgetsData, [2]string{models.ResourceAnalytics, models.ActionRead}, false, ""},
		{"GET", "/api/widgets/{id}", app.GetWidget, [2]string{models.ResourceAnalytics, models.ActionRead}, false, ""},
		{"PUT", "/api/widgets/{id}", app.UpdateWidget, [2]string{models.ResourceAnalytics, models.ActionWrite}, false, ""},
		{"DELETE", "/api/widgets/{id}", app.DeleteWidget, [2]string{models.ResourceAnalytics, models.ActionDelete}, false, ""},
		{"GET", "/api/widgets/{id}/data", app.GetWidgetData, [2]string{models.ResourceAnalytics, models.ActionRead}, false, ""},
		{"POST", "/api/widgets/layout", app.SaveWidgetLayout, [2]string{models.ResourceAnalytics, models.ActionWrite}, false, ""},
		// Organization Settings
		{"GET", "/api/org/settings", app.GetOrganizationSettings, [2]string{models.ResourceSettingsGeneral, models.ActionRead}, false, ""},
		{"PUT", "/api/org/settings", app.UpdateOrganizationSettings, [2]string{models.ResourceSettingsGeneral, models.ActionWrite}, false, ""},
		{"POST", "/api/org/audio", app.UploadOrgAudio, [2]string{models.ResourceOrganizations, models.ActionWrite}, false, ""},
		// Organizations
		{"GET", "/api/organizations", app.ListOrganizations, [2]string{models.ResourceOrganizations, models.ActionRead}, false, ""},
		{"POST", "/api/organizations", app.CreateOrganization, [2]string{models.ResourceOrganizations, models.ActionWrite}, false, ""},
		{"GET", "/api/organizations/current", app.GetCurrentOrganization, [2]string{models.ResourceOrganizations, models.ActionRead}, false, ""},
		{"GET", "/api/organizations/members", app.ListOrganizationMembers, [2]string{models.ResourceOrganizations, models.ActionRead}, false, ""},
		{"POST", "/api/organizations/members", app.AddOrganizationMember, [2]string{models.ResourceOrganizations, models.ActionAssign}, false, ""},
		{"PUT", "/api/organizations/members/{member_id}", app.UpdateOrganizationMemberRole, [2]string{models.ResourceOrganizations, models.ActionAssign}, false, ""},
		{"DELETE", "/api/organizations/members/{member_id}", app.RemoveOrganizationMember, [2]string{models.ResourceOrganizations, models.ActionAssign}, false, ""},
		// SSO Settings (admin only - enforced by middleware)
		{"GET", "/api/settings/sso", app.GetSSOSettings, [2]string{models.ResourceSettingsSSO, models.ActionRead}, false, ""},
		{"PUT", "/api/settings/sso/{provider}", app.UpdateSSOProvider, [2]string{models.ResourceSettingsSSO, models.ActionWrite}, false, ""},
		{"DELETE", "/api/settings/sso/{provider}", app.DeleteSSOProvider, [2]string{models.ResourceSettingsSSO, models.ActionWrite}, false, ""},
		// Webhooks
		{"GET", "/api/webhooks", app.ListWebhooks, [2]string{models.ResourceWebhooks, models.ActionRead}, false, ""},
		{"POST", "/api/webhooks", app.CreateWebhook, [2]string{models.ResourceWebhooks, models.ActionWrite}, false, ""},
		{"GET", "/api/webhooks/{id}", app.GetWebhook, [2]string{models.ResourceWebhooks, models.ActionRead}, false, ""},
		{"PUT", "/api/webhooks/{id}", app.UpdateWebhook, [2]string{models.ResourceWebhooks, models.ActionWrite}, false, ""},
		{"DELETE", "/api/webhooks/{id}", app.DeleteWebhook, [2]string{models.ResourceWebhooks, models.ActionDelete}, false, ""},
		{"POST", "/api/webhooks/{id}/test", app.TestWebhook, [2]string{models.ResourceWebhooks, models.ActionWrite}, false, ""},
		// Custom Actions
		{"GET", "/api/custom-actions", app.ListCustomActions, [2]string{models.ResourceCustomActions, models.ActionRead}, false, ""},
		{"POST", "/api/custom-actions", app.CreateCustomAction, [2]string{models.ResourceCustomActions, models.ActionWrite}, false, ""},
		{"GET", "/api/custom-actions/{id}", app.GetCustomAction, [2]string{models.ResourceCustomActions, models.ActionRead}, false, ""},
		{"PUT", "/api/custom-actions/{id}", app.UpdateCustomAction, [2]string{models.ResourceCustomActions, models.ActionWrite}, false, ""},
		{"DELETE", "/api/custom-actions/{id}", app.DeleteCustomAction, [2]string{models.ResourceCustomActions, models.ActionDelete}, false, ""},
		{"POST", "/api/custom-actions/{id}/execute", app.ExecuteCustomAction, [2]string{models.ResourceCustomActions, models.ActionWrite}, false, ""},
		{"GET", "/api/custom-actions/redirect/{token}", app.CustomActionRedirect, [2]string{models.ResourceCustomActions, models.ActionRead}, false, ""},
		// IVR Flows
		{"GET", "/api/ivr-flows", app.ListIVRFlows, [2]string{models.ResourceIVRFlows, models.ActionRead}, false, ""},
		{"GET", "/api/ivr-flows/{id}", app.GetIVRFlow, [2]string{models.ResourceIVRFlows, models.ActionRead}, false, ""},
		{"POST", "/api/ivr-flows", app.CreateIVRFlow, [2]string{models.ResourceIVRFlows, models.ActionWrite}, false, ""},
		{"PUT", "/api/ivr-flows/{id}", app.UpdateIVRFlow, [2]string{models.ResourceIVRFlows, models.ActionWrite}, false, ""},
		{"DELETE", "/api/ivr-flows/{id}", app.DeleteIVRFlow, [2]string{models.ResourceIVRFlows, models.ActionDelete}, false, ""},
		{"POST", "/api/ivr-flows/audio", app.UploadIVRAudio, [2]string{models.ResourceIVRFlows, models.ActionWrite}, false, ""},
		{"GET", "/api/ivr-flows/audio/{filename}", app.ServeIVRAudio, [2]string{models.ResourceIVRFlows, models.ActionRead}, false, ""},
		// Call Logs
		{"GET", "/api/call-logs", app.ListCallLogs, [2]string{models.ResourceCallLogs, models.ActionRead}, false, ""},
		{"GET", "/api/call-logs/{id}", app.GetCallLog, [2]string{models.ResourceCallLogs, models.ActionRead}, false, ""},
		{"GET", "/api/call-logs/{id}/recording", app.GetCallRecording, [2]string{models.ResourceCallLogs, models.ActionRead}, false, ""},
		// Call Transfers
		{"GET", "/api/call-transfers", app.ListCallTransfers, [2]string{models.ResourceCallTransfers, models.ActionRead}, false, ""},
		{"GET", "/api/call-transfers/{id}", app.GetCallTransfer, [2]string{models.ResourceCallTransfers, models.ActionRead}, false, ""},
		{"POST", "/api/call-transfers/{id}/connect", app.ConnectCallTransfer, [2]string{models.ResourceCallTransfers, models.ActionWrite}, false, ""},
		{"POST", "/api/call-transfers/{id}/hangup", app.HangupCallTransfer, [2]string{models.ResourceCallTransfers, models.ActionWrite}, false, ""},
		{"POST", "/api/call-transfers/initiate", app.InitiateAgentTransfer, [2]string{models.ResourceCallTransfers, models.ActionWrite}, false, ""},
		// Call Hold
		{"POST", "/api/call-logs/{id}/hold", app.HoldCall, [2]string{models.ResourceCallTransfers, models.ActionWrite}, false, ""},
		{"POST", "/api/call-logs/{id}/resume", app.ResumeCall, [2]string{models.ResourceCallTransfers, models.ActionWrite}, false, ""},
		// Outgoing Calls
		{"POST", "/api/calls/outgoing", app.InitiateOutgoingCall, [2]string{models.ResourceOutgoingCalls, models.ActionWrite}, false, ""},
		{"POST", "/api/calls/outgoing/{id}/hangup", app.HangupOutgoingCall, [2]string{models.ResourceOutgoingCalls, models.ActionWrite}, false, ""},
		{"POST", "/api/calls/permission-request", app.SendCallPermissionRequest, [2]string{models.ResourceOutgoingCalls, models.ActionWrite}, false, ""},
		{"GET", "/api/calls/permission/{contactId}", app.GetCallPermission, [2]string{models.ResourceOutgoingCalls, models.ActionRead}, false, ""},
		{"GET", "/api/calls/ice-servers", app.GetICEServers, [2]string{models.ResourceOutgoingCalls, models.ActionRead}, false, ""},
		// Catalogs
		{"GET", "/api/catalogs", app.ListCatalogs, [2]string{models.ResourceAccounts, models.ActionRead}, false, ""},
		{"POST", "/api/catalogs", app.CreateCatalog, [2]string{models.ResourceAccounts, models.ActionWrite}, false, ""},
		{"GET", "/api/catalogs/{id}", app.GetCatalog, [2]string{models.ResourceAccounts, models.ActionRead}, false, ""},
		{"DELETE", "/api/catalogs/{id}", app.DeleteCatalog, [2]string{models.ResourceAccounts, models.ActionDelete}, false, ""},
		{"POST", "/api/catalogs/sync", app.SyncCatalogs, [2]string{models.ResourceAccounts, models.ActionWrite}, false, ""},
		// Catalog Products
		{"GET", "/api/catalogs/{id}/products", app.ListCatalogProducts, [2]string{models.ResourceAccounts, models.ActionRead}, false, ""},
		{"POST", "/api/catalogs/{id}/products", app.CreateCatalogProduct, [2]string{models.ResourceAccounts, models.ActionWrite}, false, ""},
		{"GET", "/api/products/{id}", app.GetCatalogProduct, [2]string{models.ResourceAccounts, models.ActionRead}, false, ""},
		{"PUT", "/api/products/{id}", app.UpdateCatalogProduct, [2]string{models.ResourceAccounts, models.ActionWrite}, false, ""},
		{"DELETE", "/api/products/{id}", app.DeleteCatalogProduct, [2]string{models.ResourceAccounts, models.ActionDelete}, false, ""},
	}
}
