package handlers

import (
	"github.com/google/uuid"
	"github.com/shridarpatil/whatomate/internal/models"
	"github.com/shridarpatil/whatomate/internal/privacy"
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// ListCallTransfers returns call transfers for the organization
func (a *App) ListCallTransfers(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceCallTransfers, models.ActionRead)
	if err != nil {
		return nil
	}

	pg := parsePagination(r)
	status := string(r.RequestCtx.QueryArgs().Peek("status"))

	query := a.DB.Where("call_transfers.organization_id = ?", orgID).
		Preload("Contact").
		Preload("Agent").
		Preload("InitiatingAgent").
		Preload("Team").
		Preload("CallLog").
		Order("call_transfers.created_at DESC")

	countQuery := a.DB.Model(&models.CallTransfer{}).Where("organization_id = ?", orgID)

	if status != "" {
		query = query.Where("call_transfers.status = ?", status)
		countQuery = countQuery.Where("status = ?", status)
	}

	var total int64
	countQuery.Count(&total)

	var transfers []models.CallTransfer
	if err := pg.Apply(query).Find(&transfers).Error; err != nil {
		a.Log.Error("Failed to fetch call transfers", "error", err)
		return a.sendError(r, internalError("Failed to fetch call transfers", err))
	}

	// Mask phone numbers if enabled for this organization
	if a.ShouldMaskPhoneNumbers(orgID) {
		for i := range transfers {
			transfers[i].CallerPhone = privacy.MaskPhoneNumber(transfers[i].CallerPhone)
			if transfers[i].Contact != nil {
				transfers[i].Contact.PhoneNumber = privacy.MaskPhoneNumber(transfers[i].Contact.PhoneNumber)
				transfers[i].Contact.ProfileName = privacy.MaskIfPhoneNumber(transfers[i].Contact.ProfileName)
			}
		}
	}

	return a.sendJSON(r, listEnvelope("call_transfers", transfers, total, pg))
}

// GetCallTransfer returns a single call transfer by ID
func (a *App) GetCallTransfer(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceCallTransfers, models.ActionRead)
	if err != nil {
		return nil
	}

	transferID, err := parsePathUUID(r, "id", "call transfer")
	if err != nil {
		return nil
	}

	var transfer models.CallTransfer
	if err := a.DB.Where("id = ? AND organization_id = ?", transferID, orgID).
		Preload("Contact").
		Preload("Agent").
		Preload("InitiatingAgent").
		Preload("Team").
		Preload("CallLog").
		First(&transfer).Error; err != nil {
		return a.sendError(r, notFound("Call transfer"))
	}

	if a.ShouldMaskPhoneNumbers(orgID) {
		transfer.CallerPhone = privacy.MaskPhoneNumber(transfer.CallerPhone)
		if transfer.Contact != nil {
			transfer.Contact.PhoneNumber = privacy.MaskPhoneNumber(transfer.Contact.PhoneNumber)
			transfer.Contact.ProfileName = privacy.MaskIfPhoneNumber(transfer.Contact.ProfileName)
		}
	}

	return a.sendJSON(r, transfer)
}

// ConnectCallTransfer handles an agent accepting a call transfer via WebRTC SDP exchange
func (a *App) ConnectCallTransfer(r *fastglue.Request) error {
	orgID, userID, err := a.requireAuth(r, models.ResourceCallTransfers, models.ActionWrite)
	if err != nil {
		return nil
	}

	transferID, err := parsePathUUID(r, "id", "call transfer")
	if err != nil {
		return nil
	}

	// Validate transfer exists and belongs to this org
	var transfer models.CallTransfer
	if err := a.DB.Where("id = ? AND organization_id = ?", transferID, orgID).
		First(&transfer).Error; err != nil {
		return a.sendError(r, notFound("Call transfer"))
	}

	if transfer.Status != models.CallTransferStatusWaiting {
		return a.sendError(r, conflict("Transfer is no longer waiting"))
	}

	// Check eligibility BEFORE atomically claiming the transfer.
	// This avoids claiming and then reverting, which creates a window where
	// the transfer is stuck as "connected" with no agent.

	// If transfer is directed to a specific agent (no team), reject other agents.
	// For team transfers with rotation, any team member can accept — the atomic
	// UPDATE below is the sole concurrency guard.
	if transfer.AgentID != nil && *transfer.AgentID != userID && transfer.TeamID == nil {
		return r.SendErrorEnvelope(fasthttp.StatusForbidden,
			"This transfer is directed to a specific agent", nil, "")
	}

	// If transfer has a team_id, check agent is a member (unless super admin)
	if transfer.TeamID != nil && !a.IsSuperAdmin(userID) {
		var memberCount int64
		a.DB.Table("team_members").
			Where("team_id = ? AND user_id = ? AND deleted_at IS NULL", transfer.TeamID, userID).
			Count(&memberCount)
		if memberCount == 0 {
			return a.sendError(r, forbidden("You are not a member of the target team"))
		}
	}

	// Atomically claim the transfer — concurrent accepts are rejected
	res := a.DB.Model(&models.CallTransfer{}).
		Where("id = ? AND status = ?", transferID, models.CallTransferStatusWaiting).
		Update("status", models.CallTransferStatusConnected)
	if res.RowsAffected == 0 {
		return a.sendError(r, conflict("Transfer was already accepted by another agent"))
	}

	// Parse SDP offer from body
	var req struct {
		SDPOffer string `json:"sdp_offer"`
	}
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}
	if req.SDPOffer == "" {
		return a.sendError(r, invalidRequest("sdp_offer is required"))
	}

	if err := a.requireCallingEnabled(r, orgID); err != nil {
		return nil
	}

	sdpAnswer, err := a.CallManager.ConnectAgentToTransfer(transferID, userID, req.SDPOffer)
	if err != nil {
		// Revert DB status so another agent can try
		a.DB.Model(&models.CallTransfer{}).
			Where("id = ? AND status = ?", transferID, models.CallTransferStatusConnected).
			Update("status", models.CallTransferStatusWaiting)
		return a.sendError(r, internalError("Failed to connect to the call", err))
	}

	return a.sendJSON(r, map[string]string{
		"sdp_answer": sdpAnswer,
	})
}

// HangupCallTransfer ends a connected call transfer
func (a *App) HangupCallTransfer(r *fastglue.Request) error {
	orgID, _, err := a.requireAuth(r, models.ResourceCallTransfers, models.ActionWrite)
	if err != nil {
		return nil
	}

	transferID, err := parsePathUUID(r, "id", "call transfer")
	if err != nil {
		return nil
	}

	// Validate transfer belongs to this org
	var transfer models.CallTransfer
	if err := a.DB.Where("id = ? AND organization_id = ?", transferID, orgID).
		First(&transfer).Error; err != nil {
		return a.sendError(r, notFound("Call transfer"))
	}

	if a.CallManager == nil {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable, "Calling is not enabled", nil, "")
	}

	a.CallManager.EndTransfer(transferID)

	// Mark the call as disconnected by agent
	a.DB.Model(&models.CallLog{}).
		Where("id = ?", transfer.CallLogID).
		Update("disconnected_by", models.DisconnectedByAgent)

	return a.sendJSON(r, map[string]string{
		"status": "completed",
	})
}

// HoldCall puts an active call on hold and plays hold music to the caller.
func (a *App) HoldCall(r *fastglue.Request) error {
	_, _, err := a.requireAuth(r, models.ResourceCallTransfers, models.ActionWrite)
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

	if err := a.CallManager.HoldCall(callLogID); err != nil {
		return a.sendError(r, invalidRequest(err.Error()))
	}

	return a.sendJSON(r, map[string]string{"status": "on_hold"})
}

// ResumeCall takes an active call off hold and restores the audio bridge.
func (a *App) ResumeCall(r *fastglue.Request) error {
	_, _, err := a.requireAuth(r, models.ResourceCallTransfers, models.ActionWrite)
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

	if err := a.CallManager.ResumeCall(callLogID); err != nil {
		return a.sendError(r, invalidRequest(err.Error()))
	}

	return a.sendJSON(r, map[string]string{"status": "connected"})
}

// InitiateAgentTransfer allows a connected agent to transfer their active call to another team/agent
func (a *App) InitiateAgentTransfer(r *fastglue.Request) error {
	orgID, userID, err := a.requireAuth(r, models.ResourceCallTransfers, models.ActionWrite)
	if err != nil {
		return nil
	}
	if err := a.requireCallingEnabled(r, orgID); err != nil {
		return nil
	}

	var req struct {
		CallLogID string `json:"call_log_id"`
		TeamID    string `json:"team_id"`
		AgentID   string `json:"agent_id"`
	}
	if err := a.decodeRequest(r, &req); err != nil {
		return nil
	}

	if req.CallLogID == "" || req.TeamID == "" {
		return a.sendError(r, invalidRequest("call_log_id and team_id are required"))
	}

	callLogID, err := uuid.Parse(req.CallLogID)
	if err != nil {
		return a.sendError(r, invalidRequest("Invalid call_log_id"))
	}

	teamID, err := uuid.Parse(req.TeamID)
	if err != nil {
		return a.sendError(r, invalidRequest("Invalid team_id"))
	}

	// Verify team belongs to this org
	var teamCount int64
	a.DB.Model(&models.Team{}).Where("id = ? AND organization_id = ?", teamID, orgID).Count(&teamCount)
	if teamCount == 0 {
		return a.sendError(r, notFound("Team"))
	}

	var targetAgentID *uuid.UUID
	if req.AgentID != "" {
		agentID, err := uuid.Parse(req.AgentID)
		if err != nil {
			return a.sendError(r, invalidRequest("Invalid agent_id"))
		}
		// Verify agent is a member of the team
		var memberCount int64
		a.DB.Table("team_members").
			Where("team_id = ? AND user_id = ? AND deleted_at IS NULL", teamID, agentID).
			Count(&memberCount)
		if memberCount == 0 {
			return a.sendError(r, invalidRequest("Agent is not a member of the specified team"))
		}
		targetAgentID = &agentID
	}

	if a.CallManager == nil {
		return r.SendErrorEnvelope(fasthttp.StatusServiceUnavailable, "Calling is not enabled", nil, "")
	}

	if err := a.CallManager.InitiateAgentTransfer(callLogID, userID, &teamID, targetAgentID); err != nil {
		return a.sendError(r, internalError("Failed to initiate transfer", err))
	}

	return a.sendJSON(r, map[string]string{
		"status": "transferring",
	})
}
