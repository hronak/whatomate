package contactutil

import (
	"fmt"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"gorm.io/gorm"
)

// GetOrCreateContact finds or creates a contact for the given phone number.
// Merges behaviors from both handler and worker implementations:
//   - Normalizes phone (strips leading "+")
//   - Tries both normalized and +prefix forms
//   - Updates profile name if changed
//   - Handles race conditions on create by re-fetching
//   - Restores soft-deleted contacts if found
//
// Returns the contact, whether it was newly created, and any error.
func GetOrCreateContact(db *gorm.DB, orgID uuid.UUID, phoneNumber, profileName string) (*models.Contact, bool, error) {
	// Normalize phone number (remove + prefix if present)
	normalizedPhone := phoneNumber
	if len(normalizedPhone) > 0 && normalizedPhone[0] == '+' {
		normalizedPhone = normalizedPhone[1:]
	}

	// Try to find existing contact with normalized phone (including soft-deleted)
	var contact models.Contact
	if err := db.Unscoped().Where("organization_id = ? AND phone_number = ?", orgID, normalizedPhone).First(&contact).Error; err == nil {
		// Restore if soft-deleted. A failed restore must not be swallowed:
		// the caller would treat the returned contact as live while every
		// subsequent scoped query keeps filtering it out.
		if contact.DeletedAt.Valid {
			if err := db.Unscoped().Model(&contact).Update("deleted_at", nil).Error; err != nil {
				return nil, false, fmt.Errorf("failed to restore soft-deleted contact: %w", err)
			}
			contact.DeletedAt.Valid = false
		}
		// Update profile name if changed. Best-effort: a stale display name is
		// not worth failing message ingest over.
		if profileName != "" && contact.ProfileName != profileName {
			_ = db.Model(&contact).Update("profile_name", profileName).Error
		}
		return &contact, false, nil
	}

	// Also try with + prefix (contacts may have been stored with it)
	if err := db.Unscoped().Where("organization_id = ? AND phone_number = ?", orgID, "+"+normalizedPhone).First(&contact).Error; err == nil {
		// Restore if soft-deleted (see above).
		if contact.DeletedAt.Valid {
			if err := db.Unscoped().Model(&contact).Update("deleted_at", nil).Error; err != nil {
				return nil, false, fmt.Errorf("failed to restore soft-deleted contact: %w", err)
			}
			contact.DeletedAt.Valid = false
		}
		if profileName != "" && contact.ProfileName != profileName {
			_ = db.Model(&contact).Update("profile_name", profileName).Error
		}
		return &contact, false, nil
	}

	// Create new contact
	contact = models.Contact{
		BaseModel:      models.BaseModel{ID: uuid.New()},
		OrganizationID: orgID,
		PhoneNumber:    normalizedPhone,
		ProfileName:    profileName,
	}
	if err := db.Create(&contact).Error; err != nil {
		// Race condition: another goroutine may have created the contact
		if err2 := db.Unscoped().Where("organization_id = ? AND phone_number = ?", orgID, normalizedPhone).First(&contact).Error; err2 == nil {
			// Restore if soft-deleted (see above).
			if contact.DeletedAt.Valid {
				if err := db.Unscoped().Model(&contact).Update("deleted_at", nil).Error; err != nil {
					return nil, false, fmt.Errorf("failed to restore soft-deleted contact: %w", err)
				}
				contact.DeletedAt.Valid = false
			}
			return &contact, false, nil
		}
		return nil, false, err
	}
	return &contact, true, nil
}

// FindContact finds a contact for the given phone number with both forms (normalized and +prefix).
func FindContact(db *gorm.DB, orgID uuid.UUID, phoneNumber string) (*models.Contact, error) {
	normalizedPhone := phoneNumber
	if len(normalizedPhone) > 0 && normalizedPhone[0] == '+' {
		normalizedPhone = normalizedPhone[1:]
	}

	var contact models.Contact
	if err := db.Where("organization_id = ? AND phone_number = ?", orgID, normalizedPhone).First(&contact).Error; err == nil {
		return &contact, nil
	}

	if err := db.Where("organization_id = ? AND phone_number = ?", orgID, "+"+normalizedPhone).First(&contact).Error; err == nil {
		return &contact, nil
	}

	return nil, gorm.ErrRecordNotFound
}
