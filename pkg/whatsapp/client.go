package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"mime/multipart"
	"net/http"
	"net/textproto"
	netURL "net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zerodha/logf"
)

const (
	// DefaultTimeout for HTTP requests
	DefaultTimeout = 30 * time.Second

	// DefaultMaxAttempts is how many times a transient failure is retried,
	// including the first try. This is a bulk-messaging product and had no
	// backoff anywhere.
	DefaultMaxAttempts = 3

	// DefaultBaseBackoff is the first retry delay; it doubles each attempt and
	// carries jitter. A Retry-After header overrides it.
	DefaultBaseBackoff = 500 * time.Millisecond

	// maxRetryBackoff caps a single backoff wait.
	maxRetryBackoff = 30 * time.Second
	// BaseURL for Meta Graph API
	BaseURL = "https://graph.facebook.com"
	// DefaultAPIVersion is the Meta Graph API version used when an account or
	// config does not specify one. Keep the gorm default tag on
	// models.WhatsAppAccount.APIVersion in sync (struct tags must be literals).
	DefaultAPIVersion = "v26.0"
)

// Client is the WhatsApp Cloud API client. It is safe for concurrent use.
//
// Per-request credentials travel in an Account, so one Client serves every
// WhatsApp business account in a multi-tenant deployment.
type Client struct {
	httpClient *http.Client
	log        logf.Logger
	baseURL    string

	// retry policy for transient failures
	maxAttempts int
	baseBackoff time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithLogger sets the logger. Without it the client logs nothing.
func WithLogger(log logf.Logger) Option {
	return func(c *Client) { c.log = log }
}

// WithTimeout sets the per-request timeout on the default HTTP client. It has
// no effect alongside WithHTTPClient, which supplies its own.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient.Timeout = d }
}

// WithBaseURL overrides the Graph API base URL. Used to point at a mock server
// in tests and to honor the whatsapp.base_url config setting.
func WithBaseURL(baseURL string) Option {
	return func(c *Client) {
		if baseURL != "" {
			c.baseURL = baseURL
		}
	}
}

// WithHTTPClient supplies the HTTP client, for callers that need their own
// transport (connection pooling, proxies, an SSRF-safe dialer).
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		if hc != nil {
			c.httpClient = hc
		}
	}
}

// WithRetry sets the retry policy for transient failures. attempts <= 1
// disables retrying.
func WithRetry(attempts int, baseBackoff time.Duration) Option {
	return func(c *Client) {
		c.maxAttempts = attempts
		c.baseBackoff = baseBackoff
	}
}

// New creates a WhatsApp client.
//
// It replaces three constructors that each fixed one setting and hardcoded the
// rest, so there was no way to set a timeout and a base URL together — which is
// why one test silently talked to the production Graph API. Options compose.
func New(opts ...Option) *Client {
	c := &Client{
		httpClient:  &http.Client{Timeout: DefaultTimeout},
		baseURL:     BaseURL,
		maxAttempts: DefaultMaxAttempts,
		baseBackoff: DefaultBaseBackoff,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// escapeQuotes makes a value safe inside a quoted MIME header parameter.
func escapeQuotes(s string) string {
	return strings.NewReplacer("\\", "\\\\", `"`, `\"`, "\r", "", "\n", "").Replace(s)
}

// truncate shortens s to at most n bytes, appending an ellipsis when it cut.
// Slicing directly panics whenever the value is shorter than the bound.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// getBaseURL returns the base URL for API requests
func (c *Client) getBaseURL() string {
	if c.baseURL != "" {
		return c.baseURL
	}
	return BaseURL
}

// doRequest performs a JSON request to the Meta API, retrying transient
// failures with exponential backoff and jitter.
func (c *Client) doRequest(ctx context.Context, method, url string, body any, accessToken string) ([]byte, error) {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	return c.send(ctx, func() (*http.Request, error) {
		var reqBody io.Reader
		if payload != nil {
			// Fresh reader per attempt: a retry cannot reuse a consumed body.
			reqBody = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
}

// send executes the request built by newRequest and returns its body.
//
// It is the single place a response from Meta is interpreted: any 2xx is
// success (several endpoints answer 201 or 202, and demanding exactly 200 is
// why some callers bypassed this path entirely), any other status becomes a
// *MetaAPIError, and transient failures are retried with backoff.
//
// newRequest is a factory rather than a request because a retry needs a fresh,
// unconsumed body — including the multipart and form-encoded bodies that made
// the upload and OAuth paths bypass this in the first place.
func (c *Client) send(ctx context.Context, newRequest func() (*http.Request, error)) ([]byte, error) {
	attempts := max(c.maxAttempts, 1)

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if attempt > 1 {
			if err := c.waitBeforeRetry(ctx, attempt, lastErr); err != nil {
				return nil, err
			}
		}

		respBody, err := c.attempt(ctx, newRequest)
		if err == nil {
			return respBody, nil
		}
		lastErr = err

		if !isRetryable(err) {
			return nil, err
		}
		c.log.Debug("Retrying Meta API request", "attempt", attempt, "max", attempts, "error", err)
	}
	return nil, lastErr
}

// attempt performs one HTTP round trip.
func (c *Client) attempt(ctx context.Context, newRequest func() (*http.Request, error)) ([]byte, error) {
	req, err := newRequest()
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := ParseMetaAPIError(resp.StatusCode, respBody)
		if ra := parseRetryAfter(resp.Header.Get("Retry-After")); ra > 0 {
			var metaErr *MetaAPIError
			if errors.As(apiErr, &metaErr) {
				metaErr.RetryAfter = ra
			}
		}
		return nil, apiErr
	}

	return respBody, nil
}

// waitBeforeRetry sleeps before the next attempt, honoring a Retry-After from
// the previous response and otherwise backing off exponentially with jitter.
// It returns early if ctx is cancelled.
func (c *Client) waitBeforeRetry(ctx context.Context, attempt int, lastErr error) error {
	delay := c.baseBackoff << (attempt - 2) // attempt 2 waits baseBackoff
	if delay <= 0 {
		delay = DefaultBaseBackoff
	}

	// Meta told us how long to wait; that beats guessing.
	var metaErr *MetaAPIError
	if errors.As(lastErr, &metaErr) && metaErr.RetryAfter > 0 {
		delay = metaErr.RetryAfter
	}

	// Full jitter, so a fleet of workers throttled at the same moment does not
	// retry in lockstep.
	delay += time.Duration(rand.Int64N(int64(delay)/2 + 1))
	if delay > maxRetryBackoff {
		delay = maxRetryBackoff
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// isRetryable reports whether another attempt could plausibly succeed:
// throttling, 5xx, and anything that never reached Meta at all.
func isRetryable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var apiErr *MetaAPIError
	if errors.As(err, &apiErr) {
		return apiErr.Retryable()
	}
	// Transport failure, or a malformed response we could not even parse.
	return true
}

// parseRetryAfter reads a Retry-After header in either of its RFC 9110 forms:
// delay-seconds or an HTTP date.
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// doJSON performs an HTTP request to the Meta API and unmarshals the JSON
// response body into a value of type T. It is a thin generic wrapper over
// doRequest for the common "request then decode" pattern.
func doJSON[T any](ctx context.Context, c *Client, method, url string, body any, accessToken string) (T, error) {
	var result T
	respBody, err := c.doRequest(ctx, method, url, body, accessToken)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return result, fmt.Errorf("failed to parse response: %w", err)
	}
	return result, nil
}

// parseMessageID extracts the WhatsApp message ID from a successful send
// response body, returning an error if the body is malformed or carries no ID.
func parseMessageID(respBody []byte) (string, error) {
	var resp MetaAPIResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if len(resp.Messages) == 0 {
		return "", fmt.Errorf("no message ID in response")
	}
	return resp.Messages[0].ID, nil
}

// CredentialsValidationResult contains the result of credentials validation
type CredentialsValidationResult struct {
	PhoneNumber            string
	VerifiedName           string
	AccountMode            string
	IsTestNumber           bool
	QualityRating          string
	CodeVerificationStatus string
	Warning                string
}

// ValidateCredentials validates WhatsApp account credentials with Meta API
// It checks the phone number endpoint, business account endpoint, and verifies
// that the phone number belongs to the specified business account
func (c *Client) ValidateCredentials(ctx context.Context, phoneID, businessID, accessToken, apiVersion string) (*CredentialsValidationResult, error) {
	// 1. Validate PhoneID
	phoneURL := fmt.Sprintf("%s/%s/%s?fields=display_phone_number,verified_name,code_verification_status,account_mode,quality_rating,is_on_biz_app,platform_type",
		c.getBaseURL(), apiVersion, phoneID)
	phoneBody, err := c.doRequest(ctx, http.MethodGet, phoneURL, nil, accessToken)
	if err != nil {
		return nil, fmt.Errorf("invalid phone_id or access_token: %w", err)
	}

	var phoneResult struct {
		DisplayPhoneNumber     string `json:"display_phone_number"`
		VerifiedName           string `json:"verified_name"`
		AccountMode            string `json:"account_mode"`
		CodeVerificationStatus string `json:"code_verification_status"`
		QualityRating          string `json:"quality_rating"`
		IsOnBizApp             bool   `json:"is_on_biz_app"`
		PlatformType           string `json:"platform_type"`
	}
	if err := json.Unmarshal(phoneBody, &phoneResult); err != nil {
		return nil, fmt.Errorf("failed to parse phone response: %w", err)
	}

	// Check verification status (skip for sandbox/test numbers and SMB accounts)
	isTestNumber := phoneResult.AccountMode == "SANDBOX" || phoneResult.VerifiedName == "Test Number"
	isSMB := phoneResult.IsOnBizApp || phoneResult.PlatformType == "SMB" || phoneResult.PlatformType == "SMB_CLOUD_API"

	var warning string
	if !isTestNumber && !isSMB {
		if phoneResult.CodeVerificationStatus == "NOT_VERIFIED" {
			return nil, fmt.Errorf("phone number is not verified. Please register it at: https://business.facebook.com/wa/manage/phone-numbers/")
		}
		if phoneResult.CodeVerificationStatus == "EXPIRED" {
			warning = "Phone verification has expired. Consider re-verifying at: https://business.facebook.com/wa/manage/phone-numbers/"
		}
	}

	// 2. Validate BusinessID
	businessURL := fmt.Sprintf("%s/%s/%s?fields=id,name", c.getBaseURL(), apiVersion, businessID)
	if _, err := c.doRequest(ctx, http.MethodGet, businessURL, nil, accessToken); err != nil {
		return nil, fmt.Errorf("invalid business_id: %w", err)
	}

	// 3. Verify phone belongs to business account
	phonesURL := fmt.Sprintf("%s/%s/%s/phone_numbers", c.getBaseURL(), apiVersion, businessID)
	phonesBody, err := c.doRequest(ctx, http.MethodGet, phonesURL, nil, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to verify phone-business relationship: %w", err)
	}

	var phonesResult struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(phonesBody, &phonesResult); err != nil {
		return nil, fmt.Errorf("failed to parse phone numbers list: %w", err)
	}

	phoneFound := false
	for _, phone := range phonesResult.Data {
		if phone.ID == phoneID {
			phoneFound = true
			break
		}
	}
	if !phoneFound {
		return nil, fmt.Errorf("phone_id '%s' does not belong to business_id '%s'. Please verify your configuration", phoneID, businessID)
	}

	return &CredentialsValidationResult{
		PhoneNumber:            phoneResult.DisplayPhoneNumber,
		VerifiedName:           phoneResult.VerifiedName,
		AccountMode:            phoneResult.AccountMode,
		IsTestNumber:           isTestNumber,
		QualityRating:          phoneResult.QualityRating,
		CodeVerificationStatus: phoneResult.CodeVerificationStatus,
		Warning:                warning,
	}, nil
}

// buildMessagesURL builds the messages endpoint URL
func (c *Client) buildMessagesURL(account *Account) string {
	return fmt.Sprintf("%s/%s/%s/messages", c.getBaseURL(), account.APIVersion, account.PhoneID)
}

// buildTemplatesURL builds the message_templates endpoint URL
func (c *Client) buildTemplatesURL(account *Account) string {
	return fmt.Sprintf("%s/%s/%s/message_templates", c.getBaseURL(), account.APIVersion, account.BusinessID)
}

// MediaURLResponse represents the response from Meta's media endpoint
type MediaURLResponse struct {
	URL              string `json:"url"`
	MimeType         string `json:"mime_type"`
	SHA256           string `json:"sha256"`
	FileSize         int64  `json:"file_size"`
	MessagingProduct string `json:"messaging_product"`
}

// GetMediaURL retrieves the download URL for a media file from Meta's API
func (c *Client) GetMediaURL(ctx context.Context, mediaID string, account *Account) (string, error) {
	url := fmt.Sprintf("%s/%s/%s", c.getBaseURL(), account.APIVersion, mediaID)

	respBody, err := c.doRequest(ctx, http.MethodGet, url, nil, account.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to get media URL: %w", err)
	}

	var mediaResp MediaURLResponse
	if err := json.Unmarshal(respBody, &mediaResp); err != nil {
		return "", fmt.Errorf("failed to parse media response: %w", err)
	}

	if mediaResp.URL == "" {
		return "", fmt.Errorf("no URL in media response")
	}

	return mediaResp.URL, nil
}

// UploadProfilePicture uploads a profile picture and returns its handle.
// Profile pictures must go through the Resumable Upload API.
func (c *Client) UploadProfilePicture(ctx context.Context, account *Account, fileData []byte, mimeType string) (string, error) {
	return c.ResumableUpload(ctx, account, fileData, mimeType, "profile_picture")
}

// DownloadMedia downloads media content from Meta's CDN URL
func (c *Client) DownloadMedia(ctx context.Context, mediaURL string, accessToken string) ([]byte, error) {
	data, err := c.send(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create download request: %w", err)
		}
		// Meta requires Bearer token for media download
		req.Header.Set("Authorization", "Bearer "+accessToken)
		return req, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to download media: %w", err)
	}
	return data, nil
}

// UploadMediaResponse represents the response from uploading media
type UploadMediaResponse = idResponse

// UploadMedia uploads media to WhatsApp's servers and returns the media ID
func (c *Client) UploadMedia(ctx context.Context, account *Account, data []byte, mimeType, filename string) (string, error) {
	url := fmt.Sprintf("%s/%s/%s/media", c.getBaseURL(), account.APIVersion, account.PhoneID)

	// Build the multipart body with mime/multipart rather than by hand.
	//
	// The hand-rolled version used a fixed boundary string — so a payload
	// containing that literal would corrupt the request — and interpolated the
	// filename into the Content-Disposition header unescaped, letting a quote
	// or CRLF in a filename inject headers.
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)

	if err := mw.WriteField("messaging_product", "whatsapp"); err != nil {
		return "", fmt.Errorf("failed to write multipart field: %w", err)
	}

	fileHeader := make(textproto.MIMEHeader)
	fileHeader.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeQuotes(filename)))
	fileHeader.Set("Content-Type", mimeType)
	part, err := mw.CreatePart(fileHeader)
	if err != nil {
		return "", fmt.Errorf("failed to create multipart part: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("failed to write multipart body: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("failed to finalize multipart body: %w", err)
	}

	contentType := mw.FormDataContentType()
	payload := body.Bytes()

	respBody, err := c.send(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("failed to create upload request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+account.AccessToken)
		req.Header.Set("Content-Type", contentType)
		return req, nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload media: %w", err)
	}

	var uploadResp UploadMediaResponse
	if err := json.Unmarshal(respBody, &uploadResp); err != nil {
		return "", fmt.Errorf("failed to parse upload response: %w", err)
	}

	if uploadResp.ID == "" {
		return "", fmt.Errorf("no media ID in upload response")
	}

	c.log.Debug("Media uploaded", "media_id", uploadResp.ID)
	return uploadResp.ID, nil
}

// sendMediaMessage is the shared implementation for all media message types.
func (c *Client) sendMediaMessage(ctx context.Context, account *Account, rcpt Recipient, mediaType MessageType, media *mediaContent) (string, error) {
	payload := newOutboundMessage(rcpt, mediaType)
	switch mediaType {
	case MessageTypeImage:
		payload.Image = media
	case MessageTypeVideo:
		payload.Video = media
	case MessageTypeAudio:
		payload.Audio = media
	case MessageTypeDocument:
		payload.Document = media
	default:
		return "", fmt.Errorf("unsupported media type: %s", mediaType)
	}

	url := c.buildMessagesURL(account)
	c.log.Debug("Sending media message", "type", mediaType, "phone", rcpt.Phone, "media_id", media.ID)

	respBody, err := c.doRequest(ctx, http.MethodPost, url, payload, account.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to send %s message: %w", mediaType, err)
	}

	messageID, err := parseMessageID(respBody)
	if err != nil {
		return "", err
	}
	c.log.Debug("Media message sent", "type", mediaType, "message_id", messageID, "phone", rcpt.Phone)
	return messageID, nil
}

// SendImageMessage sends an image message using a media ID
func (c *Client) SendImageMessage(ctx context.Context, account *Account, rcpt Recipient, mediaID, caption string) (string, error) {
	return c.sendMediaMessage(ctx, account, rcpt, MessageTypeImage, &mediaContent{
		ID: mediaID, Caption: caption,
	})
}

// SendDocumentMessage sends a document message using a media ID
func (c *Client) SendDocumentMessage(ctx context.Context, account *Account, rcpt Recipient, mediaID, filename, caption string) (string, error) {
	return c.sendMediaMessage(ctx, account, rcpt, MessageTypeDocument, &mediaContent{
		ID: mediaID, Filename: filename, Caption: caption,
	})
}

// SendVideoMessage sends a video message using a media ID
func (c *Client) SendVideoMessage(ctx context.Context, account *Account, rcpt Recipient, mediaID, caption string) (string, error) {
	return c.sendMediaMessage(ctx, account, rcpt, MessageTypeVideo, &mediaContent{
		ID: mediaID, Caption: caption,
	})
}

// SendAudioMessage sends an audio message using a media ID
func (c *Client) SendAudioMessage(ctx context.Context, account *Account, rcpt Recipient, mediaID string) (string, error) {
	return c.sendMediaMessage(ctx, account, rcpt, MessageTypeAudio, &mediaContent{
		ID: mediaID,
	})
}

// MarkMessageRead sends a read receipt for a message
func (c *Client) MarkMessageRead(ctx context.Context, account *Account, messageID string) error {
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"status":            "read",
		"message_id":        messageID,
	}

	url := c.buildMessagesURL(account)
	c.log.Debug("Sending read receipt", "message_id", messageID)

	_, err := c.doRequest(ctx, http.MethodPost, url, payload, account.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to send read receipt: %w", err)
	}

	c.log.Debug("Read receipt sent", "message_id", messageID)
	return nil
}

// ResumableUploadResponse represents response from creating upload session
type ResumableUploadResponse struct {
	ID string `json:"id"` // Upload session ID
}

// ResumableUploadFinishResponse represents response from completing upload
type ResumableUploadFinishResponse struct {
	Handle string `json:"h"` // File handle for use in templates
}

// ResumableUpload performs a resumable upload to get a file handle for template media samples.
// This is required for IMAGE, VIDEO, DOCUMENT header types in templates.
// Returns a handle (like "4::aW1hZ2...") that can be used in template creation.
func (c *Client) ResumableUpload(ctx context.Context, account *Account, data []byte, mimeType, filename string) (string, error) {
	if account.AppID == "" {
		return "", fmt.Errorf("app_id is required for resumable upload")
	}

	// Step 1: Create upload session
	sessionURL := fmt.Sprintf("%s/%s/%s/uploads", c.getBaseURL(), account.APIVersion, account.AppID)

	sessionPayload := map[string]any{
		"file_length": len(data),
		"file_type":   mimeType,
		"file_name":   filename,
	}

	c.log.Debug("Creating upload session", "url", sessionURL, "file_size", len(data), "mime_type", mimeType)

	sessionResp, err := c.doRequest(ctx, http.MethodPost, sessionURL, sessionPayload, account.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to create upload session: %w", err)
	}

	var uploadSession ResumableUploadResponse
	if err := json.Unmarshal(sessionResp, &uploadSession); err != nil {
		return "", fmt.Errorf("failed to parse upload session response: %w", err)
	}

	if uploadSession.ID == "" {
		return "", fmt.Errorf("no session ID in upload response")
	}

	c.log.Debug("Upload session created", "session_id", uploadSession.ID)

	// Step 2: Upload file data to session
	uploadURL := fmt.Sprintf("%s/%s/%s", c.getBaseURL(), account.APIVersion, uploadSession.ID)

	respBody, err := c.send(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("failed to create upload request: %w", err)
		}
		req.Header.Set("Authorization", "OAuth "+account.AccessToken)
		req.Header.Set("file_offset", "0")
		req.Header.Set("Content-Type", "application/octet-stream")
		return req, nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to upload file data: %w", err)
	}

	var finishResp ResumableUploadFinishResponse
	if err := json.Unmarshal(respBody, &finishResp); err != nil {
		return "", fmt.Errorf("failed to parse upload finish response: %w", err)
	}

	if finishResp.Handle == "" {
		return "", fmt.Errorf("no handle in upload response")
	}

	c.log.Debug("Resumable upload completed", "handle", truncate(finishResp.Handle, 20))
	return finishResp.Handle, nil
}

// BusinessProfileResponse represents the response containing business profile
type BusinessProfileResponse = listResponse[BusinessProfile]

// GetBusinessProfile retrieves the business profile settings
func (c *Client) GetBusinessProfile(ctx context.Context, account *Account) (*BusinessProfile, error) {
	// Requesting specific fields to optimize performance
	fields := "about,address,description,email,profile_picture_url,websites,vertical,messaging_product"
	url := fmt.Sprintf("%s/%s/%s/whatsapp_business_profile?fields=%s", c.getBaseURL(), account.APIVersion, account.PhoneID, fields)

	respBody, err := c.doRequest(ctx, http.MethodGet, url, nil, account.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get business profile: %w", err)
	}

	var response BusinessProfileResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse business profile response: %w", err)
	}

	if len(response.Data) == 0 {
		return nil, fmt.Errorf("no business profile found")
	}

	return &response.Data[0], nil
}

// UpdateBusinessProfile updates the business profile settings
func (c *Client) UpdateBusinessProfile(ctx context.Context, account *Account, input BusinessProfileInput) error {
	url := fmt.Sprintf("%s/%s/%s/whatsapp_business_profile", c.getBaseURL(), account.APIVersion, account.PhoneID)

	// Ensure messaging_product is set
	input.MessagingProduct = "whatsapp"

	_, err := c.doRequest(ctx, http.MethodPost, url, input, account.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to update business profile: %w", err)
	}

	return nil
}

// SubscribeAppResponse represents the response from subscribing an app to webhooks
type SubscribeAppResponse struct {
	Success bool `json:"success"`
}

// SubscribeApp subscribes the app to webhooks for the WhatsApp Business Account.
// This is required after phone number registration to receive incoming messages.
// Calls POST /{api_version}/{waba_id}/subscribed_apps
func (c *Client) SubscribeApp(ctx context.Context, account *Account) error {
	url := fmt.Sprintf("%s/%s/%s/subscribed_apps", c.getBaseURL(), account.APIVersion, account.BusinessID)

	respBody, err := c.doRequest(ctx, http.MethodPost, url, nil, account.AccessToken)
	if err != nil {
		return fmt.Errorf("failed to subscribe app to webhooks: %w", err)
	}

	var resp SubscribeAppResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return fmt.Errorf("failed to parse subscribe response: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("subscription was not successful")
	}

	c.log.Debug("App subscribed to webhooks", "business_id", account.BusinessID)
	return nil
}

// TokenExchangeResponse represents the response from OAuth token exchange
type TokenExchangeResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
}

// ExchangeCodeForToken exchanges a Facebook authorization code for a permanent access token
func (c *Client) ExchangeCodeForToken(ctx context.Context, code, appID, appSecret, apiVersion string) (string, error) {
	endpoint := fmt.Sprintf("%s/%s/oauth/access_token", c.getBaseURL(), apiVersion)

	// Credentials go in the POST form body, not the query string: a URL
	// carrying client_secret lands in proxy logs, browser history and any
	// intermediary's access log.
	form := netURL.Values{}
	form.Set("client_id", appID)
	form.Set("client_secret", appSecret)
	form.Set("code", code)

	encoded := form.Encode()

	respBody, err := c.send(ctx, func() (*http.Request, error) {
		// OAuth endpoint doesn't require an Authorization header.
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(encoded))
		if err != nil {
			return nil, fmt.Errorf("failed to create token exchange request: %w", err)
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req, nil
	})
	if err != nil {
		return "", fmt.Errorf("token exchange failed: %w", err)
	}

	var tokenResp TokenExchangeResponse
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("no access token in response")
	}

	c.log.Debug("Token exchange successful")
	return tokenResp.AccessToken, nil
}

// PhoneNumberInfo represents phone number information from Meta
type PhoneNumberInfo struct {
	VerifiedName       string `json:"verified_name"`
	DisplayPhoneNumber string `json:"display_phone_number"`
	QualityRating      string `json:"quality_rating"`
	IsOnBizApp         bool   `json:"is_on_biz_app"`
	PlatformType       string `json:"platform_type"`
}

// GetPhoneNumberInfo retrieves information about a phone number
func (c *Client) GetPhoneNumberInfo(ctx context.Context, phoneID, accessToken, apiVersion string) (*PhoneNumberInfo, error) {
	url := fmt.Sprintf("%s/%s/%s?fields=verified_name,display_phone_number,quality_rating,is_on_biz_app,platform_type",
		c.getBaseURL(), apiVersion, phoneID)

	respBody, err := c.doRequest(ctx, http.MethodGet, url, nil, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get phone number info: %w", err)
	}

	var info PhoneNumberInfo
	if err := json.Unmarshal(respBody, &info); err != nil {
		return nil, fmt.Errorf("failed to parse phone number info: %w", err)
	}

	return &info, nil
}

// RegisterPhoneNumber registers a phone number with Two-Step Verification PIN
func (c *Client) RegisterPhoneNumber(ctx context.Context, phoneID, pin, accessToken, apiVersion string) error {
	url := fmt.Sprintf("%s/%s/%s/register", c.getBaseURL(), apiVersion, phoneID)

	payload := map[string]string{
		"messaging_product": "whatsapp",
		"pin":               pin,
	}

	_, err := c.doRequest(ctx, http.MethodPost, url, payload, accessToken)
	if err != nil {
		return fmt.Errorf("phone registration failed: %w", err)
	}

	c.log.Debug("Phone number registered successfully", "phone_id", phoneID)
	return nil
}

// TokenDebugInfo represents the response from the debug_token endpoint
type TokenDebugInfo struct {
	AppID               string   `json:"app_id"`
	Type                string   `json:"type"`
	Application         string   `json:"application"`
	DataAccessExpiresAt int64    `json:"data_access_expires_at"`
	ExpiresAt           int64    `json:"expires_at"`
	IsValid             bool     `json:"is_valid"`
	IssuedAt            int64    `json:"issued_at"`
	Scopes              []string `json:"scopes"`
	UserID              string   `json:"user_id"`
	GranularScopes      []struct {
		Scope     string   `json:"scope"`
		TargetIds []string `json:"target_ids,omitempty"`
	} `json:"granular_scopes"`
}

// GetTokenDebugInfo retrieves information about an access token
func (c *Client) GetTokenDebugInfo(ctx context.Context, inputToken, accessToken, apiVersion string) (*TokenDebugInfo, error) {
	if apiVersion == "" {
		apiVersion = DefaultAPIVersion
	}
	url := fmt.Sprintf("%s/%s/debug_token?input_token=%s", c.getBaseURL(), apiVersion, inputToken)

	// debug_token requires an app access token or a user access token
	respBody, err := c.doRequest(ctx, http.MethodGet, url, nil, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get token debug info: %w", err)
	}

	var resp struct {
		Data TokenDebugInfo `json:"data"`
	}
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse token debug info: %w", err)
	}

	return &resp.Data, nil
}

// SharedWABAResponse represents the response structure for shared WABA request
type SharedWABAResponse struct {
	Data []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Phone struct {
			Data []struct {
				ID                 string `json:"id"`
				DisplayPhoneNumber string `json:"display_phone_number"`
				VerifiedName       string `json:"verified_name"`
			} `json:"data"`
		} `json:"phone_numbers"`
	} `json:"data"`
}

// GetSharedWABA discovers the WABA and Phone Number shared with the app
// This is useful when the embedded signup only returns a code and we need to find the connected account
func (c *Client) GetSharedWABA(ctx context.Context, accessToken, apiVersion string) (*SharedWABAResponse, error) {
	if apiVersion == "" {
		apiVersion = DefaultAPIVersion
	}
	// Query /me/accounts to find the WABA and its phone numbers
	// granular_scopes might be needed to filter this if the user has many accounts,
	// but for embedded signup, usually only the shared account is accessible or relevant.
	url := fmt.Sprintf("%s/%s/me/accounts?fields=id,name,phone_numbers{id,display_phone_number,verified_name}", c.getBaseURL(), apiVersion)

	respBody, err := c.doRequest(ctx, http.MethodGet, url, nil, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get shared WABA info: %w", err)
	}

	var resp SharedWABAResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse shared WABA response: %w", err)
	}

	return &resp, nil
}

// WABAPhoneNumbersResponse represents the response containing phone numbers for a WABA
type WABAPhoneNumbersResponse struct {
	Data []struct {
		ID                 string `json:"id"`
		DisplayPhoneNumber string `json:"display_phone_number"`
		VerifiedName       string `json:"verified_name"`
		QualityRating      string `json:"quality_rating"`
	} `json:"data"`
}

// GetWABAPhoneNumbers retrieves all phone numbers associated with a WABA
func (c *Client) GetWABAPhoneNumbers(ctx context.Context, wabaID, accessToken, apiVersion string) (*WABAPhoneNumbersResponse, error) {
	if apiVersion == "" {
		apiVersion = DefaultAPIVersion
	}
	url := fmt.Sprintf("%s/%s/%s/phone_numbers?fields=id,display_phone_number,verified_name,quality_rating", c.getBaseURL(), apiVersion, wabaID)

	respBody, err := c.doRequest(ctx, http.MethodGet, url, nil, accessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get WABA phone numbers: %w", err)
	}

	var resp WABAPhoneNumbersResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse WABA phone numbers response: %w", err)
	}

	return &resp, nil
}
