package rbac_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/rbac"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// recordingNotifier captures permission-change announcements.
type recordingNotifier struct {
	calls []uuid.UUID
}

func (n *recordingNotifier) PermissionsChanged(_, userID uuid.UUID) {
	n.calls = append(n.calls, userID)
}

// newEngine builds an Engine against the test database. Redis is optional:
// without it every lookup goes to Postgres, which is a case worth covering.
func newEngine(t *testing.T, notifier rbac.Notifier) (*rbac.Engine, *gorm.DB) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return rbac.New(db, testutil.SetupTestRedis(t), testutil.NopLogger(), notifier), db
}

// seedUserWithPermissions creates an org, a role holding the given
// resource:action pairs, and a user in that role.
func seedUserWithPermissions(t *testing.T, db *gorm.DB, superAdmin bool, want ...[2]string) (*models.User, *models.Organization) {
	t.Helper()
	uniq := uuid.New().String()[:8]

	org := &models.Organization{Name: "rbac-" + uniq, Slug: "rbac-" + uniq}
	require.NoError(t, db.Create(org).Error)

	var perms []models.Permission
	for _, w := range want {
		p := models.Permission{
			BaseModel:   models.BaseModel{ID: uuid.New()},
			Resource:    w[0],
			Action:      w[1],
			Description: "test",
		}
		// Permissions are global; reuse an existing row when present.
		var existing models.Permission
		if err := db.Where("resource = ? AND action = ?", w[0], w[1]).First(&existing).Error; err == nil {
			perms = append(perms, existing)
			continue
		}
		require.NoError(t, db.Create(&p).Error)
		perms = append(perms, p)
	}

	role := &models.CustomRole{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		Name:           "role-" + uniq,
		Permissions:    perms,
	}
	require.NoError(t, db.Create(role).Error)

	user := &models.User{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		Email:          "rbac-" + uniq + "@example.com",
		PasswordHash:   "x",
		FullName:       "RBAC Test",
		RoleID:         &role.ID,
		IsActive:       true,
		IsSuperAdmin:   superAdmin,
	}
	require.NoError(t, db.Create(user).Error)
	return user, org
}

func TestHas_GrantedPermission(t *testing.T) {
	e, db := newEngine(t, nil)
	user, org := seedUserWithPermissions(t, db, false, [2]string{models.ResourceContacts, models.ActionRead})

	assert.True(t, e.Has(user.ID, models.ResourceContacts, models.ActionRead, org.ID))
}

func TestHas_MissingPermissionIsDenied(t *testing.T) {
	e, db := newEngine(t, nil)
	user, org := seedUserWithPermissions(t, db, false, [2]string{models.ResourceContacts, models.ActionRead})

	// Held read, never granted delete.
	assert.False(t, e.Has(user.ID, models.ResourceContacts, models.ActionDelete, org.ID))
	// A resource the role does not mention at all.
	assert.False(t, e.Has(user.ID, models.ResourceUsers, models.ActionRead, org.ID))
}

func TestHas_SuperAdminHoldsEverything(t *testing.T) {
	e, db := newEngine(t, nil)
	user, org := seedUserWithPermissions(t, db, true) // no permissions granted

	assert.True(t, e.Has(user.ID, models.ResourceUsers, models.ActionDelete, org.ID))
}

func TestHas_UnknownUserIsDenied(t *testing.T) {
	e, _ := newEngine(t, nil)

	// A lookup failure must deny, not default to allow.
	assert.False(t, e.Has(uuid.New(), models.ResourceContacts, models.ActionRead))
}

func TestHasAny(t *testing.T) {
	e, db := newEngine(t, nil)
	user, _ := seedUserWithPermissions(t, db, false, [2]string{models.ResourceTags, models.ActionWrite})

	assert.True(t, e.HasAny(user.ID, "users:read", "tags:write"))
	assert.False(t, e.HasAny(user.ID, "users:read", "teams:read"))
}

func TestInvalidateUser_NotifiesAndRefetches(t *testing.T) {
	notifier := &recordingNotifier{}
	e, db := newEngine(t, notifier)
	user, org := seedUserWithPermissions(t, db, false, [2]string{models.ResourceContacts, models.ActionRead})

	// Warm the cache.
	require.True(t, e.Has(user.ID, models.ResourceContacts, models.ActionRead, org.ID))

	// Revoke by pointing the user at a role with nothing.
	empty := &models.CustomRole{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		Name:           "empty-" + uuid.New().String()[:8],
	}
	require.NoError(t, db.Create(empty).Error)
	require.NoError(t, db.Model(user).Update("role_id", empty.ID).Error)

	e.InvalidateUser(user.ID)

	// Without invalidation this would still answer true for the full TTL.
	assert.False(t, e.Has(user.ID, models.ResourceContacts, models.ActionRead, org.ID))
	assert.Contains(t, notifier.calls, user.ID, "clients must be told to re-fetch permissions")
}

func TestRolePermissions(t *testing.T) {
	e, db := newEngine(t, nil)
	user, _ := seedUserWithPermissions(t, db, false,
		[2]string{models.ResourceContacts, models.ActionRead},
		[2]string{models.ResourceContacts, models.ActionWrite},
	)

	perms, err := e.UserPermissions(user.ID)
	require.NoError(t, err)

	got, err := e.RolePermissions(perms.RoleID)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"contacts:read", "contacts:write"}, got)
}
