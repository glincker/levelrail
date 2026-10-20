package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_SecretsBindingCommands(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
		response   any
		wantOut    []string
	}{
		{
			name:       "binding-status",
			args:       []string{"secrets", "binding-status"},
			wantMethod: http.MethodGet,
			wantPath:   "/api/v1/system/secrets/binding",
			response:   secretBindingStatus{Total: 4, Bound: 1, Legacy: 3},
			wantOut:    []string{"legacy: 3", "total:  4"},
		},
		{
			name:       "rebind",
			args:       []string{"secrets", "rebind"},
			wantMethod: http.MethodPost,
			wantPath:   "/api/v1/system/secrets/rebind",
			response: secretRebindResult{Scanned: 4, Rebound: 2, AlreadyBound: 1, FailedCount: 1,
				Failed: []apiclient.SecretRebindFailure{{Owner: "web", Key: "K", Reason: "ciphertext is bound to a different slot"}}},
			wantOut: []string{"rebound:       2", "web/K: ciphertext is bound to a different slot"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotMethod, gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(tt.response)
			}))
			defer srv.Close()

			stdout, _ := runCLIExpectOK(t, append(tt.args, "--api-url", srv.URL))
			if gotMethod != tt.wantMethod || gotPath != tt.wantPath {
				t.Errorf("request = %s %s, want %s %s", gotMethod, gotPath, tt.wantMethod, tt.wantPath)
			}
			for _, want := range tt.wantOut {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout = %q, want it to contain %q", stdout, want)
				}
			}
		})
	}
}

func TestRun_SecretsRebind_APIError(t *testing.T) {
	srv := newJSONErrorServer(t, http.StatusInternalServerError, `{"error":"rebind stopped after 2 values, progress is kept and a rerun resumes: disk full"}`)
	stderr := runCLIExpectAPIError(t, []string{"secrets", "rebind", "--api-url", srv.URL})
	if !strings.Contains(stderr, "rerun resumes") {
		t.Errorf("stderr = %q, want the API's own error surfaced", stderr)
	}
}

func TestRun_SecretsRotateMasterKey_PrintsRebindSummary(t *testing.T) {
	rebind := secretRebindResult{Rebound: 5, Remaining: 0}
	var stdout strings.Builder
	printRotateMasterKeyResultHuman(&stdout, rotateMasterKeyResult{Rebind: &rebind})
	if !strings.Contains(stdout.String(), "rebound:           5") {
		t.Errorf("stdout = %q, want the rebind summary", stdout.String())
	}
}
