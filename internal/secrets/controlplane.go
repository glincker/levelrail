package secrets

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// controlPlanePrefix starts with an underscore, which no app name can, so the
// control plane's own keys never share a namespace with app secrets.
const controlPlanePrefix = "_cp/"

// ControlPlaneService maps a control plane key owner to its private namespace.
func ControlPlaneService(name string) string { return controlPlanePrefix + name }

// KeyMover is the slice of Manager a key relocation needs.
type KeyMover interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	DeleteValue(ctx context.Context, serviceName, envKey string) error
}

// MigrateKey copies a key from its legacy namespace to the private one and
// removes the legacy entry only after the copy reads back identical. It is
// idempotent and never overwrites an existing new entry. valid screens out a
// legacy value that is not a control plane key (an app may own that slot).
func MigrateKey(ctx context.Context, km KeyMover, legacySvc, newSvc, envKey string, valid func(string) bool, logger *slog.Logger) error {
	old, err := km.Resolve(ctx, legacySvc, envKey)
	if errors.Is(err, ErrValueNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("secrets: read legacy control plane key: %w", err)
	}
	cur, err := km.Resolve(ctx, newSvc, envKey)
	switch {
	case err == nil:
		if cur != old {
			warn(logger, "legacy control plane key differs from the current one, leaving it in place", legacySvc)
			return nil
		}
		return km.DeleteValue(ctx, legacySvc, envKey)
	case !errors.Is(err, ErrValueNotFound):
		return fmt.Errorf("secrets: read control plane key: %w", err)
	}
	if valid != nil && !valid(old) {
		warn(logger, "an app already uses a reserved control plane name, rename it", legacySvc)
		return nil
	}
	if err := km.SetValue(ctx, newSvc, envKey, old); err != nil {
		return fmt.Errorf("secrets: write control plane key: %w", err)
	}
	if got, err := km.Resolve(ctx, newSvc, envKey); err != nil || got != old {
		return errors.New("secrets: control plane key did not verify after copy, legacy entry kept")
	}
	return km.DeleteValue(ctx, legacySvc, envKey)
}

func warn(l *slog.Logger, msg, svc string) {
	if l == nil {
		l = slog.Default()
	}
	l.Warn("secrets: "+msg, slog.String("namespace", svc))
}
