package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_NodesSetLabels(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantBody string
	}{
		{"all flags", []string{"--region", "eu", "--provider", "hetzner", "--zone", "fsn1"}, `{"region":"eu","provider":"hetzner","zone":"fsn1"}`},
		{"zone only", []string{"--zone", "a"}, `{"zone":"a"}`},
		{"explicit empty clears", []string{"--zone="}, `{"zone":""}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotMethod, gotPath, gotBody string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var raw json.RawMessage
				_ = json.NewDecoder(r.Body).Decode(&raw)
				gotMethod, gotPath, gotBody = r.Method, r.URL.Path, string(raw)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(nodeResource{ID: "nd_1", Region: "eu", Provider: "hetzner", Zone: "fsn1"})
			}))
			defer srv.Close()

			args := append([]string{"nodes", "set-labels", "nd_1", "--api-url", srv.URL}, tc.args...)
			stdout, _ := runCLIExpectOK(t, args)
			if gotMethod != http.MethodPut || gotPath != "/api/v1/nodes/nd_1/region" {
				t.Errorf("request = %s %s", gotMethod, gotPath)
			}
			if gotBody != tc.wantBody {
				t.Errorf("body = %s, want %s", gotBody, tc.wantBody)
			}
			if !strings.Contains(stdout, `zone="fsn1"`) {
				t.Errorf("stdout = %q", stdout)
			}
		})
	}
}
