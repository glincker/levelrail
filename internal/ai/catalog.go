package ai

// Kind selects the wire adapter a provider uses.
type Kind string

// Adapter kinds.
const (
	KindAnthropic    Kind = "anthropic"
	KindOpenAICompat Kind = "openai_compat"
	KindGemini       Kind = "gemini"
)

// ProviderInfo is one static catalog entry.
type ProviderInfo struct {
	ID              string
	Label           string
	Kind            Kind
	DefaultBaseURL  string
	BaseURLEditable bool
	KeyURL          string
	DefaultModel    string
	Brand           string
	// KeyRequired is false for self-hosted servers that run without auth.
	KeyRequired bool
}

// Catalog returns every supported provider in display order. Default
// models are suggestions from the catalog author, not live-verified.
func Catalog() []ProviderInfo {
	return []ProviderInfo{
		{ID: "anthropic", Label: "Anthropic", Kind: KindAnthropic, DefaultBaseURL: "https://api.anthropic.com", KeyURL: "https://console.anthropic.com/settings/keys", DefaultModel: "claude-sonnet-5-5", Brand: "anthropic", KeyRequired: true},
		{ID: "openai", Label: "OpenAI", Kind: KindOpenAICompat, DefaultBaseURL: "https://api.openai.com/v1", KeyURL: "https://platform.openai.com/api-keys", DefaultModel: "gpt-4.1", Brand: "openai", KeyRequired: true},
		{ID: "gemini", Label: "Google Gemini", Kind: KindGemini, DefaultBaseURL: "https://generativelanguage.googleapis.com/v1beta", KeyURL: "https://aistudio.google.com/apikey", DefaultModel: "gemini-2.5-flash", Brand: "google-gemini", KeyRequired: true},
		{ID: "azure_openai", Label: "Azure OpenAI", Kind: KindOpenAICompat, BaseURLEditable: true, KeyURL: "https://portal.azure.com/", Brand: "azure", KeyRequired: true},
		{ID: "openrouter", Label: "OpenRouter", Kind: KindOpenAICompat, DefaultBaseURL: "https://openrouter.ai/api/v1", KeyURL: "https://openrouter.ai/keys", Brand: "openrouter", KeyRequired: true},
		{ID: "groq", Label: "Groq", Kind: KindOpenAICompat, DefaultBaseURL: "https://api.groq.com/openai/v1", KeyURL: "https://console.groq.com/keys", DefaultModel: "llama-3.3-70b-versatile", Brand: "groq", KeyRequired: true},
		{ID: "together", Label: "Together AI", Kind: KindOpenAICompat, DefaultBaseURL: "https://api.together.xyz/v1", KeyURL: "https://api.together.ai/settings/api-keys", Brand: "together-ai", KeyRequired: true},
		{ID: "mistral", Label: "Mistral", Kind: KindOpenAICompat, DefaultBaseURL: "https://api.mistral.ai/v1", KeyURL: "https://console.mistral.ai/api-keys", DefaultModel: "mistral-large-latest", Brand: "mistral", KeyRequired: true},
		{ID: "deepseek", Label: "DeepSeek", Kind: KindOpenAICompat, DefaultBaseURL: "https://api.deepseek.com/v1", KeyURL: "https://platform.deepseek.com/api_keys", DefaultModel: "deepseek-chat", Brand: "deepseek", KeyRequired: true},
		{ID: "xai", Label: "xAI", Kind: KindOpenAICompat, DefaultBaseURL: "https://api.x.ai/v1", KeyURL: "https://console.x.ai/", Brand: "xai", KeyRequired: true},
		{ID: "fireworks", Label: "Fireworks AI", Kind: KindOpenAICompat, DefaultBaseURL: "https://api.fireworks.ai/inference/v1", KeyURL: "https://fireworks.ai/account/api-keys", Brand: "fireworks", KeyRequired: true},
		{ID: "ollama", Label: "Ollama", Kind: KindOpenAICompat, DefaultBaseURL: "http://localhost:11434/v1", BaseURLEditable: true, KeyURL: "https://ollama.com/", Brand: "ollama"},
		{ID: "vllm", Label: "vLLM", Kind: KindOpenAICompat, DefaultBaseURL: "http://localhost:8000/v1", BaseURLEditable: true, KeyURL: "https://docs.vllm.ai/", Brand: "vllm"},
		{ID: "lmstudio", Label: "LM Studio", Kind: KindOpenAICompat, DefaultBaseURL: "http://localhost:1234/v1", BaseURLEditable: true, KeyURL: "https://lmstudio.ai/", Brand: "lm-studio"},
		{ID: "openai_compatible", Label: "Other (OpenAI compatible)", Kind: KindOpenAICompat, BaseURLEditable: true},
	}
}

// LookupProvider returns the catalog entry for id.
func LookupProvider(id string) (ProviderInfo, bool) {
	for _, p := range Catalog() {
		if p.ID == id {
			return p, true
		}
	}
	return ProviderInfo{}, false
}
