package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"
)

func TestRun_TokensCreate_Preset(t *testing.T) {
	tests := []struct {
		name      string
		extra     []string
		want      []string
		wantExit  int
		wantCalls int
	}{
		{"observer", []string{"--preset", "observer"}, []string{"read"}, exitOK, 1},
		{"deployer", []string{"--preset", "deployer"}, []string{"read", "deploy"}, exitOK, 1},
		{"operator has no root", []string{"--preset", "operator"}, []string{"read", "read:sensitive", "write", "write:sensitive", "deploy"}, exitOK, 1},
		{"unknown preset", []string{"--preset", "god"}, nil, exitValidation, 0},
		{"preset with abilities", []string{"--preset", "observer", "--abilities", "read"}, nil, exitValidation, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotReq createTokenRequest
			calls := 0
			srv := fakeSessionAuthServer(t, "POST", "/api/v1/auth/tokens", func(w http.ResponseWriter, r *http.Request) {
				calls++
				if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
					t.Fatalf("decode: %v", err)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_ = json.NewEncoder(w).Encode(createTokenResponse{tokenResource: tokenResource{ID: "tok_p", Name: gotReq.Name, CreatedAt: time.Now()}, Token: "v"})
			})
			args := append([]string{"tokens", "create", "--name", "bot", "--username", "admin", "--password", "x", "--api-url", srv.URL}, tc.extra...)
			var stdout, stderr bytes.Buffer
			if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != tc.wantExit {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, tc.wantExit, stderr.String())
			}
			if calls != tc.wantCalls {
				t.Fatalf("create calls = %d, want %d", calls, tc.wantCalls)
			}
			if tc.want != nil && !reflect.DeepEqual(gotReq.Abilities, tc.want) {
				t.Errorf("abilities = %v, want %v", gotReq.Abilities, tc.want)
			}
		})
	}
}
