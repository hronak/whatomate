package whatsapp

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
)

// PhoneNumberDetails is the subset of a phone number's Graph API fields the
// account-status screen displays.
type PhoneNumberDetails struct {
	DisplayPhoneNumber     string `json:"display_phone_number"`
	VerifiedName           string `json:"verified_name"`
	CodeVerificationStatus string `json:"code_verification_status"`
	AccountMode            string `json:"account_mode"`
	QualityRating          string `json:"quality_rating"`

	// MessagingLimitTier is the legacy per-number limit. Meta is migrating to
	// the portfolio-level field below, and returns one or the other depending
	// on the account, so callers must check both.
	MessagingLimitTier string `json:"messaging_limit_tier"`

	// PortfolioMessagingLimit is the newer WABA-level messaging limit.
	PortfolioMessagingLimit string `json:"whatsapp_business_manager_messaging_limit"`
}

// IsTestNumber reports whether this is a sandbox number, which cannot be used
// for production traffic.
func (d PhoneNumberDetails) IsTestNumber() bool { return d.AccountMode == "SANDBOX" }

// MessagingLimit returns whichever of the two limit fields Meta populated.
func (d PhoneNumberDetails) MessagingLimit() string {
	return cmp.Or(d.MessagingLimitTier, d.PortfolioMessagingLimit)
}

// phoneDetailFields are the fields requested from the phone number endpoint.
const phoneDetailFields = "display_phone_number,verified_name,code_verification_status," +
	"account_mode,quality_rating,messaging_limit_tier,whatsapp_business_manager_messaging_limit"

// GetPhoneNumberDetails fetches display and status details for a phone number.
func (c *Client) GetPhoneNumberDetails(ctx context.Context, account *Account) (*PhoneNumberDetails, error) {
	url := fmt.Sprintf("%s/%s/%s?fields=%s",
		c.getBaseURL(), account.APIVersion, account.PhoneID, phoneDetailFields)

	details, err := doJSON[PhoneNumberDetails](ctx, c, http.MethodGet, url, nil, account.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch phone number details: %w", err)
	}
	return &details, nil
}

// GetBusinessMessagingLimit fetches the portfolio-level messaging limit for a
// WhatsApp Business Account. Used as a fallback when the per-number field is
// absent.
func (c *Client) GetBusinessMessagingLimit(ctx context.Context, account *Account) (string, error) {
	url := fmt.Sprintf("%s/%s/%s?fields=whatsapp_business_manager_messaging_limit",
		c.getBaseURL(), account.APIVersion, account.BusinessID)

	resp, err := doJSON[struct {
		Limit string `json:"whatsapp_business_manager_messaging_limit"`
	}](ctx, c, http.MethodGet, url, nil, account.AccessToken)
	if err != nil {
		return "", fmt.Errorf("failed to fetch business messaging limit: %w", err)
	}
	return resp.Limit, nil
}
