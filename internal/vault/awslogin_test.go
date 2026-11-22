package vault

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAWSLoginSignsIdentityRequestAndCachesToken(t *testing.T) {
	var logins atomic.Int32
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/aws2/login":
			logins.Add(1)
			_ = json.NewDecoder(r.Body).Decode(&got)
			_ = json.NewEncoder(w).Encode(map[string]any{"auth": map[string]any{"client_token": "tok", "lease_duration": 1000}})
		case "/v1/sys/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"initialized": true})
		case "/v1/aws/roles/lr":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"credential_type": "iam_user"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	tun := func(context.Context) Tunables {
		return Tunables{Timeout: 5 * time.Second, AllowPrivate: true, RenewPercent: 80}
	}
	l := NewAWSLogin(tun)
	spec := AWSLoginSpec{Address: srv.URL, Mount: "aws2", Role: "levelrail", ServerID: "vault.example"}
	keys := func(context.Context) (AWSCredentials, error) {
		return AWSCredentials{AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "secretexample", SessionToken: "sess"}, nil
	}
	for i := 0; i < 2; i++ {
		tok, err := l.Token(context.Background(), spec, keys)
		if err != nil || tok != "tok" {
			t.Fatalf("token = %q, err %v", tok, err)
		}
	}
	if logins.Load() != 1 {
		t.Fatalf("logins = %d, want 1", logins.Load())
	}
	rawHdrs, _ := base64.StdEncoding.DecodeString(got["iam_request_headers"].(string))
	if got["role"] != "levelrail" || !strings.Contains(string(rawHdrs), "X-Vault-Aws-Iam-Server-Id") || !strings.Contains(string(rawHdrs), "AWS4-HMAC-SHA256") {
		t.Fatalf("login payload missing signed server id header: %v %s", got["role"], rawHdrs)
	}
	if strings.Contains(string(rawHdrs), "secretexample") {
		t.Fatal("secret key leaked into the login payload")
	}

	b := NewAWSBroker(tun)
	if err := b.Probe(context.Background(), AWSSpec{Address: srv.URL, Role: "lr"}, "tok"); err != nil {
		t.Fatalf("probe: %v", err)
	}
	if err := b.Probe(context.Background(), AWSSpec{Address: srv.URL, Role: "missing"}, "tok"); err == nil {
		t.Fatal("probe of an unknown role must fail")
	}
}
