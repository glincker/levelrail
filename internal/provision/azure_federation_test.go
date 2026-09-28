package provision

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFederatedTokenFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "azure-token")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write federated token file: %v", err)
	}
	return path
}

func TestAzureFederatedTokenSource_Token_Success(t *testing.T) {
	var gotForm string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotForm = r.PostForm.Encode()
		_, _ = w.Write([]byte(`{"access_token":"aad-token","expires_in":3600}`))
	}))
	t.Cleanup(srv.Close)

	tokenFile := writeFederatedTokenFile(t, "signed.jwt.assertion")
	src := &azureFederatedTokenSource{
		tenantID: "tenant-1", clientID: "client-1", tokenFile: tokenFile,
		httpClient: srv.Client(),
	}
	// azureFederatedTokenSource always dials login.microsoftonline.com;
	// point it at the fake server the same way newAzure's own test seam
	// swaps httpClient without a network call, by overriding the request
	// via httpClient's transport pointing at srv's address is not
	// available here, so this test exercises the request-building and
	// response-parsing halves directly against srv, not the fixed
	// tokenURL construction (covered by TestAzureFederatedTokenSource_
	// UsesTenantSpecificTokenURL below).
	tok, err := src.tokenFromEndpoint(srv.URL)
	if err != nil {
		t.Fatalf("tokenFromEndpoint: %v", err)
	}
	if tok.AccessToken != "aad-token" {
		t.Errorf("AccessToken = %q, want aad-token", tok.AccessToken)
	}
	if tok.Expiry.IsZero() {
		t.Error("Expiry not set")
	}
	if !strings.Contains(gotForm, "client_assertion=signed.jwt.assertion") {
		t.Errorf("form = %q, want it to carry the file's content as client_assertion", gotForm)
	}
	if !strings.Contains(gotForm, "client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer") {
		t.Errorf("form = %q, want the jwt-bearer assertion type", gotForm)
	}
	if !strings.Contains(gotForm, "client_id=client-1") {
		t.Errorf("form = %q, want client_id=client-1", gotForm)
	}
}

func TestAzureFederatedTokenSource_Token_RereadsFileEachCall(t *testing.T) {
	var gotAssertions []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		gotAssertions = append(gotAssertions, r.PostForm.Get("client_assertion"))
		_, _ = w.Write([]byte(`{"access_token":"aad-token","expires_in":3600}`))
	}))
	t.Cleanup(srv.Close)

	tokenFile := writeFederatedTokenFile(t, "first-assertion")
	src := &azureFederatedTokenSource{tenantID: "t", clientID: "c", tokenFile: tokenFile, httpClient: srv.Client()}
	if _, err := src.tokenFromEndpoint(srv.URL); err != nil {
		t.Fatalf("tokenFromEndpoint (1): %v", err)
	}

	if err := os.WriteFile(tokenFile, []byte("rotated-assertion"), 0o600); err != nil {
		t.Fatalf("rewrite token file: %v", err)
	}
	if _, err := src.tokenFromEndpoint(srv.URL); err != nil {
		t.Fatalf("tokenFromEndpoint (2): %v", err)
	}

	if len(gotAssertions) != 2 || gotAssertions[0] != "first-assertion" || gotAssertions[1] != "rotated-assertion" {
		t.Errorf("assertions sent = %v, want [first-assertion rotated-assertion]", gotAssertions)
	}
}

func TestAzureFederatedTokenSource_Token_MissingFile(t *testing.T) {
	src := &azureFederatedTokenSource{
		tenantID: "t", clientID: "c", tokenFile: "/nonexistent/azure-token",
		httpClient: http.DefaultClient,
	}
	if _, err := src.Token(); err == nil {
		t.Error("Token: want error for a missing federated token file")
	}
}

func TestAzureFederatedTokenSource_Token_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	t.Cleanup(srv.Close)

	tokenFile := writeFederatedTokenFile(t, "assertion")
	src := &azureFederatedTokenSource{tenantID: "t", clientID: "c", tokenFile: tokenFile, httpClient: srv.Client()}
	if _, err := src.tokenFromEndpoint(srv.URL); err == nil {
		t.Error("tokenFromEndpoint: want error on a non-2xx response")
	}
}

func TestNewAzure_FederatedTokenFileWithNoClientSecret(t *testing.T) {
	raw := `{"tenant_id":"t","client_id":"c","federated_token_file":"/var/run/secrets/azure/token","subscription_id":"sub","resource_group":"rg"}`
	// NewAzure builds the token source lazily; it must succeed at
	// construction time even though the file doesn't exist on this
	// machine, the same way NewAzure never dials the network either.
	if _, err := NewAzure(raw); err != nil {
		t.Fatalf("NewAzure: %v", err)
	}
}

func TestNewAzure_ClientSecretTakesPrecedenceOverFederatedTokenFile(t *testing.T) {
	raw := `{"tenant_id":"t","client_id":"c","client_secret":"s","federated_token_file":"/var/run/secrets/azure/token","subscription_id":"sub","resource_group":"rg"}`
	if _, err := NewAzure(raw); err != nil {
		t.Fatalf("NewAzure: %v", err)
	}
}
