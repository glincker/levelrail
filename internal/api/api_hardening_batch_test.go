package api

import (
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
	first, second, third := rt.ingestLookupAllowed(req), rt.ingestLookupAllowed(req), rt.ingestLookupAllowed(req)
	if !first || !second || third {
		t.Fatal("expected two lookups then a refusal")
	}
}
