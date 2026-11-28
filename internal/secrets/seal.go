package secrets

import (
	"context"
	"fmt"

	"github.com/glincker/levelrail/internal/secretmask"
)

// SealValue encrypts plaintext for one (service, key) slot without saving it,
// so a caller can persist the ciphertext inside its own transaction.
func (m *Manager) SealValue(ctx context.Context, serviceName, envKey, plaintext string) ([]byte, error) {
	m.migrateLegacy(ctx, serviceName, envKey)
	m.valueMu.RLock()
	defer m.valueMu.RUnlock()
	dek, err := m.dekFor(ctx, serviceName)
	if err != nil {
		return nil, err
	}
	ct, err := EncryptValue(dek, slotBinding(serviceName, envKey), plaintext)
	if err != nil {
		return nil, fmt.Errorf("secrets: seal value for %q/%q: %w", serviceName, envKey, err)
	}
	secretmask.Add(plaintext)
	return ct, nil
}
