// Package rbac answers "may this user do this?" and caches the answer.
//
// It owns the permission half of what used to be internal/handlers/cache.go, a
// file with two unrelated jobs: caching business entities and resolving
// authorization. Splitting them lets the permission engine be constructed,
// tested and reasoned about without an HTTP handler in sight — it is the one
// piece of that package every other domain depends on.
//
// Permissions are "resource:action" strings resolved through a user's role.
// Super admins short-circuit to true.
package rbac

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/zerodha/logf"
	"gorm.io/gorm"
)

const (
	// permissionsTTL is how long a resolved permission set is cached. Writes
	// invalidate explicitly, so this is only a backstop.
	permissionsTTL = 6 * time.Hour

	userPermissionsPrefix = "permissions:user:"
	rolePermissionsPrefix = "permissions:role:"

	// cacheTimeout bounds one Redis operation. The cache is an optimisation:
	// a slow Redis should fall through to Postgres, not stall the caller.
	cacheTimeout = 3 * time.Second
)

// Notifier is told when a user's permissions change, so open clients can
// re-fetch them.
//
// Consumer-defined on purpose: rbac needs to announce a change, not to know
// what a WebSocket hub is.
type Notifier interface {
	PermissionsChanged(orgID, userID uuid.UUID)
}

// UserPermissions is a user's resolved role and permission set.
type UserPermissions struct {
	RoleID       uuid.UUID `json:"role_id"`
	RoleName     string    `json:"role_name"`
	IsSystem     bool      `json:"is_system"`
	IsSuperAdmin bool      `json:"is_super_admin"`
	Permissions  []string  `json:"permissions"` // "resource:action"
}

// Engine resolves and caches permissions.
type Engine struct {
	db       *gorm.DB
	redis    *redis.Client
	log      logf.Logger
	notifier Notifier
}

// New creates an Engine. redis and notifier may be nil: without Redis every
// lookup hits the database, and without a notifier permission changes are not
// pushed to connected clients.
func New(db *gorm.DB, rdb *redis.Client, log logf.Logger, notifier Notifier) *Engine {
	return &Engine{db: db, redis: rdb, log: log, notifier: notifier}
}

// SetNotifier installs the notifier after construction, for wiring orders where
// the hub is not available when the Engine is built.
func (e *Engine) SetNotifier(n Notifier) { e.notifier = n }

func (e *Engine) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), cacheTimeout)
}

// userCacheKey returns the cache key for a user's permissions, per-org when an
// org is given (a user may hold a different role in each organization).
func userCacheKey(userID, orgID uuid.UUID) string {
	if orgID != uuid.Nil {
		return fmt.Sprintf("%s%s:%s", userPermissionsPrefix, userID, orgID)
	}
	return userPermissionsPrefix + userID.String()
}

// UserPermissions resolves a user's permissions, from cache when possible.
//
// With an orgID it uses the role from that organization's membership, falling
// back to the user's default role; without one it uses the default role.
func (e *Engine) UserPermissions(userID uuid.UUID, orgIDs ...uuid.UUID) (*UserPermissions, error) {
	var orgID uuid.UUID
	if len(orgIDs) > 0 {
		orgID = orgIDs[0]
	}
	cacheKey := userCacheKey(userID, orgID)

	ctx, cancel := e.ctx()
	defer cancel()

	if e.redis != nil {
		if cached, err := e.redis.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var perms UserPermissions
			if err := json.Unmarshal([]byte(cached), &perms); err == nil {
				return &perms, nil
			}
		}
	}

	var user models.User
	if err := e.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		return nil, err
	}

	roleID := user.RoleID
	if orgID != uuid.Nil {
		var userOrg models.UserOrganization
		if err := e.db.WithContext(ctx).
			Where("user_id = ? AND organization_id = ?", userID, orgID).
			First(&userOrg).Error; err == nil && userOrg.RoleID != nil {
			roleID = userOrg.RoleID
		}
	}
	if roleID == nil {
		return nil, gorm.ErrRecordNotFound
	}

	var role models.CustomRole
	if err := e.db.WithContext(ctx).Where("id = ?", roleID).First(&role).Error; err != nil {
		return nil, err
	}
	if err := e.LoadRolePermissions(&role); err != nil {
		return nil, err
	}

	perms := UserPermissions{
		RoleID:       role.ID,
		RoleName:     role.Name,
		IsSystem:     role.IsSystem,
		IsSuperAdmin: user.IsSuperAdmin,
		Permissions:  permissionKeys(role.Permissions),
	}

	if e.redis != nil {
		if data, err := json.Marshal(perms); err == nil {
			if err := e.redis.Set(ctx, cacheKey, data, permissionsTTL).Err(); err != nil {
				e.log.Warn("Failed to cache user permissions", "error", err, "user_id", userID)
			}
		}
	}
	return &perms, nil
}

// permissionKeys renders permissions as "resource:action" strings.
func permissionKeys(perms []models.Permission) []string {
	keys := make([]string, 0, len(perms))
	for _, p := range perms {
		keys = append(keys, p.Resource+":"+p.Action)
	}
	return keys
}

// Has reports whether a user holds resource:action. Super admins hold
// everything. A lookup failure is a denial, and is logged.
func (e *Engine) Has(userID uuid.UUID, resource, action string, orgIDs ...uuid.UUID) bool {
	perms, err := e.UserPermissions(userID, orgIDs...)
	if err != nil {
		e.log.Error("Failed to get user permissions", "error", err, "user_id", userID)
		return false
	}
	if perms.IsSuperAdmin {
		return true
	}
	return slices.Contains(perms.Permissions, resource+":"+action)
}

// HasAny reports whether a user holds any of the given "resource:action" keys.
func (e *Engine) HasAny(userID uuid.UUID, permissions ...string) bool {
	perms, err := e.UserPermissions(userID)
	if err != nil {
		e.log.Error("Failed to get user permissions", "error", err, "user_id", userID)
		return false
	}
	if perms.IsSuperAdmin {
		return true
	}
	for _, want := range permissions {
		if slices.Contains(perms.Permissions, want) {
			return true
		}
	}
	return false
}

// IsSuperAdmin reports whether a user can access every organization.
func (e *Engine) IsSuperAdmin(userID uuid.UUID) bool {
	perms, err := e.UserPermissions(userID)
	return err == nil && perms.IsSuperAdmin
}

// RolePermissions resolves a role's permissions, from cache when possible.
func (e *Engine) RolePermissions(roleID uuid.UUID) ([]string, error) {
	ctx, cancel := e.ctx()
	defer cancel()
	cacheKey := rolePermissionsPrefix + roleID.String()

	if e.redis != nil {
		if cached, err := e.redis.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var perms []string
			if err := json.Unmarshal([]byte(cached), &perms); err == nil {
				return perms, nil
			}
		}
	}

	var role models.CustomRole
	if err := e.db.WithContext(ctx).Where("id = ?", roleID).First(&role).Error; err != nil {
		return nil, err
	}
	if err := e.LoadRolePermissions(&role); err != nil {
		return nil, err
	}

	perms := permissionKeys(role.Permissions)
	if e.redis != nil {
		if data, err := json.Marshal(perms); err == nil {
			if err := e.redis.Set(ctx, cacheKey, data, permissionsTTL).Err(); err != nil {
				e.log.Warn("Failed to cache role permissions", "error", err, "role_id", roleID)
			}
		}
	}
	return perms, nil
}

// LoadRolePermissions fills each role's Permissions via one JOIN, rather than
// the N+1 a Preload per role would produce.
func (e *Engine) LoadRolePermissions(roles ...*models.CustomRole) error {
	if len(roles) == 0 {
		return nil
	}
	roleIDs := make([]uuid.UUID, len(roles))
	roleMap := make(map[uuid.UUID]*models.CustomRole, len(roles))
	for i, r := range roles {
		roleIDs[i] = r.ID
		r.Permissions = []models.Permission{}
		roleMap[r.ID] = r
	}

	var results []struct {
		models.Permission
		CustomRoleID uuid.UUID `gorm:"column:custom_role_id"`
	}
	err := e.db.Table("permissions").
		Select("permissions.*, role_permissions.custom_role_id").
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.custom_role_id IN ?", roleIDs).
		Where("permissions.deleted_at IS NULL").
		Find(&results).Error
	if err != nil {
		return err
	}

	for _, row := range results {
		if role, ok := roleMap[row.CustomRoleID]; ok {
			role.Permissions = append(role.Permissions, row.Permission)
		}
	}
	return nil
}

// InvalidateUser drops a user's cached permissions and tells them to re-fetch.
//
// Every write path that can change a user's effective permissions must call
// this: a missed invalidation serves stale authorization for the full TTL.
func (e *Engine) InvalidateUser(userID uuid.UUID) {
	ctx, cancel := e.ctx()
	defer cancel()

	e.del(ctx, "user_permissions", userPermissionsPrefix+userID.String())
	// Plus every per-org variant.
	e.delByPattern(ctx, fmt.Sprintf("%s%s:*", userPermissionsPrefix, userID))

	e.notifyChanged(userID)
}

// InvalidateRole drops a role's cached permissions and those of every user
// holding it, by default role or by organization membership.
func (e *Engine) InvalidateRole(roleID uuid.UUID) {
	ctx, cancel := e.ctx()
	defer cancel()
	e.del(ctx, "role_permissions", rolePermissionsPrefix+roleID.String())

	userIDs := make(map[uuid.UUID]bool)

	var users []models.User
	if err := e.db.Select("id").Where("role_id = ?", roleID).Find(&users).Error; err != nil {
		e.log.Error("Failed to find users for role permission cache invalidation", "error", err, "role_id", roleID)
	}
	for _, u := range users {
		userIDs[u.ID] = true
	}

	var orgUserIDs []uuid.UUID
	if err := e.db.Table("user_organizations").
		Select("user_id").
		Where("role_id = ? AND deleted_at IS NULL", roleID).
		Pluck("user_id", &orgUserIDs).Error; err != nil {
		e.log.Error("Failed to find org users for role permission cache invalidation", "error", err, "role_id", roleID)
	}
	for _, uid := range orgUserIDs {
		userIDs[uid] = true
	}

	for uid := range userIDs {
		e.InvalidateUser(uid)
	}
}

// InvalidateOrg drops the cached permissions of every role in an organization.
func (e *Engine) InvalidateOrg(orgID uuid.UUID) {
	var roles []models.CustomRole
	if err := e.db.Select("id").Where("organization_id = ?", orgID).Find(&roles).Error; err != nil {
		e.log.Error("Failed to find roles for org permission cache invalidation", "error", err, "org_id", orgID)
		return
	}
	for _, role := range roles {
		e.InvalidateRole(role.ID)
	}
}

// del removes cache keys, reporting failure: a dropped invalidation serves
// stale authorization until the TTL expires.
func (e *Engine) del(ctx context.Context, what string, keys ...string) {
	if e.redis == nil || len(keys) == 0 {
		return
	}
	if err := e.redis.Del(ctx, keys...).Err(); err != nil {
		e.log.Error("Permission cache invalidation failed — stale authorization until TTL",
			"cache", what, "keys", keys, "error", err)
	}
}

// delByPattern removes every key matching a glob.
func (e *Engine) delByPattern(ctx context.Context, pattern string) {
	if e.redis == nil {
		return
	}
	iter := e.redis.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		e.del(ctx, "pattern:"+pattern, iter.Val())
	}
	if err := iter.Err(); err != nil && !errors.Is(err, redis.Nil) {
		e.log.Error("Permission cache invalidation scan failed", "pattern", pattern, "error", err)
	}
}

// notifyChanged pushes a permissions-updated event to the user's clients.
func (e *Engine) notifyChanged(userID uuid.UUID) {
	if e.notifier == nil {
		return
	}
	var user models.User
	if err := e.db.Select("organization_id").Where("id = ?", userID).First(&user).Error; err != nil {
		e.log.Error("Failed to find user for permissions notification", "error", err, "user_id", userID)
		return
	}
	e.notifier.PermissionsChanged(user.OrganizationID, userID)
}
