package api

import (
	"context"
	"fmt"
	"sync"

	"github.com/coreos/go-oidc/v3/oidc"
)

const githubActionsOIDCIssuer = "https://token.actions.githubusercontent.com"

type githubOIDCVerifier struct {
	mu       sync.Mutex
	provider *oidc.Provider
}

var defaultGitHubOIDCVerifier = &githubOIDCVerifier{}

func (v *githubOIDCVerifier) discover(ctx context.Context) (*oidc.Provider, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.provider != nil {
		return v.provider, nil
	}
	p, err := oidc.NewProvider(ctx, githubActionsOIDCIssuer)
	if err != nil {
		return nil, fmt.Errorf("api: discover github oidc issuer: %w", err)
	}
	v.provider = p
	return p, nil
}

// Verify checks signature, issuer, audience and expiry against GitHub's published keys.
func (v *githubOIDCVerifier) Verify(ctx context.Context, rawToken, audience string) (CIClaims, error) {
	p, err := v.discover(ctx)
	if err != nil {
		return CIClaims{}, err
	}
	tok, err := p.Verifier(&oidc.Config{ClientID: audience}).Verify(ctx, rawToken)
	if err != nil {
		return CIClaims{}, fmt.Errorf("api: verify github oidc token: %w", err)
	}
	var c struct {
		Repository  string `json:"repository"`
		Ref         string `json:"ref"`
		Environment string `json:"environment"`
	}
	if err := tok.Claims(&c); err != nil {
		return CIClaims{}, fmt.Errorf("api: read github oidc claims: %w", err)
	}
	if c.Repository == "" || c.Ref == "" {
		return CIClaims{}, fmt.Errorf("api: github oidc token has no repository or ref claim")
	}
	return CIClaims{Repository: c.Repository, Ref: c.Ref, Environment: c.Environment, Subject: tok.Subject}, nil
}
