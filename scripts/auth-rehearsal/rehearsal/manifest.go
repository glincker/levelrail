// Package rehearsal builds a synthetic legacy database and verifies the
// auth backfill against it. It is dev tooling and is never linked into the
// production binaries.
package rehearsal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Token states a seeded API token can be in.
const (
	StateActive    = "active"
	StateExpired   = "expired"
	StateRevoked   = "revoked"
	StateOrphan    = "orphan"
	StateOwnerless = "ownerless"
)

// User kinds a seeded user can be.
const (
	KindNormal    = "normal"
	KindOAuthOnly = "oauth-only"
	KindMixedCase = "mixed-case"
	KindUnicode   = "unicode"
	KindLong      = "long"
	KindNoAt      = "no-at-sign"
	KindCaseDupe  = "case-dupe"
)

// Config sizes and seeds one synthetic database.
type Config struct {
	Dir        string `json:"dir"`
	Seed       uint64 `json:"seed"`
	Users      int    `json:"users"`
	Tokens     int    `json:"tokens"`
	Passkeys   int    `json:"passkeys"`
	TOTPUsers  int    `json:"totp_users"`
	CaseDupes  int    `json:"case_dupes"`
	OAuth      int    `json:"oauth_identities"`
	BcryptCost int    `json:"bcrypt_cost"`
}

// DefaultConfig is the default rehearsal size.
func DefaultConfig(dir string) Config {
	return Config{Dir: dir, Seed: 1, Users: 2000, Tokens: 5000, Passkeys: 300, TOTPUsers: 400, CaseDupes: 5, BcryptCost: 10}
}

// Scaled returns c with every count multiplied by n.
func (c Config) Scaled(n int) Config {
	c.Users *= n
	c.Tokens *= n
	c.Passkeys *= n
	c.TOTPUsers *= n
	c.CaseDupes *= n
	c.OAuth *= n
	return c
}

// User is one seeded legacy user and what the harness knows about it.
type User struct {
	ID            string   `json:"id"`
	Email         string   `json:"email"`
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	Password      string   `json:"password,omitempty"`
	Abilities     []string `json:"abilities"`
	TOTPSecret    string   `json:"totp_secret,omitempty"`
	RecoveryCodes int      `json:"recovery_codes,omitempty"`
	Passkeys      int      `json:"passkeys,omitempty"`
	DupeOf        string   `json:"dupe_of,omitempty"`
}

// Token is one seeded legacy API token; Raw is the plaintext, known only to the seeder.
type Token struct {
	ID        string   `json:"id"`
	Raw       string   `json:"raw"`
	Owner     string   `json:"owner"`
	Abilities []string `json:"abilities"`
	State     string   `json:"state"`
}

// Passkey is one seeded legacy passkey as stored, for read-back comparison.
type Passkey struct {
	UserID       string `json:"user_id"`
	CredentialID []byte `json:"credential_id"`
	PublicKey    []byte `json:"public_key"`
	SignCount    uint32 `json:"sign_count"`
}

// OAuthIdentity is one seeded linked external account. Legacy stores no
// provider tokens, so there is nothing else to carry.
type OAuthIdentity struct {
	UserID         string `json:"user_id"`
	Provider       string `json:"provider"`
	ProviderUserID string `json:"provider_user_id"`
}

// Expected holds the counts a correct backfill must report.
type Expected struct {
	Users             int            `json:"users"`
	Passwords         int            `json:"passwords"`
	Tokens            int            `json:"tokens"`
	TokensSkipped     int            `json:"tokens_skipped"`
	Passkeys          int            `json:"passkeys"`
	TOTP              int            `json:"totp"`
	CaseCollisions    int            `json:"case_collisions"`
	OAuth             int            `json:"oauth_identities"`
	TokensByState     map[string]int `json:"tokens_by_state"`
	UsersByKind       map[string]int `json:"users_by_kind"`
	ExpectedRecovered int            `json:"recovery_not_moved"`
}

// Manifest is the answer key written next to the seeded database.
type Manifest struct {
	Config   Config          `json:"config"`
	Users    []User          `json:"users"`
	Tokens   []Token         `json:"tokens"`
	Passkeys []Passkey       `json:"passkeys"`
	OAuth    []OAuthIdentity `json:"oauth"`
	Expected Expected        `json:"expected"`
}

const manifestFile = "manifest.json"

// Save writes the manifest into its data directory.
func (m *Manifest) Save() error {
	b, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return fmt.Errorf("rehearsal: encode manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(m.Config.Dir, manifestFile), b, 0o600); err != nil {
		return fmt.Errorf("rehearsal: write manifest: %w", err)
	}
	return nil
}

// LoadManifest reads the manifest Seed wrote into dir.
func LoadManifest(dir string) (*Manifest, error) {
	b, err := os.ReadFile(filepath.Join(dir, manifestFile)) //nolint:gosec // operator-supplied harness path
	if err != nil {
		return nil, fmt.Errorf("rehearsal: read manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("rehearsal: decode manifest: %w", err)
	}
	return &m, nil
}
