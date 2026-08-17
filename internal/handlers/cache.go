package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"gorm.io/gorm"
)

const (
	// Cache TTLs - 6 hours since these rarely change (invalidated on update anyway)
	settingsCacheTTL        = 6 * time.Hour
	flowsCacheTTL           = 6 * time.Hour
	keywordRulesCacheTTL    = 6 * time.Hour
	whatsappAccountCacheTTL = 6 * time.Hour
	webhooksCacheTTL        = 6 * time.Hour
	slaSettingsCacheTTL     = 6 * time.Hour
	aiContextsCacheTTL      = 6 * time.Hour
	userPermissionsCacheTTL = 6 * time.Hour
	rolePermissionsCacheTTL = 6 * time.Hour
	tagsCacheTTL            = 6 * time.Hour

	// Cache key prefixes
	settingsCachePrefix        = "chatbot:settings:"
	flowsCachePrefix           = "chatbot:flows:"
	keywordRulesCachePrefix    = "chatbot:keywords:"
	whatsappAccountCachePrefix = "whatsapp:account:"
	webhooksCachePrefix        = "webhooks:"
	slaSettingsCacheKey        = "chatbot:sla_enabled_settings"
	aiContextsCachePrefix      = "chatbot:ai_contexts:"
	userPermissionsCachePrefix = "permissions:user:"
	rolePermissionsCachePrefix = "permissions:role:"
	tagsCachePrefix            = "tags:"
)

// chatbotSettingsCache is used for caching since AI.APIKey has json:"-" tag
type chatbotSettingsCache struct {
	models.ChatbotSettings
	AIAPIKey string `json:"ai_api_key_cache"`
}

// accountCacheKeyPart renders a nullable WhatsApp account reference for use in a
// cache key. A nil reference is the organization-wide default, keyed as "".
func accountCacheKeyPart(accountID *uuid.UUID) string {
	if accountID == nil {
		return ""
	}
	return accountID.String()
}

// getChatbotSettingsCached retrieves chatbot settings from cache or database.
//
// A nil accountID asks for the organization-wide default settings.
func (a *App) getChatbotSettingsCached(orgID uuid.UUID, accountID *uuid.UUID) (*models.ChatbotSettings, error) {
	ctx, cancel := cacheContext()
	defer cancel()
	cacheKey := fmt.Sprintf("%s%s:%s", settingsCachePrefix, orgID.String(), accountCacheKeyPart(accountID))

	// Try cache first
	cached, err := a.Redis.Get(ctx, cacheKey).Result()
	if err == nil && cached != "" {
		var cacheData chatbotSettingsCache
		if err := json.Unmarshal([]byte(cached), &cacheData); err == nil {
			// Restore the API key from the cache wrapper
			cacheData.AI.APIKey = cacheData.AIAPIKey
			return &cacheData.ChatbotSettings, nil
		}
	}

	// Cache miss - fetch from database
	var settings models.ChatbotSettings
	result := a.DB.Where("organization_id = ? AND (whatsapp_account_id = ? OR whatsapp_account_id IS NULL)",
		orgID, accountID).
		Order("CASE WHEN whatsapp_account_id IS NULL THEN 1 ELSE 0 END"). // Prefer account-specific settings
		First(&settings)

	if result.Error != nil {
		return nil, result.Error
	}

	// Cache the result (include AI APIKey explicitly since it has json:"-" tag)
	cacheData := chatbotSettingsCache{
		ChatbotSettings: settings,
		AIAPIKey:        settings.AI.APIKey,
	}
	if data, err := json.Marshal(cacheData); err == nil {
		a.Redis.Set(ctx, cacheKey, data, settingsCacheTTL)
	}

	return &settings, nil
}

// getChatbotFlowsCached retrieves all enabled flows with steps from cache or database
func (a *App) getChatbotFlowsCached(orgID uuid.UUID) ([]models.ChatbotFlow, error) {
	ctx, cancel := cacheContext()
	defer cancel()
	cacheKey := flowsCachePrefix + orgID.String()

	// Try cache first
	cached, err := a.Redis.Get(ctx, cacheKey).Result()
	if err == nil && cached != "" {
		var flows []models.ChatbotFlow
		if err := json.Unmarshal([]byte(cached), &flows); err == nil {
			return flows, nil
		}
	}

	// Cache miss - fetch from database
	var flows []models.ChatbotFlow
	if err := a.DB.Where("organization_id = ? AND is_enabled = true", orgID).
		Find(&flows).Error; err != nil {
		return nil, err
	}

	// Cache the result
	if data, err := json.Marshal(flows); err == nil {
		a.Redis.Set(ctx, cacheKey, data, flowsCacheTTL)
	}

	return flows, nil
}

// getChatbotFlowByIDCached retrieves a specific flow by ID from the cached flows list
func (a *App) getChatbotFlowByIDCached(orgID uuid.UUID, flowID uuid.UUID) (*models.ChatbotFlow, error) {
	flows, err := a.getChatbotFlowsCached(orgID)
	if err != nil {
		return nil, err
	}

	for i := range flows {
		if flows[i].ID == flowID {
			return &flows[i], nil
		}
	}

	return nil, gorm.ErrRecordNotFound
}

// getKeywordRulesCached retrieves keyword rules from cache or database.
//
// A nil accountID yields the organization's global rules only.
func (a *App) getKeywordRulesCached(orgID uuid.UUID, accountID *uuid.UUID) ([]models.KeywordRule, error) {
	ctx, cancel := cacheContext()
	defer cancel()
	cacheKey := fmt.Sprintf("%s%s:%s", keywordRulesCachePrefix, orgID.String(), accountCacheKeyPart(accountID))

	// Try cache first
	cached, err := a.Redis.Get(ctx, cacheKey).Result()
	if err == nil && cached != "" {
		var rules []models.KeywordRule
		if err := json.Unmarshal([]byte(cached), &rules); err == nil {
			return rules, nil
		}
	}

	// Cache miss - fetch from database (account-specific + global)
	var rules []models.KeywordRule

	// Get account-specific rules
	var accountRules []models.KeywordRule
	if accountID != nil {
		if err := a.DB.Where("organization_id = ? AND whatsapp_account_id = ? AND is_enabled = true",
			orgID, accountID).
			Order("priority DESC").
			Find(&accountRules).Error; err != nil {
			a.Log.Error("Failed to fetch account keyword rules", "error", err, "org_id", orgID)
		}
	}

	// Get global rules (whatsapp_account_id IS NULL)
	var globalRules []models.KeywordRule
	if err := a.DB.Where("organization_id = ? AND whatsapp_account_id IS NULL AND is_enabled = true",
		orgID).
		Order("priority DESC").
		Find(&globalRules).Error; err != nil {
		a.Log.Error("Failed to fetch global keyword rules", "error", err, "org_id", orgID)
	}

	// Merge: account-specific first, then global
	rules = append(accountRules, globalRules...)

	// Cache the result
	if data, err := json.Marshal(rules); err == nil {
		a.Redis.Set(ctx, cacheKey, data, keywordRulesCacheTTL)
	}

	return rules, nil
}

// InvalidateChatbotSettingsCache invalidates the settings cache for an organization
func (a *App) InvalidateChatbotSettingsCache(orgID uuid.UUID) {
	ctx, cancel := cacheContext()
	defer cancel()
	pattern := fmt.Sprintf("%s%s:*", settingsCachePrefix, orgID.String())
	a.deleteKeysByPattern(ctx, pattern)
}

// cacheTimeout bounds a single Redis cache operation. The cache is an
// optimisation: if Redis is slow the request should fall through to Postgres,
// not wait on it. These calls previously used context.Background() and so had
// no ceiling at all.
const cacheTimeout = 3 * time.Second

// cacheContext returns the context for one cache operation, plus its cancel.
//
// Not derived from a request context: these getters are called from inbound
// webhook processing and background tasks as often as from handlers, and
// Phase 5 threads a real caller context through per domain. Bounding them is
// the part that matters now.
func cacheContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), cacheTimeout)
}

// invalidate deletes a cache key and reports failure.
//
// A dropped invalidation is not benign here: these entries carry permissions,
// WhatsApp account credentials and routing config with a 6-hour TTL, so a
// failed Del serves stale authorization or misroutes inbound webhooks for the
// rest of that window. Logging is the minimum; it makes the cause visible when
// the symptom shows up hours later.
func (a *App) invalidate(ctx context.Context, what string, keys ...string) {
	if len(keys) == 0 {
		return
	}
	if err := a.Redis.Del(ctx, keys...).Err(); err != nil {
		a.Log.Error("Cache invalidation failed — stale data will be served until TTL",
			"cache", what, "keys", keys, "error", err)
	}
}

// InvalidateChatbotFlowsCache invalidates the flows cache for an organization
func (a *App) InvalidateChatbotFlowsCache(orgID uuid.UUID) {
	ctx, cancel := cacheContext()
	defer cancel()
	a.invalidate(ctx, "chatbot_flows", flowsCachePrefix+orgID.String())
}

// InvalidateKeywordRulesCache invalidates the keyword rules cache for an organization
func (a *App) InvalidateKeywordRulesCache(orgID uuid.UUID) {
	ctx, cancel := cacheContext()
	defer cancel()
	pattern := fmt.Sprintf("%s%s:*", keywordRulesCachePrefix, orgID.String())
	a.deleteKeysByPattern(ctx, pattern)
}

// deleteKeysByPattern deletes all keys matching a pattern
func (a *App) deleteKeysByPattern(ctx context.Context, pattern string) {
	iter := a.Redis.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		a.invalidate(ctx, "pattern:"+pattern, iter.Val())
	}
	if err := iter.Err(); err != nil {
		a.Log.Error("Cache invalidation scan failed — stale data will be served until TTL",
			"pattern", pattern, "error", err)
	}
}

// whatsAppAccountCache is used for caching since AccessToken, AppSecret, and Pin have json:"-" tag
type whatsAppAccountCache struct {
	models.WhatsAppAccount
	AccessToken string `json:"access_token"`
	AppSecret   string `json:"app_secret"`
	Pin         string `json:"pin"`
}

// getWhatsAppAccountCached retrieves WhatsApp account by phone_id from cache or database
func (a *App) getWhatsAppAccountCached(phoneID string) (*models.WhatsAppAccount, error) {
	ctx, cancel := cacheContext()
	defer cancel()
	cacheKey := whatsappAccountCachePrefix + phoneID

	// Try cache first
	cached, err := a.Redis.Get(ctx, cacheKey).Result()
	if err == nil && cached != "" {
		var cacheData whatsAppAccountCache
		if err := json.Unmarshal([]byte(cached), &cacheData); err == nil {
			cacheData.WhatsAppAccount.AccessToken = cacheData.AccessToken
			cacheData.WhatsAppAccount.AppSecret = cacheData.AppSecret
			cacheData.WhatsAppAccount.Pin = cacheData.Pin
			a.decryptAccountSecrets(&cacheData.WhatsAppAccount)
			return &cacheData.WhatsAppAccount, nil
		}
	}

	// Cache miss - fetch from database
	var account models.WhatsAppAccount
	if err := a.DB.Where("phone_id = ?", phoneID).First(&account).Error; err != nil {
		return nil, err
	}

	// Cache the result (include AccessToken, AppSecret, and Pin explicitly since they have json:"-")
	cacheData := whatsAppAccountCache{
		WhatsAppAccount: account,
		AccessToken:     account.AccessToken,
		AppSecret:       account.AppSecret,
		Pin:             account.Pin,
	}
	if data, err := json.Marshal(cacheData); err == nil {
		a.Redis.Set(ctx, cacheKey, data, whatsappAccountCacheTTL)
	}

	// Decrypt secrets before returning
	a.decryptAccountSecrets(&account)
	return &account, nil
}

// decryptAccountSecrets decrypts the encrypted secrets on a WhatsApp account.
// Handles both encrypted ("enc:" prefixed) and legacy unencrypted values transparently.
func (a *App) decryptAccountSecrets(account *models.WhatsAppAccount) {
	account.DecryptSecrets(a.Config.App.EncryptionKey)
}

// InvalidateWhatsAppAccountCache invalidates the WhatsApp account cache
func (a *App) InvalidateWhatsAppAccountCache(phoneID string) {
	ctx, cancel := cacheContext()
	defer cancel()
	a.invalidate(ctx, "whatsapp_account", whatsappAccountCachePrefix+phoneID)
}

// getWebhooksCached retrieves active webhooks for an organization from cache or database
func (a *App) getWebhooksCached(orgID uuid.UUID) ([]models.Webhook, error) {
	ctx, cancel := cacheContext()
	defer cancel()
	cacheKey := webhooksCachePrefix + orgID.String()

	// Try cache first
	cached, err := a.Redis.Get(ctx, cacheKey).Result()
	if err == nil && cached != "" {
		var webhooks []models.Webhook
		if err := json.Unmarshal([]byte(cached), &webhooks); err == nil {
			return webhooks, nil
		}
	}

	// Cache miss - fetch from database
	var webhooks []models.Webhook
	if err := a.DB.Where("organization_id = ? AND is_active = ?", orgID, true).Find(&webhooks).Error; err != nil {
		return nil, err
	}

	// Cache the result
	if data, err := json.Marshal(webhooks); err == nil {
		a.Redis.Set(ctx, cacheKey, data, webhooksCacheTTL)
	}

	return webhooks, nil
}

// InvalidateWebhooksCache invalidates the webhooks cache for an organization
func (a *App) InvalidateWebhooksCache(orgID uuid.UUID) {
	ctx, cancel := cacheContext()
	defer cancel()
	a.invalidate(ctx, "webhooks", webhooksCachePrefix+orgID.String())
}

// getSLAEnabledSettingsCached retrieves all SLA-enabled chatbot settings from cache or database
func (a *App) getSLAEnabledSettingsCached() ([]models.ChatbotSettings, error) {
	ctx, cancel := cacheContext()
	defer cancel()

	// Try cache first
	cached, err := a.Redis.Get(ctx, slaSettingsCacheKey).Result()
	if err == nil && cached != "" {
		var settings []models.ChatbotSettings
		if err := json.Unmarshal([]byte(cached), &settings); err == nil {
			return settings, nil
		}
	}

	// Cache miss - fetch from database
	var settings []models.ChatbotSettings
	if err := a.DB.Where("sla_enabled = ?", true).Find(&settings).Error; err != nil {
		return nil, err
	}

	// Cache the result
	if data, err := json.Marshal(settings); err == nil {
		a.Redis.Set(ctx, slaSettingsCacheKey, data, slaSettingsCacheTTL)
	}

	return settings, nil
}

// InvalidateSLASettingsCache invalidates the SLA settings cache
func (a *App) InvalidateSLASettingsCache() {
	ctx, cancel := cacheContext()
	defer cancel()
	a.invalidate(ctx, "sla_settings", slaSettingsCacheKey)
}

// getAIContextsCached retrieves AI contexts from cache or database.
//
// A nil accountID yields the organization's global contexts only.
func (a *App) getAIContextsCached(orgID uuid.UUID, accountID *uuid.UUID) ([]models.AIContext, error) {
	ctx, cancel := cacheContext()
	defer cancel()
	cacheKey := fmt.Sprintf("%s%s:%s", aiContextsCachePrefix, orgID.String(), accountCacheKeyPart(accountID))

	// Try cache first
	cached, err := a.Redis.Get(ctx, cacheKey).Result()
	if err == nil && cached != "" {
		var contexts []models.AIContext
		if err := json.Unmarshal([]byte(cached), &contexts); err == nil {
			return contexts, nil
		}
	}

	// Cache miss - fetch from database (account-specific + global)
	var contexts []models.AIContext

	// Get account-specific contexts
	var accountContexts []models.AIContext
	if accountID != nil {
		if err := a.DB.Where("organization_id = ? AND whatsapp_account_id = ? AND is_enabled = true",
			orgID, accountID).
			Order("priority DESC").
			Find(&accountContexts).Error; err != nil {
			a.Log.Error("Failed to fetch account AI contexts", "error", err, "org_id", orgID)
		}
	}

	// Get global contexts (whatsapp_account_id IS NULL)
	var globalContexts []models.AIContext
	if err := a.DB.Where("organization_id = ? AND whatsapp_account_id IS NULL AND is_enabled = true",
		orgID).
		Order("priority DESC").
		Find(&globalContexts).Error; err != nil {
		a.Log.Error("Failed to fetch global AI contexts", "error", err, "org_id", orgID)
	}

	// Merge: account-specific first, then global
	contexts = append(accountContexts, globalContexts...)

	// Cache the result
	if data, err := json.Marshal(contexts); err == nil {
		a.Redis.Set(ctx, cacheKey, data, aiContextsCacheTTL)
	}

	return contexts, nil
}

// InvalidateAIContextsCache invalidates the AI contexts cache for an organization
func (a *App) InvalidateAIContextsCache(orgID uuid.UUID) {
	ctx, cancel := cacheContext()
	defer cancel()
	pattern := fmt.Sprintf("%s%s:*", aiContextsCachePrefix, orgID.String())
	a.deleteKeysByPattern(ctx, pattern)
}

// getTagsCached retrieves tags for an organization from cache or database
func (a *App) getTagsCached(orgID uuid.UUID) ([]models.Tag, error) {
	ctx, cancel := cacheContext()
	defer cancel()
	cacheKey := tagsCachePrefix + orgID.String()

	// Try cache first
	cached, err := a.Redis.Get(ctx, cacheKey).Result()
	if err == nil && cached != "" {
		var tags []models.Tag
		if err := json.Unmarshal([]byte(cached), &tags); err == nil {
			return tags, nil
		}
	}

	// Cache miss - fetch from database
	var tags []models.Tag
	if err := a.DB.Where("organization_id = ?", orgID).Order("name ASC").Find(&tags).Error; err != nil {
		return nil, err
	}

	// Cache the result
	if data, err := json.Marshal(tags); err == nil {
		a.Redis.Set(ctx, cacheKey, data, tagsCacheTTL)
	}

	return tags, nil
}

// InvalidateTagsCache invalidates the tags cache for an organization
func (a *App) InvalidateTagsCache(orgID uuid.UUID) {
	ctx, cancel := cacheContext()
	defer cancel()
	a.invalidate(ctx, "tags", tagsCachePrefix+orgID.String())
}
