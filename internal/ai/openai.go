package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// OpenAICompat implements Backend for any server speaking OpenAI Chat
// Completions: OpenAI, Azure OpenAI, OpenRouter, Groq, Ollama, vLLM and so on.
type OpenAICompat struct {
	cfg AdapterConfig
}

// NewOpenAICompat builds the adapter; BaseURL must be set.
func NewOpenAICompat(cfg AdapterConfig) *OpenAICompat {
	return &OpenAICompat{cfg: cfg}
}

type oaiToolCallFn struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type oaiToolCall struct {
	Index    int           `json:"index"`
	ID       string        `json:"id,omitempty"`
	Type     string        `json:"type,omitempty"`
	Function oaiToolCallFn `json:"function"`
}

type oaiReqToolCall struct {
	ID       string        `json:"id"`
	Type     string        `json:"type"`
	Function oaiToolCallFn `json:"function"`
}

type oaiMessage struct {
	Role       string           `json:"role"`
	Content    *string          `json:"content"`
	ToolCalls  []oaiReqToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type oaiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description,omitempty"`
		Parameters  json.RawMessage `json:"parameters"`
	} `json:"function"`
}

type oaiRequest struct {
	Model         string       `json:"model"`
	Messages      []oaiMessage `json:"messages"`
	Tools         []oaiTool    `json:"tools,omitempty"`
	Stream        bool         `json:"stream"`
	StreamOptions *struct {
		IncludeUsage bool `json:"include_usage"`
	} `json:"stream_options,omitempty"`
}

func strPtr(s string) *string { return &s }

func toOpenAIRequest(req ChatRequest) oaiRequest {
	out := oaiRequest{Model: req.Model, Stream: true}
	out.StreamOptions = &struct {
		IncludeUsage bool `json:"include_usage"`
	}{IncludeUsage: true}
	if req.System != "" {
		out.Messages = append(out.Messages, oaiMessage{Role: "system", Content: strPtr(req.System)})
	}
	for _, m := range req.Messages {
		out.Messages = append(out.Messages, toOpenAIMessages(m)...)
	}
	for _, t := range req.Tools {
		var ot oaiTool
		ot.Type = "function"
		ot.Function.Name = t.Name
		ot.Function.Description = t.Description
		ot.Function.Parameters = t.InputSchema
		if len(ot.Function.Parameters) == 0 {
			ot.Function.Parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out.Tools = append(out.Tools, ot)
	}
	return out
}

func toOpenAIMessages(m Message) []oaiMessage {
	var texts []string
	var calls []oaiReqToolCall
	var results []oaiMessage
	for _, b := range m.Content {
		switch b.Type {
		case ContentBlockText:
			texts = append(texts, b.Text)
		case ContentBlockToolUse:
			args := string(b.ToolInput)
			if args == "" {
				args = "{}"
			}
			calls = append(calls, oaiReqToolCall{ID: b.ToolUseID, Type: "function", Function: oaiToolCallFn{Name: b.ToolName, Arguments: args}})
		case ContentBlockToolResult:
			results = append(results, oaiMessage{Role: "tool", ToolCallID: b.ToolResultForID, Content: strPtr(b.ToolResultContent)})
		}
	}
	var out []oaiMessage
	out = append(out, results...)
	if len(texts) > 0 || len(calls) > 0 {
		msg := oaiMessage{Role: m.Role, ToolCalls: calls}
		if len(texts) > 0 || len(calls) == 0 {
			msg.Content = strPtr(strings.Join(texts, "\n"))
		}
		out = append(out, msg)
	}
	return out
}

type oaiStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string        `json:"content"`
			ToolCalls []oaiToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

type oaiAccum struct {
	id, name string
	args     strings.Builder
}

// Complete implements Provider with a streaming Chat Completions call.
func (p *OpenAICompat) Complete(ctx context.Context, req ChatRequest, onText TextDeltaFunc) (TurnResult, error) {
	body, err := json.Marshal(toOpenAIRequest(req))
	if err != nil {
		return TurnResult{}, fmt.Errorf("ai: marshal openai request: %w", err)
	}
	httpReq, err := p.newRequest(ctx, http.MethodPost, "/chat/completions", body)
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
	return parseOpenAIStream(p.cfg.ProviderID, resp.Body, onText)
}

func (p *OpenAICompat) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, joinURL(p.cfg.BaseURL, path), rd)
	if err != nil {
		return nil, fmt.Errorf("ai: build %s request: %w", p.cfg.ProviderID, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		if p.cfg.ProviderID == "azure_openai" {
			req.Header.Set("api-key", p.cfg.APIKey)
		} else {
			req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
		}
	}
	return req, nil
}

func parseOpenAIStream(provider string, body io.Reader, onText TextDeltaFunc) (TurnResult, error) {
	var result TurnResult
	var text strings.Builder
	calls := map[int]*oaiAccum{}
	finished := false

	err := readSSEData(body, func(data string) error {
		if data == "[DONE]" {
			finished = true
			return nil
		}
		var ch oaiStreamChunk
		if err := json.Unmarshal([]byte(data), &ch); err != nil {
			return &ProviderError{Provider: provider, Class: ClassBadRequest, Detail: "undecodable stream chunk"}
		}
		if ch.Error != nil {
			return ClassifyHTTP(provider, http.StatusInternalServerError, []byte(fmt.Sprintf(`{"error":{"message":%q,"type":%q}}`, ch.Error.Message, ch.Error.Type)), nil, "", time.Now())
		}
		if ch.Usage != nil {
			result.Usage = Usage{PromptTokens: ch.Usage.PromptTokens, CompletionTokens: ch.Usage.CompletionTokens}
		}
		for _, c := range ch.Choices {
			if c.Delta.Content != "" {
				text.WriteString(c.Delta.Content)
				if onText != nil {
					onText(c.Delta.Content)
				}
			}
			for _, tc := range c.Delta.ToolCalls {
				acc := calls[tc.Index]
				if acc == nil {
					acc = &oaiAccum{}
					calls[tc.Index] = acc
				}
				if tc.ID != "" {
					acc.id = tc.ID
				}
				if tc.Function.Name != "" {
					acc.name = tc.Function.Name
				}
				acc.args.WriteString(tc.Function.Arguments)
			}
			if c.FinishReason != "" {
				finished = true
				result.StopReason = mapOpenAIFinish(c.FinishReason)
			}
		}
		return nil
	})
	if err != nil {
		if _, ok := AsProviderError(err); ok {
			return TurnResult{Text: text.String()}, err
		}
		return TurnResult{Text: text.String()}, ClassifyTransport(provider, err, "")
	}
	if !finished {
		return TurnResult{Text: text.String()}, &ProviderError{Provider: provider, Class: ClassStreamCut, Detail: "stream ended before the final chunk"}
	}

	result.Text = text.String()
	idx := make([]int, 0, len(calls))
	for i := range calls {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for _, i := range idx {
		acc := calls[i]
		args := acc.args.String()
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		id := acc.id
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		result.ToolCalls = append(result.ToolCalls, ToolUseCall{ID: id, Name: acc.name, Input: json.RawMessage(args)})
	}
	if result.StopReason == StopReasonMaxTokens && anyInvalidToolInput(result.ToolCalls) {
		return TurnResult{Text: result.Text}, &ProviderError{Provider: provider, Class: ClassStreamCut, Detail: "output limit hit mid tool call"}
	}
	if len(result.ToolCalls) > 0 {
		result.StopReason = StopReasonToolUse
	} else if result.StopReason == "" {
		result.StopReason = StopReasonEndTurn
	}
	return result, nil
}

func mapOpenAIFinish(r string) string {
	switch r {
	case "stop":
		return StopReasonEndTurn
	case "tool_calls", "function_call":
		return StopReasonToolUse
	case "length":
		return StopReasonMaxTokens
	default:
		return StopReasonOther
	}
}

// ListModels implements Backend via GET {base}/models.
func (p *OpenAICompat) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := p.newRequest(ctx, http.MethodGet, "/models", nil)
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
			ID            string `json:"id"`
			ContextLength int    `json:"context_length"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&out); err != nil {
		return nil, &ProviderError{Provider: p.cfg.ProviderID, Class: ClassBadRequest, Detail: "model list was not valid JSON"}
	}
	models := make([]ModelInfo, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			models = append(models, ModelInfo{ID: m.ID, Label: m.ID, ContextWindow: m.ContextLength})
		}
	}
	return models, nil
}

// Validate lists models; a server without a models route (404 or 405) still
// counts as reachable, since only auth and transport failures matter here.
func (p *OpenAICompat) Validate(ctx context.Context) error {
	_, err := p.ListModels(ctx)
	if pe, ok := AsProviderError(err); ok && (pe.Status == http.StatusNotFound || pe.Status == http.StatusMethodNotAllowed) {
		return nil
	}
	return err
}
