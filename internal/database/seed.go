package database

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/shridarpatil/whatomate/internal/models"
)

// seedMarker tags every row this file creates, in the metadata/description
// fields the app already carries. It is what makes seeding idempotent: a second
// run finds the marker and does nothing rather than creating a second set.
const seedMarker = "whatomate:demo-seed"

// SeedDemoData populates an organization with a small amount of sample content
// so a freshly installed instance opens onto a usable UI instead of a set of
// empty tables.
//
// It is opt-in (`whatomate install -seed`) and never runs as part of plain
// migrations, because the data is fictional and has no place in a real deploy.
// Everything it writes is marked with seedMarker, so calling it repeatedly is a
// no-op and the demo rows stay identifiable.
//
// Contacts and tags are always created. A starter chatbot flow is created only
// when the organization already has a WhatsApp account to attach it to —
// ChatbotFlow.WhatsAppAccountID is NOT NULL, and inventing a fake account would
// mean a flow that silently fails to send and an account row that looks real in
// Settings. Connect an account first, then re-run with -seed to get the flow.
func SeedDemoData(db *gorm.DB) (summary string, err error) {
	var org models.Organization
	if err := db.First(&org).Error; err != nil {
		return "", fmt.Errorf("no organization to seed (run migrations first): %w", err)
	}

	// Already seeded? The contacts are the marker we look for, since they are
	// the one thing seeded unconditionally.
	var seeded int64
	if err := db.Model(&models.Contact{}).
		Where("organization_id = ? AND metadata->>'source' = ?", org.ID, seedMarker).
		Count(&seeded).Error; err != nil {
		return "", fmt.Errorf("check existing demo data: %w", err)
	}
	if seeded > 0 {
		return "demo data already present, nothing to do", nil
	}

	if err := seedTags(db, org.ID); err != nil {
		return "", err
	}

	contacts, err := seedContacts(db, org.ID)
	if err != nil {
		return "", err
	}

	flowNote := ", no WhatsApp account yet so the starter flow was skipped"
	created, err := seedStarterFlow(db, org.ID)
	if err != nil {
		return "", err
	}
	if created {
		flowNote = ", 1 starter chatbot flow"
	}

	return fmt.Sprintf("seeded %d contacts, %d tags%s (organization %q)",
		contacts, len(demoTags), flowNote, org.Name), nil
}

var demoTags = []struct{ Name, Color string }{
	{"vip", "purple"},
	{"support", "blue"},
	{"lead", "green"},
}

func seedTags(db *gorm.DB, orgID uuid.UUID) error {
	for _, t := range demoTags {
		tag := models.Tag{OrganizationID: orgID, Name: t.Name, Color: t.Color}
		// Tags are keyed on (organization_id, name); leave any existing one alone.
		if err := db.Where("organization_id = ? AND name = ?", orgID, t.Name).
			FirstOrCreate(&tag).Error; err != nil {
			return fmt.Errorf("seed tag %q: %w", t.Name, err)
		}
	}
	return nil
}

func seedContacts(db *gorm.DB, orgID uuid.UUID) (int, error) {
	// Numbers come from the +1 555-01xx range reserved for fiction, so they
	// can never reach a real handset even if something tries to send.
	demo := []struct {
		Phone, Name, Preview string
		Tags                 []string
	}{
		{"+15550100", "Ada Lovelace", "Hi! Is the enterprise plan available?", []string{"lead", "vip"}},
		{"+15550101", "Grace Hopper", "Thanks, that fixed it.", []string{"support"}},
		{"+15550102", "Alan Turing", "Can someone call me back today?", []string{"support"}},
		{"+15550103", "Katherine Johnson", "Order #4417 hasn't arrived yet.", []string{"vip"}},
		{"+15550104", "Radia Perlman", "Please add me to the newsletter.", []string{"lead"}},
	}

	now := time.Now()
	for i, d := range demo {
		// Stagger the timestamps so the conversation list has a sensible order
		// rather than five contacts sharing one instant.
		seenAt := now.Add(-time.Duration(i) * 37 * time.Minute)

		tags := make(models.JSONBArray, 0, len(d.Tags))
		for _, t := range d.Tags {
			tags = append(tags, t)
		}

		c := models.Contact{
			OrganizationID:     orgID,
			PhoneNumber:        d.Phone,
			ProfileName:        d.Name,
			LastMessageAt:      &seenAt,
			LastInboundAt:      &seenAt,
			LastMessagePreview: d.Preview,
			IsRead:             i%2 == 0,
			Tags:               tags,
			Metadata:           models.JSONB{"source": seedMarker},
		}
		if err := db.Create(&c).Error; err != nil {
			return 0, fmt.Errorf("seed contact %s: %w", d.Phone, err)
		}
	}
	return len(demo), nil
}

// seedStarterFlow creates a minimal but complete v2 graph — greet, offer two
// buttons, answer, end — so the flow builder opens onto a working example
// rather than a blank canvas. Reports false when there is no WhatsApp account
// to attach it to.
func seedStarterFlow(db *gorm.DB, orgID uuid.UUID) (bool, error) {
	var account models.WhatsAppAccount
	if err := db.Where("organization_id = ?", orgID).First(&account).Error; err != nil {
		return false, nil //nolint:nilerr // absence is an expected, non-fatal case
	}

	graph := models.JSONB{
		"version":    2,
		"entry_node": "start",
		"nodes": []any{
			map[string]any{
				"id": "start", "type": "start", "label": "Start",
				"position": map[string]any{"x": 0, "y": 0},
				"config":   map[string]any{},
			},
			map[string]any{
				"id": "greet", "type": "message", "label": "Greeting",
				"position": map[string]any{"x": 0, "y": 120},
				"config":   map[string]any{"message": "Hi {{profile_name}}! Thanks for reaching out."},
			},
			map[string]any{
				"id": "menu", "type": "buttons", "label": "Main menu",
				"position": map[string]any{"x": 0, "y": 240},
				"config": map[string]any{
					"body": "What can we help you with?",
					"buttons": []any{
						map[string]any{"id": "sales", "title": "Sales"},
						map[string]any{"id": "support", "title": "Support"},
					},
				},
			},
			map[string]any{
				"id": "sales", "type": "message", "label": "Sales reply",
				"position": map[string]any{"x": -160, "y": 360},
				"config":   map[string]any{"message": "Great — someone from sales will be with you shortly."},
			},
			map[string]any{
				"id": "support", "type": "message", "label": "Support reply",
				"position": map[string]any{"x": 160, "y": 360},
				"config":   map[string]any{"message": "Sorry about that. Describe the issue and we'll take a look."},
			},
			map[string]any{
				"id": "end", "type": "end", "label": "End",
				"position": map[string]any{"x": 0, "y": 480},
				"config":   map[string]any{},
			},
		},
		"edges": []any{
			map[string]any{"from": "start", "to": "greet", "condition": ""},
			map[string]any{"from": "greet", "to": "menu", "condition": ""},
			map[string]any{"from": "menu", "to": "sales", "condition": "button:sales"},
			map[string]any{"from": "menu", "to": "support", "condition": "button:support"},
			map[string]any{"from": "sales", "to": "end", "condition": ""},
			map[string]any{"from": "support", "to": "end", "condition": ""},
		},
	}

	flow := models.ChatbotFlow{
		OrganizationID:    orgID,
		WhatsAppAccountID: &account.ID,
		Name:              "Demo: welcome menu",
		Description:       "Sample flow created by `whatomate install -seed` (" + seedMarker + "). Safe to delete.",
		TriggerKeywords:   models.StringArray{"hi", "hello", "menu"},
		CancelKeywords:    models.StringArray{"cancel", "stop"},
		Graph:             graph,
		PanelConfig:       models.JSONB{},
	}
	if err := db.Create(&flow).Error; err != nil {
		return false, fmt.Errorf("seed starter flow: %w", err)
	}

	// Disable it in a second statement rather than setting IsEnabled: false
	// above. ChatbotFlow.IsEnabled is declared `default:true`, and GORM omits
	// false — a zero value — from the INSERT, so Postgres fills in the default
	// and the flow comes up live. The account this attaches to may be a real
	// connected number, and a demo flow that starts answering inbound messages
	// the moment you seed is not a demo.
	if err := db.Model(&flow).Update("is_enabled", false).Error; err != nil {
		return false, fmt.Errorf("disable starter flow: %w", err)
	}
	return true, nil
}
