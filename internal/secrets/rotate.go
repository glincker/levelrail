package secrets

import (
	"context"
	"fmt"
)

// RotateStoredDEKs re-wraps every DEK s holds from oldKey to newKey,
// via Store.RotateServiceDEKs's single transaction: if any DEK fails to
// unwrap under oldKey (wrong key supplied, corrupt data), the callback
// returns an error, the transaction rolls back, and no row changes.
// This is the low-level primitive Manager.RotateMasterKey builds on; it
// takes both keys explicitly (rather than reading Manager state) so it
// can be tested in isolation, including the wrong-old-key case.
func RotateStoredDEKs(ctx context.Context, s Store, oldKey, newKey *MasterKey) error {
	err := s.RotateServiceDEKs(ctx, func(serviceName string, wrapped []byte) ([]byte, error) {
		raw, err := oldKey.UnwrapDEK(WrappedDEK(wrapped))
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", serviceName, err)
		}
		rewrapped, err := newKey.wrap(raw)
		if err != nil {
			return nil, fmt.Errorf("service %q: %w", serviceName, err)
		}
		return rewrapped, nil
	})
	if err != nil {
		return fmt.Errorf("secrets: rotate master key: %w", err)
	}
	return nil
}
