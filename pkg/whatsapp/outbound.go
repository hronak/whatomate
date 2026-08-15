package whatsapp

// Typed request bodies for the outbound message endpoint.
//
// These replace the hand-built map[string]any payloads that every send method
// assembled independently. A map made every wire key a string literal repeated
// per call site, so "messaging_product" misspelled once produced a request Meta
// rejected at runtime with no compile-time signal; the shared envelope also had
// to be re-created, correctly, twenty times over.
//
// Where Meta's schema is fixed the shape is a struct. Where it is genuinely
// caller-driven — template components assembled from operator-configured data —
// the field stays []map[string]any rather than pretending to a structure the
// caller does not actually have.

// outboundMessage is the envelope shared by every message sent to the
// /messages endpoint.
type outboundMessage struct {
	MessagingProduct string      `json:"messaging_product"`
	RecipientType    string      `json:"recipient_type,omitempty"`
	Type             MessageType `json:"type"`

	// Recipient: Meta accepts a phone number in "to" or a Business-Scoped User
	// ID in "recipient".
	To        string `json:"to,omitempty"`
	Recipient string `json:"recipient,omitempty"`

	// Context quotes an earlier message.
	Context *messageContext `json:"context,omitempty"`

	// Exactly one of the following is set, matching Type.
	Text        *textContent        `json:"text,omitempty"`
	Image       *mediaContent       `json:"image,omitempty"`
	Video       *mediaContent       `json:"video,omitempty"`
	Audio       *mediaContent       `json:"audio,omitempty"`
	Document    *mediaContent       `json:"document,omitempty"`
	Reaction    *reactionContent    `json:"reaction,omitempty"`
	Interactive *interactiveContent `json:"interactive,omitempty"`
	Template    *templateContent    `json:"template,omitempty"`
}

// newOutboundMessage builds the envelope for a message of the given type.
func newOutboundMessage(rcpt Recipient, msgType MessageType) *outboundMessage {
	msg := &outboundMessage{
		MessagingProduct: "whatsapp",
		RecipientType:    "individual",
		Type:             msgType,
	}
	rcpt.setOn(msg)
	return msg
}

// messageContext quotes the message being replied to.
type messageContext struct {
	MessageID string `json:"message_id"`
}

// textContent is a plain text body.
type textContent struct {
	Body       string `json:"body"`
	PreviewURL bool   `json:"preview_url"`
}

// mediaContent references media by ID or link, with an optional caption.
// Audio accepts neither caption nor filename; document accepts both.
type mediaContent struct {
	ID       string `json:"id,omitempty"`
	Link     string `json:"link,omitempty"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// reactionContent reacts to an earlier message. An empty Emoji removes a
// previously sent reaction.
type reactionContent struct {
	MessageID string `json:"message_id"`
	Emoji     string `json:"emoji"`
}

// interactiveContent is a button, list, CTA-URL, flow or voice-call message.
type interactiveContent struct {
	Type   string             `json:"type"`
	Header *interactiveHeader `json:"header,omitempty"`
	Body   *interactiveBody   `json:"body,omitempty"`
	Footer *interactiveFooter `json:"footer,omitempty"`
	Action *interactiveAction `json:"action,omitempty"`
}

type interactiveHeader struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type interactiveBody struct {
	Text string `json:"text"`
}

type interactiveFooter struct {
	Text string `json:"text"`
}

// interactiveAction carries whichever action fields the interactive type uses.
type interactiveAction struct {
	// Reply buttons (type "button").
	Buttons []replyButton `json:"buttons,omitempty"`

	// List (type "list").
	Button   string        `json:"button,omitempty"`
	Sections []listSection `json:"sections,omitempty"`

	// Named actions: cta_url, flow, voice_call.
	Name       string `json:"name,omitempty"`
	Parameters any    `json:"parameters,omitempty"`
}

// replyButton is one quick-reply button.
type replyButton struct {
	Type  string     `json:"type"`
	Reply buttonBody `json:"reply"`
}

type buttonBody struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// listSection groups list rows under a title.
type listSection struct {
	Title string    `json:"title"`
	Rows  []listRow `json:"rows"`
}

// listRow is one selectable row in a list message.
type listRow struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// ctaURLParameters are the parameters of a cta_url action.
type ctaURLParameters struct {
	DisplayText string `json:"display_text"`
	URL         string `json:"url"`
}

// voiceCallParameters are the parameters of a voice_call action.
//
// Payload is echoed back as biz_opaque_callback_data on the resulting incoming
// call webhook, which is how a call is sticky-routed to the agent who sent the
// button.
type voiceCallParameters struct {
	DisplayText string `json:"display_text"`
	TTLMinutes  int    `json:"ttl_minutes,omitempty"`
	Payload     string `json:"payload,omitempty"`
}

// flowParameters are the parameters of a flow action.
type flowParameters struct {
	FlowMessageVersion string            `json:"flow_message_version"`
	FlowToken          string            `json:"flow_token"`
	FlowID             string            `json:"flow_id"`
	FlowCTA            string            `json:"flow_cta"`
	FlowAction         string            `json:"flow_action"`
	FlowActionPayload  flowActionPayload `json:"flow_action_payload"`
}

// flowActionPayload selects the flow's entry screen.
type flowActionPayload struct {
	Screen string `json:"screen"`
}

// templateContent sends an approved template.
//
// Components stays untyped: they are assembled from operator-configured
// template definitions stored as JSONB, so their shape is not known here.
type templateContent struct {
	Name       string           `json:"name"`
	Language   templateLanguage `json:"language"`
	Components []map[string]any `json:"components,omitempty"`
}

type templateLanguage struct {
	Code string `json:"code"`
}

// Meta's length limits for interactive elements, in characters.
const (
	maxButtonTitleLen  = 20
	maxListRowTitleLen = 24
)

// clampRunes truncates s to at most n characters.
//
// By runes, not bytes: the byte slicing this replaces could cut a multi-byte
// character in half and emit invalid UTF-8, which Meta rejects. It also
// under-counted, since Meta's limits are in characters.
func clampRunes(s string, n int) string {
	if len(s) <= n {
		// Fast path: fewer bytes than n means fewer runes than n.
		return s
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// callRequest is the body of a request to the /calls endpoint.
//
// Meta reuses one shape for every call action; which fields are meaningful
// depends on Action.
type callRequest struct {
	MessagingProduct string       `json:"messaging_product"`
	Action           string       `json:"action"`
	CallID           string       `json:"call_id,omitempty"`
	Session          *callSession `json:"session,omitempty"`

	// Recipient, for outgoing calls.
	To        string `json:"to,omitempty"`
	Recipient string `json:"recipient,omitempty"`
}

// callSession carries the SDP offer or answer for a call.
type callSession struct {
	SDPType string `json:"sdp_type"`
	SDP     string `json:"sdp"`
}

// newCallRequest builds a call request for the given action.
func newCallRequest(action, callID string) *callRequest {
	return &callRequest{
		MessagingProduct: "whatsapp",
		Action:           action,
		CallID:           callID,
	}
}

// setCallRecipient fills the recipient fields on an outgoing call request.
func (r Recipient) setCallRecipient(req *callRequest) {
	if r.Phone != "" {
		req.To = r.Phone
	}
	if r.BSUID != "" {
		req.Recipient = r.BSUID
	}
}
