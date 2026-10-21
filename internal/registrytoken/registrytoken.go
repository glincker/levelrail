// Package registrytoken mints short-lived container registry passwords from
// an existing cloud connection (ECR, Artifact Registry, ACR) and keeps the
// stored registry credential fresh.
package registrytoken

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Preset names stored in registry_credentials.preset.
const (
	PresetECR              = "ecr"
	PresetArtifactRegistry = "artifact_registry"
	PresetACR              = "acr"
)

// ProviderFor returns the node-provider secret namespace whose stored
// credential a preset mints from.
func ProviderFor(preset string) (string, error) {
	switch preset {
	case PresetECR:
		return "aws", nil
	case PresetArtifactRegistry:
		return "gcp", nil
	case PresetACR:
		return "azure", nil
	default:
		return "", fmt.Errorf("registrytoken: unknown preset %q", preset)
	}
}

// ErrNoCloudConnection means the cloud credential a preset needs is not stored.
var ErrNoCloudConnection = errors.New("registrytoken: no cloud connection for this preset")

// Config is the non-secret, per-credential preset setting.
type Config struct {
	Region       string `json:"region,omitempty"`
	RegistryHost string `json:"registry_host,omitempty"`
}

// Minted is one freshly issued registry login.
type Minted struct {
	RegistryHost string
	Username     string
	Password     string
	ExpiresAt    time.Time
}

// Minter exchanges the stored cloud credential (cloudSecret) for a registry login.
type Minter interface {
	Mint(ctx context.Context, cloudSecret string, cfg Config) (Minted, error)
}

// DefaultMinters returns the real minters, keyed by preset. fallbackTTL
// bounds a token whose expiry cannot be read from the provider response.
func DefaultMinters(fallbackTTL time.Duration) map[string]Minter {
	client := &http.Client{Timeout: 20 * time.Second}
	return map[string]Minter{
		PresetECR:              &ECR{Client: client},
		PresetArtifactRegistry: &ArtifactRegistry{Client: client},
		PresetACR:              &ACR{Client: client, FallbackTTL: fallbackTTL},
	}
}

const maxErrBody = 200

func statusError(what string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	msg := strings.Join(strings.Fields(string(body)), " ")
	if len(msg) > maxErrBody {
		msg = msg[:maxErrBody]
	}
	return fmt.Errorf("registrytoken: %s: status %d: %s", what, resp.StatusCode, msg)
}

// SafeError flattens an error to one bounded line for storage and alerts.
func SafeError(err error) string {
	msg := strings.Join(strings.Fields(err.Error()), " ")
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}
