package handlers

import (
	"context"
	"errors"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/shridarpatil/whatomate/internal/middleware"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shridarpatil/whatomate/internal/assignment"
	"github.com/shridarpatil/whatomate/internal/calling"
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/queue"
	"github.com/shridarpatil/whatomate/internal/rbac"
	"github.com/shridarpatil/whatomate/internal/storage"
	"github.com/shridarpatil/whatomate/internal/tts"
	"github.com/shridarpatil/whatomate/internal/websocket"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/zerodha/fastglue"
	"github.com/zerodha/logf"
	"gorm.io/gorm"
)

// App holds all dependencies for handlers
type App struct {
	Config            *config.Config
	DB                *gorm.DB
	Redis             *redis.Client
	Log               logf.Logger
	WhatsApp          *whatsapp.Client
	WSHub             *websocket.Hub
	Queue             queue.Queue
	CampaignSubCancel context.CancelFunc
	// HTTPClient is a shared HTTP client with connection pooling for external API calls
	HTTPClient *http.Client
	// Assigner provides shared team-based agent assignment (used by both chat and call transfers)
	Assigner *assignment.Assigner
	// CallManager handles WebRTC call sessions (nil when calling is disabled)
	CallManager *calling.Manager
	// TTS generates audio from text for IVR greetings (nil when not configured)
	TTS tts.Generator
	// S3Client for serving call recording presigned URLs (nil when not configured)
	S3Client *storage.S3Client
	// wg tracks background goroutines for graceful shutdown
	wg sync.WaitGroup

	// ingestSem bounds webhook fan-out; built lazily by ingestSlots
	ingestSemOnce sync.Once
	ingestSem     chan struct{}

	// auditSem bounds audit persistence; built lazily by auditSlots
	auditSemOnce sync.Once
	auditSem     chan struct{}

	// campaignSub is the Redis pub/sub subscriber, retained so shutdown can
	// close its connection
	campaignSub *queue.Subscriber

	// rbac is the permission engine; built lazily by rbacEngine
	rbacOnce sync.Once
	rbac     *rbac.Engine
}

// WaitForBackgroundTasks blocks until all background goroutines complete.
// Call this during graceful shutdown to ensure all async work finishes.
func (a *App) WaitForBackgroundTasks() {
	a.wg.Wait()
}

// backgroundTaskTimeout bounds any single detached background task. Without a
// ceiling a wedged external call would hold up shutdown indefinitely, since
// WaitForBackgroundTasks waits for every spawned goroutine.
const backgroundTaskTimeout = 2 * time.Minute

// spawn runs fn on a tracked background goroutine.
//
// It is the single entry point for fire-and-forget work, and supplies the three
// things the raw `go func()` launches it replaces did not:
//
//   - the goroutine is registered with a.wg, so WaitForBackgroundTasks (and
//     therefore graceful shutdown) actually waits for it;
//   - fn receives a detached context with a timeout, so work started while
//     serving a request outlives that request but not the process;
//   - a panic is recovered and logged instead of killing the process, which is
//     what a panic on any unrecovered goroutine does.
//
// name identifies the task in panic logs.
func (a *App) spawn(name string, fn func(ctx context.Context)) {
	a.wg.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				a.Log.Error("Recovered from panic in background task",
					"task", name, "error", r, "stack", string(debug.Stack()))
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), backgroundTaskTimeout)
		defer cancel()
		fn(ctx)
	})
}

const (
	// maxConcurrentIngestTasks bounds the goroutines a single inbound Meta
	// webhook POST may have in flight. Meta batches, so one POST can carry many
	// messages and statuses; the previous unbounded fan-out let one request
	// launch hundreds.
	maxConcurrentIngestTasks = 64

	// maxConcurrentAuditWrites bounds audit persistence. Every mutation logs an
	// entry, so a burst of writes previously meant a burst of goroutines.
	maxConcurrentAuditWrites = 16
)

// ingestSlots returns the webhook-ingest semaphore, creating it on first use.
// App is built as a struct literal in main and in tests, so these cannot be
// constructor fields.
func (a *App) ingestSlots() chan struct{} {
	a.ingestSemOnce.Do(func() {
		a.ingestSem = make(chan struct{}, maxConcurrentIngestTasks)
	})
	return a.ingestSem
}

// auditSlots returns the audit-write semaphore, creating it on first use.
func (a *App) auditSlots() chan struct{} {
	a.auditSemOnce.Do(func() {
		a.auditSem = make(chan struct{}, maxConcurrentAuditWrites)
	})
	return a.auditSem
}

// spawnBounded is spawn with a concurrency ceiling.
//
// The slot is acquired on the caller's goroutine, so a saturated queue applies
// backpressure to the caller rather than growing without limit. That is
// deliberate: Meta retries a slow webhook, but a process that has spawned ten
// thousand goroutines recovers from nothing.
func (a *App) spawnBounded(name string, sem chan struct{}, fn func(ctx context.Context)) {
	sem <- struct{}{}
	a.spawn(name, func(ctx context.Context) {
		defer func() { <-sem }()
		fn(ctx)
	})
}

// spawnIngest runs webhook fan-out work under the ingest bound.
func (a *App) spawnIngest(name string, fn func(ctx context.Context)) {
	a.spawnBounded(name, a.ingestSlots(), fn)
}

// getOrgID extracts organization ID from request context (set by auth middleware)
// Super admins can override the org by passing X-Organization-ID header
// Super admins MUST select an organization - no "all organizations" view
func (a *App) getOrgID(r *fastglue.Request) (uuid.UUID, error) {
	// Get user's default organization ID from JWT
	var defaultOrgID uuid.UUID
	orgIDVal := r.RequestCtx.UserValue(middleware.ContextKeyOrganizationID)
	if orgIDVal == nil {
		return uuid.Nil, errors.New("organization_id not found in context")
	}
	switch v := orgIDVal.(type) {
	case uuid.UUID:
		defaultOrgID = v
	case string:
		parsed, err := uuid.Parse(v)
		if err != nil {
			return uuid.Nil, errors.New("organization_id is not a valid UUID")
		}
		defaultOrgID = parsed
	default:
		return uuid.Nil, errors.New("organization_id is not a valid UUID")
	}

	// Check for X-Organization-ID header to switch organizations
	userID, _ := r.RequestCtx.UserValue(middleware.ContextKeyUserID).(uuid.UUID)
	overrideOrgID := string(r.RequestCtx.Request.Header.Peek("X-Organization-ID"))
	if overrideOrgID != "" {
		parsedOrgID, err := uuid.Parse(overrideOrgID)
		if err == nil && parsedOrgID != defaultOrgID {
			if a.IsSuperAdmin(userID) {
				// Super admins can access any org
				var count int64
				if err := a.DB.Table("organizations").Where("id = ?", parsedOrgID).Count(&count).Error; err == nil && count > 0 {
					return parsedOrgID, nil
				}
			} else {
				// Non-super-admins can switch if they have membership
				var count int64
				if err := a.DB.Table("user_organizations").
					Where("user_id = ? AND organization_id = ? AND deleted_at IS NULL", userID, parsedOrgID).
					Count(&count).Error; err == nil && count > 0 {
					return parsedOrgID, nil
				}
			}
		}
	}

	return defaultOrgID, nil
}

// HealthCheck returns server health status
func (a *App) HealthCheck(r *fastglue.Request) error {
	return r.SendEnvelope(map[string]string{
		"status":  "ok",
		"service": "whatomate",
	})
}

// ReadyCheck returns server readiness status
func (a *App) ReadyCheck(r *fastglue.Request) error {
	// Check database connection
	sqlDB, err := a.DB.DB()
	if err != nil {
		a.Log.Error("Database connection error", "error", err)
		return r.SendErrorEnvelope(500, "Database connection error", nil, "")
	}
	if err := sqlDB.Ping(); err != nil {
		a.Log.Error("Database ping failed", "error", err)
		return r.SendErrorEnvelope(500, "Database ping failed", nil, "")
	}

	// Check Redis connection
	if err := a.Redis.Ping(r.RequestCtx).Err(); err != nil {
		a.Log.Error("Redis connection error", "error", err)
		return r.SendErrorEnvelope(500, "Redis connection error", nil, "")
	}

	return r.SendEnvelope(map[string]string{
		"status": "ready",
	})
}

// GetEmbeddedSignupConfig returns public configuration values for the embedded signup flow
func (a *App) GetEmbeddedSignupConfig(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return a.sendError(r, unauthorized("Unauthorized"))
	}

	appID, _, configID, err := a.resolveMetaAppCreds(orgID)
	if err != nil {
		return a.sendError(r, internalError("Failed to resolve credentials", err))
	}

	type EmbeddedSignupConfig struct {
		WhatsAppAppID      string `json:"whatsapp_app_id,omitempty"`
		WhatsAppConfigID   string `json:"whatsapp_config_id,omitempty"`
		WhatsAppAPIVersion string `json:"whatsapp_api_version,omitempty"`
	}

	config := EmbeddedSignupConfig{
		WhatsAppAppID:      appID,
		WhatsAppConfigID:   configID,
		WhatsAppAPIVersion: a.Config.WhatsApp.APIVersion,
	}

	return r.SendEnvelope(config)
}

// StartCampaignStatsSubscriber starts listening for campaign stats updates from Redis pub/sub
// and broadcasts them via WebSocket
func (a *App) StartCampaignStatsSubscriber() error {
	if a.WSHub == nil {
		a.Log.Warn("WebSocket hub not initialized, skipping campaign stats subscriber")
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.CampaignSubCancel = cancel

	subscriber := queue.NewSubscriber(a.Redis, a.Log)
	// Held on App so shutdown can close it. Previously this local went out of
	// scope and its Redis pub/sub connection was never released.
	a.campaignSub = subscriber

	err := subscriber.SubscribeCampaignStats(ctx, func(update *queue.CampaignStatsUpdate) {
		a.Log.Debug("Received campaign stats update from Redis",
			"campaign_id", update.CampaignID,
			"status", update.Status,
			"sent", update.SentCount,
		)

		// Broadcast to organization via WebSocket
		a.WSHub.BroadcastToOrg(update.OrganizationID, websocket.WSMessage{
			Type: websocket.TypeCampaignStatsUpdate,
			Payload: map[string]any{
				"campaign_id":     update.CampaignID,
				"status":          update.Status,
				"sent_count":      update.SentCount,
				"delivered_count": update.DeliveredCount,
				"read_count":      update.ReadCount,
				"failed_count":    update.FailedCount,
			},
		})
	})

	if err != nil {
		cancel()
		return err
	}

	a.Log.Info("Campaign stats subscriber started")
	return nil
}

// StopCampaignStatsSubscriber cancels the subscriber's context and closes its
// Redis pub/sub connection.
func (a *App) StopCampaignStatsSubscriber() {
	if a.CampaignSubCancel != nil {
		a.CampaignSubCancel()
	}
	if a.campaignSub != nil {
		if err := a.campaignSub.Close(); err != nil {
			a.Log.Error("Failed to close campaign stats subscriber", "error", err)
		}
		a.campaignSub = nil
	}
}

// getOrgAndUserID extracts both organization ID and user ID from the request context.
// Returns an error if either is missing or invalid.
func (a *App) getOrgAndUserID(r *fastglue.Request) (orgID, userID uuid.UUID, err error) {
	orgID, err = a.getOrgID(r)
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}

	userIDVal := r.RequestCtx.UserValue(middleware.ContextKeyUserID)
	if userIDVal == nil {
		return uuid.Nil, uuid.Nil, errors.New("user_id not found in context")
	}
	switch v := userIDVal.(type) {
	case uuid.UUID:
		userID = v
	case string:
		userID, err = uuid.Parse(v)
		if err != nil {
			return uuid.Nil, uuid.Nil, errors.New("user_id is not a valid UUID")
		}
	default:
		return uuid.Nil, uuid.Nil, errors.New("user_id is not a valid UUID")
	}

	return orgID, userID, nil
}

// requirePermission checks if the user has the required permission.
// Returns nil if permitted, otherwise sends a 403 error envelope and returns errEnvelopeSent.
// Automatically extracts orgID from the request for org-aware permission checks.
func (a *App) requirePermission(r *fastglue.Request, userID uuid.UUID, resource, action string) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		a.Log.Error("Failed to get organization ID for permission check", "error", err, "user_id", userID)
		_ = a.sendError(r, forbidden("Insufficient permissions"))
		return errEnvelopeSent
	}
	if !a.HasPermission(userID, resource, action, orgID) {
		_ = a.sendError(r, forbidden("Insufficient permissions"))
		return errEnvelopeSent
	}
	return nil
}

// requireAuth extracts the organization ID and user ID from the request and
// verifies the user holds the given permission. On failure it writes the
// appropriate error envelope (401 if unauthenticated, 403 if the permission is
// missing) and returns errEnvelopeSent, so callers should `return nil` early.
func (a *App) requireAuth(r *fastglue.Request, resource, action string) (orgID, userID uuid.UUID, err error) {
	orgID, userID, err = a.getOrgAndUserID(r)
	if err != nil {
		_ = a.sendError(r, unauthorized("Unauthorized"))
		return uuid.Nil, uuid.Nil, errEnvelopeSent
	}
	if !a.HasPermission(userID, resource, action, orgID) {
		_ = a.sendError(r, forbidden("Insufficient permissions"))
		return uuid.Nil, uuid.Nil, errEnvelopeSent
	}
	return orgID, userID, nil
}

// decodeRequest decodes a JSON request body into the provided struct.
// Returns nil on success, otherwise sends a 400 error envelope and returns errEnvelopeSent.
func (a *App) decodeRequest(r *fastglue.Request, v any) error {
	if err := r.Decode(v, "json"); err != nil {
		_ = a.sendError(r, invalidRequest("Invalid request body"))
		return errEnvelopeSent
	}
	return nil
}
