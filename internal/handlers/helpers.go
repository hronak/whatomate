package handlers

import (
	"context"
	"errors"
	"maps"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"

	"github.com/shridarpatil/whatomate/internal/audit"
	"github.com/shridarpatil/whatomate/internal/models"
)

// errEnvelopeSent is a sentinel returned by helpers after they have already
// written an error envelope to the response. Callers should return nil to the framework.
var errEnvelopeSent = errors.New("error envelope sent")

// parsePathUUID extracts a UUID from a path parameter. On failure, it sends a
// 400 error envelope and returns uuid.Nil plus an error.
func parsePathUUID(r *fastglue.Request, param, label string) (uuid.UUID, error) {
	idStr, _ := r.RequestCtx.UserValue(param).(string)
	id, err := uuid.Parse(idStr)
	if err != nil {
		// Free function: no *App in scope, so this writes the envelope
		// directly rather than going through sendError.
		_ = r.SendErrorEnvelope(fasthttp.StatusBadRequest, "Invalid "+label+" ID", nil, "")
		return uuid.Nil, errEnvelopeSent
	}
	return id, nil
}

// Pagination holds parsed pagination parameters.
type Pagination struct {
	Page   int
	Limit  int
	Offset int
}

// Apply adds Offset and Limit to a GORM query.
func (pg Pagination) Apply(query *gorm.DB) *gorm.DB {
	return query.Offset(pg.Offset).Limit(pg.Limit)
}

// parsePagination extracts page-based pagination from query params with
// default limit=50 and max limit=100.
func parsePagination(r *fastglue.Request) Pagination {
	return parsePaginationWithDefaults(r, 50, 100)
}

// parsePaginationWithDefaults extracts page-based pagination with custom defaults.
func parsePaginationWithDefaults(r *fastglue.Request, defaultLimit, maxLimit int) Pagination {
	page, _ := strconv.Atoi(string(r.RequestCtx.QueryArgs().Peek("page")))
	limit, _ := strconv.Atoi(string(r.RequestCtx.QueryArgs().Peek("limit")))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > maxLimit {
		limit = defaultLimit
	}
	return Pagination{
		Page:   page,
		Limit:  limit,
		Offset: (page - 1) * limit,
	}
}

// parseDateParam parses a YYYY-MM-DD date from the named query parameter.
// Returns the parsed time and true on success, or zero time and false if the
// parameter is missing or malformed.
func parseDateParam(r *fastglue.Request, param string) (time.Time, bool) {
	s := string(r.RequestCtx.QueryArgs().Peek(param))
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// endOfDay returns the last nanosecond of the given day.
func endOfDay(t time.Time) time.Time {
	return t.Add(24*time.Hour - time.Nanosecond)
}

// findByIDAndOrg fetches a single record scoped by ID and organization.
// It writes the error envelope on failure and returns errEnvelopeSent.
//
// A missing row is a 404; anything else is a 500. Collapsing the two — which
// this did at all 60-odd call sites — made a Postgres outage indistinguishable
// from a bad ID, so every client saw "not found" and nothing was logged.
func findByIDAndOrg[T any](a *App, r *fastglue.Request, id, orgID uuid.UUID, label string) (*T, error) {
	return findByIDAndOrgWith[T](a, r, a.DB, id, orgID, label)
}

// requestDBTimeout bounds a single database call made while serving a request.
const requestDBTimeout = 30 * time.Second

// dbContext returns the context to attach to database work done on behalf of a
// request, along with its cancel func.
//
// Deliberately not r.RequestCtx. fasthttp's RequestCtx implements
// context.Context in name only: Done() dereferences its server pointer, which
// is nil for any RequestCtx a running server did not create — so it panics in
// every unit test — and even when set it is closed only on server shutdown,
// never on client disconnect. A plain timeout context is honest about what it
// actually provides, and gives queries a ceiling they previously had none of.
func dbContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), requestDBTimeout)
}

// findByIDAndOrgWith is findByIDAndOrg over a pre-scoped query, for callers
// that need Preloads.
func findByIDAndOrgWith[T any](a *App, r *fastglue.Request, query *gorm.DB, id, orgID uuid.UUID, label string) (*T, error) {
	ctx, cancel := dbContext()
	defer cancel()

	var model T
	err := query.WithContext(ctx).
		Where("id = ? AND organization_id = ?", id, orgID).
		First(&model).Error
	switch {
	case err == nil:
		return &model, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, a.failRequest(r, notFound(label))
	default:
		return nil, a.failRequest(r, internalError("Failed to load "+label, err))
	}
}

// logAudit records an audit-log entry for a resource mutation, resolving the
// actor's display name automatically. It wraps audit.LogAudit to remove the
// repeated a.DB + GetUserName boilerplate at call sites.
func (a *App) logAudit(orgID, userID uuid.UUID, resourceType string, resourceID uuid.UUID, action models.AuditAction, oldData, newData any, extraChanges ...map[string]any) {
	a.logAuditAs(orgID, userID, "", resourceType, resourceID, action, oldData, newData, extraChanges...)
}

// logAuditAs is logAudit for callers that already know the actor's display
// name and would otherwise pay for a redundant lookup. An empty userName is
// resolved in the background.
//
// The write runs on a bounded, WaitGroup-tracked goroutine, so it neither
// blocks the response nor escapes graceful shutdown: WaitForBackgroundTasks
// flushes pending audit entries.
func (a *App) logAuditAs(orgID, userID uuid.UUID, userName string, resourceType string, resourceID uuid.UUID, action models.AuditAction, oldData, newData any, extraChanges ...map[string]any) {
	a.spawnBounded("audit_log", a.auditSlots(), func(context.Context) {
		if userName == "" {
			userName = audit.GetUserName(a.DB, userID)
		}
		audit.LogAudit(a.DB, orgID, userID, userName, resourceType, resourceID, action, oldData, newData, extraChanges...)
	})
}

// logWrite reports a failed database write at a site that has no error path to
// return to — webhook processing, async updates, best-effort bookkeeping.
//
// These writes previously discarded their result entirely: a failed contact
// assignment, call-log transition or campaign counter left the row stale with
// nothing recorded anywhere. Passing the *gorm.DB through here keeps the call
// a single expression while making the failure visible.
//
// op names the operation; kv are extra log fields.
func (a *App) logWrite(op string, tx *gorm.DB, kv ...any) {
	if tx.Error != nil {
		a.Log.Error("Database write failed", append([]any{"op", op, "error", tx.Error}, kv...)...)
	}
}

// listEnvelope builds the standard paginated list response payload used across
// list handlers: {<key>: items, total, page, limit}.
func listEnvelope(key string, items, total any, pg Pagination) map[string]any {
	return map[string]any{
		key:     items,
		"total": total,
		"page":  pg.Page,
		"limit": pg.Limit,
	}
}

// listEnvelopeWith is listEnvelope plus extra top-level fields, for the list
// endpoints that return something alongside the page (has_more, online_count).
func listEnvelopeWith(key string, items, total any, pg Pagination, extra map[string]any) map[string]any {
	env := listEnvelope(key, items, total, pg)
	maps.Copy(env, extra)
	return env
}

// parseDateRange parses start and end date strings in YYYY-MM-DD format.
// Applies end-of-day to the end date. Returns an error message suitable for
// display if parsing fails.
func parseDateRange(startStr, endStr string) (start, end time.Time, errMsg string) {
	var err error
	start, err = time.Parse("2006-01-02", startStr)
	if err != nil {
		return time.Time{}, time.Time{}, "Invalid start date format. Use YYYY-MM-DD"
	}
	end, err = time.Parse("2006-01-02", endStr)
	if err != nil {
		return time.Time{}, time.Time{}, "Invalid end date format. Use YYYY-MM-DD"
	}
	end = endOfDay(end)
	return start, end, ""
}
