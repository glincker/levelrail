package provision

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// azureFederatedTokenSource implements oauth2.TokenSource for Azure
// workload identity federation (OIDC): it exchanges a freshly-read
// federated JWT for an AAD access token via the OAuth2 jwt-bearer
// client-assertion grant (RFC 7523), the same mechanism Kubernetes and
// GitHub Actions use to authenticate to Azure AD without a long-lived
// client secret. The app registration needs a federated credential
// configured on the Azure side trusting whatever issued the JWT at
// TokenFile; this provisioner never issues or signs one itself, only
// presents it.
//
// TokenFile is re-read on every call rather than cached, since the
// platform mounting it (a projected Kubernetes service account token,
// for example) rotates it on its own schedule, independent of this
// process's lifetime. NewAzure wraps this in oauth2.ReuseTokenSource, so
// the file is only actually re-read when the previous AAD access token
// has expired, not on every provider API call.
type azureFederatedTokenSource struct {
	tenantID, clientID, tokenFile string
	httpClient                    *http.Client
}

func newAzureFederatedTokenSource(cred azureCredential) oauth2.TokenSource {
	return &azureFederatedTokenSource{
		tenantID:   cred.TenantID,
		clientID:   cred.ClientID,
		tokenFile:  cred.FederatedTokenFile,
		httpClient: &http.Client{Timeout: requestTimeout},
	}
}

func (s *azureFederatedTokenSource) Token() (*oauth2.Token, error) {
	return s.tokenFromEndpoint("https://login.microsoftonline.com/" + s.tenantID + "/oauth2/v2.0/token")
}

// tokenFromEndpoint is Token's own implementation, taking tokenURL as a
// parameter so azure_federation_test.go can point it at a fake server
// instead of Microsoft's real login endpoint.
func (s *azureFederatedTokenSource) tokenFromEndpoint(tokenURL string) (*oauth2.Token, error) {
	assertion, err := os.ReadFile(s.tokenFile)
	if err != nil {
		return nil, fmt.Errorf("provision: azure read federated token file %q: %w", s.tokenFile, err)
	}

	form := url.Values{
		"grant_type":            {"client_credentials"},
		"client_id":             {s.clientID},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      {strings.TrimSpace(string(assertion))},
		"scope":                 {"https://management.azure.com/.default"},
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("provision: build azure federated token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("provision: azure federated token exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("provision: read azure federated token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &ProviderError{Status: resp.StatusCode, Body: string(body)}
	}

	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("provision: decode azure federated token response: %w", err)
	}
	return &oauth2.Token{
		AccessToken: out.AccessToken,
		TokenType:   "Bearer",
		Expiry:      time.Now().Add(time.Duration(out.ExpiresIn) * time.Second),
	}, nil
}
