package whatsapp

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// SendReaction reacts to a message with an emoji. An empty emoji removes a
// previously sent reaction.
//
// Meta returns no usable message ID for reactions, so there is nothing to
// return but an error.
func (c *Client) SendReaction(ctx context.Context, account *Account, rcpt Recipient, targetMessageID, emoji string) error {
	if targetMessageID == "" {
		return fmt.Errorf("reaction requires the target message ID")
	}

	payload := newOutboundMessage(rcpt, MessageTypeReaction)
	payload.Reaction = &reactionContent{MessageID: targetMessageID, Emoji: emoji}

	url := c.buildMessagesURL(account)
	c.log.Debug("Sending reaction", "message_id", targetMessageID, "emoji", emoji)

	if _, err := c.doRequest(ctx, http.MethodPost, url, payload, account.AccessToken); err != nil {
		return fmt.Errorf("failed to send reaction: %w", err)
	}
	return nil
}

// SendTextMessage sends a text message. replyToMsgID quotes an earlier message
// when non-empty.
//
// This took a variadic "optional" parameter, which let a caller pass two reply
// IDs and silently ignored the second. An explicit argument says what the
// function actually accepts.
func (c *Client) SendTextMessage(ctx context.Context, account *Account, rcpt Recipient, text, replyToMsgID string) (string, error) {
	payload := newOutboundMessage(rcpt, MessageTypeText)
	payload.Text = &textContent{Body: text}

	// Add reply context if provided
	if replyToMsgID != "" {
		payload.Context = &messageContext{MessageID: replyToMsgID}
	}

	url := c.buildMessagesURL(account)
	c.log.Debug("Sending text message", "phone", rcpt.Phone, "url", url)

	respBody, err := c.doRequest(ctx, http.MethodPost, url, payload, account.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to send text message: %w", err)
	}

	messageID, err := parseMessageID(respBody)
	if err != nil {
		return "", err
	}
	c.log.Debug("Text message sent", "message_id", messageID, "phone", rcpt.Phone)
	return messageID, nil
}

// SendInteractiveButtons sends an interactive message with buttons or list
// If buttons <= 3, sends as buttons; if 4-10, sends as list
func (c *Client) SendInteractiveButtons(ctx context.Context, account *Account, rcpt Recipient, bodyText string, buttons []Button) (string, error) {
	if len(buttons) == 0 {
		return "", fmt.Errorf("at least one button is required")
	}
	if len(buttons) > 10 {
		return "", fmt.Errorf("maximum 10 buttons allowed")
	}

	interactive := &interactiveContent{
		Body: &interactiveBody{Text: bodyText},
	}

	if len(buttons) <= 3 {
		// Use button format
		replies := make([]replyButton, 0, len(buttons))
		for _, btn := range buttons {
			replies = append(replies, replyButton{
				Type:  "reply",
				Reply: buttonBody{ID: btn.ID, Title: clampRunes(btn.Title, maxButtonTitleLen)},
			})
		}
		interactive.Type = "button"
		interactive.Action = &interactiveAction{Buttons: replies}
	} else {
		// Use list format for 4-10 items
		rows := make([]listRow, 0, len(buttons))
		for _, btn := range buttons {
			rows = append(rows, listRow{ID: btn.ID, Title: clampRunes(btn.Title, maxListRowTitleLen)})
		}
		interactive.Type = "list"
		interactive.Action = &interactiveAction{
			Button:   "Select an option",
			Sections: []listSection{{Title: "Options", Rows: rows}},
		}
	}

	payload := newOutboundMessage(rcpt, MessageTypeInteractive)
	payload.Interactive = interactive

	url := c.buildMessagesURL(account)
	c.log.Debug("Sending interactive message", "phone", rcpt.Phone, "button_count", len(buttons))

	respBody, err := c.doRequest(ctx, http.MethodPost, url, payload, account.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to send interactive message: %w", err)
	}

	messageID, err := parseMessageID(respBody)
	if err != nil {
		return "", err
	}
	c.log.Debug("Interactive message sent", "message_id", messageID, "phone", rcpt.Phone)
	return messageID, nil
}

// SendCTAURLButton sends an interactive message with a CTA URL button
// This opens a URL when clicked instead of sending a reply
func (c *Client) SendCTAURLButton(ctx context.Context, account *Account, rcpt Recipient, bodyText, buttonText, url string) (string, error) {
	if buttonText == "" || url == "" {
		return "", fmt.Errorf("button text and URL are required")
	}

	// Truncate button text to 20 chars (WhatsApp limit)
	if len(buttonText) > 20 {
		buttonText = buttonText[:20]
	}

	payload := newOutboundMessage(rcpt, MessageTypeInteractive)
	payload.Interactive = &interactiveContent{
		Type: "cta_url",
		Body: &interactiveBody{Text: bodyText},
		Action: &interactiveAction{
			Name:       "cta_url",
			Parameters: ctaURLParameters{DisplayText: buttonText, URL: url},
		},
	}

	apiURL := c.buildMessagesURL(account)
	c.log.Debug("Sending CTA URL button message", "phone", rcpt.Phone, "url", url)

	respBody, err := c.doRequest(ctx, http.MethodPost, apiURL, payload, account.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to send CTA URL button message: %w", err)
	}

	messageID, err := parseMessageID(respBody)
	if err != nil {
		return "", err
	}
	c.log.Debug("CTA URL button message sent", "message_id", messageID, "phone", rcpt.Phone)
	return messageID, nil
}

// SendVoiceCallButton sends an interactive message with a WhatsApp Business
// Calling voice_call button. When the recipient taps the button, Meta
// initiates a voice call back to our number; the resulting incoming-call
// webhook echoes the `payload` string back as `biz_opaque_callback_data`, so
// callers can use it for routing (e.g. sticky-assigning the call to the
// agent who sent the button).
//
// ttlMinutes is how long the button remains clickable; pass 0 to use Meta's
// default (15 min). The sending phone number must be enrolled in the
// WhatsApp Business Calling API or Meta rejects the send.
func (c *Client) SendVoiceCallButton(ctx context.Context, account *Account, rcpt Recipient, bodyText, displayText string, ttlMinutes int, payload string) (string, error) {
	if bodyText == "" {
		return "", fmt.Errorf("body text is required")
	}
	if displayText == "" {
		return "", fmt.Errorf("display text is required")
	}
	displayText = clampRunes(displayText, maxButtonTitleLen)

	parameters := voiceCallParameters{
		DisplayText: displayText,
		TTLMinutes:  ttlMinutes,
		Payload:     payload,
	}

	msg := newOutboundMessage(rcpt, MessageTypeInteractive)
	msg.Interactive = &interactiveContent{
		Type: "voice_call",
		Body: &interactiveBody{Text: bodyText},
		Action: &interactiveAction{
			Name:       "voice_call",
			Parameters: parameters,
		},
	}

	url := c.buildMessagesURL(account)
	// Logged at info during the sticky-routing rollout: confirms display_text,
	// ttl_minutes, and the agent-id payload actually leave our box, so when
	// the incoming-call webhook arrives we know whether Meta echoed it back.
	// The payload is an opaque "agent:<uuid>" — not PII.
	c.log.Debug("Sending voice_call button message",
		"phone", rcpt.Phone,
		"parameters", parameters,
	)

	respBody, err := c.doRequest(ctx, http.MethodPost, url, msg, account.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to send voice_call button message: %w", err)
	}

	messageID, err := parseMessageID(respBody)
	if err != nil {
		return "", err
	}
	c.log.Debug("voice_call button message sent", "message_id", messageID, "phone", rcpt.Phone)
	return messageID, nil
}

// TemplateParam represents a parameter for template message
type TemplateParam struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	Image *struct {
		Link string `json:"link"`
	} `json:"image,omitempty"`
	Document *struct {
		Link     string `json:"link"`
		Filename string `json:"filename"`
	} `json:"document,omitempty"`
	Video *struct {
		Link string `json:"link"`
	} `json:"video,omitempty"`
}

// SendTemplateMessage sends a template message
// sortParamKeys returns the keys of paramMap in the order they should be sent
// to Meta. Named templates (forceLexical=true, or any non-numeric key) sort
// lexicographically. Otherwise keys are treated as positional indices and
// sorted numerically — required so that "1","2",..,"10","11" stay in order
// instead of becoming "1","10","11",..,"2","9".
func sortParamKeys(paramMap map[string]string, forceLexical bool) []string {
	if forceLexical {
		return slices.Sorted(maps.Keys(paramMap))
	}

	// Parse each key exactly once. A single non-numeric key means mixed/named
	// keys, where lexical order is the stable fallback.
	nums := make(map[string]int, len(paramMap))
	for k := range paramMap {
		n, err := strconv.Atoi(k)
		if err != nil {
			return slices.Sorted(maps.Keys(paramMap))
		}
		nums[k] = n
	}

	keys := slices.Collect(maps.Keys(paramMap))
	slices.SortFunc(keys, func(a, b string) int { return cmp.Compare(nums[a], nums[b]) })
	return keys
}

// BodyParamsToComponents converts a bodyParams map into WhatsApp template components.
// Supports both positional (numeric keys) and named parameters.
func BodyParamsToComponents(bodyParams map[string]string) []map[string]any {
	if len(bodyParams) == 0 {
		return nil
	}

	// Check if using named parameters (non-numeric keys like "name", "order_id")
	isNamedParams := false
	for key := range bodyParams {
		if _, err := strconv.Atoi(key); err != nil {
			isNamedParams = true
			break
		}
	}

	// Get sorted keys for deterministic ordering. For positional templates the
	// keys are numeric strings ("1".."14") and MUST be ordered numerically —
	// sort.Strings would yield "1","10","11",..,"2",..,"9" and ship parameters
	// to Meta in the wrong slot, so {{2}}..{{9}} render as the values that
	// belonged in {{10}}+ on the recipient's device (issue #354).
	keys := sortParamKeys(bodyParams, isNamedParams)

	params := make([]map[string]any, 0, len(bodyParams))
	for _, key := range keys {
		param := map[string]any{
			"type": "text",
			"text": bodyParams[key],
		}
		if isNamedParams {
			param["parameter_name"] = key
		}
		params = append(params, param)
	}

	return []map[string]any{
		{
			"type":       "body",
			"parameters": params,
		},
	}
}

// HeaderTextParamsComponent builds the header component for a TEXT header that
// contains one variable. Meta restricts TEXT headers to at most one variable,
// so this returns an error if `headerContent` has more.
//
// `params` is the caller's value map (e.g. {"order_id": "ORD-1"} or {"1": "ORD-1"}).
// `fallback` is consulted if the value isn't found in `params` — useful for
// callers that share one flat map across header + body.
//
// Returns (nil, nil) when the header has no variable.
func HeaderTextParamsComponent(headerContent string, params, fallback map[string]string) (map[string]any, error) {
	if !strings.Contains(headerContent, "{{") {
		return nil, nil
	}
	names := TemplateParamNames(headerContent)
	if len(names) == 0 {
		return nil, nil
	}
	if len(names) > 1 {
		return nil, fmt.Errorf("header text may contain at most one variable; found %d", len(names))
	}

	name := names[0]
	value := params[name]
	if value == "" {
		value = fallback[name]
	}

	param := map[string]any{
		"type": "text",
		"text": value,
	}
	// Named params include parameter_name; positional ("1") don't.
	if _, err := strconv.Atoi(name); err != nil {
		param["parameter_name"] = name
	}

	return map[string]any{
		"type":       "header",
		"parameters": []map[string]any{param},
	}, nil
}

// BuildTemplateComponents builds the full WhatsApp template components array,
// including an optional header component (TEXT with variable, or IMAGE/VIDEO/
// DOCUMENT media) and body parameters.
//
// headerMediaFilename is required by Meta for DOCUMENT headers — without it, the
// API returns error 132012 "Header Format Mismatch (Expected DOCUMENT, received
// UNKNOWN)". It is ignored for IMAGE/VIDEO.
//
// headerContent and headerParams are only consulted for TEXT headers. For media
// headers (IMAGE/VIDEO/DOCUMENT) the existing headerMediaID/Filename path is used.
// Returns an error if the TEXT header declares more than one variable.
func BuildTemplateComponents(
	bodyParams map[string]string,
	headerType, headerContent string,
	headerParams map[string]string,
	headerMediaID, headerMediaFilename string,
) ([]map[string]any, error) {
	var components []map[string]any

	switch strings.ToUpper(headerType) {
	case "TEXT":
		// Build a text-header parameter component when the approved template
		// declares a {{var}} in the header text. Without this, Meta rejects
		// sends of templates with header variables.
		headerComp, err := HeaderTextParamsComponent(headerContent, headerParams, bodyParams)
		if err != nil {
			return nil, err
		}
		if headerComp != nil {
			components = append(components, headerComp)
		}
	case "IMAGE", "VIDEO", "DOCUMENT":
		if headerMediaID != "" {
			mediaType := strings.ToLower(headerType)
			mediaObj := map[string]any{"id": headerMediaID}
			if mediaType == "document" && headerMediaFilename != "" {
				mediaObj["filename"] = headerMediaFilename
			}
			headerParam := map[string]any{
				"type":    mediaType,
				mediaType: mediaObj,
			}
			components = append(components, map[string]any{
				"type":       "header",
				"parameters": []map[string]any{headerParam},
			})
		}
	}

	// Add body component with text parameters
	bodyComponents := BodyParamsToComponents(bodyParams)
	components = append(components, bodyComponents...)

	if len(components) == 0 {
		return nil, nil
	}
	return components, nil
}

// AutoButtonComponents generates button components for button types that require
// server-generated parameters (FLOW needs flow_token, OTP needs the code).
// These are auto-generated and don't require user input.
func AutoButtonComponents(templateButtons []any) []map[string]any {
	if len(templateButtons) == 0 {
		return nil
	}

	var components []map[string]any
	for i, raw := range templateButtons {
		btn, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		t, _ := btn["type"].(string)
		t = strings.ToUpper(t)

		switch t {
		case "FLOW":
			components = append(components, map[string]any{
				"type":     "button",
				"sub_type": "flow",
				"index":    fmt.Sprintf("%d", i),
				"parameters": []map[string]any{
					{
						"type": "action",
						"action": map[string]any{
							"flow_token": fmt.Sprintf("flow_%d", time.Now().UnixNano()),
						},
					},
				},
			})
		}
	}
	return components
}

// ButtonURLParamsToComponents converts button parameters to WhatsApp API button components.
// buttonParams maps button index (as string like "0", "1") to the dynamic parameter value.
// templateButtons is the JSONB buttons array from the template, used to determine button type.
// URL buttons produce: {"type": "button", "sub_type": "url", "index": "0", "parameters": [{"type": "text", "text": "value"}]}
// COPY_CODE buttons produce: {"type": "button", "sub_type": "copy_code", "index": "0", "parameters": [{"type": "coupon_code", "coupon_code": "value"}]}
func ButtonURLParamsToComponents(buttonParams map[string]string, templateButtons []any) []map[string]any {
	if len(buttonParams) == 0 {
		return nil
	}

	// Build a lookup of button index -> effective type from template buttons.
	// OTP buttons resolve to their otp_type (COPY_CODE, ONE_TAP, ZERO_TAP)
	// so the message sending logic handles them correctly.
	// btnIsOTP tracks whether the button was originally an OTP button (auth templates
	// need sub_type "url" instead of "copy_code").
	btnTypes := map[string]string{}
	btnIsOTP := map[string]bool{}
	{
		for i, raw := range templateButtons {
			if btn, ok := raw.(map[string]any); ok {
				if t, ok := btn["type"].(string); ok {
					key := fmt.Sprintf("%d", i)
					effectiveType := strings.ToUpper(t)
					if effectiveType == "OTP" {
						btnIsOTP[key] = true
						if otpType, ok := btn["otp_type"].(string); ok {
							effectiveType = strings.ToUpper(otpType)
						}
					}
					btnTypes[key] = effectiveType
				}
			}
		}
	}

	// Button indices are always numeric strings ("0", "1", ...) so sort
	// numerically — same lexical-sort hazard as positional body params.
	keys := sortParamKeys(buttonParams, false)

	components := make([]map[string]any, 0, len(buttonParams))
	for _, index := range keys {
		value := buttonParams[index]
		// Skip button types that don't accept dynamic parameters
		if t := btnTypes[index]; t == "QUICK_REPLY" || t == "FLOW" || t == "PHONE_NUMBER" || t == "VOICE_CALL" || t == "ONE_TAP" || t == "ZERO_TAP" {
			continue
		}
		if btnTypes[index] == "COPY_CODE" && !btnIsOTP[index] {
			// Regular COPY_CODE button (e.g. coupon codes)
			components = append(components, map[string]any{
				"type":     "button",
				"sub_type": "copy_code",
				"index":    index,
				"parameters": []map[string]any{
					{"type": "coupon_code", "coupon_code": value},
				},
			})
		} else {
			components = append(components, map[string]any{
				"type":     "button",
				"sub_type": "url",
				"index":    index,
				"parameters": []map[string]any{
					{"type": "text", "text": value},
				},
			})
		}
	}
	return components
}

// SendFlowMessage sends an interactive WhatsApp Flow message
// flowID is the Meta Flow ID, headerText is optional header, bodyText is the message body,
// ctaText is the button text, flowToken is a unique token for tracking the flow response,
// and firstScreen is the name of the first screen to navigate to
func (c *Client) SendFlowMessage(ctx context.Context, account *Account, rcpt Recipient, flowID, headerText, bodyText, ctaText, flowToken, firstScreen string) (string, error) {
	if flowID == "" {
		return "", fmt.Errorf("flow ID is required")
	}
	if bodyText == "" {
		return "", fmt.Errorf("body text is required")
	}
	if ctaText == "" {
		ctaText = "Open" // Default CTA text
	}
	if flowToken == "" {
		flowToken = fmt.Sprintf("flow_%d", time.Now().UnixNano())
	}
	if firstScreen == "" {
		firstScreen = "FIRST_SCREEN" // Default fallback
	}

	// Truncate CTA text to Meta's limit
	ctaText = clampRunes(ctaText, maxButtonTitleLen)

	interactive := &interactiveContent{
		Type: "flow",
		Body: &interactiveBody{Text: bodyText},
		Action: &interactiveAction{
			Name: "flow",
			Parameters: flowParameters{
				FlowMessageVersion: "3",
				FlowToken:          flowToken,
				FlowID:             flowID,
				FlowCTA:            ctaText,
				FlowAction:         "navigate",
				FlowActionPayload:  flowActionPayload{Screen: firstScreen},
			},
		},
	}

	// Add header if provided
	if headerText != "" {
		interactive.Header = &interactiveHeader{Type: "text", Text: headerText}
	}

	payload := newOutboundMessage(rcpt, MessageTypeInteractive)
	payload.Interactive = interactive

	url := c.buildMessagesURL(account)
	c.log.Debug("Sending flow message", "phone", rcpt.Phone, "flow_id", flowID)

	respBody, err := c.doRequest(ctx, http.MethodPost, url, payload, account.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to send flow message: %w", err)
	}

	messageID, err := parseMessageID(respBody)
	if err != nil {
		return "", err
	}
	c.log.Debug("Flow message sent", "message_id", messageID, "phone", rcpt.Phone, "flow_id", flowID)
	return messageID, nil
}

// SendTemplateMessage sends a template message with optional components (header, body, buttons, etc.)
func (c *Client) SendTemplateMessage(ctx context.Context, account *Account, rcpt Recipient, templateName, languageCode string, components []map[string]any) (string, error) {
	payload := newOutboundMessage(rcpt, MessageTypeTemplate)
	// Template sends omit recipient_type, matching what this endpoint has
	// always put on the wire.
	payload.RecipientType = ""
	payload.Template = &templateContent{
		Name:       templateName,
		Language:   templateLanguage{Code: languageCode},
		Components: components,
	}

	url := c.buildMessagesURL(account)
	c.log.Debug("Sending template message with components", "phone", rcpt.Phone, "template", templateName)

	respBody, err := c.doRequest(ctx, http.MethodPost, url, payload, account.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to send template message: %w", err)
	}

	messageID, err := parseMessageID(respBody)
	if err != nil {
		return "", err
	}
	c.log.Debug("Template message sent", "message_id", messageID, "phone", rcpt.Phone, "template", templateName)
	return messageID, nil
}
