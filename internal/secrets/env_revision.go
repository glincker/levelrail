package secrets

import (
	"encoding/json"
	"fmt"
)

// ScopeEnvRevision binds an env snapshot to its app so it cannot be replayed under another one.
const ScopeEnvRevision = "env_revision"

const envRevisionKeyPurpose = "env-revision"

func envRevisionBinding(app string) Binding {
	return Binding{Scope: ScopeEnvRevision, Owner: app, Key: "snapshot"}
}

// SealEnvRevision encrypts an app's variable map. It returns nil with no error when no master key is loaded.
func (m *Manager) SealEnvRevision(app string, env map[string]string) ([]byte, error) {
	key := m.DeriveKey(envRevisionKeyPurpose)
	if key == nil {
		return nil, nil
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("secrets: seal env revision: %w", err)
	}
	out, err := EncryptValue(key, envRevisionBinding(app), string(raw))
	if err != nil {
		return nil, fmt.Errorf("secrets: seal env revision: %w", err)
	}
	return out, nil
}

// OpenEnvRevision decrypts a snapshot made by SealEnvRevision.
func (m *Manager) OpenEnvRevision(app string, blob []byte) (map[string]string, error) {
	key := m.DeriveKey(envRevisionKeyPurpose)
	if key == nil {
		return nil, fmt.Errorf("secrets: open env revision: no master key")
	}
	plain, _, err := DecryptValue(key, envRevisionBinding(app), blob)
	if err != nil {
		return nil, fmt.Errorf("secrets: open env revision: %w", err)
	}
	var env map[string]string
	if err := json.Unmarshal([]byte(plain), &env); err != nil {
		return nil, fmt.Errorf("secrets: open env revision: %w", err)
	}
	return env, nil
}
