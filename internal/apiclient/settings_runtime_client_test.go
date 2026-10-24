package apiclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_RuntimeSettings(t *testing.T) {
	setting := RuntimeSettingResource{Key: "security.webhook_replay_protection", Kind: "bool", Value: true, Source: "default"}
	list := RuntimeSettingsListResource{
		Settings: []RuntimeSettingResource{setting},
		Restart:  RuntimeSettingsRestart{Supported: true, PendingRestart: true},
	}

	tests := []struct {
		name     string
		call     func(*Client) (any, error)
		wantVerb string
		wantPath string
		wantBody string
		check    func(t *testing.T, got any)
	}{
		{
			name:     "list",
			call:     func(c *Client) (any, error) { return c.GetRuntimeSettings(context.Background()) },
			wantVerb: http.MethodGet,
			wantPath: "/api/v1/settings/runtime",
			check: func(t *testing.T, got any) {
				l := got.(RuntimeSettingsListResource)
				if len(l.Settings) != 1 || !l.Restart.PendingRestart {
					t.Errorf("list = %+v", l)
				}
			},
		},
		{
			name:     "get escapes key",
			call:     func(c *Client) (any, error) { return c.GetRuntimeSetting(context.Background(), "security.a b") },
			wantVerb: http.MethodGet,
			wantPath: "/api/v1/settings/runtime/security.a b",
			check: func(t *testing.T, got any) {
				if got.(RuntimeSettingResource).Key != setting.Key {
					t.Errorf("get = %+v", got)
				}
			},
		},
		{
			name: "put sends typed value and confirm",
			call: func(c *Client) (any, error) {
				return c.PutRuntimeSetting(context.Background(), "security.audit_sensitive_reads", RuntimeSettingWrite{Value: false, ConfirmWeakening: true})
			},
			wantVerb: http.MethodPut,
			wantPath: "/api/v1/settings/runtime/security.audit_sensitive_reads",
			wantBody: `{"value":false,"confirm_weakening":true}`,
		},
		{
			name: "put omits confirm when unset",
			call: func(c *Client) (any, error) {
				return c.PutRuntimeSetting(context.Background(), "security.bind_allowed_roots", RuntimeSettingWrite{Value: []string{"/srv"}})
			},
			wantVerb: http.MethodPut,
			wantPath: "/api/v1/settings/runtime/security.bind_allowed_roots",
			wantBody: `{"value":["/srv"]}`,
		},
		{
			name: "delete",
			call: func(c *Client) (any, error) {
				return c.DeleteRuntimeSetting(context.Background(), "security.bind_allowed_roots")
			},
			wantVerb: http.MethodDelete,
			wantPath: "/api/v1/settings/runtime/security.bind_allowed_roots",
		},
		{
			name:     "apply",
			call:     func(c *Client) (any, error) { return c.ApplyRuntimeSettings(context.Background()) },
			wantVerb: http.MethodPost,
			wantPath: "/api/v1/settings/runtime/apply",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotVerb, gotPath, gotBody string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotVerb, gotPath = r.Method, r.URL.Path
				raw, _ := io.ReadAll(r.Body)
				gotBody = string(raw)
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/v1/settings/runtime":
					_ = json.NewEncoder(w).Encode(list)
				case "/api/v1/settings/runtime/apply":
					_, _ = w.Write([]byte(`{"message":"restart scheduled"}`))
				default:
					_ = json.NewEncoder(w).Encode(setting)
				}
			}))
			defer srv.Close()

			got, err := tt.call(NewClient(srv.URL, "test-token"))
			if err != nil {
				t.Fatalf("call error = %v", err)
			}
			if gotVerb != tt.wantVerb || gotPath != tt.wantPath {
				t.Errorf("request = %s %s, want %s %s", gotVerb, gotPath, tt.wantVerb, tt.wantPath)
			}
			if tt.wantBody != "" && gotBody != tt.wantBody {
				t.Errorf("body = %s, want %s", gotBody, tt.wantBody)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}
