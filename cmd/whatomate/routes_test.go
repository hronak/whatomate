package main

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/shridarpatil/whatomate/internal/models"
)

// The route table is the authorization surface: every endpoint's permission is
// declared in one place, so these checks are the regression test for the RBAC
// fix. They need no database — the table is a pure value.

// knownPermissions is the set of resource:action pairs the permission system
// actually seeds.
func knownPermissions() map[string]bool {
	known := make(map[string]bool)
	for _, p := range models.DefaultPermissions() {
		known[p.Resource+":"+p.Action] = true
	}
	return known
}

func TestRouteTable_NoDuplicateRoutes(t *testing.T) {
	seen := make(map[string]bool)
	for _, rt := range appRoutes(nil) {
		key := rt.method + " " + rt.path
		if seen[key] {
			t.Errorf("duplicate route registered twice: %s", key)
		}
		seen[key] = true
	}
}

func TestRouteTable_PermissionsAreSeeded(t *testing.T) {
	known := knownPermissions()
	for _, rt := range appRoutes(nil) {
		if rt.public || rt.permission == authenticatedOnly {
			continue
		}
		perm := rt.permission[0] + ":" + rt.permission[1]
		if !known[perm] {
			// A route demanding a permission nobody can hold denies everyone
			// the moment enforcement is switched on.
			t.Errorf("%s %s requires %q, which DefaultPermissions never seeds",
				rt.method, rt.path, perm)
		}
	}
}

func TestRouteTable_EveryRouteIsDecided(t *testing.T) {
	// A route must be explicitly public, explicitly authenticated-only, or
	// carry a permission. The point of the table is that nothing is decided by
	// omission.
	for _, rt := range appRoutes(nil) {
		if rt.public {
			continue
		}
		if rt.permission == authenticatedOnly {
			// Allowed, but keep the list short and deliberate.
			if !strings.HasPrefix(rt.path, "/api/me") && !strings.HasPrefix(rt.path, "/api/auth/") &&
				rt.path != "/api/client-config" {
				t.Errorf("%s %s is authenticated-only; if that is intended, add it to the "+
					"documented exceptions here so the choice stays visible", rt.method, rt.path)
			}
			continue
		}
		if rt.permission[0] == "" || rt.permission[1] == "" {
			t.Errorf("%s %s has a half-filled permission %v", rt.method, rt.path, rt.permission)
		}
	}
}

func TestRouteTable_PublicRoutesAreExpected(t *testing.T) {
	// Anything reachable without authentication is a deliberate decision, so
	// changing this set should require changing this test.
	want := map[string]bool{
		"/health":                           true,
		"/ready":                            true,
		"/api/auth/login":                   true,
		"/api/auth/register":                true,
		"/api/auth/refresh":                 true,
		"/api/auth/logout":                  true,
		"/api/auth/sso/providers":           true,
		"/api/auth/sso/{provider}/init":     true,
		"/api/auth/sso/{provider}/callback": true,
		"/api/webhook":                      true,
		"/ws":                               true,
	}
	got := publicPaths()
	for path := range got {
		if !want[path] {
			t.Errorf("route %q is public but not in the expected set — is that intended?", path)
		}
	}
	for path := range want {
		if !got[path] {
			t.Errorf("route %q was public and no longer is", path)
		}
	}
}

// TestRouteTable_MatchesFrontendPermissions cross-checks the resources the
// frontend router guards against the resources the backend enforces. A
// resource the frontend hides but the backend does not check is a hole; one the
// backend requires but the frontend never mentions usually means a menu entry
// that 403s.
func TestRouteTable_MatchesFrontendPermissions(t *testing.T) {
	src, err := os.ReadFile("../../frontend/src/router/index.ts")
	if err != nil {
		t.Skipf("frontend router not readable: %v", err)
	}

	re := regexp.MustCompile(`permission:\s*'([^']+)'`)
	frontend := make(map[string]bool)
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		frontend[m[1]] = true
	}
	if len(frontend) == 0 {
		t.Fatal("parsed no permissions from the frontend router; the format must have changed")
	}

	backend := make(map[string]bool)
	for _, rt := range appRoutes(nil) {
		if !rt.public && rt.permission != authenticatedOnly {
			backend[rt.permission[0]] = true
		}
	}

	for resource := range frontend {
		if !backend[resource] {
			t.Errorf("frontend guards routes with %q but no backend route requires it", resource)
		}
	}
}
