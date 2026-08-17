package handlers

import (
	"time"

	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// InitiateOutgoingCall handles POST /api/calls/outgoing
// Lets an agent start a voice call to a WhatsApp consumer.
func (a *App) InitiateOutgoingCall(r *fastglue.Request) error {
	orgID, userID, err := a.requireAuth(r, models.ResourceOutgoingCalls, models.ActionWrite)
	if err != nil {
		return nil
	}

	var req struct {
		ContactID         string `json:"contact_id"`
		WhatsAppAccountID string `json:"whatsapp_account_id"`
		SDPOffer          string `json:"sdp_offer"`
	}
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	if req.ContactID == "" || req.WhatsAppAccountID == "" || req.SDPOffer == "" {
		return a.sendError(r, invalidRequest("contact_id, whatsapp_account_id, and sdp_offer are required"))
	}

	if err := a.requireCallingEnabled(r, orgID); err != nil {
		return nil
	}

	// Look up account
	var account models.WhatsAppAccount
	if err := a.DB.Where("organization_id = ? AND name = ?", orgID, req.WhatsAppAccountID).
		First(&account).Error; err != nil {
		return a.sendError(r, notFound("WhatsApp account"))
	}

	// Look up contact by ID
	contactID, parseErr := uuid.Parse(req.ContactID)
	if parseErr != nil {
		return a.sendError(r, invalidRequest("Invalid contact_id"))
	}

	var contact models.Contact
	if err := a.DB.Where("id = ? AND organization_id = ?", contactID, orgID).
		First(&contact).Error; err != nil {
		return a.sendError(r, notFound("Contact"))
	}

	waAccount := account.ToWAAccount()

	callLogID, sdpAnswer, err := a.CallManager.InitiateOutgoingCall(
		orgID, userID, contact.ID,
		contact.PhoneNumber, func(s string) uuid.UUID { u, _ := uuid.Parse(s); return u }(req.WhatsAppAccountID),
		waAccount, req.SDPOffer,
	)
	if err != nil {
		return a.sendError(r, internalError("Failed to initiate call", err))
	}

	return a.sendJSON(r, map[string]string{
		"call_log_id": callLogID.String(),
		"sdp_answer":  sdpAnswer,
	})
}

// HangupOutgoingCall handles POST /api/calls/outgoing/{id}/hangup
func (a *App) HangupOutgoingCall(r *fastglue.Request) error {
	_, userID, err := a.requireAuth(r, models.ResourceOutgoingCalls, models.ActionWrite)
	if err != nil {
		return nil
	}

	callLogID, err := parsePathUUID(r, "id", "call log")
	if err != nil {
		return nil
	}

	if a.CallManager == nil {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable, "Calling is not enabled", nil, "")
	}

	if err := a.CallManager.HangupOutgoingCall(callLogID, userID); err != nil {
		return a.sendError(r, invalidRequest(err.Error()))
	}

	// Mark the call as disconnected by agent
	a.DB.Model(&models.CallLog{}).
		Where("id = ?", callLogID).
		Update("disconnected_by", models.DisconnectedByAgent)

	return a.sendJSON(r, map[string]string{"status": "ok"})
}

// SendCallPermissionRequest handles POST /api/calls/permission-request
func (a *App) SendCallPermissionRequest(r *fastglue.Request) error {
	orgID, userID, err := a.requireAuth(r, models.ResourceOutgoingCalls, models.ActionWrite)
	if err != nil {
		return nil
	}

	var req struct {
		ContactID         string `json:"contact_id"`
		WhatsAppAccountID string `json:"whatsapp_account_id"`
	}
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	if req.ContactID == "" || req.WhatsAppAccountID == "" {
		return a.sendError(r, invalidRequest("contact_id and whatsapp_account are required"))
	}

	if err := a.requireCallingEnabled(r, orgID); err != nil {
		return nil
	}

	contactID, parseErr := uuid.Parse(req.ContactID)
	if parseErr != nil {
		return a.sendError(r, invalidRequest("Invalid contact_id"))
	}

	// Verify contact exists
	var contact models.Contact
	if err := a.DB.Where("id = ? AND organization_id = ?", contactID, orgID).First(&contact).Error; err != nil {
		return a.sendError(r, notFound("Contact"))
	}

	// Look up account
	var account models.WhatsAppAccount
	if err := a.DB.Where("organization_id = ? AND name = ?", orgID, req.WhatsAppAccountID).
		First(&account).Error; err != nil {
		return a.sendError(r, notFound("WhatsApp account"))
	}

	waAccount := account.ToWAAccount()

	// Send permission request via WhatsApp Messages API
	ctx := r.RequestCtx
	rcpt := whatsapp.Recipient{Phone: contact.PhoneNumber, BSUID: contact.BSUID}
	messageID, err := a.WhatsApp.SendCallPermissionRequest(ctx, waAccount, rcpt, "")
	if err != nil {
		a.Log.Error("Failed to send call permission request", "error", err)
		return a.sendError(r, internalError("Failed to send permission request", err))
	}

	// Create CallPermission record
	permission := models.CallPermission{
		BaseModel:         models.BaseModel{ID: uuid.New()},
		OrganizationID:    orgID,
		ContactID:         contactID,
		WhatsAppAccountID: func(s string) *uuid.UUID { u, _ := uuid.Parse(s); return &u }(req.WhatsAppAccountID),
		Status:            models.CallPermissionPending,
		MessageID:         messageID,
		RequestedByID:     &userID,
	}
	if err := a.DB.Create(&permission).Error; err != nil {
		a.Log.Error("Failed to create call permission record", "error", err)
		return a.sendError(r, internalError("Failed to save permission", err))
	}

	return a.sendJSON(r, map[string]string{
		"permission_id": permission.ID.String(),
	})
}

// GetICEServers handles GET /api/calls/ice-servers
// Returns the configured ICE (STUN/TURN) servers for the frontend to use in WebRTC peer connections.
func (a *App) GetICEServers(r *fastglue.Request) error {
	_, _, err := a.getOrgAndUserID(r)
	if err != nil {
		return a.sendError(r, unauthorized("Unauthorized"))
	}

	type iceServer struct {
		URLs       []string `json:"urls"`
		Username   string   `json:"username,omitempty"`
		Credential string   `json:"credential,omitempty"`
	}

	now := time.Now()
	servers := make([]iceServer, 0, len(a.Config.Calling.ICEServers))
	for _, s := range a.Config.Calling.ICEServers {
		username, credential := s.ResolveCredentials(now)
		servers = append(servers, iceServer{
			URLs:       s.URLs,
			Username:   username,
			Credential: credential,
		})
	}

	return a.sendJSON(r, map[string]any{
		"ice_servers": servers,
	})
}

// GetCallPermission handles GET /api/calls/permission/{contactId}?whatsapp_account_id=X
// Checks call permission state directly via WhatsApp API.
func (a *App) GetCallPermission(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceOutgoingCalls, models.ActionRead)
	if err != nil {
		return nil
	}

	contactID, err := parsePathUUID(r, "contactId", "contact")
	if err != nil {
		return nil
	}

	accountName := string(r.RequestCtx.QueryArgs().Peek("whatsapp_account"))
	if accountName == "" {
		return a.sendError(r, invalidRequest("whatsapp_account query param is required"))
	}

	// Look up contact
	var contact models.Contact
	if err := a.DB.Where("id = ? AND organization_id = ?", contactID, orgID).First(&contact).Error; err != nil {
		return a.sendError(r, notFound("Contact"))
	}

	// Look up WhatsApp account
	var account models.WhatsAppAccount
	if err := a.DB.Where("organization_id = ? AND name = ?", orgID, accountName).First(&account).Error; err != nil {
		return a.sendError(r, notFound("WhatsApp account"))
	}

	waAccount := account.ToWAAccount()

	// Check permission via WhatsApp API
	ctx := r.RequestCtx
	status, err := a.WhatsApp.GetCallPermission(ctx, waAccount, contact.PhoneNumber)
	if err != nil {
		a.Log.Error("Failed to check call permission via API", "error", err, "phone", contact.PhoneNumber)
		return a.sendError(r, internalError("Failed to check permission", err))
	}

	a.Log.Info("Call permission check result", "contact_id", contactID, "phone", contact.PhoneNumber, "status", status)

	return a.sendJSON(r, map[string]string{
		"status": status,
	})
}
