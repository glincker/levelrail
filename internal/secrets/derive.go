package secrets

import (
	"crypto/hmac"
	"crypto/sha256"
)

// DeriveKey returns a 32 byte key bound to purpose, or nil when no master
// key is loaded. The master key itself never leaves this package.
func (m *Manager) DeriveKey(purpose string) []byte {
	if m == nil {
		return nil
	}
	mk := m.masterKey()
	if mk == nil {
		return nil
	}
	h := hmac.New(sha256.New, []byte(mk.String()))
	h.Write([]byte("derive:" + purpose))
	return h.Sum(nil)
}
