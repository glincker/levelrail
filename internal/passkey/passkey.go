// Package passkey wraps github.com/go-webauthn/webauthn for this
// project's two ceremonies (registering a credential, signing in with
// one). Unlike internal/totp's from-scratch RFC 6238 implementation,
// WebAuthn is not simple enough for stdlib: it's a multi-step
// challenge/response protocol with CBOR attestation objects and COSE
// key parsing, and hand-rolling that verification is a real security
// risk. go-webauthn is the de facto standard Go implementation and is
// used by many production Go auth systems, so this is a deliberate
// exception to this codebase's usual "avoid dependencies stdlib already
// covers" instinct.
package passkey

import (
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// Credential is one stored WebAuthn credential (migrations/0255), the
// shape internal/store persists and internal/api converts to and from
// webauthn.Credential.
type Credential struct {
	ID           string
	UserID       string
	CredentialID []byte
	PublicKey    []byte
	SignCount    uint32
	AAGUID       []byte
	Transports   []string
	Label        string
	CreatedAt    time.Time
	LastUsedAt   *time.Time
}

// User adapts an account and its stored credentials to webauthn.User.
// WebAuthnID is the account's own store.User.ID, which is why login
// ceremonies never need a separate lookup table: webauthn.SessionData
// carries it as UserID, and that value alone resolves back to the
// account signing in.
type User struct {
	ID          string
	Name        string
	DisplayName string
	Credentials []Credential
}

// WebAuthnID returns u's account ID as the WebAuthn user handle.
func (u User) WebAuthnID() []byte { return []byte(u.ID) }

// WebAuthnName returns u's account name (its email).
func (u User) WebAuthnName() string { return u.Name }

// WebAuthnDisplayName returns u's human-facing display name.
func (u User) WebAuthnDisplayName() string { return u.DisplayName }

// WebAuthnCredentials returns u's stored credentials, converted to the
// library's own shape.
func (u User) WebAuthnCredentials() []webauthn.Credential {
	out := make([]webauthn.Credential, len(u.Credentials))
	for i, c := range u.Credentials {
		out[i] = ToWebAuthnCredential(c)
	}
	return out
}

// ToWebAuthnCredential converts a stored Credential to the shape the
// library's ceremonies compare against.
func ToWebAuthnCredential(c Credential) webauthn.Credential {
	transports := make([]protocol.AuthenticatorTransport, len(c.Transports))
	for i, t := range c.Transports {
		transports[i] = protocol.AuthenticatorTransport(t)
	}
	return webauthn.Credential{
		ID:        c.CredentialID,
		PublicKey: c.PublicKey,
		Transport: transports,
		Authenticator: webauthn.Authenticator{
			AAGUID:    c.AAGUID,
			SignCount: c.SignCount,
		},
	}
}

// FromWebAuthnCredential converts a freshly created library credential
// (webauthn.WebAuthn.CreateCredential's result) into the row
// internal/store persists. id, userID and label are supplied by the
// caller since the library has no notion of either.
func FromWebAuthnCredential(id, userID, label string, wc *webauthn.Credential, now time.Time) Credential {
	transports := make([]string, len(wc.Transport))
	for i, t := range wc.Transport {
		transports[i] = string(t)
	}
	return Credential{
		ID:           id,
		UserID:       userID,
		CredentialID: wc.ID,
		PublicKey:    wc.PublicKey,
		SignCount:    wc.Authenticator.SignCount,
		AAGUID:       wc.Authenticator.AAGUID,
		Transports:   transports,
		Label:        label,
		CreatedAt:    now,
	}
}

// NewRelyingParty builds a *webauthn.WebAuthn scoped to one HTTP
// request's own host and scheme. There is no single fixed domain to
// configure at startup: this is a self-hosted platform, so the relying
// party ID is whatever domain the operator is actually reached on, the
// same request-derived origin internal/api's oauthRedirectURL already
// uses for OAuth callbacks. Timeouts.Enforce is set on both ceremonies
// so an expired challenge is rejected by the library itself, on top of
// the caller's own short-TTL, single-use session store.
func NewRelyingParty(rpID, rpDisplayName, origin string) (*webauthn.WebAuthn, error) {
	w, err := webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: rpDisplayName,
		RPOrigins:     []string{origin},
		Timeouts: webauthn.TimeoutsConfig{
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: 2 * time.Minute},
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: 2 * time.Minute},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("passkey: build relying party: %w", err)
	}
	return w, nil
}
