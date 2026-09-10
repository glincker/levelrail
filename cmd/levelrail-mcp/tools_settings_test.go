package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGetOAuthProviders(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/settings/oauth" {
			t.Errorf("path = %q, want /api/v1/settings/oauth", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]apiclient.OAuthProviderSettingsResource{
			{Provider: "google", Enabled: true, ClientID: "gid", HasClientSecret: true},
			{Provider: "github", Enabled: false},
			{Provider: "oidc", Enabled: false},
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_oauth_providers", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(get_oauth_providers) error = %v", err)
	}
	var settings []apiclient.OAuthProviderSettingsResource
	decodeStructured(t, result, &settings)
	if len(settings) != 3 || !settings[0].Enabled || !settings[0].HasClientSecret {
		t.Errorf("settings = %+v, want google enabled with HasClientSecret=true", settings)
	}
	if strings.Contains(toolResultText(result), "shh-secret-value") {
		t.Errorf("result text = %q, must never contain a raw client secret value", toolResultText(result))
	}
}

func TestGetOAuthProviders_NotFound(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_oauth_providers", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(get_oauth_providers) transport error = %v", err)
	}
	if !result.IsError {
		t.Fatalf("IsError = false, want true for a 500 response")
	}
	if !strings.Contains(toolResultText(result), "internal error") {
		t.Errorf("error text = %q, want the server's own error", toolResultText(result))
	}
}

func TestGetEmailSettings(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/settings/email" {
			t.Errorf("path = %q, want /api/v1/settings/email", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(apiclient.EmailSettingsResource{
			Backend: "smtp", SMTPHost: "smtp.example.com", SMTPPasswordSet: true,
		})
	})

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "get_email_settings", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("CallTool(get_email_settings) error = %v", err)
	}
	var settings apiclient.EmailSettingsResource
	decodeStructured(t, result, &settings)
	if settings.Backend != "smtp" || !settings.SMTPPasswordSet {
		t.Errorf("settings = %+v, want backend=smtp SMTPPasswordSet=true", settings)
	}
	if strings.Contains(toolResultText(result), "hunter2") {
		t.Errorf("result text = %q, must never contain a raw credential value", toolResultText(result))
	}
}

// TestSettingsTools_Surface403 mirrors TestPlatformVisibilityTools_Surface403
// for the two tools added in this file.
func TestSettingsTools_Surface403(t *testing.T) {
	tests := []struct {
		tool string
		args map[string]any
	}{
		{"get_oauth_providers", map[string]any{}},
		{"get_email_settings", map[string]any{}},
	}

	for _, tt := range tests {
		t.Run(tt.tool, func(t *testing.T) {
			session := newTestSession(t, func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"token lacks the required ability"}`))
			})

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.tool, Arguments: tt.args})
			if err != nil {
				t.Fatalf("CallTool(%s) transport error = %v", tt.tool, err)
			}
			if !result.IsError {
				t.Fatalf("IsError = false, want true for a 403 response")
			}
			text := toolResultText(result)
			if !strings.Contains(text, "token lacks the required ability") {
				t.Errorf("error text = %q, want it to contain the server's own 403 message", text)
			}
		})
	}
}
