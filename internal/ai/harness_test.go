package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestClassifyHTTP_Matrix(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		status int
		body   string
		want   ErrorClass
	}{
		{"401", 401, `{"error":{"message":"nope"}}`, ClassInvalidKey},
		{"403", 403, ``, ClassInvalidKey},
		{"429 rate", 429, `{"error":{"message":"Rate limit reached"}}`, ClassRateLimited},
		{"429 quota", 429, `{"error":{"type":"insufficient_quota","message":"You exceeded your current quota"}}`, ClassQuotaExhausted},
		{"402", 402, ``, ClassQuotaExhausted},
		{"anthropic credit", 400, `{"error":{"message":"Your credit balance is too low"}}`, ClassQuotaExhausted},
		{"529", 529, ``, ClassOverloaded},
		{"500", 500, `oops`, ClassOverloaded},
		{"503", 503, ``, ClassOverloaded},
		{"context", 400, `{"error":{"message":"prompt is too long: 250000 tokens > 200000 maximum"}}`, ClassContextTooLong},
		{"openai context", 400, `{"error":{"code":"context_length_exceeded","message":"maximum context length is 8192"}}`, ClassContextTooLong},
		{"model 404", 404, `{"error":{"message":"The model 'x' does not exist"}}`, ClassModelNotFound},
		{"model gemini", 404, `{"error":{"message":"models/x is not found for API version"}}`, ClassModelNotFound},
		{"plain 400", 400, `{"error":{"message":"bad field"}}`, ClassBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pe := ClassifyHTTP("p", tc.status, []byte(tc.body), http.Header{}, "", now)
			if pe.Class != tc.want {
				t.Errorf("class = %s, want %s", pe.Class, tc.want)
			}
			if msg, fix := pe.Explain(); msg == "" || fix == "" {
				t.Errorf("Explain() = %q, %q, want both set", msg, fix)
			}
		})
	}
}

func TestClassifyTransportAndFallbackRules(t *testing.T) {
	if pe := ClassifyTransport("p", context.DeadlineExceeded, ""); pe.Class != ClassNetwork || pe.Detail != "timeout" {
		t.Errorf("deadline = %+v", pe)
	}
	if pe := ClassifyTransport("p", fmt.Errorf("dial: %w", errBlockedAddress), ""); pe.Class != ClassBlockedAddress {
		t.Errorf("blocked = %+v", pe)
	}
	for class, want := range map[ErrorClass]bool{
		ClassRateLimited: true, ClassOverloaded: true, ClassNetwork: true, ClassQuotaExhausted: true,
		ClassInvalidKey: false, ClassContentRefused: false, ClassContextTooLong: false, ClassModelNotFound: false,
	} {
		if got := (&ProviderError{Class: class}).FallbackEligible(); got != want {
			t.Errorf("FallbackEligible(%s) = %v, want %v", class, got, want)
		}
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Duration{
		"":                              0,
		"7":                             7 * time.Second,
		"1.5":                           1500 * time.Millisecond,
		"Fri, 16 Oct 2026 12:00:30 GMT": 30 * time.Second,
		"Fri, 16 Oct 2026 11:00:00 GMT": 0,
		"garbage":                       0,
	}
	for in, want := range cases {
		if got := parseRetryAfter(http.Header{"Retry-After": {in}}, now); got != want {
			t.Errorf("Retry-After %q = %v, want %v", in, got, want)
		}
	}
}

func TestScrubRemovesKeys(t *testing.T) {
	got := Scrub("bad key sk-abcdefghijkl1234 and "+testKey, testKey)
	if strings.Contains(got, "abcdefghijkl") || strings.Contains(got, "SECRETKEY") {
		t.Errorf("Scrub left a key: %q", got)
	}
}

type scriptedProvider struct {
	errs  []error
	calls int
	text  string
}

func (s *scriptedProvider) Complete(_ context.Context, _ ChatRequest, onText TextDeltaFunc) (TurnResult, error) {
	i := s.calls
	s.calls++
	if i < len(s.errs) && s.errs[i] != nil {
		if s.text != "" && onText != nil {
			onText(s.text)
		}
		return TurnResult{}, s.errs[i]
	}
	return TurnResult{Text: "done"}, nil
}

func TestRetryProvider_RetryAfterJitterAndLimits(t *testing.T) {
	var slept []time.Duration
	policy := RetryPolicy{
		MaxAttempts: 4, BaseDelay: 100 * time.Millisecond, MaxDelay: 2 * time.Second,
		Jitter: func() float64 { return 0.5 },
		Sleep:  func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil },
	}
	rl := func(after time.Duration) error {
		return &ProviderError{Provider: "p", Class: ClassRateLimited, RetryAfter: after}
	}

	inner := &scriptedProvider{errs: []error{rl(1500 * time.Millisecond), &ProviderError{Class: ClassOverloaded}, nil}}
	res, err := (&RetryProvider{Inner: inner, Policy: policy}).Complete(context.Background(), ChatRequest{}, nil)
	if err != nil || res.Text != "done" || inner.calls != 3 {
		t.Fatalf("res=%+v err=%v calls=%d", res, err, inner.calls)
	}
	if slept[0] != 1500*time.Millisecond {
		t.Errorf("first sleep = %v, want Retry-After 1.5s", slept[0])
	}
	// attempt 1 backoff: base<<1 = 200ms, jitter 0.5 -> 100ms + 50ms
	if slept[1] != 150*time.Millisecond {
		t.Errorf("second sleep = %v, want 150ms", slept[1])
	}

	// Jitter bounds across the full range stay within [d/2, d).
	for _, j := range []float64{0, 0.999} {
		p := RetryPolicy{BaseDelay: time.Second, MaxDelay: time.Minute, Jitter: func() float64 { return j }}
		d, _ := p.delay(2, 0)
		if d < 2*time.Second || d >= 4*time.Second {
			t.Errorf("jitter %v gave %v outside [2s,4s)", j, d)
		}
	}

	// Retry-After beyond MaxDelay is not waited out.
	slept = nil
	inner = &scriptedProvider{errs: []error{rl(time.Hour)}}
	if _, err := (&RetryProvider{Inner: inner, Policy: policy}).Complete(context.Background(), ChatRequest{}, nil); err == nil || inner.calls != 1 || len(slept) != 0 {
		t.Errorf("long Retry-After: err=%v calls=%d slept=%v", err, inner.calls, slept)
	}

	// Never retried: 401, or after text has streamed.
	inner = &scriptedProvider{errs: []error{&ProviderError{Class: ClassInvalidKey}}}
	_, _ = (&RetryProvider{Inner: inner, Policy: policy}).Complete(context.Background(), ChatRequest{}, nil)
	streamed := &scriptedProvider{errs: []error{&ProviderError{Class: ClassOverloaded}}, text: "partial"}
	_, _ = (&RetryProvider{Inner: streamed, Policy: policy}).Complete(context.Background(), ChatRequest{}, nil)
	if inner.calls != 1 || streamed.calls != 1 {
		t.Errorf("calls = %d, %d, want 1, 1", inner.calls, streamed.calls)
	}

	// Attempts cap.
	inner = &scriptedProvider{errs: []error{&ProviderError{Class: ClassOverloaded}, &ProviderError{Class: ClassOverloaded}, &ProviderError{Class: ClassOverloaded}, &ProviderError{Class: ClassOverloaded}, &ProviderError{Class: ClassOverloaded}}}
	_, err = (&RetryProvider{Inner: inner, Policy: policy}).Complete(context.Background(), ChatRequest{}, nil)
	if err == nil || inner.calls != 4 {
		t.Errorf("cap: err=%v calls=%d, want 4 calls", err, inner.calls)
	}
}

func TestChainProvider_Fallback(t *testing.T) {
	rl := &ProviderError{Provider: "a", Class: ClassRateLimited}
	bad := &ProviderError{Provider: "a", Class: ClassInvalidKey}
	chain := func(a, b *scriptedProvider) *ChainProvider {
		return &ChainProvider{DefaultContext: 100000, Entries: []ChainEntry{
			{Provider: "a", Model: "ma", Backend: a}, {Provider: "b", Model: "mb", Backend: b},
		}}
	}
	var attempts []Attempt
	ctx := WithAttemptObserver(context.Background(), func(a Attempt) { attempts = append(attempts, a) })

	a, b := &scriptedProvider{errs: []error{rl}}, &scriptedProvider{}
	res, err := chain(a, b).Complete(ctx, ChatRequest{}, nil)
	if err != nil || res.Text != "done" || b.calls != 1 {
		t.Fatalf("429 then next: res=%+v err=%v", res, err)
	}
	if len(attempts) != 2 || attempts[0].ErrorClass != ClassRateLimited || attempts[0].Provider != "a" || attempts[1].ErrorClass != "" {
		t.Errorf("attempts = %+v", attempts)
	}

	a, b = &scriptedProvider{errs: []error{bad}}, &scriptedProvider{}
	if _, err := chain(a, b).Complete(context.Background(), ChatRequest{}, nil); err == nil || b.calls != 0 {
		t.Errorf("401 must not fall back: err=%v b.calls=%d", err, b.calls)
	}

	a, b = &scriptedProvider{errs: []error{rl}, text: "half"}, &scriptedProvider{}
	if _, err := chain(a, b).Complete(context.Background(), ChatRequest{}, nil); err == nil || b.calls != 0 {
		t.Errorf("fallback after streamed text would duplicate output: err=%v b.calls=%d", err, b.calls)
	}

	a, b = &scriptedProvider{errs: []error{rl}}, &scriptedProvider{errs: []error{&ProviderError{Provider: "b", Class: ClassOverloaded}}}
	_, err = chain(a, b).Complete(context.Background(), ChatRequest{}, nil)
	if pe, ok := AsProviderError(err); !ok || pe.Provider != "b" || a.calls != 1 || b.calls != 1 {
		t.Errorf("all fail should return the last error: %v", err)
	}
}

func TestCheckBaseURL_PolicyMatrix(t *testing.T) {
	resolver := func(m map[string]string) HostResolver {
		return func(_ context.Context, host string) ([]netip.Addr, error) {
			ip, ok := m[host]
			if !ok {
				return nil, errors.New("no such host")
			}
			return []netip.Addr{netip.MustParseAddr(ip)}, nil
		}
	}
	dns := resolver(map[string]string{
		"api.example.com": "93.184.216.34", "rebind.example.com": "127.0.0.1", "gpu.internal": "10.0.0.7",
	})
	cases := []struct {
		name    string
		url     string
		kind    Kind
		root    bool
		wantErr bool
		private bool
	}{
		{"public https", "https://api.example.com/v1", KindOpenAICompat, false, false, false},
		{"public https anthropic", "https://api.example.com", KindAnthropic, true, false, false},
		{"public http rejected", "http://api.example.com/v1", KindOpenAICompat, true, true, false},
		{"loopback as root", "http://127.0.0.1:11434/v1", KindOpenAICompat, true, false, true},
		{"localhost as root", "http://localhost:11434/v1", KindOpenAICompat, true, false, true},
		{"private hostname as root", "http://gpu.internal:8000/v1", KindOpenAICompat, true, false, true},
		{"loopback non-root", "http://127.0.0.1:11434/v1", KindOpenAICompat, false, true, false},
		{"private non-root", "http://10.0.0.7:8000/v1", KindOpenAICompat, false, true, false},
		{"private for anthropic kind even as root", "http://10.0.0.7:8000", KindAnthropic, true, true, false},
		{"private for gemini kind even as root", "http://10.0.0.7:8000", KindGemini, true, true, false},
		{"dns rebinding host non-root", "https://rebind.example.com/v1", KindOpenAICompat, false, true, false},
		{"dns rebinding host root", "http://rebind.example.com/v1", KindOpenAICompat, true, false, true},
		{"link local metadata", "http://169.254.169.254/latest", KindOpenAICompat, false, true, false},
		{"credentials in url", "https://user:pw@api.example.com", KindOpenAICompat, true, true, false},
		{"not http", "ftp://api.example.com", KindOpenAICompat, true, true, false},
		{"empty", "", KindOpenAICompat, true, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CheckBaseURL(context.Background(), tc.url, tc.kind, tc.root, dns)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err != nil && !errors.Is(err, ErrBaseURLRejected) {
				t.Errorf("err = %v, want ErrBaseURLRejected", err)
			}
			if err == nil && got.Private != tc.private {
				t.Errorf("private = %v, want %v", got.Private, tc.private)
			}
		})
	}
}

func TestHTTPClient_DialGuardAndRedirects(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer ok.Close()

	// httptest binds loopback: a guarded client must refuse it at dial time,
	// which is also what stops a public name that rebinds to a private IP.
	_, err := NewHTTPClient(false).Get(ok.URL)
	if err == nil || !errors.Is(err, errBlockedAddress) {
		t.Fatalf("guarded client reached loopback: err = %v", err)
	}
	if pe := ClassifyTransport("p", err, ""); pe.Class != ClassBlockedAddress {
		t.Errorf("class = %s", pe.Class)
	}
	resp, err := NewHTTPClient(true).Get(ok.URL)
	if err != nil {
		t.Fatalf("private-allowed client: %v", err)
	}
	_ = resp.Body.Close()

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("secret")) }))
	defer other.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer redirector.Close()
	if resp, err := NewHTTPClient(true).Get(redirector.URL); err == nil {
		_ = resp.Body.Close()
		t.Error("cross-host redirect was followed")
	}
}

func TestTruncateToolOutput(t *testing.T) {
	if got := TruncateToolOutput("short", 100); got != "short" {
		t.Errorf("short = %q", got)
	}
	got := TruncateToolOutput(strings.Repeat("a", 1000), 100)
	if !strings.HasPrefix(got, strings.Repeat("a", 100)) || !strings.HasSuffix(got, "[truncated 900 bytes]") {
		t.Errorf("got %q", got)
	}
	multi := TruncateToolOutput(strings.Repeat("é", 100), 51)
	if !strings.Contains(multi, "[truncated") || strings.ContainsRune(multi[:len(multi)-30], '�') {
		t.Errorf("rune boundary broken: %q", multi)
	}
	huge := TruncateToolOutput(strings.Repeat("x", 5<<20), 32*1024)
	if len(huge) > 32*1024+64 {
		t.Errorf("huge output not capped: %d", len(huge))
	}
}

func userText(s string) Message {
	return Message{Role: RoleUser, Content: []ContentBlock{{Type: ContentBlockText, Text: s}}}
}

func toolPair(id, result string) []Message {
	return []Message{
		{Role: RoleAssistant, Content: []ContentBlock{{Type: ContentBlockToolUse, ToolUseID: id, ToolName: "t", ToolInput: []byte(`{}`)}}},
		{Role: RoleUser, Content: []ContentBlock{{Type: ContentBlockToolResult, ToolResultForID: id, ToolResultContent: result}}},
	}
}

func TestGuardContext(t *testing.T) {
	big := strings.Repeat("x", 40000) // ~10k tokens
	msgs := []Message{userText("first")}
	msgs = append(msgs, toolPair("a", big)...)
	msgs = append(msgs, Message{Role: RoleAssistant, Content: []ContentBlock{{Type: ContentBlockText, Text: "answer"}}})
	msgs = append(msgs, userText("second"))
	msgs = append(msgs, toolPair("b", big)...)
	msgs = append(msgs, userText("latest question"))

	t.Run("fits untouched", func(t *testing.T) {
		out, rep, err := GuardContext("sys", nil, msgs, 1_000_000)
		if err != nil || rep != (GuardReport{}) || len(out) != len(msgs) {
			t.Errorf("out=%d rep=%+v err=%v", len(out), rep, err)
		}
	})
	t.Run("oldest tool results go first", func(t *testing.T) {
		out, rep, err := GuardContext("sys", nil, msgs, 4096+12000)
		if err != nil || rep.DroppedToolResults == 0 || rep.DroppedMessages != 0 {
			t.Fatalf("rep=%+v err=%v", rep, err)
		}
		if out[2].Content[0].ToolResultContent != droppedToolOutput {
			t.Error("oldest tool result should be a placeholder")
		}
		if msgs[2].Content[0].ToolResultContent != big {
			t.Error("input messages must not be mutated")
		}
		if out[len(out)-1].Content[0].Text != "latest question" {
			t.Error("latest user turn lost")
		}
	})
	t.Run("drops oldest exchanges, keeps latest turn and pairing", func(t *testing.T) {
		out, rep, err := GuardContext("sys", nil, msgs, 4096+65)
		if err != nil || rep.DroppedMessages == 0 {
			t.Fatalf("rep=%+v err=%v", rep, err)
		}
		if out[len(out)-1].Content[0].Text != "latest question" {
			t.Error("latest user turn lost")
		}
		if out[0].Role != RoleUser || !strings.Contains(out[0].Content[0].Text, "trimmed") {
			t.Errorf("first kept message = %+v, want a user message with a trim note", out[0])
		}
		for i, m := range out {
			for _, b := range m.Content {
				if b.Type == ContentBlockToolResult && (i == 0 || out[i-1].Content[0].Type != ContentBlockToolUse) {
					t.Errorf("orphaned tool_result at %d", i)
				}
			}
		}
	})
	t.Run("latest turn alone too large", func(t *testing.T) {
		_, _, err := GuardContext("sys", nil, []Message{userText(big)}, 4096+100)
		if pe, ok := AsProviderError(err); !ok || pe.Class != ClassContextTooLong {
			t.Errorf("err = %v, want context_too_long", err)
		}
	})
}

func TestValidateToolArgs(t *testing.T) {
	schema := []byte(`{"type":"object","required":["name"],"properties":{"name":{"type":"string"},"replicas":{"type":"integer"},"mode":{"type":"string","enum":["a","b"]}}}`)
	cases := []struct {
		args string
		ok   bool
	}{
		{`{"name":"web"}`, true},
		{``, false},
		{`{}`, false},
		{`[]`, false},
		{`{"name":1}`, false},
		{`{"name":"w","replicas":2}`, true},
		{`{"name":"w","replicas":2.5}`, false},
		{`{"name":"w","mode":"c"}`, false},
		{`{"name":"w","mode":"a"}`, true},
		{`{"name":"w"`, false},
	}
	for _, tc := range cases {
		if err := ValidateToolArgs(schema, []byte(tc.args)); (err == nil) != tc.ok {
			t.Errorf("args %q: err = %v, want ok=%v", tc.args, err, tc.ok)
		}
	}
	if err := ValidateToolArgs(nil, []byte(``)); err != nil {
		t.Errorf("no schema and no args: %v", err)
	}
}
