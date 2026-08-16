package handlers

import (
	"github.com/zerodha/fastglue"
)

// Stub handlers - not yet implemented

// MarkMessageRead is not yet implemented and always returns 501.
func (a *App) MarkMessageRead(r *fastglue.Request) error {
	return a.sendError(r, invalidRequest("Not implemented yet"))
}

// GetMessageAnalytics is not yet implemented and always returns 501.
func (a *App) GetMessageAnalytics(r *fastglue.Request) error {
	return a.sendError(r, invalidRequest("Not implemented yet"))
}

func (a *App) GetChatbotAnalytics(r *fastglue.Request) error {
	return a.sendError(r, invalidRequest("Not implemented yet"))
}
