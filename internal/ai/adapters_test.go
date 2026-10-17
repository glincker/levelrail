package ai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testKey = "sk-live-SECRETKEY1234567890"

func sseServer(t *testing.T, status int, body string, capture *capturedReq) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if capture != nil {
			b, _ := io.ReadAll(r.Body)
			capture.path, capture.query, capture.header, capture.body = r.URL.Path, r.URL.RawQuery, r.Header.Clone(), string(b)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

type capturedReq struct {
	path, query, body string
	header            http.Header
}

var weatherTool = ToolSpec{Name: "get_app", Description: "d", InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"],"additionalProperties":false}`)}

func toolConversation() ChatRequest {
	return ChatRequest{
		System: "sys", Model: "m1", Tools: []ToolSpec{weatherTool},
		Messages: []Message{
			{Role: RoleUser, Content: []ContentBlock{{Type: ContentBlockText, Text: "hi"}}},
			{Role: RoleAssistant, Content: []ContentBlock{{Type: ContentBlockToolUse, ToolUseID: "c1", ToolName: "get_app", ToolInput: json.RawMessage(`{"name":"web"}`)}}},
			{Role: RoleUser, Content: []ContentBlock{{Type: ContentBlockToolResult, ToolResultForID: "c1", ToolResultContent: "ok"}}},
		},
	}
}

func TestAnthropic_RequestAndStream(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":5}}}`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"tu1","name":"get_app"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"na"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"me\":\"web\"}"}}`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}`,
		`data: {"type":"message_stop"}`,
	}, "\n\n") + "\n\n"
	var got capturedReq
	srv := sseServer(t, 200, sse, &got)
	p := NewAnthropic(AdapterConfig{ProviderID: "anthropic", BaseURL: srv.URL, APIKey: testKey, HTTP: srv.Client()})
	res, err := p.Complete(context.Background(), toolConversation(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.path != "/v1/messages" || got.header.Get("x-api-key") != testKey || got.header.Get("anthropic-version") == "" {
		t.Errorf("bad request: %+v", got)
	}
	var sent struct {
		System []struct {
			CacheControl *struct{ Type string } `json:"cache_control"`
		} `json:"system"`
		Tools []struct {
			CacheControl *struct{ Type string } `json:"cache_control"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(got.body), &sent); err != nil {
		t.Fatal(err)
	}
	if len(sent.System) != 1 || sent.System[0].CacheControl == nil || len(sent.Tools) != 1 || sent.Tools[0].CacheControl == nil {
		t.Errorf("cache_control missing on system or last tool: %s", got.body)
	}
	if len(res.ToolCalls) != 1 || string(res.ToolCalls[0].Input) != `{"name":"web"}` || res.StopReason != StopReasonToolUse {
		t.Errorf("result = %+v", res)
	}
	if res.Usage.PromptTokens != 15 || res.Usage.CompletionTokens != 7 {
		t.Errorf("usage = %+v", res.Usage)
	}
}

func TestAnthropic_StreamCutMidToolCall(t *testing.T) {
	sse := strings.Join([]string{
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text"}}`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial answer"}}`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tu1","name":"get_app"}}`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"na"}}`,
	}, "\n\n") + "\n\n"
	srv := sseServer(t, 200, sse, nil)
	p := NewAnthropic(AdapterConfig{BaseURL: srv.URL, APIKey: testKey, HTTP: srv.Client()})
	res, err := p.Complete(context.Background(), toolConversation(), nil)
	pe, ok := AsProviderError(err)
	if !ok || pe.Class != ClassStreamCut {
		t.Fatalf("err = %v, want stream_interrupted", err)
	}
	if res.Text != "partial answer" || len(res.ToolCalls) != 0 {
		t.Errorf("partial result = %+v, want text kept and no tool call", res)
	}
}

func TestAnthropic_ErrorsListAndValidate(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   ErrorClass
	}{
		{"invalid key echoes key", 401, `{"error":{"type":"authentication_error","message":"invalid x-api-key ` + testKey + `"}}`, ClassInvalidKey},
		{"overloaded", 529, `{"error":{"type":"overloaded_error","message":"Overloaded"}}`, ClassOverloaded},
		{"non json", 502, `<html>bad gateway</html>`, ClassOverloaded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := sseServer(t, tc.status, tc.body, nil)
			p := NewAnthropic(AdapterConfig{BaseURL: srv.URL, APIKey: testKey, HTTP: srv.Client()})
			err := p.Validate(context.Background())
			pe, ok := AsProviderError(err)
			if !ok || pe.Class != tc.want {
				t.Fatalf("err = %v, want class %s", err, tc.want)
			}
			if strings.Contains(err.Error(), "SECRETKEY") {
				t.Errorf("error text leaks the key: %v", err)
			}
		})
	}
	srv := sseServer(t, 200, `{"data":[{"id":"claude-sonnet-5-5","display_name":"Sonnet 5.5"}]}`, nil)
	p := NewAnthropic(AdapterConfig{BaseURL: srv.URL, APIKey: testKey, HTTP: srv.Client()})
	models, err := p.ListModels(context.Background())
	if err != nil || len(models) != 1 || models[0].Label != "Sonnet 5.5" {
		t.Errorf("models = %+v, err = %v", models, err)
	}
}

func TestOpenAICompat_RequestMapping(t *testing.T) {
	var got capturedReq
	srv := sseServer(t, 200, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", &got)
	p := NewOpenAICompat(AdapterConfig{ProviderID: "openai", BaseURL: srv.URL + "/v1", APIKey: testKey, HTTP: srv.Client()})
	if _, err := p.Complete(context.Background(), toolConversation(), nil); err != nil {
		t.Fatal(err)
	}
	if got.path != "/v1/chat/completions" || got.header.Get("Authorization") != "Bearer "+testKey {
		t.Errorf("request = %+v", got)
	}
	var sent struct {
		Messages []struct {
			Role       string `json:"role"`
			ToolCallID string `json:"tool_call_id"`
			ToolCalls  []struct {
				ID string `json:"id"`
			} `json:"tool_calls"`
		} `json:"messages"`
		Tools []struct {
			Type string `json:"type"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(got.body), &sent); err != nil {
		t.Fatal(err)
	}
	roles := []string{}
	for _, m := range sent.Messages {
		roles = append(roles, m.Role)
	}
	if strings.Join(roles, ",") != "system,user,assistant,tool" || sent.Messages[2].ToolCalls[0].ID != "c1" || sent.Messages[3].ToolCallID != "c1" || sent.Tools[0].Type != "function" {
		t.Errorf("messages = %v body = %s", roles, got.body)
	}
}

func TestOpenAICompat_AzureUsesAPIKeyHeader(t *testing.T) {
	var got capturedReq
	srv := sseServer(t, 200, `{"data":[]}`, &got)
	p := NewOpenAICompat(AdapterConfig{ProviderID: "azure_openai", BaseURL: srv.URL + "/openai/v1?api-version=x", APIKey: testKey, HTTP: srv.Client()})
	models, err := p.ListModels(context.Background())
	if err != nil || models == nil || len(models) != 0 {
		t.Fatalf("empty list: models = %v err = %v", models, err)
	}
	if got.header.Get("api-key") != testKey || got.header.Get("Authorization") != "" || got.path != "/openai/v1/models" || got.query != "api-version=x" {
		t.Errorf("azure request = %+v", got)
	}
}

func TestOpenAICompat_StreamPartialToolJSONAndUsage(t *testing.T) {
	chunks := []string{
		`{"choices":[{"delta":{"content":"Let me "}}]}`,
		`{"choices":[{"delta":{"content":"look."}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"get_app","arguments":"{\"na"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"me\":\"web\"}"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"name":"get_app","arguments":"{\"name\":\"api\"}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":11,"completion_tokens":4}}`,
	}
	body := "data: " + strings.Join(chunks, "\n\ndata: ") + "\n\ndata: [DONE]\n\n"
	srv := sseServer(t, 200, body, nil)
	p := NewOpenAICompat(AdapterConfig{ProviderID: "ollama", BaseURL: srv.URL, HTTP: srv.Client()})
	var streamed []string
	res, err := p.Complete(context.Background(), toolConversation(), func(s string) { streamed = append(streamed, s) })
	if err != nil {
		t.Fatal(err)
	}
	if res.Text != "Let me look." || len(streamed) != 2 || len(res.ToolCalls) != 2 {
		t.Fatalf("res = %+v streamed = %v", res, streamed)
	}
	if string(res.ToolCalls[0].Input) != `{"name":"web"}` || res.ToolCalls[1].ID != "call_1" || res.StopReason != StopReasonToolUse {
		t.Errorf("tool calls = %+v stop = %s", res.ToolCalls, res.StopReason)
	}
	if res.Usage.PromptTokens != 11 || res.Usage.CompletionTokens != 4 {
		t.Errorf("usage = %+v", res.Usage)
	}
}

func TestOpenAICompat_MalformedAndCutStreams(t *testing.T) {
	cases := []struct {
		name, body string
		want       ErrorClass
	}{
		{"cut before finish", "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n", ClassStreamCut},
		{"non json chunk", "data: not-json\n\n", ClassBadRequest},
		{"error chunk", "data: {\"error\":{\"message\":\"The server is overloaded\",\"type\":\"server_error\"}}\n\n", ClassOverloaded},
		{"length mid tool call", "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"a\",\"function\":{\"name\":\"get_app\",\"arguments\":\"{\\\"na\"}}]},\"finish_reason\":\"length\"}]}\n\ndata: [DONE]\n\n", ClassStreamCut},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := sseServer(t, 200, tc.body, nil)
			p := NewOpenAICompat(AdapterConfig{ProviderID: "groq", BaseURL: srv.URL, APIKey: testKey, HTTP: srv.Client()})
			_, err := p.Complete(context.Background(), toolConversation(), nil)
			if pe, ok := AsProviderError(err); !ok || pe.Class != tc.want {
				t.Fatalf("err = %v, want %s", err, tc.want)
			}
		})
	}
}

func TestOpenAICompat_ListModelsNonJSONAndValidate404(t *testing.T) {
	srv := sseServer(t, 200, "<html>nope</html>", nil)
	p := NewOpenAICompat(AdapterConfig{ProviderID: "vllm", BaseURL: srv.URL, HTTP: srv.Client()})
	if _, err := p.ListModels(context.Background()); err == nil {
		t.Error("non-JSON model list should be an error")
	}
	srv404 := sseServer(t, 404, `{"error":"no route"}`, nil)
	p = NewOpenAICompat(AdapterConfig{ProviderID: "vllm", BaseURL: srv404.URL, HTTP: srv404.Client()})
	if err := p.Validate(context.Background()); err != nil {
		t.Errorf("Validate on a server without /models = %v, want nil", err)
	}
	srv401 := sseServer(t, 401, `{"error":{"message":"Incorrect API key provided: `+testKey+`"}}`, nil)
	p = NewOpenAICompat(AdapterConfig{ProviderID: "openai", BaseURL: srv401.URL, APIKey: testKey, HTTP: srv401.Client()})
	err := p.Validate(context.Background())
	if pe, ok := AsProviderError(err); !ok || pe.Class != ClassInvalidKey || strings.Contains(err.Error(), "SECRETKEY") {
		t.Errorf("err = %v", err)
	}
}

func TestGemini_RequestMappingAndStream(t *testing.T) {
	body := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok \"}]}}]}\n\n" +
		"data: {\"candidates\":[{\"content\":{\"parts\":[{\"functionCall\":{\"name\":\"get_app\",\"args\":{\"name\":\"web\"}}}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":9,\"candidatesTokenCount\":3}}\n\n"
	var got capturedReq
	srv := sseServer(t, 200, body, &got)
	p := NewGemini(AdapterConfig{BaseURL: srv.URL + "/v1beta", APIKey: testKey, HTTP: srv.Client()})
	res, err := p.Complete(context.Background(), toolConversation(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.path != "/v1beta/models/m1:streamGenerateContent" || got.query != "alt=sse" || got.header.Get("x-goog-api-key") != testKey {
		t.Errorf("request = %+v", got)
	}
	if strings.Contains(got.body, "additionalProperties") || !strings.Contains(got.body, `"functionResponse":{"name":"get_app"`) || !strings.Contains(got.body, `"role":"model"`) {
		t.Errorf("body not mapped: %s", got.body)
	}
	if res.Text != "ok " || len(res.ToolCalls) != 1 || string(res.ToolCalls[0].Input) != `{"name":"web"}` || res.StopReason != StopReasonToolUse {
		t.Errorf("res = %+v", res)
	}
	if res.Usage.PromptTokens != 9 || res.Usage.CompletionTokens != 3 {
		t.Errorf("usage = %+v", res.Usage)
	}
}

func TestGemini_ModelsAndErrors(t *testing.T) {
	srv := sseServer(t, 200, `{"models":[{"name":"models/gemini-2.5-flash","displayName":"Gemini 2.5 Flash","inputTokenLimit":1048576,"supportedGenerationMethods":["generateContent"]},{"name":"models/embedding-001","supportedGenerationMethods":["embedContent"]}]}`, nil)
	p := NewGemini(AdapterConfig{BaseURL: srv.URL, APIKey: testKey, HTTP: srv.Client()})
	models, err := p.ListModels(context.Background())
	if err != nil || len(models) != 1 || models[0].ID != "gemini-2.5-flash" || models[0].ContextWindow != 1048576 {
		t.Fatalf("models = %+v err = %v", models, err)
	}
	bad := sseServer(t, 400, `{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT"}}`, nil)
	p = NewGemini(AdapterConfig{BaseURL: bad.URL, APIKey: testKey, HTTP: bad.Client()})
	if pe, ok := AsProviderError(p.Validate(context.Background())); !ok || pe.Class != ClassInvalidKey {
		t.Errorf("gemini bad key = %v", pe)
	}
}
