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
}
