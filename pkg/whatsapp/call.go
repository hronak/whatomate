package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// buildCallsURL builds the calls endpoint URL for the WhatsApp Calling API
func (c *Client) buildCallsURL(account *Account) string {
	return fmt.Sprintf("%s/%s/%s/calls", c.getBaseURL(), account.APIVersion, account.PhoneID)
}

// PreAcceptCall sends the SDP answer to Meta as a pre-accept signal.
// Per the WhatsApp Business Calling API, pre_accept requires the session object
// with the SDP answer to keep the call alive while WebRTC is finalized.
func (c *Client) PreAcceptCall(ctx context.Context, account *Account, callID, sdpAnswer string) error {
	payload := newCallRequest("pre_accept", callID)
	payload.Session = &callSession{SDPType: "answer", SDP: sdpAnswer}

	url := c.buildCallsURL(account)
	c.log.Debug("Pre-accepting call", "call_id", callID)

	_, err := c.doRequest(ctx, http.MethodPost, url, payload, account.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to pre-accept call: %w", err)
	}

	c.log.Debug("Call pre-accepted", "call_id", callID)
	return nil
}

// AcceptCall accepts an incoming call by sending our SDP answer.
// Per the WhatsApp Business Calling API, accept uses the same session object format.
// The API returns { success: true } on success.
func (c *Client) AcceptCall(ctx context.Context, account *Account, callID, sdpAnswer string) error {
	payload := newCallRequest("accept", callID)
	payload.Session = &callSession{SDPType: "answer", SDP: sdpAnswer}

	url := c.buildCallsURL(account)
	c.log.Debug("Accepting call", "call_id", callID)

	_, err := c.doRequest(ctx, http.MethodPost, url, payload, account.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to accept call: %w", err)
	}

	c.log.Debug("Call accepted", "call_id", callID)
	return nil
}

// RejectCall rejects an incoming call.
func (c *Client) RejectCall(ctx context.Context, account *Account, callID string) error {
	payload := newCallRequest("reject", callID)

	url := c.buildCallsURL(account)
	c.log.Debug("Rejecting call", "call_id", callID)

	_, err := c.doRequest(ctx, http.MethodPost, url, payload, account.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to reject call: %w", err)
	}

	c.log.Debug("Call rejected", "call_id", callID)
	return nil
}

// SendCallPermissionRequest sends an interactive call_permission_request message
// to the consumer. The consumer must accept before outgoing calls can be placed.
// Permission is valid for 72 hours once accepted.
func (c *Client) SendCallPermissionRequest(ctx context.Context, account *Account, rcpt Recipient, bodyText string) (string, error) {
	if bodyText == "" {
		bodyText = "We'd like to call you to assist with your query."
	}

	payload := newOutboundMessage(rcpt, MessageTypeInteractive)
	payload.Interactive = &interactiveContent{
		Type:   "call_permission_request",
		Body:   &interactiveBody{Text: bodyText},
		Action: &interactiveAction{Name: "call_permission_request"},
	}

	url := c.buildMessagesURL(account)
	c.log.Debug("Sending call permission request", "phone", rcpt.Phone)

	respBody, err := c.doRequest(ctx, http.MethodPost, url, payload, account.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to send call permission request: %w", err)
	}

	// Report a malformed or ID-less response as an error. Returning ("", nil)
	// told the caller the request had succeeded while handing back nothing to
	// track it by, so a lost permission request looked identical to a sent one.
	msgID, err := parseMessageID(respBody)
	if err != nil {
		return "", fmt.Errorf("call permission request sent but response was unusable: %w", err)
	}
	c.log.Debug("Call permission request sent", "phone", rcpt.Phone, "message_id", msgID)
	return msgID, nil
}

// GetCallPermission checks the current call permission state for a user.
// Returns the permission status ("no_permission", "temporary", "permanent").
func (c *Client) GetCallPermission(ctx context.Context, account *Account, userPhone string) (string, error) {
	url := fmt.Sprintf("%s/%s/%s/call_permissions?user_wa_id=%s",
		c.getBaseURL(), account.APIVersion, account.PhoneID, userPhone)

	respBody, err := c.doRequest(ctx, http.MethodGet, url, nil, account.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to get call permission: %w", err)
	}

	var resp struct {
		Permission struct {
			Status string `json:"status"`
		} `json:"permission"`
	}
	if parseErr := json.Unmarshal(respBody, &resp); parseErr != nil {
		return "", fmt.Errorf("failed to parse call permission response: %w", parseErr)
	}

	return resp.Permission.Status, nil
}

// InitiateCall places an outgoing call to a WhatsApp user with an SDP offer.
// Returns the call_id assigned by WhatsApp on success.
func (c *Client) InitiateCall(ctx context.Context, account *Account, rcpt Recipient, sdpOffer string) (string, error) {
	payload := newCallRequest("connect", "")
	payload.Session = &callSession{SDPType: "offer", SDP: sdpOffer}
	rcpt.setCallRecipient(payload)

	url := c.buildCallsURL(account)
	c.log.Debug("Initiating outgoing call", "phone", rcpt.Phone)

	respBody, err := c.doRequest(ctx, http.MethodPost, url, payload, account.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to initiate call: %w", err)
	}

	// Parse call ID from response: {"calls": [{"id": "wacid.xxx"}]}
	var resp struct {
		Calls []struct {
			ID string `json:"id"`
		} `json:"calls"`
	}
	if parseErr := json.Unmarshal(respBody, &resp); parseErr != nil || len(resp.Calls) == 0 || resp.Calls[0].ID == "" {
		return "", fmt.Errorf("failed to parse call_id from response: %s", string(respBody))
	}

	c.log.Debug("Outgoing call initiated", "phone", rcpt.Phone, "call_id", resp.Calls[0].ID)
	return resp.Calls[0].ID, nil
}

// TerminateCall terminates an active call.
func (c *Client) TerminateCall(ctx context.Context, account *Account, callID string) error {
	payload := newCallRequest("terminate", callID)

	url := c.buildCallsURL(account)
	c.log.Debug("Terminating call", "call_id", callID)

	_, err := c.doRequest(ctx, http.MethodPost, url, payload, account.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to terminate call: %w", err)
	}

	c.log.Debug("Call terminated", "call_id", callID)
	return nil
}
