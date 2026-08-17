package database_test

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/config"
	"github.com/shridarpatil/whatomate/internal/database"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/test/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// cleanAll truncates every table so each test starts with a blank slate.
func cleanAll(t *testing.T, db *gorm.DB) {
	t.Helper()
	testutil.TruncateTables(db)
}

// --- SeedPermissionsAndRoles ---

func TestSeedPermissionsAndRoles_CreatesAllDefaultPermissions(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanAll(t, db)

	err := database.SeedPermissionsAndRoles(db)
	require.NoError(t, err)

	var count int64
	db.Model(&models.Permission{}).Count(&count)

	expected := len(models.DefaultPermissions())
	assert.Equal(t, int64(expected), count, "all default permissions should be created")
}

func TestSeedPermissionsAndRoles_Idempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanAll(t, db)

	// Seed twice
	require.NoError(t, database.SeedPermissionsAndRoles(db))
	require.NoError(t, database.SeedPermissionsAndRoles(db))

	var count int64
	db.Model(&models.Permission{}).Count(&count)

	expected := len(models.DefaultPermissions())
	assert.Equal(t, int64(expected), count, "idempotent: count should remain the same after two seeds")
}

func TestSeedPermissionsAndRoles_PermissionsHaveResourceAndAction(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanAll(t, db)

	require.NoError(t, database.SeedPermissionsAndRoles(db))

	var perms []models.Permission
	db.Find(&perms)

	for _, p := range perms {
		assert.NotEmpty(t, p.Resource, "permission resource must not be empty")
		assert.NotEmpty(t, p.Action, "permission action must not be empty")
		assert.NotEqual(t, uuid.Nil, p.ID, "permission ID must be set")
	}
}

// --- SeedSystemRolesForOrg ---

func TestSeedSystemRolesForOrg_CreatesThreeSystemRoles(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanAll(t, db)

	// Need permissions first
	require.NoError(t, database.SeedPermissionsAndRoles(db))

	org := models.Organization{
		BaseModel: models.BaseModel{ID: uuid.New()},
		Name:      "Test Org",
		Settings:  models.JSONB{},
	}
	require.NoError(t, db.Create(&org).Error)

	err := database.SeedSystemRolesForOrg(db, org.ID)
	require.NoError(t, err)

	var roles []models.CustomRole
	db.Where("organization_id = ? AND is_system = ?", org.ID, true).Find(&roles)
	assert.Len(t, roles, 3, "should create admin, manager, agent roles")

	names := make(map[string]bool)
	for _, r := range roles {
		names[r.Name] = true
	}
	assert.True(t, names["admin"], "admin role should exist")
	assert.True(t, names["manager"], "manager role should exist")
	assert.True(t, names["agent"], "agent role should exist")
}

func TestSeedSystemRolesForOrg_Idempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanAll(t, db)

	require.NoError(t, database.SeedPermissionsAndRoles(db))

	org := models.Organization{
		BaseModel: models.BaseModel{ID: uuid.New()},
		Name:      "Idempotent Org",
		Settings:  models.JSONB{},
	}
	require.NoError(t, db.Create(&org).Error)

	require.NoError(t, database.SeedSystemRolesForOrg(db, org.ID))
	require.NoError(t, database.SeedSystemRolesForOrg(db, org.ID))

	var count int64
	db.Model(&models.CustomRole{}).Where("organization_id = ? AND is_system = ?", org.ID, true).Count(&count)
	assert.Equal(t, int64(3), count, "idempotent: still exactly 3 system roles")
}

func TestSeedSystemRolesForOrg_AgentIsDefault(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanAll(t, db)

	require.NoError(t, database.SeedPermissionsAndRoles(db))

	org := models.Organization{
		BaseModel: models.BaseModel{ID: uuid.New()},
		Name:      "Default Role Org",
		Settings:  models.JSONB{},
	}
	require.NoError(t, db.Create(&org).Error)
	require.NoError(t, database.SeedSystemRolesForOrg(db, org.ID))

	var agentRole models.CustomRole
	err := db.Where("organization_id = ? AND name = ? AND is_system = ?", org.ID, "agent", true).First(&agentRole).Error
	require.NoError(t, err)
	assert.True(t, agentRole.IsDefault, "agent role should be the default role")
}

func TestSeedSystemRolesForOrg_AdminRoleHasAllPermissions(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanAll(t, db)

	require.NoError(t, database.SeedPermissionsAndRoles(db))

	org := models.Organization{
		BaseModel: models.BaseModel{ID: uuid.New()},
		Name:      "Admin Perms Org",
		Settings:  models.JSONB{},
	}
	require.NoError(t, db.Create(&org).Error)
	require.NoError(t, database.SeedSystemRolesForOrg(db, org.ID))

	var adminRole models.CustomRole
	err := db.Where("organization_id = ? AND name = ? AND is_system = ?", org.ID, "admin", true).First(&adminRole).Error
	require.NoError(t, err)

	// Load permissions through the association
	var perms []models.Permission
	err = db.Model(&adminRole).Association("Permissions").Find(&perms)
	require.NoError(t, err)

	totalPerms := len(models.DefaultPermissions())
	assert.Equal(t, totalPerms, len(perms), "admin role should have all permissions")
}

// --- CreateDefaultAdmin ---

func TestCreateDefaultAdmin_CreatesOrgAndUser(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanAll(t, db)

	cfg := &config.DefaultAdminConfig{
		Email:    "test-admin@example.com",
		Password: "testpassword123",
		FullName: "Test Admin",
	}

	err := database.CreateDefaultAdmin(db, cfg)
	require.NoError(t, err)

	// Verify user was created
	var user models.User
	err = db.Where("email = ?", cfg.Email).First(&user).Error
	require.NoError(t, err)
	assert.Equal(t, cfg.FullName, user.FullName)
	assert.True(t, user.IsActive)
	assert.True(t, user.IsSuperAdmin)
	assert.NotEmpty(t, user.PasswordHash)

	// Verify an organization was created
	var org models.Organization
	err = db.First(&org).Error
	require.NoError(t, err)
	assert.Equal(t, "Default Organization", org.Name)

	// Verify the user belongs to the organization
	assert.Equal(t, org.ID, user.OrganizationID)
}

func TestCreateDefaultAdmin_Idempotent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanAll(t, db)

	cfg := &config.DefaultAdminConfig{
		Email:    "idempotent-admin@example.com",
		Password: "pass123",
		FullName: "Idempotent Admin",
	}

	require.NoError(t, database.CreateDefaultAdmin(db, cfg))
	require.NoError(t, database.CreateDefaultAdmin(db, cfg))

	var count int64
	db.Model(&models.User{}).Where("email = ?", cfg.Email).Count(&count)
	assert.Equal(t, int64(1), count, "should not create duplicate admin")
}

func TestCreateDefaultAdmin_UsesExistingOrg(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanAll(t, db)

	// Pre-create an organization
	existingOrg := models.Organization{
		BaseModel: models.BaseModel{ID: uuid.New()},
		Name:      "Pre-existing Org",
		Settings:  models.JSONB{},
	}
	require.NoError(t, db.Create(&existingOrg).Error)

	cfg := &config.DefaultAdminConfig{
		Email:    "admin-existing-org@example.com",
		Password: "password",
		FullName: "Admin",
	}

	err := database.CreateDefaultAdmin(db, cfg)
	require.NoError(t, err)

	var user models.User
	err = db.Where("email = ?", cfg.Email).First(&user).Error
	require.NoError(t, err)
	assert.Equal(t, existingOrg.ID, user.OrganizationID, "admin should belong to existing org")

	// Should not have created a new org
	var orgCount int64
	db.Model(&models.Organization{}).Count(&orgCount)
	assert.Equal(t, int64(1), orgCount, "should reuse existing organization")
}

// --- MigrateWhatsAppAccountNames ---

// reintroduceLegacyAccountColumn puts a table back into its pre-v0.3.0 shape:
// the account stored as a NOT NULL name, with no whatsapp_account_id set.
func reintroduceLegacyAccountColumn(t *testing.T, db *gorm.DB, table, nameCol string) {
	t.Helper()
	require.NoError(t, db.Exec(fmt.Sprintf(
		`ALTER TABLE %q ADD COLUMN %q varchar(100) NOT NULL DEFAULT ''`, table, nameCol)).Error)
	t.Cleanup(func() {
		_ = db.Exec(fmt.Sprintf(`ALTER TABLE %q DROP COLUMN IF EXISTS %q`, table, nameCol)).Error
	})
}

// seedLegacyAccountFixture creates an org plus a WhatsApp account, and returns
// both so a test can link rows to it by name the way v0.2.0 did.
func seedLegacyAccountFixture(t *testing.T, db *gorm.DB, accountName string) (models.Organization, models.WhatsAppAccount) {
	t.Helper()
	org := models.Organization{
		BaseModel: models.BaseModel{ID: uuid.New()},
		Name:      "Legacy Upgrade Org",
		Slug:      "legacy-upgrade-" + uuid.NewString(),
		Settings:  models.JSONB{},
	}
	require.NoError(t, db.Create(&org).Error)

	account := models.WhatsAppAccount{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: org.ID,
		Name:           accountName,
		PhoneID:        "phone-" + accountName,
		BusinessID:     "business-" + accountName,
		AccessToken:    "token",
	}
	require.NoError(t, db.Create(&account).Error)
	return org, account
}

func TestMigrateWhatsAppAccountNames_BackfillsIDAndDropsLegacyColumn(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanAll(t, db)

	org, account := seedLegacyAccountFixture(t, db, "Legacy Account")
	reintroduceLegacyAccountColumn(t, db, "contacts", "whats_app_account")

	// A v0.2.0 row: linked by name, no ID.
	contactID := uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO contacts (id, created_at, updated_at, organization_id, phone_number, whats_app_account)
		VALUES (?, now(), now(), ?, '919999900001', ?)`,
		contactID, org.ID, account.Name).Error)

	require.NoError(t, database.MigrateWhatsAppAccountNames(db))

	assert.False(t, db.Migrator().HasColumn("contacts", "whats_app_account"),
		"legacy name column should be dropped so inserts stop hitting its NOT NULL")

	var linked string
	require.NoError(t, db.Raw(
		`SELECT whatsapp_account_id FROM contacts WHERE id = ?`, contactID).Scan(&linked).Error)
	assert.Equal(t, account.ID.String(), linked, "row should be relinked to the account by ID")
}

func TestMigrateWhatsAppAccountNames_EmptyNameBecomesOrgWideNull(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanAll(t, db)

	org, _ := seedLegacyAccountFixture(t, db, "Some Account")
	reintroduceLegacyAccountColumn(t, db, "contacts", "whats_app_account")

	// v0.2.0 spelled "organization-wide" as an empty name; v0.3.x spells it NULL.
	contactID := uuid.New()
	require.NoError(t, db.Exec(`
		INSERT INTO contacts (id, created_at, updated_at, organization_id, phone_number, whats_app_account)
		VALUES (?, now(), now(), ?, '919999900002', '')`, contactID, org.ID).Error)

	require.NoError(t, database.MigrateWhatsAppAccountNames(db))

	var linked *string
	require.NoError(t, db.Raw(
		`SELECT whatsapp_account_id FROM contacts WHERE id = ?`, contactID).Scan(&linked).Error)
	assert.Nil(t, linked, "an empty legacy name means org-wide, which is NULL now")
}

func TestMigrateWhatsAppAccountNames_UnresolvableNameAbortsWithoutDropping(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanAll(t, db)

	org, _ := seedLegacyAccountFixture(t, db, "Real Account")
	reintroduceLegacyAccountColumn(t, db, "contacts", "whats_app_account")

	require.NoError(t, db.Exec(`
		INSERT INTO contacts (id, created_at, updated_at, organization_id, phone_number, whats_app_account)
		VALUES (?, now(), now(), ?, '919999900003', 'Account That Never Existed')`,
		uuid.New(), org.ID).Error)

	err := database.MigrateWhatsAppAccountNames(db)
	require.Error(t, err, "an unresolvable name must not be silently discarded")
	assert.Contains(t, err.Error(), "contacts")
	assert.True(t, db.Migrator().HasColumn("contacts", "whats_app_account"),
		"the column must survive so the operator can still see and fix the data")
}

func TestMigrateWhatsAppAccountNames_DoesNotResolveAcrossOrganizations(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanAll(t, db)

	// whatsapp_accounts.name is globally unique, so the risk is not two orgs
	// sharing a name — it is resolving a name against some *other* org's account.
	_, accountA := seedLegacyAccountFixture(t, db, "Org A Account "+uuid.NewString())

	orgB := models.Organization{
		BaseModel: models.BaseModel{ID: uuid.New()},
		Name:      "Second Org",
		Slug:      "second-org-" + uuid.NewString(),
		Settings:  models.JSONB{},
	}
	require.NoError(t, db.Create(&orgB).Error)

	reintroduceLegacyAccountColumn(t, db, "contacts", "whats_app_account")
	require.NoError(t, db.Exec(`
		INSERT INTO contacts (id, created_at, updated_at, organization_id, phone_number, whats_app_account)
		VALUES (?, now(), now(), ?, '919999900004', ?)`,
		uuid.New(), orgB.ID, accountA.Name).Error)

	// Had the backfill ignored organization_id this would have linked and passed
	// silently, handing org B a pointer into org A's data.
	err := database.MigrateWhatsAppAccountNames(db)
	require.Error(t, err, "a name owned by another organization must not resolve")
	assert.Contains(t, err.Error(), "contacts")
}

func TestMigrateWhatsAppAccountNames_IdempotentOnCurrentSchema(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanAll(t, db)

	// No legacy columns present — this is the fresh-install path.
	require.NoError(t, database.MigrateWhatsAppAccountNames(db))
	require.NoError(t, database.MigrateWhatsAppAccountNames(db))
}
