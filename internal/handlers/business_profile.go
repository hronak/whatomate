package handlers

import (
	"context"
	"time"

	"github.com/shridarpatil/whatomate/pkg/whatsapp"
	"github.com/zerodha/fastglue"
)

// businessProfileHTTPTimeout bounds calls to Meta's profile endpoints. We use a
// detached context (not r.RequestCtx) so a client disconnect mid-call doesn't
// cancel an in-flight Meta update — and so that fasthttp.RequestCtx, which has
// no ctx.Done() implementation outside the server, doesn't crash the http client.
const businessProfileHTTPTimeout = 30 * time.Second

// GetBusinessProfile returns the business profile for a WhatsApp account
func (a *App) GetBusinessProfile(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return a.sendError(r, unauthorized("Unauthorized"))
	}

	id, err := parsePathUUID(r, "id", "account")
	if err != nil {
		return nil
	}

	account, err := a.resolveWhatsAppAccountByID(r, id, orgID)
	if err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), businessProfileHTTPTimeout)
	defer cancel()

	profile, err := a.WhatsApp.GetBusinessProfile(ctx, a.toWhatsAppAccount(account))
	if err != nil {
		a.Log.Error("Failed to get business profile", "error", err)
		return a.sendError(r, internalError("Failed to get business profile", err))
	}

	return r.SendEnvelope(profile)
}

// UpdateBusinessProfile updates the business profile for a WhatsApp account
func (a *App) UpdateBusinessProfile(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return a.sendError(r, unauthorized("Unauthorized"))
	}

	id, err := parsePathUUID(r, "id", "account")
	if err != nil {
		return nil
	}

	account, err := a.resolveWhatsAppAccountByID(r, id, orgID)
	if err != nil {
		return nil
	}

	var input whatsapp.BusinessProfileInput
	if err := a.decodeRequest(r, &input); err != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), businessProfileHTTPTimeout)
	defer cancel()
	waAccount := a.toWhatsAppAccount(account)

	if err := a.WhatsApp.UpdateBusinessProfile(ctx, waAccount, input); err != nil {
		a.Log.Error("Failed to update business profile", "error", err)
		return a.sendError(r, internalError("Failed to update business profile", err))
	}

	// Re-fetch to ensure we have the latest state
	profile, err := a.WhatsApp.GetBusinessProfile(ctx, waAccount)
	if err != nil {
		// If re-fetch fails, just return success message
		return r.SendEnvelope(map[string]string{"message": "Profile updated successfully"})
	}

	return r.SendEnvelope(profile)
}

// UpdateProfilePicture handles the profile picture upload
func (a *App) UpdateProfilePicture(r *fastglue.Request) error {
	orgID, err := a.getOrgID(r)
	if err != nil {
		return a.sendError(r, unauthorized("Unauthorized"))
	}

	id, err := parsePathUUID(r, "id", "account")
	if err != nil {
		return nil
	}

	account, err := a.resolveWhatsAppAccountByID(r, id, orgID)
	if err != nil {
		return nil
	}

	// 1. Get the file from request
	fileHeader, err := r.RequestCtx.FormFile("file")
	if err != nil {
		return a.sendError(r, invalidRequest("Missing file"))
	}

	// 2. Open and read file
	file, err := fileHeader.Open()
	if err != nil {
		a.Log.Error("Failed to open file", "error", err)
		return a.sendError(r, internalError("Failed to open file", err))
	}
	defer file.Close() //nolint:errcheck

	fileSize := fileHeader.Size
	fileContent := make([]byte, fileSize)
	_, err = file.Read(fileContent)
	if err != nil {
		a.Log.Error("Failed to read file", "error", err)
		return a.sendError(r, internalError("Failed to read file", err))
	}

	// Use a longer timeout for upload — profile pictures can be a few MB.
	ctx, cancel := context.WithTimeout(context.Background(), 2*businessProfileHTTPTimeout)
	defer cancel()
	waAccount := a.toWhatsAppAccount(account)

	// Upload to Meta to get handle
	handle, err := a.WhatsApp.UploadProfilePicture(ctx, waAccount, fileContent, fileHeader.Header.Get("Content-Type"))
	if err != nil {
		a.Log.Error("Failed to upload profile picture", "error", err)
		return a.sendError(r, internalError("Failed to upload profile picture", err))
	}

	// Update Business Profile with the handle
	input := whatsapp.BusinessProfileInput{
		MessagingProduct:     "whatsapp",
		ProfilePictureHandle: handle,
	}

	err = a.WhatsApp.UpdateBusinessProfile(ctx, waAccount, input)

	if err != nil {
		a.Log.Error("Failed to update profile request", "error", err)
		return a.sendError(r, internalError("Uploaded but failed to set profile picture", err))
	}

	return r.SendEnvelope(map[string]string{
		"message": "Profile picture updated successfully",
		"handle":  handle,
	})
}
