package api

import (
	"time"

	"github.com/go-webauthn/webauthn/protocol"
)

type passkeyRegistrationBeginResponse struct {
	SessionID string                       `json:"session_id"`
	Options   *protocol.CredentialCreation `json:"options"`
}

type passkeyLoginBeginRequest struct {
	Username string `json:"username"`
	// Discoverable asks for a challenge with no account named (usernameless sign-in).
	Discoverable bool `json:"discoverable,omitempty"`
}

type passkeyLoginBeginResponse struct {
	SessionID string                        `json:"session_id"`
	Options   *protocol.CredentialAssertion `json:"options"`
}

type passkeyResource struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	Transports []string   `json:"transports,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	// Provider and ProviderKey come from the AAGUID table; ProviderName is empty when the authenticator is unknown.
	ProviderKey  string `json:"provider_key"`
	ProviderName string `json:"provider_name,omitempty"`
	// Synced is nil when the backup-eligible flag was never recorded for this credential.
	Synced    *bool  `json:"synced,omitempty"`
	BackedUp  *bool  `json:"backed_up,omitempty"`
	SignCount uint32 `json:"sign_count"`
}
