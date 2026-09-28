package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// anthropicAPIURL is the real Messages API endpoint. AnthropicProvider
// always calls this directly over HTTP with the BYOK key from settings,
// never through a vendored SDK: one dependency-free HTTP client, the
// same shape internal/apiclient already uses for the platform's own
// REST API.
const anthropicAPIURL = "https://api.anthropic.com/v1/messages"

// anthropicVersion is the API version header every request must carry.
const anthropicVersion = "2023-06-01"

// AnthropicProvider implements Provider against the real Anthropic
// Messages API, using the streaming (server-sent events) variant so
// TextDeltaFunc receives real incremental chunks as the model generates
// them, not a single chunk after the full response lands.
type AnthropicProvider struct {
	apiKey     string
	httpClient *http.Client
}

// NewAnthropicProvider builds a provider using apiKey (the BYOK key
// resolved from internal/secrets at call time, never persisted here) and
// httpClient (defaulted to http.DefaultClient if nil).
func NewAnthropicProvider(apiKey string, httpClient *http.Client) *AnthropicProvider {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &AnthropicProvider{apiKey: apiKey, httpClient: httpClient}
}

// anthropicMessage/anthropicContentBlock/anthropicTool are the request
// wire shapes, matching the Messages API's own JSON exactly (verified
// against the Anthropic Go SDK's type definitions: ToolUseBlock{Type,
// ID, Name, Input}, ToolResultBlockParam{Type, ToolUseID, Content,
// IsError}).
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
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Tools     []anthropicTool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream"`
}

// defaultMaxTokens bounds a single turn's output. No env var override
// today: this is an implementation-detail request cap, not an
// operator-facing threshold like the timeouts CLAUDE.md's "no hardcoded
// thresholds" rule targets.
const defaultMaxTokens = 4096

func toAnthropicRequest(req ChatRequest) anthropicRequest {
	out := anthropicRequest{
		Model:     req.Model,
		MaxTokens: defaultMaxTokens,
		System:    req.System,
		Stream:    true,
	}
	for _, m := range req.Messages {
		out.Messages = append(out.Messages, toAnthropicMessage(m))
	}
	for _, t := range req.Tools {
		out.Tools = append(out.Tools, anthropicTool{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
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

// anthropicSSEEvent is the envelope every "data:" line in the stream
// decodes to; Type discriminates which of the fields below applies,
// matching the go-sdk's own MessageStreamEventUnion variants
// (message_start, content_block_start, content_block_delta,
// content_block_stop, message_delta, message_stop).
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

	// error
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// streamingToolUse accumulates one tool_use content block's id/name
// (known from content_block_start) and its input, streamed as
// input_json_delta chunks that must be concatenated before parsing.
type streamingToolUse struct {
	id, name  string
	inputJSON strings.Builder
}

// Complete implements Provider by POSTing req to the real Messages API
// with stream:true and parsing the server-sent event stream, forwarding
// every text delta to onText as it arrives.
func (p *AnthropicProvider) Complete(ctx context.Context, req ChatRequest, onText TextDeltaFunc) (TurnResult, error) {
	body, err := json.Marshal(toAnthropicRequest(req))
	if err != nil {
		return TurnResult{}, fmt.Errorf("ai: marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicAPIURL, bytes.NewReader(body))
	if err != nil {
		return TurnResult{}, fmt.Errorf("ai: build anthropic request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return TurnResult{}, fmt.Errorf("ai: call anthropic api: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return TurnResult{}, fmt.Errorf("ai: anthropic api returned %d: %s", resp.StatusCode, string(respBody))
	}

	return parseAnthropicStream(resp.Body, onText)
}

func parseAnthropicStream(body io.Reader, onText TextDeltaFunc) (TurnResult, error) {
	var result TurnResult
	toolUses := map[int]*streamingToolUse{} // index -> accumulating tool_use block
	var textBuilder strings.Builder

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue // blank lines, "event: ..." lines, and ": comment" pings are all ignorable
		}

		var ev anthropicSSEEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return TurnResult{}, fmt.Errorf("ai: decode anthropic stream event: %w", err)
		}

		switch ev.Type {
		case "content_block_start":
			if ev.ContentBlock == nil {
				continue
			}
			if ev.ContentBlock.Type == "tool_use" {
				toolUses[ev.Index] = &streamingToolUse{id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
			}
		case "content_block_delta":
			if ev.Delta == nil {
				continue
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
		case "message_stop":
			// Nothing further to accumulate; the loop ends when the
			// stream itself closes.
		case "error":
			if ev.Error != nil {
				return TurnResult{}, fmt.Errorf("ai: anthropic stream error: %s", ev.Error.Message)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return TurnResult{}, fmt.Errorf("ai: read anthropic stream: %w", err)
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

	if result.StopReason == "" {
		if len(result.ToolCalls) > 0 {
			result.StopReason = StopReasonToolUse
		} else {
			result.StopReason = StopReasonEndTurn
		}
	}
	return result, nil
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
