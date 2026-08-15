package handlers

import (
	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/rbac"
	"github.com/shridarpatil/whatomate/internal/websocket"
	"gorm.io/gorm"
)

// The permission engine now lives in internal/rbac. These methods forward to
// it so the ~100 existing call sites keep working while the package split
// proceeds; the later handler-struct split removes them by injecting the
// engine where it is used.

// UserPermissions is re-exported so handler responses keep their shape.
type UserPermissions = rbac.UserPermissions

// hubNotifier adapts the WebSocket hub to rbac.Notifier, so rbac announces a
// permission change without knowing what a hub is.
type hubNotifier struct{ hub *websocket.Hub }

func (n hubNotifier) PermissionsChanged(orgID, userID uuid.UUID) {
	if n.hub == nil {
		return
	}
	n.hub.BroadcastToUser(orgID, userID, websocket.WSMessage{
		Type:    websocket.TypePermissionsUpdated,
		Payload: map[string]string{"message": "Your permissions have been updated"},
	})
}

// rbacEngine returns the app's permission engine, building it on first use.
// App is a struct literal in main and in tests, so this cannot be a
// constructor field.
func (a *App) rbacEngine() *rbac.Engine {
	a.rbacOnce.Do(func() {
		a.rbac = rbac.New(a.DB, a.Redis, a.Log, hubNotifier{hub: a.WSHub})
	})
	return a.rbac
}

// HasPermission reports whether a user holds resource:action.
func (a *App) HasPermission(userID uuid.UUID, resource, action string, orgIDs ...uuid.UUID) bool {
	return a.rbacEngine().Has(userID, resource, action, orgIDs...)
}

// HasAnyPermission reports whether a user holds any of the given permissions.
func (a *App) HasAnyPermission(userID uuid.UUID, permissions ...string) bool {
	return a.rbacEngine().HasAny(userID, permissions...)
}

// IsSuperAdmin reports whether a user can access every organization.
func (a *App) IsSuperAdmin(userID uuid.UUID) bool {
	return a.rbacEngine().IsSuperAdmin(userID)
}

// GetRolePermissionsCached resolves a role's permissions.
func (a *App) GetRolePermissionsCached(roleID uuid.UUID) ([]string, error) {
	return a.rbacEngine().RolePermissions(roleID)
}

// loadRolePermissions fills each role's Permissions via a single JOIN.
func (a *App) loadRolePermissions(roles ...*models.CustomRole) error {
	return a.rbacEngine().LoadRolePermissions(roles...)
}

// InvalidateUserPermissionsCache drops a user's cached permissions.
func (a *App) InvalidateUserPermissionsCache(userID uuid.UUID) {
	a.rbacEngine().InvalidateUser(userID)
}

// InvalidateRolePermissionsCache drops a role's cached permissions and those of
// every user holding it.
func (a *App) InvalidateRolePermissionsCache(roleID uuid.UUID) {
	a.rbacEngine().InvalidateRole(roleID)
}

// InvalidateOrgPermissionsCache drops the cached permissions of every role in
// an organization.
func (a *App) InvalidateOrgPermissionsCache(orgID uuid.UUID) {
	a.rbacEngine().InvalidateOrg(orgID)
}

// ScopedQuery returns a query scoped to an organization. Multi-tenancy is not
// enforced by any GORM scope, so every org-scoped query must go through this
// or filter explicitly.
func (a *App) ScopedQuery(_ uuid.UUID, orgID uuid.UUID) *gorm.DB {
	return a.DB.Where("organization_id = ?", orgID)
}

// ScopeToOrg adds organization scoping to an existing query.
func (a *App) ScopeToOrg(query *gorm.DB, _ uuid.UUID, orgID uuid.UUID) *gorm.DB {
	return query.Where("organization_id = ?", orgID)
}
