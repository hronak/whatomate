package whatsapp

// Recipient identifies a WhatsApp user by phone number and/or BSUID.
// Meta accepts both: phone number via "to" and BSUID via "recipient".
// When both are provided, phone number takes precedence.
type Recipient struct {
	Phone string // Phone number (e.g., "16505551234")
	BSUID string // Business-Scoped User ID (e.g., "US.13491208655302741918")
}

// setOn fills the recipient fields on a typed outbound message.
func (r Recipient) setOn(msg *outboundMessage) {
	if r.Phone != "" {
		msg.To = r.Phone
	}
	if r.BSUID != "" {
		msg.Recipient = r.BSUID
	}
}

// SetOnPayload sets the "to" and/or "recipient" fields on a message payload.
// Retained for the payloads that are still assembled as maps.
func (r Recipient) SetOnPayload(payload map[string]any) {
	if r.Phone != "" {
		payload["to"] = r.Phone
	}
	if r.BSUID != "" {
		payload["recipient"] = r.BSUID
	}
}
