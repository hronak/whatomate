package handlers

import (
	"slices"

	"github.com/shridarpatil/whatomate/internal/middleware"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/zerodha/fastglue"
	"gorm.io/gorm"
)

// roleAuditSnapshot returns a diff-friendly representation of a role.
func roleAuditSnapshot(role *models.CustomRole) map[string]any {
	if role == nil {
		return nil
	}
	perms := make([]string, len(role.Permissions))
	for i, p := range role.Permissions {
		perms[i] = p.Resource + ":" + p.Action
	}
	slices.Sort(perms)
	return map[string]any{
		"name":        role.Name,
		"description": role.Description,
		"is_default":  role.IsDefault,
		"is_system":   role.IsSystem,
		"permissions": perms,
	}
}

// RoleRequest represents the request body for creating/updating a role
type RoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	IsDefault   bool     `json:"is_default"`
	Permissions []string `json:"permissions"` // Format: ["resource:action", ...]
}

// RoleResponse represents the response for a role
type RoleResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsSystem    bool      `json:"is_system"`
	IsDefault   bool      `json:"is_default"`
	Permissions []string  `json:"permissions"`
	UserCount   int64     `json:"user_count"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

// PermissionResponse represents a permission in the API
type PermissionResponse struct {
	ID          uuid.UUID `json:"id"`
	Resource    string    `json:"resource"`
	Action      string    `json:"action"`
	Description string    `json:"description"`
	Key         string    `json:"key"` // "resource:action"
}

// ListRoles returns all roles for the organization
func (a *App) ListRoles(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return a.sendError(r, unauthorized("Unauthorized"))
	}

	pg := parsePagination(r)
	search := string(r.RequestCtx.QueryArgs().Peek("search"))

	baseQuery := a.ScopeToOrg(a.DB, userID, orgID)
	if search != "" {
		baseQuery = baseQuery.Where("name ILIKE ?", "%"+search+"%")
	}

	// Get total count
	var total int64
	baseQuery.Model(&models.CustomRole{}).Count(&total)

	var roles []models.CustomRole
	if err := pg.Apply(baseQuery.
		Order("is_system DESC, name ASC")).
		Find(&roles).Error; err != nil {
		a.Log.Error("Failed to list roles", "error", err)
		return a.sendError(r, internalError("Failed to list roles", err))
	}

	// Load permissions via JOIN instead of GORM's Preload IN query
	rolePtrs := make([]*models.CustomRole, len(roles))
	for i := range roles {
		rolePtrs[i] = &roles[i]
	}
	if err := a.loadRolePermissions(rolePtrs...); err != nil {
		a.Log.Error("Failed to load role permissions", "error", err)
		return a.sendError(r, internalError("Failed to list roles", err))
	}

	// Convert to response format with user counts
	response := make([]RoleResponse, len(roles))
	for i, role := range roles {
		var userCount int64
		a.DB.Model(&models.User{}).Where("role_id = ?", role.ID).Count(&userCount)
		response[i] = roleToResponse(role, userCount)
	}

	return a.sendJSON(r, listEnvelope("roles", response, total, pg))
}

// GetRole returns a single role
func (a *App) GetRole(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return a.sendError(r, unauthorized("Unauthorized"))
	}

	id, err := parsePathUUID(r, "id", "role")
	if err != nil {
		return nil
	}

	var role models.CustomRole
	if err := a.DB.Where("id = ? AND organization_id = ?", id, orgID).
		First(&role).Error; err != nil {
		return a.sendError(r, notFound("Role"))
	}

	if err := a.loadRolePermissions(&role); err != nil {
		a.Log.Error("Failed to load role permissions", "error", err)
		return a.sendError(r, internalError("Failed to get role", err))
	}

	var userCount int64
	a.DB.Model(&models.User{}).Where("role_id = ?", role.ID).Count(&userCount)

	return a.sendJSON(r, roleToResponse(role, userCount))
}

// CreateRole creates a new custom role
func (a *App) CreateRole(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return a.sendError(r, unauthorized("Unauthorized"))
	}

	var req RoleRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	// Validate required fields
	if req.Name == "" {
		return a.sendError(r, invalidRequest("Name is required"))
	}

	// Check if name already exists
	var existingRole models.CustomRole
	if err := a.DB.Where("organization_id = ? AND name = ?", orgID, req.Name).First(&existingRole).Error; err == nil {
		return a.sendError(r, conflict("Role with this name already exists"))
	}

	// Get permissions from database
	permissions, err := a.getPermissionsByKeys(req.Permissions)
	if err != nil {
		a.Log.Error("Failed to fetch permissions", "error", err)
		return a.sendError(r, internalError("Failed to create role", err))
	}

	role := models.CustomRole{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: orgID,
		Name:           req.Name,
		Description:    req.Description,
		IsSystem:       false,
		IsDefault:      req.IsDefault,
		Permissions:    permissions,
	}

	// If setting as default, unset other defaults (in a transaction)
	if req.IsDefault {
		if err := a.DB.Transaction(func(tx *gorm.DB) error {
			tx.Model(&models.CustomRole{}).
				Where("organization_id = ? AND is_default = ?", orgID, true).
				Update("is_default", false)
			return tx.Create(&role).Error
		}); err != nil {
			a.Log.Error("Failed to create role", "error", err)
			return a.sendError(r, internalError("Failed to create role", err))
		}
		a.logAudit(orgID, userID,
			"role", role.ID, models.AuditActionCreated, nil, roleAuditSnapshot(&role))
		return a.sendJSON(r, roleToResponse(role, 0))
	}

	if err := a.DB.Create(&role).Error; err != nil {
		a.Log.Error("Failed to create role", "error", err)
		return a.sendError(r, internalError("Failed to create role", err))
	}

	a.logAudit(orgID, userID,
		"role", role.ID, models.AuditActionCreated, nil, roleAuditSnapshot(&role))

	return a.sendJSON(r, roleToResponse(role, 0))
}

// UpdateRole updates a custom role
func (a *App) UpdateRole(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return a.sendError(r, unauthorized("Unauthorized"))
	}

	id, err := parsePathUUID(r, "id", "role")
	if err != nil {
		return nil
	}

	var role models.CustomRole
	if err := a.DB.Where("id = ? AND organization_id = ?", id, orgID).
		First(&role).Error; err != nil {
		return a.sendError(r, notFound("Role"))
	}

	if err := a.loadRolePermissions(&role); err != nil {
		a.Log.Error("Failed to load role permissions", "error", err)
		return a.sendError(r, internalError("Failed to update role", err))
	}

	oldSnap := roleAuditSnapshot(&role)

	// System roles can only have their description updated
	var req RoleRequest
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	if role.IsSystem {
		// Check if user is super admin
		isSuperAdmin, _ := r.RequestCtx.UserValue(middleware.ContextKeyIsSuperAdmin).(bool)

		// Only allow description updates for non-super admins
		if req.Description != "" {
			role.Description = req.Description
		}

		// Super admins can update permissions for system roles
		if isSuperAdmin && len(req.Permissions) > 0 {
			permissions, err := a.getPermissionsByKeys(req.Permissions)
			if err != nil {
				a.Log.Error("Failed to fetch permissions", "error", err)
				return a.sendError(r, internalError("Failed to update role", err))
			}
			if err := a.DB.Model(&role).Association("Permissions").Replace(permissions); err != nil {
				a.Log.Error("Failed to update role permissions", "error", err)
				return a.sendError(r, internalError("Failed to update role", err))
			}
			role.Permissions = permissions
		}

		if err := a.DB.Save(&role).Error; err != nil {
			a.Log.Error("Failed to update role", "error", err)
			return a.sendError(r, internalError("Failed to update role", err))
		}

		// Invalidate permissions cache for all users with this role
		a.InvalidateRolePermissionsCache(role.ID)

		a.logAudit(orgID, userID,
			"role", role.ID, models.AuditActionUpdated, oldSnap, roleAuditSnapshot(&role))

		var userCount int64
		a.DB.Model(&models.User{}).Where("role_id = ?", role.ID).Count(&userCount)
		return a.sendJSON(r, roleToResponse(role, userCount))
	}

	// For custom roles, allow full updates
	if req.Name != "" {
		// Check if name already exists for another role
		var existingRole models.CustomRole
		if err := a.DB.Where("organization_id = ? AND name = ? AND id != ?", orgID, req.Name, id).First(&existingRole).Error; err == nil {
			return a.sendError(r, conflict("Role with this name already exists"))
		}
		role.Name = req.Name
	}
	if req.Description != "" {
		role.Description = req.Description
	}

	// Update permissions if provided
	if len(req.Permissions) > 0 {
		permissions, err := a.getPermissionsByKeys(req.Permissions)
		if err != nil {
			a.Log.Error("Failed to fetch permissions", "error", err)
			return a.sendError(r, internalError("Failed to update role", err))
		}
		// Replace associations
		if err := a.DB.Model(&role).Association("Permissions").Replace(permissions); err != nil {
			a.Log.Error("Failed to update role permissions", "error", err)
			return a.sendError(r, internalError("Failed to update role", err))
		}
		role.Permissions = permissions
	}

	// Handle default flag (in a transaction to prevent race conditions)
	if req.IsDefault && !role.IsDefault {
		role.IsDefault = true
		if err := a.DB.Transaction(func(tx *gorm.DB) error {
			tx.Model(&models.CustomRole{}).
				Where("organization_id = ? AND is_default = ? AND id != ?", orgID, true, role.ID).
				Update("is_default", false)
			return tx.Save(&role).Error
		}); err != nil {
			a.Log.Error("Failed to update role", "error", err)
			return a.sendError(r, internalError("Failed to update role", err))
		}
	} else {
		if !req.IsDefault && role.IsDefault {
			role.IsDefault = false
		}
		if err := a.DB.Save(&role).Error; err != nil {
			a.Log.Error("Failed to update role", "error", err)
			return a.sendError(r, internalError("Failed to update role", err))
		}
	}

	// Invalidate permissions cache for all users with this role
	a.InvalidateRolePermissionsCache(role.ID)

	a.logAudit(orgID, userID,
		"role", role.ID, models.AuditActionUpdated, oldSnap, roleAuditSnapshot(&role))

	var userCount int64
	a.DB.Model(&models.User{}).Where("role_id = ?", role.ID).Count(&userCount)
	return a.sendJSON(r, roleToResponse(role, userCount))
}

// DeleteRole deletes a custom role
func (a *App) DeleteRole(r *fastglue.Request) error {
	orgID, userID, err := a.getOrgAndUserID(r)
	if err != nil {
		return a.sendError(r, unauthorized("Unauthorized"))
	}

	id, err := parsePathUUID(r, "id", "role")
	if err != nil {
		return nil
	}

	role, err := findByIDAndOrg[models.CustomRole](a, r, id, orgID, "Role")
	if err != nil {
		return nil
	}

	// Cannot delete system roles
	if role.IsSystem {
		return a.sendError(r, invalidRequest("Cannot delete system roles"))
	}

	// Check if any users have this role
	var userCount int64
	a.DB.Model(&models.User{}).Where("role_id = ?", id).Count(&userCount)
	if userCount > 0 {
		return a.sendError(r, invalidRequest("Cannot delete role with assigned users"))
	}

	// Load permissions for audit snapshot before deletion
	_ = a.loadRolePermissions(role)
	oldSnap := roleAuditSnapshot(role)

	// Delete the role (permissions associations will be cleared automatically)
	if err := a.DB.Delete(role).Error; err != nil {
		a.Log.Error("Failed to delete role", "error", err)
		return a.sendError(r, internalError("Failed to delete role", err))
	}

	a.logAudit(orgID, userID,
		"role", id, models.AuditActionDeleted, oldSnap, nil)

	return a.sendJSON(r, map[string]string{"message": "Role deleted successfully"})
}

// ListPermissions returns all available permissions
func (a *App) ListPermissions(r *fastglue.Request) error {
	var permissions []models.Permission
	if err := a.DB.Order("resource ASC, action ASC").Find(&permissions).Error; err != nil {
		a.Log.Error("Failed to list permissions", "error", err)
		return a.sendError(r, internalError("Failed to list permissions", err))
	}

	response := make([]PermissionResponse, len(permissions))
	for i, p := range permissions {
		response[i] = PermissionResponse{
			ID:          p.ID,
			Resource:    p.Resource,
			Action:      p.Action,
			Description: p.Description,
			Key:         p.Resource + ":" + p.Action,
		}
	}

	return a.sendJSON(r, map[string]any{
		"permissions": response,
	})
}

// Helper function to convert CustomRole to RoleResponse
func roleToResponse(role models.CustomRole, userCount int64) RoleResponse {
	permissions := make([]string, len(role.Permissions))
	for i, p := range role.Permissions {
		permissions[i] = p.Resource + ":" + p.Action
	}

	return RoleResponse{
		ID:          role.ID,
		Name:        role.Name,
		Description: role.Description,
		IsSystem:    role.IsSystem,
		IsDefault:   role.IsDefault,
		Permissions: permissions,
		UserCount:   userCount,
		CreatedAt:   role.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   role.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// Helper function to get permissions by their keys
func (a *App) getPermissionsByKeys(keys []string) ([]models.Permission, error) {
	if len(keys) == 0 {
		return []models.Permission{}, nil
	}

	// Parse keys into resource:action pairs
	var conditions [][]string
	for _, key := range keys {
		if len(key) > 0 {
			parts := splitPermissionKey(key)
			if len(parts) == 2 {
				conditions = append(conditions, parts)
			}
		}
	}

	if len(conditions) == 0 {
		return []models.Permission{}, nil
	}

	var permissions []models.Permission
	query := a.DB.Model(&models.Permission{})

	// Build OR conditions for each permission
	for i, cond := range conditions {
		if i == 0 {
			query = query.Where("resource = ? AND action = ?", cond[0], cond[1])
		} else {
			query = query.Or("resource = ? AND action = ?", cond[0], cond[1])
		}
	}

	if err := query.Find(&permissions).Error; err != nil {
		return nil, err
	}

	return permissions, nil
}

// splitPermissionKey splits "resource:action" into ["resource", "action"]
func splitPermissionKey(key string) []string {
	for i := range len(key) {
		if key[i] == ':' {
			return []string{key[:i], key[i+1:]}
		}
	}
	return nil
}
