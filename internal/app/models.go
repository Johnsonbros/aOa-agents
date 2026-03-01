package app

// modelWindows maps known Claude model identifiers to their context window sizes (in tokens).
var modelWindows = map[string]int{
	// Claude 4 family
	"claude-opus-4-6":           200000,
	"claude-opus-4-0-20250514":  200000,
	"claude-sonnet-4-0-20250514": 200000,
	"claude-sonnet-4-6":         200000,

	// Claude 3.5 family
	"claude-3-5-sonnet-20241022": 200000,
	"claude-3-5-sonnet-20240620": 200000,
	"claude-3-5-haiku-20241022":  200000,

	// Claude 3 family
	"claude-3-opus-20240229":   200000,
	"claude-3-sonnet-20240229": 200000,
	"claude-3-haiku-20240307":  200000,
}

const defaultContextWindow = 200000

// ContextWindowSize returns the context window size for the given model.
// Returns 200k as default for unknown models.
func ContextWindowSize(model string) int {
	if size, ok := modelWindows[model]; ok {
		return size
	}
	return defaultContextWindow
}

// ModelPricing holds per-million-token pricing for a Claude model.
type ModelPricing struct {
	Input     float64 // $/MTok input
	Output    float64 // $/MTok output
	CacheRead float64 // $/MTok cache read
}

var modelPricing = map[string]ModelPricing{
	"claude-opus-4-6":            {Input: 15.0, Output: 75.0, CacheRead: 1.50},
	"claude-sonnet-4-6":          {Input: 3.0, Output: 15.0, CacheRead: 0.30},
	"claude-haiku-4-5-20251001":  {Input: 0.80, Output: 4.0, CacheRead: 0.08},
}

var defaultPricing = ModelPricing{Input: 15.0, Output: 75.0, CacheRead: 1.50}

// GetModelPricing returns pricing for the given model, matching by substring.
func GetModelPricing(model string) ModelPricing {
	if p, ok := modelPricing[model]; ok {
		return p
	}
	for key, p := range modelPricing {
		if len(model) > 0 && len(key) > 0 && contains(model, key) {
			return p
		}
	}
	return defaultPricing
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
