package registrytoken

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	garScope           = "https://www.googleapis.com/auth/cloud-platform"
	garUsername        = "oauth2accesstoken"
	garDefaultLocation = "us"
)

// ArtifactRegistry mints an OAuth access token from the same service
// account JSON Compute Engine nodes use; the token is the registry password.
type ArtifactRegistry struct {
	Client *http.Client
}

// Mint implements Minter.
func (a *ArtifactRegistry) Mint(ctx context.Context, cloudSecret string, cfg Config) (Minted, error) {
	jwtCfg, err := google.JWTConfigFromJSON([]byte(cloudSecret), garScope)
	if err != nil {
		return Minted{}, fmt.Errorf("registrytoken: artifact registry: service account key: %w", err)
	}
	tctx := ctx
	if a.Client != nil {
		tctx = context.WithValue(ctx, oauth2.HTTPClient, a.Client)
	}
	tok, err := jwtCfg.TokenSource(tctx).Token()
	if err != nil {
		return Minted{}, fmt.Errorf("registrytoken: artifact registry: mint access token: %w", err)
	}
	host := cfg.RegistryHost
	if host == "" {
		loc := cfg.Region
		if loc == "" {
			loc = garDefaultLocation
		}
		host = loc + "-docker.pkg.dev"
	}
	return Minted{RegistryHost: host, Username: garUsername, Password: tok.AccessToken, ExpiresAt: tok.Expiry.UTC()}, nil
}
