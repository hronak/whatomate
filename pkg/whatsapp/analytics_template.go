package whatsapp

// Template analytics: per-template sends, deliveries, reads and clicks.

// TemplateCostItem represents a cost item in template analytics
type TemplateCostItem struct {
	Type  string  `json:"type"`            // amount_spent, cost_per_delivered, cost_per_url_button_click
	Value float64 `json:"value,omitempty"` // The cost value
}

// TemplateClickItem represents a click item in template analytics
type TemplateClickItem struct {
	Type          string `json:"type"`           // quick_reply_button, unique_url_button
	ButtonContent string `json:"button_content"` // The button text
	Count         int64  `json:"count"`          // Number of clicks
}

// TemplateAnalyticsDataPoint represents a single data point for template analytics
// This matches Meta's actual response where template_id is in each data point
type TemplateAnalyticsDataPoint struct {
	TemplateID string              `json:"template_id"`
	Start      int64               `json:"start"`
	End        int64               `json:"end"`
	Sent       int64               `json:"sent"`
	Delivered  int64               `json:"delivered"`
	Read       int64               `json:"read"`
	Replied    int64               `json:"replied,omitempty"`
	Clicked    []TemplateClickItem `json:"clicked,omitempty"` // Array of button click details
	Cost       []TemplateCostItem  `json:"cost,omitempty"`
}

// TemplateAnalyticsDataEntry represents one entry in the data array
type TemplateAnalyticsDataEntry struct {
	Granularity Granularity                  `json:"granularity"`
	ProductType string                       `json:"product_type"`
	DataPoints  []TemplateAnalyticsDataPoint `json:"data_points"`
}

// TemplateAnalyticsRaw represents the raw response from Meta API for template analytics
type TemplateAnalyticsRaw struct {
	Data []TemplateAnalyticsDataEntry `json:"data"`
}

// TemplateAnalytics represents template analytics response (flattened for easier consumption)
type TemplateAnalytics struct {
	Granularity Granularity                  `json:"granularity"`
	DataPoints  []TemplateAnalyticsDataPoint `json:"data_points"`
}
