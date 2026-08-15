package whatsapp

// Call analytics: connected and missed calls, and duration, per phone number.

// CallAnalyticsDataPoint represents a single data point for call analytics
type CallAnalyticsDataPoint struct {
	Start           int64   `json:"start"`
	End             int64   `json:"end"`
	Count           int64   `json:"count"`
	Cost            float64 `json:"cost"`
	AverageDuration int64   `json:"average_duration"`    // Average duration in seconds
	Direction       string  `json:"direction,omitempty"` // USER_INITIATED or BUSINESS_INITIATED (from dimensions)
}

// CallAnalyticsEntry represents a single phone number's call data
type CallAnalyticsEntry struct {
	PhoneNumber string                   `json:"phone_number,omitempty"`
	DataPoints  []CallAnalyticsDataPoint `json:"data_points"`
}

// CallAnalyticsRaw represents the raw response from Meta API
type CallAnalyticsRaw struct {
	Granularity Granularity          `json:"granularity"`
	Data        []CallAnalyticsEntry `json:"data"`
	// Also support direct data_points for backward compatibility
	DataPoints []CallAnalyticsDataPoint `json:"data_points,omitempty"`
}

// CallAnalytics represents call analytics response (flattened)
type CallAnalytics struct {
	Granularity Granularity              `json:"granularity"`
	DataPoints  []CallAnalyticsDataPoint `json:"data_points"`
}
