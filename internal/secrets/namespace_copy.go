package secrets

import (
	"context"
	"fmt"
)

// CopyNamespace re-encrypts every value of from (file slots included) under to,
// with its locked and build flags, and checks each decrypts back. from is left
// untouched; on any error the partial copy in to is removed.
func (m *Manager) CopyNamespace(ctx context.Context, from, to string) (err error) {
	m.migrateLegacyNamespace(ctx, from)
	keys, err := m.store.ListSecretKeys(ctx, from)
	if err != nil {
		return fmt.Errorf("secrets: list keys for %q: %w", from, err)
	}
	if len(keys) == 0 {
		return nil
	}
	defer func() {
		if err != nil {
			_ = m.DeleteAll(context.WithoutCancel(ctx), to)
		}
	}()
	for _, k := range keys {
		plain, rerr := m.Resolve(ctx, from, k.Key)
		if rerr != nil {
			return fmt.Errorf("secrets: read %q/%q: %w", from, k.Key, rerr)
		}
		if serr := m.SetValue(ctx, to, k.Key, plain); serr != nil {
			return serr
		}
		if back, verr := m.Resolve(ctx, to, k.Key); verr != nil || back != plain {
			return fmt.Errorf("secrets: copy of %q/%q did not verify", from, k.Key)
		}
		if k.Locked {
			if lerr := m.store.SetSecretLocked(ctx, to, k.Key, true); lerr != nil {
				return lerr
			}
		}
		if k.BuildTime {
			if berr := m.store.SetSecretBuildTime(ctx, to, k.Key, true); berr != nil {
				return berr
			}
		}
	}
	return nil
}
