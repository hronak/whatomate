package whatsapp

// Pricing analytics: chargeable conversations and cost per phone number.

// PricingAnalyticsDataPoint represents a single data point for pricing analytics
// With dimensions, this includes detailed breakdown by category, type, and country
type PricingAnalyticsDataPoint struct {
	Start           int64   `json:"start"`
	End             int64   `json:"end"`
	Volume          int64   `json:"volume"`                     // Message count
	Cost            float64 `json:"cost"`                       // Cost in account currency
	Country         string  `json:"country,omitempty"`          // Country code (IN, US, etc.)
	PricingType     string  `json:"pricing_type,omitempty"`     // FREE_CUSTOMER_SERVICE, FREE_ENTRY_POINT, REGULAR
	PricingCategory string  `json:"pricing_category,omitempty"` // MARKETING, UTILITY, AUTHENTICATION, SERVICE, etc.
	Tier            string  `json:"tier,omitempty"`             // Pricing tier
}

// PricingAnalyticsEntry represents a single phone number's pricing data
type PricingAnalyticsEntry struct {
	PhoneNumber string                      `json:"phone_number,omitempty"`
	DataPoints  []PricingAnalyticsDataPoint `json:"data_points"`
}

// PricingAnalyticsRaw represents the raw response from Meta API
type PricingAnalyticsRaw struct {
	Granularity Granularity             `json:"granularity"`
	Data        []PricingAnalyticsEntry `json:"data"`
	// Also support direct data_points for backward compatibility
	DataPoints []PricingAnalyticsDataPoint `json:"data_points,omitempty"`
}

// PricingAnalytics represents pricing analytics response (flattened)
type PricingAnalytics struct {
	Granularity Granularity                 `json:"granularity"`
	DataPoints  []PricingAnalyticsDataPoint `json:"data_points"`
}
