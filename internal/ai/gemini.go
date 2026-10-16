package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Gemini implements Backend against the native Gemini generateContent API.
type Gemini struct {
	cfg AdapterConfig
}

// NewGemini builds the adapter; an empty BaseURL means the public API.
func NewGemini(cfg AdapterConfig) *Gemini {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	if cfg.ProviderID == "" {
		cfg.ProviderID = "gemini"
	}
	return &Gemini{cfg: cfg}
}

type gemPart struct {
	Text             string           `json:"text,omitempty"`
	FunctionCall     *gemFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *gemFunctionResp `json:"functionResponse,omitempty"`
}

type gemFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type gemFunctionResp struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type gemContent struct {
	Role  string    `json:"role,omitempty"`
	Parts []gemPart `json:"parts"`
}

type gemRequest struct {
	SystemInstruction *gemContent  `json:"systemInstruction,omitempty"`
	Contents          []gemContent `json:"contents"`
	Tools             []struct {
		FunctionDeclarations []gemDecl `json:"functionDeclarations"`
	} `json:"tools,omitempty"`
}

type gemDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// geminiUnsupportedSchemaKeys are JSON Schema keywords Gemini's function
// declaration schema rejects.
var geminiUnsupportedSchemaKeys = map[string]bool{
	"$schema": true, "additionalProperties": true, "$ref": true, "$defs": true,
	"definitions": true, "default": true, "examples": true, "title": true,
}

func sanitizeGeminiSchema(raw json.RawMessage) json.RawMessage {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	out, err := json.Marshal(stripSchema(v))
	if err != nil {
		return nil
	}
	return out
}

func stripSchema(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if geminiUnsupportedSchemaKeys[k] {
				continue
			}
			out[k] = stripSchema(val)
		}
		if props, ok := out["properties"].(map[string]any); ok && len(props) == 0 {
			delete(out, "properties")
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = stripSchema(val)
		}
		return out
	default:
		return v
	}
}

func toGeminiRequest(req ChatRequest) gemRequest {
	var out gemRequest
	if req.System != "" {
		out.SystemInstruction = &gemContent{Parts: []gemPart{{Text: req.System}}}
	}
	names := map[string]string{}
	for _, m := range req.Messages {
		var parts []gemPart
		for _, b := range m.Content {
			switch b.Type {
			case ContentBlockText:
				parts = append(parts, gemPart{Text: b.Text})
			case ContentBlockToolUse:
				names[b.ToolUseID] = b.ToolName
				args := b.ToolInput
				if len(args) == 0 || !json.Valid(args) {
					args = json.RawMessage(`{}`)
				}
				parts = append(parts, gemPart{FunctionCall: &gemFunctionCall{Name: b.ToolName, Args: args}})
			case ContentBlockToolResult:
				key := "output"
				if b.ToolResultIsError {
					key = "error"
				}
				parts = append(parts, gemPart{FunctionResponse: &gemFunctionResp{
					Name: names[b.ToolResultForID], Response: map[string]any{key: b.ToolResultContent},
				}})
			}
		}
		if len(parts) == 0 {
			continue
		}
		role := "user"
		if m.Role == RoleAssistant {
			role = "model"
		}
		out.Contents = append(out.Contents, gemContent{Role: role, Parts: parts})
	}
	if len(req.Tools) > 0 {
		decls := make([]gemDecl, 0, len(req.Tools))
		for _, t := range req.Tools {
			decls = append(decls, gemDecl{Name: t.Name, Description: t.Description, Parameters: sanitizeGeminiSchema(t.InputSchema)})
		}
		out.Tools = append(out.Tools, struct {
			FunctionDeclarations []gemDecl `json:"functionDeclarations"`
		}{decls})
	}
	return out
}

func (p *Gemini) newRequest(ctx context.Context, method, path string, body []byte) (*http.Request, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, joinURL(p.cfg.BaseURL, path), rd)
	if err != nil {
		return nil, fmt.Errorf("ai: build gemini request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", p.cfg.APIKey)
	return req, nil
}

type gemChunk struct {
	Candidates []struct {
		Content struct {
			Parts []gemPart `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata *struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
}

// Complete implements Provider via streamGenerateContent with alt=sse.
func (p *Gemini) Complete(ctx context.Context, req ChatRequest, onText TextDeltaFunc) (TurnResult, error) {
	body, err := json.Marshal(toGeminiRequest(req))
	if err != nil {
		return TurnResult{}, fmt.Errorf("ai: marshal gemini request: %w", err)
	}
	httpReq, err := p.newRequest(ctx, http.MethodPost, "", body)
	if err != nil {
		return TurnResult{}, err
	}
	u, perr := url.Parse(joinURL(p.cfg.BaseURL, "/models/"+strings.TrimPrefix(req.Model, "models/")+":streamGenerateContent"))
	if perr != nil {
		return TurnResult{}, fmt.Errorf("ai: build gemini url: %w", perr)
	}
	q := u.Query()
	q.Set("alt", "sse")
	u.RawQuery = q.Encode()
	httpReq.URL = u
	resp, err := p.cfg.client().Do(httpReq)
	if err != nil {
		return TurnResult{}, ClassifyTransport(p.cfg.ProviderID, err, p.cfg.APIKey)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return TurnResult{}, httpFailure(p.cfg.ProviderID, resp, p.cfg.APIKey)
	}
	return parseGeminiStream(p.cfg.ProviderID, resp.Body, onText)
}

func parseGeminiStream(provider string, body io.Reader, onText TextDeltaFunc) (TurnResult, error) {
	var result TurnResult
	var text strings.Builder
	finished := false
	n := 0

	err := readSSEData(body, func(data string) error {
		var ch gemChunk
		if err := json.Unmarshal([]byte(data), &ch); err != nil {
			return &ProviderError{Provider: provider, Class: ClassBadRequest, Detail: "undecodable stream chunk"}
		}
		if ch.UsageMetadata != nil {
			result.Usage = Usage{PromptTokens: ch.UsageMetadata.PromptTokenCount, CompletionTokens: ch.UsageMetadata.CandidatesTokenCount}
		}
		for _, c := range ch.Candidates {
			for _, part := range c.Content.Parts {
				if part.Text != "" {
					text.WriteString(part.Text)
					if onText != nil {
						onText(part.Text)
					}
				}
				if fc := part.FunctionCall; fc != nil {
					args := fc.Args
					if len(args) == 0 {
						args = json.RawMessage(`{}`)
					}
					n++
					result.ToolCalls = append(result.ToolCalls, ToolUseCall{ID: fmt.Sprintf("gemini_call_%d", n), Name: fc.Name, Input: args})
				}
			}
			if c.FinishReason != "" {
				finished = true
				result.StopReason = mapGeminiFinish(c.FinishReason)
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
		return TurnResult{Text: text.String()}, &ProviderError{Provider: provider, Class: ClassStreamCut, Detail: "stream ended before a finish reason"}
	}
	result.Text = text.String()
	if len(result.ToolCalls) > 0 {
		result.StopReason = StopReasonToolUse
	} else if result.StopReason == "" {
		result.StopReason = StopReasonEndTurn
	}
	return result, nil
}

func mapGeminiFinish(r string) string {
	switch r {
	case "STOP":
		return StopReasonEndTurn
	case "MAX_TOKENS":
		return StopReasonMaxTokens
	default:
		return StopReasonOther
	}
}

// ListModels implements Backend via GET /models, keeping models that can generate content.
func (p *Gemini) ListModels(ctx context.Context) ([]ModelInfo, error) {
	req, err := p.newRequest(ctx, http.MethodGet, "/models?pageSize=200", nil)
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
		Models []struct {
			Name             string   `json:"name"`
			DisplayName      string   `json:"displayName"`
			InputTokenLimit  int      `json:"inputTokenLimit"`
			SupportedMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&out); err != nil {
		return nil, &ProviderError{Provider: p.cfg.ProviderID, Class: ClassBadRequest, Detail: "model list was not valid JSON"}
	}
	tools := true
	models := make([]ModelInfo, 0, len(out.Models))
	for _, m := range out.Models {
		canGenerate := false
		for _, method := range m.SupportedMethods {
			canGenerate = canGenerate || method == "generateContent"
		}
		if !canGenerate {
			continue
		}
		id := strings.TrimPrefix(m.Name, "models/")
		label := m.DisplayName
		if label == "" {
			label = id
		}
		models = append(models, ModelInfo{ID: id, Label: label, ContextWindow: m.InputTokenLimit, SupportsTools: &tools})
	}
	return models, nil
}

// Validate implements Backend with one model-list call.
func (p *Gemini) Validate(ctx context.Context) error {
	_, err := p.ListModels(ctx)
	return err
}
