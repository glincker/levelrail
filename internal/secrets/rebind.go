package secrets

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	rebindPageSize = 200
	// maxRebindFailures caps how many failed slots a result lists; the
	// count keeps going past it.
	maxRebindFailures = 50
)

var (
	errNoDEK     = errors.New("no data encryption key for this owner")
	errDEKUnwrap = errors.New("data encryption key does not unwrap under the active master key")
)

// BindingStatus counts stored values by envelope format.
type BindingStatus struct {
	Total  int
	Bound  int
	Legacy int
}

// RebindFailure names one slot Rebind could not bind, never its value.
type RebindFailure struct {
	Owner  string
	Key    string
	Reason string
}

// RebindResult reports one Rebind run.
type RebindResult struct {
	Scanned      int
	Rebound      int
	AlreadyBound int
	// Changed counts rows rewritten by someone else mid-run; a concurrent
	// SetValue always writes the bound format, so nothing is lost.
	Changed     int
	FailedCount int
	Failed      []RebindFailure
	Remaining   int
}

// BindingStatus reports how many stored values still use the legacy
// unbound format, from a prefix count that never decrypts anything.
func (m *Manager) BindingStatus(ctx context.Context) (BindingStatus, error) {
	total, bound, err := m.store.CountSecretValuesByPrefix(ctx, BoundPrefix())
	if err != nil {
		return BindingStatus{}, fmt.Errorf("secrets: binding status: %w", err)
	}
	return BindingStatus{Total: total, Bound: bound, Legacy: total - bound}, nil
}

// Rebind re-encrypts every legacy value in place with its slot binding.
// Each row is swapped on its own, compare-and-swap, so an interrupted run
// keeps its progress and a rerun picks up where it stopped. Bound rows
// are verified and left alone, which makes it idempotent. A storage
// error aborts the run; a row that fails to decrypt is reported and
// skipped.
func (m *Manager) Rebind(ctx context.Context) (RebindResult, error) {
	m.rebindMu.Lock()
	defer m.rebindMu.Unlock()

	// Held for the run so a rotation cannot swap the key under the DEK cache.
	m.mu.RLock()
	mk := m.mk
	result, err := m.rebindWith(ctx, mk)
	m.mu.RUnlock()
	if err != nil {
		return result, err
	}

	status, err := m.BindingStatus(ctx)
	if err != nil {
		return result, err
	}
	result.Remaining = status.Legacy
	return result, nil
}

func (m *Manager) rebindWith(ctx context.Context, mk *MasterKey) (RebindResult, error) {
	var result RebindResult
	deks := map[string][]byte{}
	dekErrs := map[string]error{}
	dekFor := func(serviceName string) ([]byte, error) {
		if dek, ok := deks[serviceName]; ok {
			return dek, nil
		}
		if err, ok := dekErrs[serviceName]; ok {
			return nil, err
		}
		wrapped, err := m.store.GetServiceDEK(ctx, serviceName)
		if errors.Is(err, store.ErrServiceDEKNotFound) {
			dekErrs[serviceName] = errNoDEK
			return nil, errNoDEK
		}
		if err != nil {
			return nil, fmt.Errorf("secrets: rebind: get DEK for %q: %w", serviceName, err)
		}
		dek, err := mk.UnwrapDEK(WrappedDEK(wrapped))
		if err != nil {
			dekErrs[serviceName] = errDEKUnwrap
			return nil, errDEKUnwrap
		}
		deks[serviceName] = dek
		return dek, nil
	}

	var afterService, afterKey string
	for {
		rows, err := m.store.ListSecretCiphertexts(ctx, afterService, afterKey, rebindPageSize)
		if err != nil {
			return result, fmt.Errorf("secrets: rebind: list values: %w", err)
		}
		for _, row := range rows {
			if err := m.rebindRow(ctx, row, dekFor, &result); err != nil {
				return result, err
			}
		}
		if len(rows) < rebindPageSize {
			return result, nil
		}
		last := rows[len(rows)-1]
		afterService, afterKey = last.ServiceName, last.EnvKey
	}
}

func (m *Manager) rebindRow(ctx context.Context, row store.SecretCiphertext, dekFor func(string) ([]byte, error), result *RebindResult) error {
	result.Scanned++
	fail := func(reason string) {
		result.FailedCount++
		if len(result.Failed) < maxRebindFailures {
			result.Failed = append(result.Failed, RebindFailure{Owner: row.ServiceName, Key: row.EnvKey, Reason: reason})
		}
	}

	dek, err := dekFor(row.ServiceName)
	switch {
	case errors.Is(err, errNoDEK), errors.Is(err, errDEKUnwrap):
		fail(err.Error())
		return nil
	case err != nil:
		return err
	}

	b := slotBinding(row.ServiceName, row.EnvKey)
	plaintext, legacy, err := DecryptValue(dek, b, row.Ciphertext)
	switch {
	case errors.Is(err, ErrBindingMismatch):
		fail("ciphertext is bound to a different slot")
		return nil
	case err != nil:
		fail("ciphertext does not decrypt")
		return nil
	case !legacy:
		result.AlreadyBound++
		return nil
	}

	bound, err := EncryptValue(dek, b, plaintext)
	if err != nil {
		return fmt.Errorf("secrets: rebind %q/%q: %w", row.ServiceName, row.EnvKey, err)
	}
	swapped, err := m.store.ReplaceSecretCiphertext(ctx, row.ServiceName, row.EnvKey, row.Ciphertext, bound)
	if err != nil {
		return fmt.Errorf("secrets: rebind %q/%q: %w", row.ServiceName, row.EnvKey, err)
	}
	if swapped {
		result.Rebound++
	} else {
		result.Changed++
	}
	return nil
}
