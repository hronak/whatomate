package main

import (
	"strings"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
	"github.com/zerodha/logf"
)

// rateLimiter wraps a handler with a rate-limit bucket.
type rateLimiter func(fastglue.FastRequestHandler) fastglue.FastRequestHandler

// registerRoutes installs every route in the table, wrapping each in the
// permission check and rate limit its entry declares.
//
// limiters maps a bucket name to its middleware; a bucket with no entry (rate
// limiting disabled) is simply not applied.
func registerRoutes(g *fastglue.Fastglue, app *handlers.App, lo logf.Logger, enforce bool, limiters map[string]rateLimiter) {
	for _, rt := range appRoutes(app) {
		handler := rt.handler
		if !rt.public && rt.permission != authenticatedOnly {
			handler = requirePermission(app, lo, rt, enforce)
		}
		if rt.rateLimit != "" {
			if limit, ok := limiters[rt.rateLimit]; ok {
				handler = limit(handler)
			}
		}

		switch rt.method {
		case fasthttp.MethodGet:
			g.GET(rt.path, handler)
		case fasthttp.MethodPost:
			g.POST(rt.path, handler)
		case fasthttp.MethodPut:
			g.PUT(rt.path, handler)
		case fasthttp.MethodDelete:
			g.DELETE(rt.path, handler)
		default:
			lo.Fatal("Unsupported method in route table", "method", rt.method, "path", rt.path)
		}
	}
}

// requirePermission wraps a handler with the route's permission check.
//
// When enforce is false the check still runs and still logs, but the request
// proceeds — see AppConfig.EnforceRoutePermissions for why that is the default.
// Handlers that already perform their own requireAuth keep doing so; this is an
// additional gate, not a replacement, so a route whose table entry is wrong
// cannot accidentally *widen* access.
func requirePermission(app *handlers.App, lo logf.Logger, rt route, enforce bool) fastglue.FastRequestHandler {
	resource, action := rt.permission[0], rt.permission[1]

	return func(r *fastglue.Request) error {
		orgID, userID, ok := requestIdentity(r)
		if !ok {
			// Unauthenticated on a non-public route: the auth middleware
			// should already have rejected this, so treat it as a denial.
			return r.SendErrorEnvelope(fasthttp.StatusUnauthorized, "Unauthorized", nil, "")
		}

		if app.HasPermission(userID, resource, action, orgID) {
			return rt.handler(r)
		}

		if !enforce {
			lo.Warn("Route permission would be denied (shadow mode)",
				"method", rt.method,
				"path", rt.path,
				"permission", resource+":"+action,
				"user_id", userID,
				"org_id", orgID,
			)
			return rt.handler(r)
		}

		lo.Info("Route permission denied",
			"method", rt.method,
			"path", rt.path,
			"permission", resource+":"+action,
			"user_id", userID,
			"org_id", orgID,
		)
		return r.SendErrorEnvelope(fasthttp.StatusForbidden, "Insufficient permissions", nil, "")
	}
}

// requestIdentity reads the organization and user the auth middleware stored on
// the request.
func requestIdentity(r *fastglue.Request) (orgID, userID uuid.UUID, ok bool) {
	orgID, ok1 := ctxUUID(r, "organization_id")
	userID, ok2 := ctxUUID(r, "user_id")
	return orgID, userID, ok1 && ok2
}

// ctxUUID reads a UUID the auth middleware stored under key, tolerating both
// the uuid.UUID and string forms it may have used.
func ctxUUID(r *fastglue.Request, key string) (uuid.UUID, bool) {
	switch v := r.RequestCtx.UserValue(key).(type) {
	case uuid.UUID:
		return v, v != uuid.Nil
	case string:
		id, err := uuid.Parse(v)
		return id, err == nil
	default:
		return uuid.Nil, false
	}
}

// publicPaths returns the set of paths reachable without authentication,
// derived from the route table rather than hand-maintained beside it.
func publicPaths() map[string]bool {
	paths := make(map[string]bool)
	for _, rt := range appRoutes(nil) {
		if rt.public {
			paths[rt.path] = true
		}
	}
	return paths
}

// isPublicPath reports whether a request path needs no authentication.
//
// The table holds fastglue patterns like /api/auth/sso/{provider}/init, so
// parameterized public routes are matched by prefix on the literal portion.
func isPublicPath(path string, public map[string]bool) bool {
	if public[path] {
		return true
	}
	// SSO callbacks carry a provider segment and manage their own auth via
	// state tokens; custom-action redirects use a one-time token.
	return strings.HasPrefix(path, "/api/auth/sso") ||
		strings.HasPrefix(path, "/api/custom-actions/redirect")
}
