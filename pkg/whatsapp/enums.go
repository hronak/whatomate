package whatsapp

// Named string types for the values Meta treats as closed sets.
//
// These were bare strings compared against literals scattered through the
// package and its callers, so a typo — "APPROVED" vs "Approved" — produced a
// silently wrong branch instead of a compile error. They follow the
// AnalyticsType model already established here: a defined string type plus
// constants, still marshalling as plain JSON strings.

// TemplateStatus is a template's review state at Meta.
type TemplateStatus string

const (
	TemplateStatusApproved         TemplateStatus = "APPROVED"
	TemplateStatusPending          TemplateStatus = "PENDING"
	TemplateStatusRejected         TemplateStatus = "REJECTED"
	TemplateStatusPaused           TemplateStatus = "PAUSED"
	TemplateStatusDisabled         TemplateStatus = "DISABLED"
	TemplateStatusInAppeal         TemplateStatus = "IN_APPEAL"
	TemplateStatusPendingDeletion  TemplateStatus = "PENDING_DELETION"
	TemplateStatusDeleted          TemplateStatus = "DELETED"
	TemplateStatusLimitExceeded    TemplateStatus = "LIMIT_EXCEEDED"
	TemplateStatusArchived         TemplateStatus = "ARCHIVED"
	TemplateStatusReinstated       TemplateStatus = "REINSTATED"
	TemplateStatusFlagged          TemplateStatus = "FLAGGED"
	TemplateStatusPendingReviewMsg TemplateStatus = "PENDING_REVIEW"
)

// IsUsable reports whether a template in this state can be sent.
func (s TemplateStatus) IsUsable() bool {
	return s == TemplateStatusApproved
}

// TemplateCategory is Meta's template classification, which determines pricing
// and the rules a template is reviewed against.
type TemplateCategory string

const (
	TemplateCategoryMarketing      TemplateCategory = "MARKETING"
	TemplateCategoryUtility        TemplateCategory = "UTILITY"
	TemplateCategoryAuthentication TemplateCategory = "AUTHENTICATION"
)

// ComponentType identifies a template component.
type ComponentType string

const (
	ComponentTypeHeader  ComponentType = "HEADER"
	ComponentTypeBody    ComponentType = "BODY"
	ComponentTypeFooter  ComponentType = "FOOTER"
	ComponentTypeButtons ComponentType = "BUTTONS"
)

// ComponentFormat is a HEADER component's media format.
type ComponentFormat string

const (
	ComponentFormatText     ComponentFormat = "TEXT"
	ComponentFormatImage    ComponentFormat = "IMAGE"
	ComponentFormatVideo    ComponentFormat = "VIDEO"
	ComponentFormatDocument ComponentFormat = "DOCUMENT"
	ComponentFormatLocation ComponentFormat = "LOCATION"
)

// ButtonType identifies a template button's behavior.
type ButtonType string

const (
	ButtonTypeQuickReply  ButtonType = "QUICK_REPLY"
	ButtonTypeURL         ButtonType = "URL"
	ButtonTypePhoneNumber ButtonType = "PHONE_NUMBER"
	ButtonTypeCopyCode    ButtonType = "COPY_CODE"
	ButtonTypeFlow        ButtonType = "FLOW"
	ButtonTypeOTP         ButtonType = "OTP"
	ButtonTypeVoiceCall   ButtonType = "VOICE_CALL"
)

// OTPType is the delivery mechanism for an authentication template's code.
type OTPType string

const (
	OTPTypeCopyCode OTPType = "COPY_CODE"
	OTPTypeOneTap   OTPType = "ONE_TAP"
	OTPTypeZeroTap  OTPType = "ZERO_TAP"
)

// MessageType is the type of an outbound or inbound message.
type MessageType string

const (
	MessageTypeText        MessageType = "text"
	MessageTypeImage       MessageType = "image"
	MessageTypeVideo       MessageType = "video"
	MessageTypeAudio       MessageType = "audio"
	MessageTypeDocument    MessageType = "document"
	MessageTypeSticker     MessageType = "sticker"
	MessageTypeLocation    MessageType = "location"
	MessageTypeContacts    MessageType = "contacts"
	MessageTypeInteractive MessageType = "interactive"
	MessageTypeTemplate    MessageType = "template"
	MessageTypeReaction    MessageType = "reaction"
	MessageTypeButton      MessageType = "button"
	MessageTypeOrder       MessageType = "order"
	MessageTypeSystem      MessageType = "system"
	MessageTypeUnknown     MessageType = "unknown"
)

// Granularity is the bucket size for an analytics query.
type Granularity string

const (
	GranularityHalfHour Granularity = "HALF_HOUR"
	GranularityDay      Granularity = "DAY"
	GranularityMonth    Granularity = "MONTH"
)

// FlowStatus is a WhatsApp Flow's publication state.
type FlowStatus string

const (
	FlowStatusDraft      FlowStatus = "DRAFT"
	FlowStatusPublished  FlowStatus = "PUBLISHED"
	FlowStatusDeprecated FlowStatus = "DEPRECATED"
	FlowStatusBlocked    FlowStatus = "BLOCKED"
	FlowStatusThrottled  FlowStatus = "THROTTLED"
)
