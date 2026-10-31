package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRedactWebhookURLMasksPath(t *testing.T) {
	got := redactWebhookURL("https://hooks.example.com/services/T000/B000/abcdefgh1234")
	if strings.Contains(got, "services") || !strings.HasPrefix(got, "https://hooks.example.com/") || !strings.HasSuffix(got, "1234") {
		t.Fatalf("unexpected redaction %q", got)
	}
	if redactWebhookURL("not a url") != "***" {
		t.Fatal("unparseable value should be fully masked")
	}
}

func TestIngestLookupLimiterCapsPerIP(t *testing.T) {
	t.Setenv(envBuildHookFailures, "2")
	rt := &Router{}
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req.RemoteAddr = "192.0.2.7:1234"
	first := rt.ingestLookupAllowed(req)
	rt.ingestLookupFailed(req)
	second := rt.ingestLookupAllowed(req)
	rt.ingestLookupFailed(req)
	third := rt.ingestLookupAllowed(req)
	if !first || !second || third {
		t.Fatal("expected two failures then a refusal")
	}
}

type ingestKeySecrets struct {
	SecretSetter
	val string
}

func (s ingestKeySecrets) Resolve(_ context.Context, _, _ string) (string, error) {
	return s.val, nil
}

func TestValidIngestKeysOnCacheMissNeverConsumeFailureBudget(t *testing.T) {
	t.Setenv(envBuildHookFailures, "2")
	tok, err := newTraceToken("shop")
	if err != nil {
		t.Fatal(err)
	}
	rt := &Router{secrets: ingestKeySecrets{val: tok}}
	for i := 0; i < 50; i++ {
		rt.traceKeys.drop("shop")
		req := httptest.NewRequest(http.MethodPost, "/x", nil)
		req.RemoteAddr = "192.0.2.9:1234"
		if app := rt.authTraceIngest(req, "Bearer "+tok); app != "shop" {
			t.Fatalf("valid key refused on miss %d", i)
		}
	}
	bad := httptest.NewRequest(http.MethodPost, "/x", nil)
	bad.RemoteAddr = "192.0.2.9:1234"
	for i := 0; i < 3; i++ {
		rt.traceKeys.drop("shop")
		if rt.authTraceIngest(bad, "Bearer "+tok+"x") != "" {
			t.Fatal("wrong key accepted")
		}
	}
	rt.traceKeys.drop("shop")
	if rt.authTraceIngest(bad, "Bearer "+tok) != "" {
		t.Fatal("expected the IP to be throttled after repeated failures")
	}
}
