// Package mail defines the engine-neutral contract the control plane uses to
// manage an in-house mail server. Adapters live in subpackages.
package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

// Typed failures every adapter maps its engine errors onto.
var (
	ErrNotFound     = errors.New("mail: not found")
	ErrExists       = errors.New("mail: already exists")
	ErrUnauthorized = errors.New("mail: engine rejected the admin credentials")
	ErrUnsupported  = errors.New("mail: not supported by this engine")
	ErrInvalid      = errors.New("mail: invalid request")
)

// Error carries the engine's own message alongside the typed sentinel.
type Error struct {
	Kind    error
	Code    string
	Message string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return e.Kind.Error()
	}
	return e.Kind.Error() + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.Kind }

// CredentialSource yields the engine admin password at call time so callers
// can read it from internal/secrets and the adapter never stores it long term.
type CredentialSource interface {
	AdminPassword(ctx context.Context) (string, error)
}

// Domain is a mail domain the engine serves.
type Domain struct {
	Name string `json:"name"`
}

// DomainSpec creates a domain. Account is an existing account that receives
// the domain's default address; leave Localpart empty to reuse it, or set both
// to create a new account with that first address.
type DomainSpec struct {
	Name      string
	Account   string
	Localpart string
}

// Account is a mailbox owner. Quota is reported only when the engine exposes it.
type Account struct {
	Name string `json:"name"`
}

// AccountSpec creates an account with its first address.
type AccountSpec struct {
	Name     string
	Address  string
	Password string
}

// String redacts the password so a printed spec never leaks it.
func (a AccountSpec) String() string {
	return fmt.Sprintf("{Name:%s Address:%s Password:[redacted]}", a.Name, a.Address)
}

// GoString covers %#v with the same redaction.
func (a AccountSpec) GoString() string { return "mail.AccountSpec" + a.String() }

// LogValue redacts the password for slog.
func (a AccountSpec) LogValue() slog.Value {
	return slog.GroupValue(slog.String("name", a.Name), slog.String("address", a.Address), slog.String("password", "[redacted]"))
}

// Alias forwards an address to targets.
type Alias struct {
	Address string   `json:"address"`
	Targets []string `json:"targets"`
}

// Quota is a per-account size limit in bytes (0 means unlimited).
type Quota struct {
	MaxBytes int64
}

// DNSRecord is one parsed zone-file line from the engine.
type DNSRecord struct {
	Name  string `json:"name"`
	TTL   int    `json:"ttl,omitempty"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// DKIMKey is a DKIM selector and public key found in the engine's records.
// PublicKey is the bare p= value, ready for mailrecords.Spec.DKIMPublicKey.
type DKIMKey struct {
	Selector  string `json:"selector"`
	PublicKey string `json:"public_key"`
	TXT       string `json:"txt"`
}

// QueueStats summarises the outbound queue.
type QueueStats struct {
	Queued  int `json:"queued"`
	Failing int `json:"failing"`
}

// Engine is the control plane's view of a mail server.
type Engine interface {
	Domains(ctx context.Context) ([]Domain, error)
	AddDomain(ctx context.Context, d DomainSpec) error
	RemoveDomain(ctx context.Context, name string) error
	DNSRecords(ctx context.Context, domain string) ([]DNSRecord, error)
	DKIMKeys(ctx context.Context, domain string) ([]DKIMKey, error)
	Accounts(ctx context.Context) ([]Account, error)
	AddAccount(ctx context.Context, a AccountSpec) error
	RemoveAccount(ctx context.Context, name string) error
	AddAddress(ctx context.Context, address, account string) error
	SetPassword(ctx context.Context, account, password string) error
	SetQuota(ctx context.Context, account string, q Quota) error
	AddAlias(ctx context.Context, a Alias) error
	RemoveAlias(ctx context.Context, address string) error
	QueueStats(ctx context.Context) (QueueStats, error)
	DropQueue(ctx context.Context) error
	Health(ctx context.Context) error
}
