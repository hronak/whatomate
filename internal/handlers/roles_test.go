package handlers_test

import (
	"encoding/json"
	"testing"

	"github.com/shridarpatil/whatomate/internal/middleware"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/handlers"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/valyala/fasthttp"
)

func TestApp_ListRoles_Success(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	permissions := testutil.GetOrCreateTestPermissions(t, app.DB)

	// Create some roles
	adminRole := testutil.CreateTestRoleExact(t, app.DB, org.ID, "Admin", true, false, permissions)
	agentRole := testutil.CreateTestRoleExact(t, app.DB, org.ID, "Agent", false, true, permissions[:3])

	// Create a user to make the request
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("list-roles")), testutil.WithRoleID(&adminRole.ID))

	req := testutil.NewGETRequest(t)
	req.RequestCtx.SetUserValue(middleware.ContextKeyUserID, user.ID)
	req.RequestCtx.SetUserValue(middleware.ContextKeyOrganizationID, org.ID)

	err := app.ListRoles(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp struct {
		Roles []handlers.RoleResponse `json:"roles"`
	}
	err = json.Unmarshal(testutil.GetResponseBody(req), &resp)
	require.NoError(t, err)
	assert.Len(t, resp.Roles, 2)

	// Check that roles are sorted (system first, then by name)
	assert.Equal(t, adminRole.Name, resp.Roles[0].Name)
	assert.True(t, resp.Roles[0].IsSystem)
	assert.Equal(t, agentRole.Name, resp.Roles[1].Name)
	assert.True(t, resp.Roles[1].IsDefault)
}

func TestApp_GetRole_Success(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	permissions := testutil.GetOrCreateTestPermissions(t, app.DB)

	role := testutil.CreateTestRoleExact(t, app.DB, org.ID, "Test Role", false, false, permissions[:2])
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("get-role")), testutil.WithRoleID(&role.ID))

	req := testutil.NewGETRequest(t)
	req.RequestCtx.SetUserValue(middleware.ContextKeyUserID, user.ID)
	req.RequestCtx.SetUserValue(middleware.ContextKeyOrganizationID, org.ID)
	req.RequestCtx.SetUserValue("id", role.ID.String())

	err := app.GetRole(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp handlers.RoleResponse
	err = json.Unmarshal(testutil.GetResponseBody(req), &resp)
	require.NoError(t, err)
	assert.Equal(t, role.ID, resp.ID)
	assert.Equal(t, role.Name, resp.Name)
	assert.Len(t, resp.Permissions, 2)
}

func TestApp_GetRole_NotFound(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("get-role-404")))

	req := testutil.NewGETRequest(t)
	req.RequestCtx.SetUserValue(middleware.ContextKeyUserID, user.ID)
	req.RequestCtx.SetUserValue(middleware.ContextKeyOrganizationID, org.ID)
	req.RequestCtx.SetUserValue("id", uuid.New().String())

	err := app.GetRole(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusNotFound, testutil.GetResponseStatusCode(req))
}

func TestApp_CreateRole_Success(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	permissions := testutil.GetOrCreateTestPermissions(t, app.DB)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("create-role")))

	reqBody := handlers.RoleRequest{
		Name:        "New Role",
		Description: "A new custom role",
		IsDefault:   false,
		Permissions: []string{"users:read", "users:write"},
	}

	req := testutil.NewJSONRequest(t, reqBody)
	req.RequestCtx.SetUserValue(middleware.ContextKeyUserID, user.ID)
	req.RequestCtx.SetUserValue(middleware.ContextKeyOrganizationID, org.ID)

	err := app.CreateRole(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp handlers.RoleResponse
	err = json.Unmarshal(testutil.GetResponseBody(req), &resp)
	require.NoError(t, err)
	assert.Equal(t, "New Role", resp.Name)
	assert.Equal(t, "A new custom role", resp.Description)
	assert.False(t, resp.IsSystem)
	assert.Len(t, resp.Permissions, 2)

	// Verify permissions were assigned correctly
	var dbRole models.CustomRole
	require.NoError(t, app.DB.Preload("Permissions").First(&dbRole, "id = ?", resp.ID).Error)
	assert.Len(t, dbRole.Permissions, 2)

	// Clean up permissions for next test
	_ = permissions
}

func TestApp_CreateRole_DuplicateName(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	_ = testutil.GetOrCreateTestPermissions(t, app.DB)

	testutil.CreateTestRoleExact(t, app.DB, org.ID, "Existing Role", false, false, nil)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("create-dup-role")))

	reqBody := handlers.RoleRequest{
		Name:        "Existing Role",
		Description: "Trying to create duplicate",
		Permissions: []string{},
	}

	req := testutil.NewJSONRequest(t, reqBody)
	req.RequestCtx.SetUserValue(middleware.ContextKeyUserID, user.ID)
	req.RequestCtx.SetUserValue(middleware.ContextKeyOrganizationID, org.ID)

	err := app.CreateRole(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusConflict, testutil.GetResponseStatusCode(req))
}

func TestApp_CreateRole_MissingName(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("create-no-name")))

	reqBody := handlers.RoleRequest{
		Name:        "",
		Description: "Role without name",
		Permissions: []string{},
	}

	req := testutil.NewJSONRequest(t, reqBody)
	req.RequestCtx.SetUserValue(middleware.ContextKeyUserID, user.ID)
	req.RequestCtx.SetUserValue(middleware.ContextKeyOrganizationID, org.ID)

	err := app.CreateRole(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusBadRequest, testutil.GetResponseStatusCode(req))
}

func TestApp_CreateRole_WithDefaultFlag(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	_ = testutil.GetOrCreateTestPermissions(t, app.DB)

	// Create an existing default role
	existingDefault := testutil.CreateTestRoleExact(t, app.DB, org.ID, "Old Default", false, true, nil)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("create-default")))

	reqBody := handlers.RoleRequest{
		Name:        "New Default Role",
		Description: "This will be the new default",
		IsDefault:   true,
		Permissions: []string{},
	}

	req := testutil.NewJSONRequest(t, reqBody)
	req.RequestCtx.SetUserValue(middleware.ContextKeyUserID, user.ID)
	req.RequestCtx.SetUserValue(middleware.ContextKeyOrganizationID, org.ID)

	err := app.CreateRole(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	// Verify the old default was unset
	var oldDefault models.CustomRole
	require.NoError(t, app.DB.First(&oldDefault, "id = ?", existingDefault.ID).Error)
	assert.False(t, oldDefault.IsDefault)
}

func TestApp_UpdateRole_Success(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	permissions := testutil.GetOrCreateTestPermissions(t, app.DB)

	role := testutil.CreateTestRoleExact(t, app.DB, org.ID, "Editable Role", false, false, permissions[:1])
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("update-role")))

	reqBody := handlers.RoleRequest{
		Name:        "Updated Role Name",
		Description: "Updated description",
		Permissions: []string{"users:read", "users:write", "contacts:read"},
	}

	req := testutil.NewJSONRequest(t, reqBody)
	req.RequestCtx.SetUserValue(middleware.ContextKeyUserID, user.ID)
	req.RequestCtx.SetUserValue(middleware.ContextKeyOrganizationID, org.ID)
	req.RequestCtx.SetUserValue("id", role.ID.String())

	err := app.UpdateRole(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp handlers.RoleResponse
	err = json.Unmarshal(testutil.GetResponseBody(req), &resp)
	require.NoError(t, err)

	assert.Equal(t, "Updated Role Name", resp.Name)
	assert.Equal(t, "Updated description", resp.Description)
	assert.Len(t, resp.Permissions, 3)
}

func TestApp_UpdateRole_SystemRoleOnlyDescription(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	permissions := testutil.GetOrCreateTestPermissions(t, app.DB)

	// Create a system role
	systemRole := testutil.CreateTestRoleExact(t, app.DB, org.ID, "System Admin", true, false, permissions)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("update-sys-role")))

	reqBody := handlers.RoleRequest{
		Name:        "Changed Name",         // Should be ignored for system roles
		Description: "Updated description",  // Only this should be updated
		Permissions: []string{"users:read"}, // Should be ignored for system roles
	}

	req := testutil.NewJSONRequest(t, reqBody)
	req.RequestCtx.SetUserValue(middleware.ContextKeyUserID, user.ID)
	req.RequestCtx.SetUserValue(middleware.ContextKeyOrganizationID, org.ID)
	req.RequestCtx.SetUserValue("id", systemRole.ID.String())

	err := app.UpdateRole(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp handlers.RoleResponse
	err = json.Unmarshal(testutil.GetResponseBody(req), &resp)
	require.NoError(t, err)

	// Name should not change for system roles
	assert.Equal(t, "System Admin", resp.Name)
	assert.Equal(t, "Updated description", resp.Description)
	// Permissions should remain the same
	assert.Len(t, resp.Permissions, len(permissions))
}

func TestApp_UpdateRole_NotFound(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("update-404")))

	reqBody := handlers.RoleRequest{
		Name: "Updated Name",
	}

	req := testutil.NewJSONRequest(t, reqBody)
	req.RequestCtx.SetUserValue(middleware.ContextKeyUserID, user.ID)
	req.RequestCtx.SetUserValue(middleware.ContextKeyOrganizationID, org.ID)
	req.RequestCtx.SetUserValue("id", uuid.New().String())

	err := app.UpdateRole(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusNotFound, testutil.GetResponseStatusCode(req))
}

func TestApp_DeleteRole_Success(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)

	role := testutil.CreateTestRoleExact(t, app.DB, org.ID, "Deletable Role", false, false, nil)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("delete-role")))

	req := testutil.NewGETRequest(t)
	req.RequestCtx.Request.Header.SetMethod("DELETE")
	req.RequestCtx.SetUserValue(middleware.ContextKeyUserID, user.ID)
	req.RequestCtx.SetUserValue(middleware.ContextKeyOrganizationID, org.ID)
	req.RequestCtx.SetUserValue("id", role.ID.String())

	err := app.DeleteRole(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	// Verify role was deleted
	var dbRole models.CustomRole
	err = app.DB.First(&dbRole, "id = ?", role.ID).Error
	assert.Error(t, err) // Should be not found
}

func TestApp_DeleteRole_SystemRole(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)

	systemRole := testutil.CreateTestRoleExact(t, app.DB, org.ID, "System Role", true, false, nil)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("delete-sys")))

	req := testutil.NewGETRequest(t)
	req.RequestCtx.Request.Header.SetMethod("DELETE")
	req.RequestCtx.SetUserValue(middleware.ContextKeyUserID, user.ID)
	req.RequestCtx.SetUserValue(middleware.ContextKeyOrganizationID, org.ID)
	req.RequestCtx.SetUserValue("id", systemRole.ID.String())

	err := app.DeleteRole(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusBadRequest, testutil.GetResponseStatusCode(req))

	// Verify role still exists
	var dbRole models.CustomRole
	require.NoError(t, app.DB.First(&dbRole, "id = ?", systemRole.ID).Error)
}

func TestApp_DeleteRole_WithAssignedUsers(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)

	role := testutil.CreateTestRoleExact(t, app.DB, org.ID, "Role With Users", false, false, nil)
	// Create a user with this role
	testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("assigned-user")), testutil.WithRoleID(&role.ID))
	adminUser := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("delete-used-role")))

	req := testutil.NewGETRequest(t)
	req.RequestCtx.Request.Header.SetMethod("DELETE")
	req.RequestCtx.SetUserValue(middleware.ContextKeyUserID, adminUser.ID)
	req.RequestCtx.SetUserValue(middleware.ContextKeyOrganizationID, org.ID)
	req.RequestCtx.SetUserValue("id", role.ID.String())

	err := app.DeleteRole(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusBadRequest, testutil.GetResponseStatusCode(req))
}

func TestApp_ListPermissions_Success(t *testing.T) {
	app := newTestApp(t)
	org := testutil.CreateTestOrganization(t, app.DB)
	permissions := testutil.GetOrCreateTestPermissions(t, app.DB)
	user := testutil.CreateTestUser(t, app.DB, org.ID, testutil.WithEmail(testutil.UniqueEmail("list-perms")))

	req := testutil.NewGETRequest(t)
	req.RequestCtx.SetUserValue(middleware.ContextKeyUserID, user.ID)
	req.RequestCtx.SetUserValue(middleware.ContextKeyOrganizationID, org.ID)

	err := app.ListPermissions(req)
	require.NoError(t, err)
	assert.Equal(t, fasthttp.StatusOK, testutil.GetResponseStatusCode(req))

	var resp struct {
		Permissions []handlers.PermissionResponse `json:"permissions"`
	}
	err = json.Unmarshal(testutil.GetResponseBody(req), &resp)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(resp.Permissions), len(permissions))

	// Verify permission format
	for _, perm := range resp.Permissions {
		assert.NotEmpty(t, perm.Resource)
		assert.NotEmpty(t, perm.Action)
		assert.Equal(t, perm.Resource+":"+perm.Action, perm.Key)
	}
}
