package whatsapp

// Messaging analytics: sent and delivered counts per phone number.

// MessagingAnalyticsDataPoint represents a single data point for messaging analytics
type MessagingAnalyticsDataPoint struct {
	Start     int64 `json:"start"`
	End       int64 `json:"end"`
	Sent      int64 `json:"sent"`
	Delivered int64 `json:"delivered"`
}

// MessagingAnalyticsEntry represents a single phone number's messaging data
type MessagingAnalyticsEntry struct {
	PhoneNumber string                        `json:"phone_number,omitempty"`
	DataPoints  []MessagingAnalyticsDataPoint `json:"data_points"`
}

// MessagingAnalyticsRaw represents the raw response from Meta API
type MessagingAnalyticsRaw struct {
	Granularity Granularity               `json:"granularity"`
	Data        []MessagingAnalyticsEntry `json:"data"`
	// Also support direct data_points for backward compatibility
	DataPoints []MessagingAnalyticsDataPoint `json:"data_points,omitempty"`
}

// MessagingAnalytics represents messaging analytics response (flattened)
type MessagingAnalytics struct {
	Granularity Granularity                   `json:"granularity"`
	DataPoints  []MessagingAnalyticsDataPoint `json:"data_points"`
}
