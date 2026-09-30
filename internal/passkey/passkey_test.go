package passkey

import (
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

func TestToWebAuthnCredential_RoundTrip(t *testing.T) {
	c := Credential{
		CredentialID: []byte("cred-id"),
		PublicKey:    []byte("public-key-bytes"),
		SignCount:    42,
		AAGUID:       []byte("aaguid-bytes"),
		Transports:   []string{"usb", "internal"},
	}
	wc := ToWebAuthnCredential(c)
	if string(wc.ID) != string(c.CredentialID) {
		t.Errorf("ID = %q, want %q", wc.ID, c.CredentialID)
	}
	if string(wc.PublicKey) != string(c.PublicKey) {
		t.Errorf("PublicKey = %q, want %q", wc.PublicKey, c.PublicKey)
	}
	if wc.Authenticator.SignCount != c.SignCount {
		t.Errorf("SignCount = %d, want %d", wc.Authenticator.SignCount, c.SignCount)
	}
	if len(wc.Transport) != 2 || wc.Transport[0] != protocol.AuthenticatorTransport("usb") {
		t.Errorf("Transport = %v, want [usb internal]", wc.Transport)
	}
}

func TestFromWebAuthnCredential_RoundTrip(t *testing.T) {
	now := time.Now().UTC()
	wc := &webauthn.Credential{
		ID:        []byte("cred-id"),
		PublicKey: []byte("public-key-bytes"),
		Transport: []protocol.AuthenticatorTransport{protocol.USB, protocol.Internal},
		Authenticator: webauthn.Authenticator{
			AAGUID:    []byte("aaguid-bytes"),
			SignCount: 9,
		},
	}
	c := FromWebAuthnCredential("pk_1", "user_1", "My Key", wc, now)
	if c.ID != "pk_1" || c.UserID != "user_1" || c.Label != "My Key" {
		t.Errorf("c = %+v, want id/userID/label pk_1/user_1/My Key", c)
	}
	if string(c.CredentialID) != "cred-id" {
		t.Errorf("CredentialID = %q, want cred-id", c.CredentialID)
	}
	if c.SignCount != 9 {
		t.Errorf("SignCount = %d, want 9", c.SignCount)
	}
	if len(c.Transports) != 2 || c.Transports[0] != "usb" {
		t.Errorf("Transports = %v, want [usb internal]", c.Transports)
	}
	if !c.CreatedAt.Equal(now) {
		t.Errorf("CreatedAt = %v, want %v", c.CreatedAt, now)
	}
}

func TestUser_WebAuthnCredentials(t *testing.T) {
	u := User{
		ID:   "user_1",
		Name: "a@example.com",
		Credentials: []Credential{
			{CredentialID: []byte("one")},
			{CredentialID: []byte("two")},
		},
	}
	if string(u.WebAuthnID()) != "user_1" {
		t.Errorf("WebAuthnID() = %q, want user_1", u.WebAuthnID())
	}
	creds := u.WebAuthnCredentials()
	if len(creds) != 2 {
		t.Fatalf("len(creds) = %d, want 2", len(creds))
	}
	if string(creds[0].ID) != "one" || string(creds[1].ID) != "two" {
		t.Errorf("creds = %+v, want IDs one, two", creds)
	}
}

func TestNewRelyingParty_ValidConfig(t *testing.T) {
	rp, err := NewRelyingParty("example.com", "Example", "https://example.com")
	if err != nil {
		t.Fatalf("NewRelyingParty() error = %v", err)
	}
	if rp.Config.RPID != "example.com" {
		t.Errorf("RPID = %q, want example.com", rp.Config.RPID)
	}
}

func TestNewRelyingParty_MalformedRPIDRejected(t *testing.T) {
	// A relying party ID must be a bare domain, not a full origin: this
	// is the mistake internal/api's own request-derived RPID must never
	// make (a scheme or port slipping into what should be host-only).
	if _, err := NewRelyingParty("https://example.com", "Example", "https://example.com"); err == nil {
		t.Fatal("NewRelyingParty() with a scheme in RPID: error = nil, want a validation error")
	}
}

func TestNewRelyingParty_EmptyOriginsRejected(t *testing.T) {
	if _, err := webauthn.New(&webauthn.Config{RPID: "example.com", RPDisplayName: "Example"}); err == nil {
		t.Fatal("webauthn.New() with no RPOrigins: error = nil, want a validation error")
	}
}
