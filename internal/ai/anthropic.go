package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// anthropicDefaultBaseURL is the real Messages API host. The adapter calls
// it directly over HTTP with the BYOK key, never through a vendored SDK.
const anthropicDefaultBaseURL = "https://api.anthropic.com"

// anthropicVersion is the API version header every request must carry.
const anthropicVersion = "2023-06-01"

const anthropicProviderID = "anthropic"

// AnthropicProvider implements Backend against the Anthropic Messages API,
// using the streaming (server-sent events) variant so TextDeltaFunc gets
// real incremental chunks.
type AnthropicProvider struct {
	cfg AdapterConfig
}

// NewAnthropicProvider builds a provider using apiKey and httpClient
// (guarded default if nil), pointed at the public API.
func NewAnthropicProvider(apiKey string, httpClient *http.Client) *AnthropicProvider {
	return NewAnthropic(AdapterConfig{ProviderID: anthropicProviderID, APIKey: apiKey, HTTP: httpClient})
}

// NewAnthropic builds the adapter from cfg; an empty BaseURL means the public API.
func NewAnthropic(cfg AdapterConfig) *AnthropicProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = anthropicDefaultBaseURL
	}
	if cfg.ProviderID == "" {
		cfg.ProviderID = anthropicProviderID
	}
	return &AnthropicProvider{cfg: cfg}
}

func (p *AnthropicProvider) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, joinURL(p.cfg.BaseURL, path), rd)
	if err != nil {
		return nil, fmt.Errorf("ai: build anthropic request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.cfg.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	return req, nil
}

// Request wire shapes, matching the Messages API JSON exactly.
type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`

	Text string `json:"text,omitempty"`

	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`

	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

type anthropicTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"input_schema"`

	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicCacheControl struct {
	Type string `json:"type"`
}

type anthropicSystemBlock struct {
	Type         string                 `json:"type"`
	Text         string                 `json:"text"`
	CacheControl *anthropicCacheControl `json:"cache_control,omitempty"`
}

type anthropicRequest struct {
	Model     string                 `json:"model"`
	MaxTokens int                    `json:"max_tokens"`
	System    []anthropicSystemBlock `json:"system,omitempty"`
	Messages  []anthropicMessage     `json:"messages"`
	Tools     []anthropicTool        `json:"tools,omitempty"`
	Stream    bool                   `json:"stream"`
}

// An implementation cap, not an operator threshold, so it is deliberately not an env var.
const defaultMaxTokens = 4096

func toAnthropicRequest(req ChatRequest) anthropicRequest {
	out := anthropicRequest{
		Model:     req.Model,
		MaxTokens: defaultMaxTokens,
		Stream:    true,
	}
	// The system prompt and tool list are stable across turns, so mark the
	// end of each as a cache breakpoint.
	if req.System != "" {
		out.System = []anthropicSystemBlock{{Type: "text", Text: req.System, CacheControl: &anthropicCacheControl{Type: "ephemeral"}}}
	}
	for _, m := range req.Messages {
		out.Messages = append(out.Messages, toAnthropicMessage(m))
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, anthropicTool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	if n := len(out.Tools); n > 0 {
		out.Tools[n-1].CacheControl = &anthropicCacheControl{Type: "ephemeral"}
	}
	return out
}

func toAnthropicMessage(m Message) anthropicMessage {
	out := anthropicMessage{Role: m.Role}
	for _, b := range m.Content {
		switch b.Type {
		case ContentBlockText:
			out.Content = append(out.Content, anthropicContentBlock{Type: "text", Text: b.Text})
		case ContentBlockToolUse:
			out.Content = append(out.Content, anthropicContentBlock{
				Type: "tool_use", ID: b.ToolUseID, Name: b.ToolName, Input: b.ToolInput,
			})
		case ContentBlockToolResult:
			out.Content = append(out.Content, anthropicContentBlock{
				Type: "tool_result", ToolUseID: b.ToolResultForID, Content: b.ToolResultContent, IsError: b.ToolResultIsError,
			})
		}
	}
	return out
}

// Type selects which field applies, mirroring the SDK's stream event variants.
type anthropicSSEEvent struct {
	Type string `json:"type"`

	// content_block_start
	Index        int `json:"index"`
	ContentBlock *struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`

	// content_block_delta
	Delta *struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`

	// message_start carries input usage; message_delta carries output usage.
	Message *struct {
		Usage anthropicUsage `json:"usage"`
	} `json:"message"`
	Usage *anthropicUsage `json:"usage"`

	// error
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

// streamingToolUse accumulates one tool_use content block's id/name
// (known from content_block_start) and its input, streamed as
// input_json_delta chunks that must be concatenated before parsing.
type streamingToolUse struct {
	id, name  string
	inputJSON strings.Builder
}

// Complete implements Provider by POSTing req to the Messages API with
// stream:true and parsing the server-sent event stream, forwarding every
// text delta to onText as it arrives.
func (p *AnthropicProvider) Complete(ctx context.Context, req ChatRequest, onText TextDeltaFunc) (TurnResult, error) {
	body, err := json.Marshal(toAnthropicRequest(req))
	if err != nil {
		return TurnResult{}, fmt.Errorf("ai: marshal anthropic request: %w", err)
	}
	httpReq, err := p.newRequest(ctx, http.MethodPost, "/v1/messages", body)
	if err != nil {
		return TurnResult{}, err
	}
	resp, err := p.cfg.client().Do(httpReq)
	if err != nil {
		return TurnResult{}, ClassifyTransport(p.cfg.ProviderID, err, p.cfg.APIKey)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return TurnResult{}, httpFailure(p.cfg.ProviderID, resp, p.cfg.APIKey)
	}
	res, err := parseAnthropicStream(resp.Body, onText)
	if pe, ok := AsProviderError(err); ok {
		pe.Provider = p.cfg.ProviderID
	}
	return res, err
}

// httpFailure reads a bounded error body and classifies it.
func httpFailure(provider string, resp *http.Response, key string) *ProviderError {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	return ClassifyHTTP(provider, resp.StatusCode, b, resp.Header, key, time.Now())
}

// ListModels implements Backend via GET /v1/models.
func (p *AnthropicProvider) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := p.newRequest(ctx, http.MethodGet, "/v1/models?limit=100", nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.cfg.client().Do(req)
	if err != nil {
		return nil, ClassifyTransport(p.cfg.ProviderID, err, p.cfg.APIKey)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, httpFailure(p.cfg.ProviderID, resp, p.cfg.APIKey)
	}
	var out struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, &ProviderError{Provider: p.cfg.ProviderID, Class: ClassBadRequest, Detail: "model list was not valid JSON"}
	}
	tools := true
	models := make([]ModelInfo, 0, len(out.Data))
	for _, m := range out.Data {
		label := m.DisplayName
		if label == "" {
			label = m.ID
		}
		models = append(models, ModelInfo{ID: m.ID, Label: label, SupportsTools: &tools})
	}
	return models, nil
}

// Validate implements Backend with one model-list call.
func (p *AnthropicProvider) Validate(ctx context.Context) error {
	_, err := p.ListModels(ctx)
	return err
}

func parseAnthropicStream(body io.Reader, onText TextDeltaFunc) (TurnResult, error) {
	var result TurnResult
	toolUses := map[int]*streamingToolUse{}
	var textBuilder strings.Builder
	stopped := false

	err := readSSEData(body, func(data string) error {
		var ev anthropicSSEEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return &ProviderError{Provider: anthropicProviderID, Class: ClassBadRequest, Detail: "undecodable stream event"}
		}
		switch ev.Type {
		case "message_start":
			if ev.Message != nil {
				u := ev.Message.Usage
				result.Usage.PromptTokens = u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
			}
		case "content_block_start":
			if ev.ContentBlock != nil && ev.ContentBlock.Type == "tool_use" {
				toolUses[ev.Index] = &streamingToolUse{id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
			}
		case "content_block_delta":
			if ev.Delta == nil {
				return nil
			}
			switch ev.Delta.Type {
			case "text_delta":
				textBuilder.WriteString(ev.Delta.Text)
				if onText != nil {
					onText(ev.Delta.Text)
				}
			case "input_json_delta":
				if tu, ok := toolUses[ev.Index]; ok {
					tu.inputJSON.WriteString(ev.Delta.PartialJSON)
				}
			}
		case "message_delta":
			if ev.Delta != nil && ev.Delta.StopReason != "" {
				result.StopReason = ev.Delta.StopReason
			}
			if ev.Usage != nil {
				result.Usage.CompletionTokens = ev.Usage.OutputTokens
			}
		case "message_stop":
			stopped = true
		case "error":
			if ev.Error != nil {
				class := ClassBadRequest
				switch ev.Error.Type {
				case "overloaded_error", "api_error":
					class = ClassOverloaded
				case "rate_limit_error":
					class = ClassRateLimited
				case "authentication_error", "permission_error":
					class = ClassInvalidKey
				}
				return &ProviderError{Provider: anthropicProviderID, Class: class, Detail: Scrub(ev.Error.Message, "")}
			}
		}
		return nil
	})
	if err != nil {
		if _, ok := AsProviderError(err); ok {
			return TurnResult{Text: textBuilder.String()}, err
		}
		return TurnResult{Text: textBuilder.String()}, ClassifyTransport(anthropicProviderID, err, "")
	}
	if !stopped {
		return TurnResult{Text: textBuilder.String()}, &ProviderError{Provider: anthropicProviderID, Class: ClassStreamCut, Detail: "stream ended before message_stop"}
	}

	result.Text = textBuilder.String()
	for _, tu := range toolUses {
		input := tu.inputJSON.String()
		if input == "" {
			input = "{}"
		}
		result.ToolCalls = append(result.ToolCalls, ToolUseCall{ID: tu.id, Name: tu.name, Input: json.RawMessage(input)})
	}
	result.ToolCalls = sortToolCallsByOrderSeen(result.ToolCalls, toolUses)

	if result.StopReason == "max_tokens" && anyInvalidToolInput(result.ToolCalls) {
		return TurnResult{Text: result.Text}, &ProviderError{Provider: anthropicProviderID, Class: ClassStreamCut, Detail: "output limit hit mid tool call"}
	}
	if result.StopReason == "" {
		if len(result.ToolCalls) > 0 {
			result.StopReason = StopReasonToolUse
		} else {
			result.StopReason = StopReasonEndTurn
		}
	}
	return result, nil
}

func anyInvalidToolInput(calls []ToolUseCall) bool {
	for _, c := range calls {
		if !json.Valid(c.Input) {
			return true
		}
	}
	return false
}

// sortToolCallsByOrderSeen restores stream order: Go map iteration above
// is unordered, but tool calls must be returned in the order the model
// emitted them for a deterministic, reproducible transcript.
func sortToolCallsByOrderSeen(calls []ToolUseCall, byIndex map[int]*streamingToolUse) []ToolUseCall {
	if len(calls) < 2 {
		return calls
	}
	order := make(map[string]int, len(byIndex))
	for idx, tu := range byIndex {
		order[tu.id] = idx
	}
	sorted := make([]ToolUseCall, len(calls))
	copy(sorted, calls)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && order[sorted[j-1].ID] > order[sorted[j].ID]; j-- {
			sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
		}
	}
	return sorted
}
