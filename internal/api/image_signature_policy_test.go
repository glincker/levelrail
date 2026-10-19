package api

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"testing"
)

func TestImageSignaturePolicy_Issues(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		opts     []Option
		wantOK   bool
		wantCode string
	}{
		{"nothing configured", nil, true, ""},
		{"unreadable key", []Option{WithImageVerifyIssue("read failed", ImageVerifyEnforce)}, false, sigIssueKeyUnreadable},
		{"enforce without key", []Option{WithImageVerifyIssue("", ImageVerifyEnforce)}, false, sigIssueEnforceNoKey},
		{"key but not enforced", []Option{WithImageVerify(&key.PublicKey, ImageVerifyOff)}, true, sigIssueKeyButNotForced},
		{"key and enforced", []Option{WithImageVerify(&key.PublicKey, ImageVerifyEnforce)}, true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rt, db := newTestRouter(t)
			for _, o := range tc.opts {
				o(rt)
			}
			cookie := loginTestSession(t, rt, db)
			rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/images/signature-policy/validate", ""))
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
			}
			var out imageSignaturePolicyResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatal(err)
			}
			if out.OK != tc.wantOK {
				t.Errorf("ok = %v, want %v (%v)", out.OK, tc.wantOK, out.Issues)
			}
			found := tc.wantCode == ""
			for _, is := range out.Issues {
				if is.Code == tc.wantCode {
					found = true
				}
			}
			if !found {
				t.Errorf("issues = %v, want code %q", out.Issues, tc.wantCode)
			}
			if tc.wantCode == "" && len(out.Issues) != 0 {
				t.Errorf("unexpected issues %v", out.Issues)
			}
			if out.KeyConfigured && out.KeyFingerprint == "" {
				t.Error("configured key reported no fingerprint")
			}
		})
	}
}

func TestImageSignaturePolicy_RequiresAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{{http.MethodPost, "/api/v1/images/signature-policy/validate"}})
}
