// Package mail defines the engine-neutral contract the control plane uses to
// manage an in-house mail server. Adapters live in subpackages.
package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

// SendCapEnv sets the default per-account hourly send cap carried on
// AccountSpec. It is data only; nothing enforces it yet.
const (
	SendCapEnv     = "APP_MAIL_DEFAULT_SEND_CAP_PER_HOUR"
	defaultSendCap = 200
)

// DefaultSendCapPerHour reads the default cap from the environment.
func DefaultSendCapPerHour() int {
	if n, err := strconv.Atoi(os.Getenv(SendCapEnv)); err == nil && n > 0 {
		return n
	}
	return defaultSendCap
}

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
	// Disabled is true while the engine refuses logins for the account.
	Disabled bool `json:"disabled"`
}

// AccountDetail is one account's management view: addresses, login state and
// disk usage. It never carries a password or message content.
type AccountDetail struct {
	Name           string   `json:"name"`
	Addresses      []string `json:"addresses"`
	LoginDisabled  bool     `json:"login_disabled"`
	DiskUsageBytes int64    `json:"disk_usage_bytes"`
}

// QueueEntry is one queued outbound message, limited to delivery metadata:
// never the subject, message id or body (ADR 059 section 2.8).
type QueueEntry struct {
	ID              int64     `json:"id"`
	Queued          time.Time `json:"queued"`
	Hold            bool      `json:"hold"`
	SenderAccount   string    `json:"sender_account"`
	RecipientDomain string    `json:"recipient_domain"`
	Attempts        int       `json:"attempts"`
	NextAttempt     time.Time `json:"next_attempt"`
	LastError       string    `json:"last_error,omitempty"`
}

// AccountSpec creates an account with its first address.
type AccountSpec struct {
	Name     string
	Address  string
	Password string
	// SendCapPerHour is advisory data for a later enforcement slice; 0 means
	// use DefaultSendCapPerHour.
	SendCapPerHour int
}

// String redacts the password so a printed spec never leaks it.
func (a AccountSpec) String() string {
	return fmt.Sprintf("{Name:%s Address:%s Password:[redacted] SendCapPerHour:%d}", a.Name, a.Address, a.SendCapPerHour)
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
	// Aliases lists the aliases of one domain.
	Aliases(ctx context.Context, domain string) ([]Alias, error)
	// AccountDetail reads one account's addresses, login state and disk usage.
	AccountDetail(ctx context.Context, name string) (AccountDetail, error)
	// SetLoginDisabled refuses logins with message as the reason; "" allows them again.
	SetLoginDisabled(ctx context.Context, account, message string) error
	QueueList(ctx context.Context, limit int) ([]QueueEntry, error)
	// HoldMessages sets hold on the listed queue ids and returns how many changed.
	HoldMessages(ctx context.Context, ids []int64, hold bool) (int, error)
	QueueStats(ctx context.Context) (QueueStats, error)
	// HoldQueue pauses delivery of every queued message; ReleaseQueue resumes it.
	HoldQueue(ctx context.Context) error
	ReleaseQueue(ctx context.Context) error
	Health(ctx context.Context) error
}
