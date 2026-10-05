package authengine

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/secrets"
)

const (
	keyService = "auth-engine"
	keyName    = "encryption_key"
	keyBytes   = 32
)

// SecretResolver reads a stored secret by service and key (internal/secrets.Manager).
type SecretResolver interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// KeyStore is the slice of secrets.Manager the engine key needs.
type KeyStore interface {
	SecretResolver
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
}

// LoadOrCreateKey returns the 32-byte key the library uses to encrypt TOTP
// secrets, generating it on first use. persist=false (dry runs) keeps a new key in memory only.
func LoadOrCreateKey(ctx context.Context, ks KeyStore, persist bool) ([]byte, error) {
	v, err := ks.Resolve(ctx, keyService, keyName)
	if err == nil {
		key, decErr := base64.StdEncoding.DecodeString(v)
		if decErr != nil || len(key) != keyBytes {
			return nil, errors.New("authengine: stored encryption key is malformed")
		}
		return key, nil
	}
	if !errors.Is(err, secrets.ErrValueNotFound) {
		return nil, fmt.Errorf("authengine: load encryption key: %w", err)
	}
	key := make([]byte, keyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("authengine: generate encryption key: %w", err)
	}
	if !persist {
		return key, nil
	}
	if err := ks.SetValue(ctx, keyService, keyName, base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, fmt.Errorf("authengine: store encryption key: %w", err)
	}
	return key, nil
}
