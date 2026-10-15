package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/secrets"
)

const (
	envKMSProvider        = "APP_KMS_PROVIDER"
	envVaultAddr          = "APP_VAULT_ADDR"
	envVaultToken         = "APP_VAULT_TOKEN"      //nolint:gosec // env var name, not a credential
	envVaultTokenFile     = "APP_VAULT_TOKEN_FILE" //nolint:gosec // env var name, not a credential
	envVaultTransitKey    = "APP_VAULT_TRANSIT_KEY"
	envVaultTransitMount  = "APP_VAULT_TRANSIT_MOUNT"
	envVaultNamespace     = "APP_VAULT_NAMESPACE"
	kmsProviderVaultTrans = secrets.TransitName
)

// keyWrapperFromEnv builds the configured external key service, or nil when none is set.
func keyWrapperFromEnv(getenv func(string) string) (secrets.KeyWrapper, error) {
	switch p := strings.TrimSpace(getenv(envKMSProvider)); p {
	case "":
		return nil, nil
	case kmsProviderVaultTrans:
		token := strings.TrimSpace(getenv(envVaultToken))
		if f := strings.TrimSpace(getenv(envVaultTokenFile)); token == "" && f != "" {
			raw, err := os.ReadFile(f) //nolint:gosec // operator-set token file path from startup config
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", envVaultTokenFile, err)
			}
			token = strings.TrimSpace(string(raw))
		}
		return secrets.NewVaultTransit(getenv(envVaultAddr), token, getenv(envVaultTransitKey), getenv(envVaultTransitMount), getenv(envVaultNamespace))
	default:
		return nil, fmt.Errorf("%s=%q is not supported (supported: %s)", envKMSProvider, p, kmsProviderVaultTrans)
	}
}

// openMasterKeyMaterial turns what is stored (plain or wrapped) into the serialized
// master key. A wrapped value without a configured key service is an error, never a fallback.
func openMasterKeyMaterial(ctx context.Context, stored string, getenv func(string) string) (string, error) {
	if !secrets.IsWrappedMasterKey(stored) {
		return stored, nil
	}
	w, err := keyWrapperFromEnv(getenv)
	if err != nil {
		return "", err
	}
	if w == nil {
		return "", fmt.Errorf("%w: set %s and the matching settings", secrets.ErrWrapperRequired, envKMSProvider)
	}
	plain, err := secrets.UnwrapMasterKey(ctx, stored, w)
	if err != nil {
		return "", err
	}
	if plain == "" {
		return "", errors.New("unwrapped master key is empty")
	}
	return plain, nil
}

// sealMasterKeyMaterial wraps a serialized key for storage when a key service is configured.
func sealMasterKeyMaterial(ctx context.Context, serialized string, getenv func(string) string) (string, error) {
	w, err := keyWrapperFromEnv(getenv)
	if err != nil || w == nil {
		return serialized, err
	}
	return secrets.WrapMasterKey(ctx, serialized, w)
}
