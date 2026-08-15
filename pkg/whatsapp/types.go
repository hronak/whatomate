package whatsapp

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Account represents WhatsApp Business Account credentials
type Account struct {
	PhoneID     string
	BusinessID  string
	AppID       string
	APIVersion  string
	AccessToken string
}

// Button represents an interactive button
type Button struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type,omitempty"` // "reply" (default) or "url"
	URL   string `json:"url,omitempty"`  // URL for type="url" buttons
}

// listResponse is Meta's standard collection envelope, {"data": [...]}.
// Several endpoints declared their own copy of this shape; one generic type
// covers them all.
type listResponse[T any] struct {
	Data   []T        `json:"data"`
	Paging metaPaging `json:"paging,omitzero"`
}

// idResponse is Meta's standard creation response, {"id": "..."}. It replaces
// nine identical struct declarations across this package.
type idResponse struct {
	ID string `json:"id"`
}

// MetaAPIResponse represents a successful API response from Meta
type MetaAPIResponse struct {
	Messages []struct {
		ID string `json:"id"`
	} `json:"messages"`
}

// MetaErrorResponse is the wire shape of a Graph API error body.
type MetaErrorResponse struct {
	Error MetaErrorDetail `json:"error"`
}

// MetaErrorDetail is the error object inside a MetaErrorResponse.
type MetaErrorDetail struct {
	Message      string `json:"message"`
	Type         string `json:"type"`
	Code         int    `json:"code"`
	ErrorSubcode int    `json:"error_subcode"`
	ErrorUserMsg string `json:"error_user_msg"`
	ErrorData    struct {
		Details string `json:"details"`
	} `json:"error_data"`
	FBTraceID string `json:"fbtrace_id"`
}

// Sentinels for the Meta failure modes callers actually branch on. Match them
// with errors.Is against any error returned by this package:
//
//	if errors.Is(err, whatsapp.ErrRateLimited) { backOffAndRetry() }
//
// Everything else is a considered rejection from Meta and will fail the same
// way on a retry; use errors.As to read the numeric code for those.
var (
	// ErrRateLimited means Meta is throttling; the send is worth retrying
	// after a backoff.
	ErrRateLimited = errors.New("whatsapp: rate limited")

	// ErrInvalidToken means the account's access token is expired or revoked.
	// Retrying cannot help until the account is re-authorized.
	ErrInvalidToken = errors.New("whatsapp: invalid or expired access token")

	// ErrReengagementRequired means the 24-hour customer service window has
	// closed and only a template message may be sent.
	ErrReengagementRequired = errors.New("whatsapp: re-engagement required, 24-hour window closed")

	// ErrTemplateNotFound means the named template does not exist or is not
	// approved for this account.
	ErrTemplateNotFound = errors.New("whatsapp: template not found")
)

// Meta error codes that map onto the sentinels above.
// See https://developers.facebook.com/docs/whatsapp/cloud-api/support/error-codes
var (
	rateLimitedCodes = map[int]bool{
		4:      true, // application request limit reached
		80007:  true, // rate limit hit
		130429: true, // Cloud API message throughput reached
		131048: true, // spam rate limit hit
		131056: true, // pair rate limit hit
		133016: true, // rate limit hit during registration
	}
	invalidTokenCodes = map[int]bool{
		0:   true, // AuthException
		102: true, // session key invalid or expired
		190: true, // access token expired/invalid/revoked
	}
)

// MetaAPIError is a structured error from the Graph API.
//
// It implements error and Unwrap, so callers can branch with errors.Is against
// the sentinels above or pull out the numeric code with errors.As:
//
//	var apiErr *whatsapp.MetaAPIError
//	if errors.As(err, &apiErr) && apiErr.Detail.Code == 131026 { ... }
//
// Previously this was flattened to a string at the point of failure, which is
// why nothing in the codebase could branch on a Meta failure at all.
type MetaAPIError struct {
	// StatusCode is the HTTP status Meta responded with.
	StatusCode int

	// Detail is Meta's parsed error object. Zero when the body was not a
	// recognizable Meta error, in which case Body holds the raw response.
	Detail MetaErrorDetail

	// Body is the raw response body, retained when parsing failed.
	Body string

	// RetryAfter carries the response's Retry-After header, when Meta sent
	// one. Zero means Meta gave no guidance and the caller should back off on
	// its own schedule.
	RetryAfter time.Duration
}

// Error implements error.
func (e *MetaAPIError) Error() string {
	if e.Detail.Message == "" {
		return fmt.Sprintf("API returned status %d: %s", e.StatusCode, e.Body)
	}
	msg := fmt.Sprintf("API error %d: %s", e.Detail.Code, e.Detail.Message)
	if e.Detail.ErrorData.Details != "" {
		msg += " - Details: " + e.Detail.ErrorData.Details
	}
	if e.Detail.ErrorUserMsg != "" {
		msg += " - " + e.Detail.ErrorUserMsg
	}
	return msg
}

// Unwrap returns the sentinel matching Meta's error code, so errors.Is works
// against ErrRateLimited and friends. It returns nil for codes with no
// sentinel, which simply means errors.Is finds no match.
func (e *MetaAPIError) Unwrap() error {
	switch {
	case rateLimitedCodes[e.Detail.Code]:
		return ErrRateLimited
	case invalidTokenCodes[e.Detail.Code]:
		return ErrInvalidToken
	case e.Detail.Code == 131047:
		return ErrReengagementRequired
	case e.Detail.Code == 132001:
		return ErrTemplateNotFound
	}
	return nil
}

// Retryable reports whether another attempt could plausibly succeed. Only
// throttling and 5xx qualify: a rejection Meta has already reasoned about
// (bad number, unapproved template, closed window) fails identically on retry.
func (e *MetaAPIError) Retryable() bool {
	return rateLimitedCodes[e.Detail.Code] || e.StatusCode >= 500
}

// ParseMetaAPIError parses respBody as a Meta API error, always returning a
// *MetaAPIError. When the body is not a recognizable Meta error it is retained
// verbatim on the Body field.
func ParseMetaAPIError(statusCode int, respBody []byte) error {
	apiErr := &MetaAPIError{StatusCode: statusCode, Body: string(respBody)}

	var envelope MetaErrorResponse
	if err := json.Unmarshal(respBody, &envelope); err == nil && envelope.Error.Message != "" {
		apiErr.Detail = envelope.Error
	}
	return apiErr
}

// TemplateResponse represents response from template submission
type TemplateResponse = idResponse

// MetaQualityScore represents quality score information from Meta
type MetaQualityScore struct {
	Score string `json:"score"`
}

// MetaTemplate represents a template fetched from Meta
type MetaTemplate struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Language      string              `json:"language"`
	Category      TemplateCategory    `json:"category"`
	Status        TemplateStatus      `json:"status"`
	QualityRating string              `json:"quality_rating,omitempty"`
	QualityScore  *MetaQualityScore   `json:"quality_score,omitempty"`
	Components    []TemplateComponent `json:"components"`
}

// TemplateComponent represents a component of a template
type TemplateComponent struct {
	Type    ComponentType    `json:"type"`
	Format  ComponentFormat  `json:"format,omitempty"`
	Text    string           `json:"text,omitempty"`
	Buttons []TemplateButton `json:"buttons,omitempty"`
	Example *TemplateExample `json:"example,omitempty"`
}

// TemplateButton represents a button in a template.
// FlowID uses json.Number because Meta returns it as a numeric ID.
type TemplateButton struct {
	Type           ButtonType  `json:"type"`
	Text           string      `json:"text"`
	URL            string      `json:"url,omitempty"`
	PhoneNumber    string      `json:"phone_number,omitempty"`
	Example        any         `json:"example,omitempty"`
	FlowID         json.Number `json:"flow_id,omitempty"`
	FlowAction     string      `json:"flow_action,omitempty"`
	NavigateScreen string      `json:"navigate_screen,omitempty"`
	OTPType        OTPType     `json:"otp_type,omitempty"`
	AutofillText   string      `json:"autofill_text,omitempty"`  // For ONE_TAP OTP
	PackageName    string      `json:"package_name,omitempty"`   // For ONE_TAP/ZERO_TAP OTP
	SignatureHash  string      `json:"signature_hash,omitempty"` // For ONE_TAP/ZERO_TAP OTP
}

// TemplateExample represents example values for template variables
type TemplateExample struct {
	HeaderText   []string   `json:"header_text,omitempty"`
	HeaderHandle []string   `json:"header_handle,omitempty"`
	BodyText     [][]string `json:"body_text,omitempty"`
}

// TemplateListResponse represents response from fetching templates
type TemplateListResponse = listResponse[MetaTemplate]

// WebhookPayload represents the incoming webhook from Meta
type WebhookPayload struct {
	Object string         `json:"object"`
	Entry  []WebhookEntry `json:"entry"`
}

// WebhookEntry represents an entry in the webhook payload
type WebhookEntry struct {
	ID      string          `json:"id"`
	Changes []WebhookChange `json:"changes"`
}

// WebhookChange represents a change in the webhook entry
type WebhookChange struct {
	Value WebhookValue `json:"value"`
	Field string       `json:"field"`
}

// WebhookValue represents the value of a webhook change
type WebhookValue struct {
	MessagingProduct string           `json:"messaging_product"`
	Metadata         WebhookMetadata  `json:"metadata"`
	Contacts         []WebhookContact `json:"contacts,omitempty"`
	Messages         []WebhookMessage `json:"messages,omitempty"`
	Statuses         []WebhookStatus  `json:"statuses,omitempty"`
}

// WebhookMetadata represents metadata in webhook
type WebhookMetadata struct {
	DisplayPhoneNumber string `json:"display_phone_number"`
	PhoneNumberID      string `json:"phone_number_id"`
}

// WebhookContact represents a contact in webhook
type WebhookContact struct {
	Profile struct {
		Name string `json:"name"`
	} `json:"profile"`
	WaID string `json:"wa_id"`
}

// WebhookMessage represents an incoming message
type WebhookMessage struct {
	From        string                 `json:"from"`
	ID          string                 `json:"id"`
	Timestamp   string                 `json:"timestamp"`
	Type        string                 `json:"type"`
	Text        *WebhookText           `json:"text,omitempty"`
	Interactive *WebhookInteractive    `json:"interactive,omitempty"`
	Image       *WebhookMedia          `json:"image,omitempty"`
	Document    *WebhookMedia          `json:"document,omitempty"`
	Audio       *WebhookMedia          `json:"audio,omitempty"`
	Video       *WebhookMedia          `json:"video,omitempty"`
	Context     *WebhookMessageContext `json:"context,omitempty"`
}

// WebhookText represents text content in a message
type WebhookText struct {
	Body string `json:"body"`
}

// WebhookInteractive represents interactive message response
type WebhookInteractive struct {
	Type        string              `json:"type"`
	ButtonReply *WebhookButtonReply `json:"button_reply,omitempty"`
	ListReply   *WebhookListReply   `json:"list_reply,omitempty"`
	NFMReply    *WebhookNFMReply    `json:"nfm_reply,omitempty"`
}

// WebhookButtonReply represents a button reply
type WebhookButtonReply struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// WebhookListReply represents a list selection reply
type WebhookListReply struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// WebhookNFMReply represents a flow reply
type WebhookNFMReply struct {
	ResponseJSON string `json:"response_json"`
	Body         string `json:"body"`
	Name         string `json:"name"`
}

// WebhookMedia represents media in a message
type WebhookMedia struct {
	ID       string `json:"id"`
	MimeType string `json:"mime_type"`
	SHA256   string `json:"sha256"`
	Caption  string `json:"caption,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// WebhookMessageContext represents message context (for replies)
type WebhookMessageContext struct {
	From      string `json:"from"`
	ID        string `json:"id"`
	Forwarded bool   `json:"forwarded,omitempty"`
}

// WebhookStatus represents a message status update
type WebhookStatus struct {
	ID          string               `json:"id"`
	Status      string               `json:"status"`
	Timestamp   string               `json:"timestamp"`
	RecipientID string               `json:"recipient_id"`
	Errors      []WebhookStatusError `json:"errors,omitempty"`
}

// WebhookStatusError represents an error in status update
type WebhookStatusError struct {
	Code    int    `json:"code"`
	Title   string `json:"title"`
	Message string `json:"message"`
}

// ParsedMessage represents a parsed incoming message
type ParsedMessage struct {
	From          string
	ID            string
	Timestamp     time.Time
	Type          string
	Text          string
	ButtonReplyID string
	ListReplyID   string
	MediaID       string
	MediaMimeType string
	Caption       string
	ContactName   string
	PhoneNumberID string
}

// ParsedStatus represents a parsed status update
type ParsedStatus struct {
	MessageID   string
	Status      string
	Timestamp   time.Time
	RecipientID string
	ErrorCode   int
	ErrorTitle  string
	ErrorMsg    string
}

// CatalogInfo represents a catalog from Meta API
type CatalogInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CatalogListResponse represents response from listing catalogs
type CatalogListResponse = listResponse[CatalogInfo]

// ProductInput represents input for creating/updating a product
type ProductInput struct {
	Name        string `json:"name"`
	Price       int64  `json:"price"` // Price in cents
	Currency    string `json:"currency"`
	URL         string `json:"url"`
	ImageURL    string `json:"image_url"`
	RetailerID  string `json:"retailer_id"` // SKU
	Description string `json:"description"`
}

// ProductInfo represents a product from Meta API
type ProductInfo struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Price       string `json:"price"`
	Currency    string `json:"currency"`
	URL         string `json:"url"`
	ImageURL    string `json:"image_url"`
	RetailerID  string `json:"retailer_id"`
	Description string `json:"description"`
}

// ProductListResponse represents response from listing products
type ProductListResponse = listResponse[ProductInfo]

// ProductCreateResponse represents response from creating a product
type ProductCreateResponse = idResponse

// BusinessProfile represents the business profile of a phone number
type BusinessProfile struct {
	MessagingProduct string   `json:"messaging_product"`
	Address          string   `json:"address"`
	Description      string   `json:"description"`
	Vertical         string   `json:"vertical"`
	Email            string   `json:"email"`
	Websites         []string `json:"websites"`
	ProfilePicture   string   `json:"profile_picture_url"`
	About            string   `json:"about"` // Status text
}

// BusinessProfileInput represents the input for updating a business profile
type BusinessProfileInput struct {
	MessagingProduct     string   `json:"messaging_product"`
	Address              string   `json:"address,omitempty"`
	Description          string   `json:"description,omitempty"`
	Vertical             string   `json:"vertical,omitempty"`
	Email                string   `json:"email,omitempty"`
	Websites             []string `json:"websites,omitempty"`
	ProfilePictureHandle string   `json:"profile_picture_handle,omitempty"`
	About                string   `json:"about,omitempty"`
}
