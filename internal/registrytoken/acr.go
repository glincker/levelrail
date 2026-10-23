package registrytoken

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/provision"
)

const (
	acrARMScope = "https://management.azure.com/.default"
	// acrUsername is the fixed username ACR documents for refresh-token logins.
	acrUsername = "00000000-0000-0000-0000-000000000000"
)

// ACR exchanges an Azure AD token, obtained with the stored Azure service
// principal (secret or federated), for an ACR refresh token.
type ACR struct {
	Client      *http.Client
	FallbackTTL time.Duration
	// LoginBase and ExchangeBase override the AAD and registry URLs, for tests.
	LoginBase    string
	ExchangeBase string
	Now          func() time.Time
}

// Mint implements Minter.
func (a *ACR) Mint(ctx context.Context, cloudSecret string, cfg Config) (Minted, error) {
	if cfg.RegistryHost == "" {
		return Minted{}, fmt.Errorf("registrytoken: acr: registry host is required, for example myregistry.azurecr.io")
	}
	ts, err := provision.NewAzureTokenSource(cloudSecret, acrARMScope, a.LoginBase)
	if err != nil {
		return Minted{}, fmt.Errorf("registrytoken: acr: %w", err)
	}
	aad, err := ts.Token()
	if err != nil {
		return Minted{}, fmt.Errorf("registrytoken: acr: azure ad token: %w", err)
	}

	base := a.ExchangeBase
	if base == "" {
		base = "https://" + cfg.RegistryHost
	}
	form := url.Values{
		"grant_type":   {"access_token"},
		"service":      {cfg.RegistryHost},
		"access_token": {aad.AccessToken},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/oauth2/exchange", strings.NewReader(form.Encode()))
	if err != nil {
		return Minted{}, fmt.Errorf("registrytoken: acr: build exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.Client.Do(req)
	if err != nil {
		return Minted{}, fmt.Errorf("registrytoken: acr: exchange token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Minted{}, statusError("acr token exchange", resp)
	}
	var out struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return Minted{}, fmt.Errorf("registrytoken: acr: decode exchange response: %w", err)
	}
	if out.RefreshToken == "" {
		return Minted{}, fmt.Errorf("registrytoken: acr: exchange returned no refresh token")
	}
	now := time.Now()
	if a.Now != nil {
		now = a.Now()
	}
	exp, ok := jwtExpiry(out.RefreshToken)
	if !ok {
		exp = now.Add(a.FallbackTTL)
	}
	return Minted{RegistryHost: cfg.RegistryHost, Username: acrUsername, Password: out.RefreshToken, ExpiresAt: exp}, nil
}

// jwtExpiry reads exp from an unverified JWT payload; the registry verifies it.
func jwtExpiry(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, false
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil || claims.Exp == 0 {
		return time.Time{}, false
	}
	return time.Unix(claims.Exp, 0).UTC(), true
}
