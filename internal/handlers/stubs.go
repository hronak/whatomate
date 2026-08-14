package handlers

import (
	"github.com/valyala/fasthttp"
	"github.com/zerodha/fastglue"
)

// Stub handlers - not yet implemented

// MarkMessageRead is not yet implemented and always returns 501.
func (a *App) MarkMessageRead(r *fastglue.Request) error {
	return r.SendErrorEnvelope(fasthttp.StatusNotImplemented, "Not implemented yet", nil, "")
}

// GetMessageAnalytics is not yet implemented and always returns 501.
func (a *App) GetMessageAnalytics(r *fastglue.Request) error {
	return r.SendErrorEnvelope(fasthttp.StatusNotImplemented, "Not implemented yet", nil, "")
}

func (a *App) GetChatbotAnalytics(r *fastglue.Request) error {
	return r.SendErrorEnvelope(fasthttp.StatusNotImplemented, "Not implemented yet", nil, "")
}
