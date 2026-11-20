package secrets

import (
	"context"
	"log/slog"
	"strings"
)

// legacyFamily describes a control plane namespace that used to live under a
// name an app could take. keyOK limits migration to the key shapes the control
// plane writes, so an app's own secret in the same slot is never swept up.
type legacyFamily struct {
	keyOK func(key string) bool
}

func anyOf(keys ...string) func(string) bool {
	return func(k string) bool {
		for _, x := range keys {
			if k == x {
				return true
			}
		}
		return false
	}
}

func withPrefix(prefixes ...string) func(string) bool {
	return func(k string) bool {
		for _, p := range prefixes {
			if strings.HasPrefix(k, p) {
				return true
			}
		}
		return false
	}
}

var legacyFixed = map[string]legacyFamily{
	"webpush":           {anyOf("vapid_private_key", "vapid_public_key")},
	"heartbeat":         {anyOf("url", "fail_url", "agent_silence_url")},
	"heartbeat-jobs":    {withPrefix("task_")},
	"token-lifecycle":   {anyOf("expiry_notices", "rotation_pending")},
	"secretconnections": {withPrefix("TOKEN_")},
	"secretproviders":   {withPrefix("CFG_", "TOKEN_", "PREV_")},
}

var legacyPrefixed = []struct {
	prefix string
	fam    legacyFamily
}{
	{"monitor-", legacyFamily{withPrefix("hdr:")}},
	{"model/", legacyFamily{anyOf("HF_TOKEN", "ENDPOINT_API_KEY")}},
}

// legacyOf returns the pre-move namespace for a control plane namespace.
func legacyOf(svc string) (string, legacyFamily, bool) {
	rest, ok := strings.CutPrefix(svc, ControlPlanePrefix)
	if !ok {
		return "", legacyFamily{}, false
	}
	if f, ok := legacyFixed[rest]; ok {
		return rest, f, true
	}
	for _, p := range legacyPrefixed {
		if strings.HasPrefix(rest, p.prefix) && len(rest) > len(p.prefix) {
			return rest, p.fam, true
		}
	}
	return "", legacyFamily{}, false
}

// legacyFor returns the old namespace for (svc, key) when the key has the
// shape the control plane wrote there.
func legacyFor(svc, key string) (string, bool) {
	legacy, fam, ok := legacyOf(svc)
	if !ok || !fam.keyOK(key) {
		return "", false
	}
	return legacy, true
}

// MigrateEntry moves one value between namespaces without judging its shape:
// copy, verify, then delete the old entry. It never overwrites an existing new
// entry and leaves the old one (with a warning) when the two differ.
func MigrateEntry(ctx context.Context, km KeyMover, legacySvc, newSvc, key string, logger *slog.Logger) error {
	return MigrateKey(ctx, km, legacySvc, newSvc, key, nil, logger)
}

// rawMover reaches the stores without the lazy migration hooks.
type rawMover struct{ m *Manager }

func (r rawMover) Resolve(ctx context.Context, s, k string) (string, error) {
	return r.m.resolve(ctx, s, k)
}
func (r rawMover) SetValue(ctx context.Context, s, k, p string) error {
	return r.m.setValue(ctx, s, k, p)
}
func (r rawMover) DeleteValue(ctx context.Context, s, k string) error {
	return r.m.deleteValue(ctx, s, k)
}

// migrateLegacy relocates one slot on first touch. It reports whether the new
// entry is now in place; failures are logged and the caller falls back to the
// old entry, so a transient error never loses a value.
func (m *Manager) migrateLegacy(ctx context.Context, svc, key string) bool {
	legacy, ok := legacyFor(svc, key)
	if !ok {
		return false
	}
	slot := svc + "\x00" + key
	if _, done := m.migrated.Load(slot); done {
		return false
	}
	if err := MigrateEntry(ctx, rawMover{m}, legacy, svc, key, m.logger); err != nil {
		m.logger.Warn("secrets: control plane entry not moved yet", slog.String("namespace", legacy))
		return false
	}
	m.migrated.Store(slot, struct{}{})
	ok2, err := m.exists(ctx, svc, key)
	return err == nil && ok2
}

// migrateLegacyNamespace relocates every control plane key in the old namespace.
func (m *Manager) migrateLegacyNamespace(ctx context.Context, svc string) {
	legacy, fam, ok := legacyOf(svc)
	if !ok {
		return
	}
	if _, done := m.migrated.Load(svc); done {
		return
	}
	keys, err := m.store.ListSecretKeys(ctx, legacy)
	if err != nil {
		m.logger.Warn("secrets: control plane namespace not listed", slog.String("namespace", legacy))
		return
	}
	failed := false
	for _, k := range keys {
		if !fam.keyOK(k.Key) {
			continue
		}
		if err := MigrateEntry(ctx, rawMover{m}, legacy, svc, k.Key, m.logger); err != nil {
			failed = true
		}
	}
	if !failed {
		m.migrated.Store(svc, struct{}{})
	}
}
