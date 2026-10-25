package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunSetupAdmin(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   int
	}{
		{"created", http.StatusOK, exitOK},
		{"admin already exists", http.StatusConflict, -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got map[string]string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&got)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"error":"an account already exists"}`))
			}))
			defer srv.Close()
			env := map[string]string{envSetupToken: "tok", envAdminPassword: "long-enough-pw"}
			var out, errb bytes.Buffer
			code := runSetupAdmin("cli", []string{"--api-url", srv.URL, "--username", "root"}, &out, &errb, func(k string) (string, bool) { v, ok := env[k]; return v, ok })
			if got["setup_token"] != "tok" || got["username"] != "root" {
				t.Errorf("body = %v", got)
			}
			if (tc.want == exitOK) != (code == exitOK) {
				t.Errorf("exit = %d, want ok=%v; stderr=%s", code, tc.want == exitOK, errb.String())
			}
			if bytes.Contains(errb.Bytes(), []byte("long-enough-pw")) || bytes.Contains(out.Bytes(), []byte("long-enough-pw")) {
				t.Error("password leaked to output")
			}
		})
	}
}
