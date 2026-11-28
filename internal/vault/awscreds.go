package vault

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	vaultapi "github.com/hashicorp/vault/api"

	"github.com/glincker/levelrail/internal/secretmask"
)

// ErrNoAWSCredentials marks a creds read that returned no usable key pair.
var ErrNoAWSCredentials = errors.New("vault: aws secrets engine returned no credentials")

// AWSSpec names one Vault aws secrets engine role for one account. Mount defaults to aws.
type AWSSpec struct {
	Account   string
	Address   string
	Namespace string
	Mount     string
	Role      string
}

// AWSCredentials is a short lived key set. It is held in memory only and never logged.
type AWSCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

type awsLease struct {
	creds   AWSCredentials
	leaseID string
	renewAt time.Time
	spec    AWSSpec
}

// AWSBroker reads aws/creds/<role>, keeps each lease until a share of its TTL has passed,
// replaces it lazily on the next use and revokes it when the account goes away.
type AWSBroker struct {
	tunables func(context.Context) Tunables
	now      func() time.Time

	mu     sync.Mutex
	leases map[string]awsLease
}

// NewAWSBroker returns a broker reading its client bounds from tunables on every call.
func NewAWSBroker(tunables func(context.Context) Tunables) *AWSBroker {
	return &AWSBroker{tunables: tunables, now: time.Now, leases: map[string]awsLease{}}
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name))); err == nil && v > 0 {
		return v
	}
	return def
}

// awsTTL is the requested lease length, clamped to 15 minutes through 1 hour.
func awsTTL() time.Duration {
	s := envInt("APP_CLOUD_AWS_VAULT_TTL_SECONDS", 900)
	return time.Duration(min(max(s, 900), 3600)) * time.Second
}

func awsRenewPercent() int { return min(envInt("APP_CLOUD_AWS_VAULT_RENEW_PERCENT", 80), 99) }

func (s AWSSpec) mount() string {
	if m := strings.Trim(s.Mount, "/"); m != "" {
		return m
	}
	return "aws"
}

// leaseKey ties a lease to the account, role and token, so a rotated token or edited role never reuses it.
func leaseKey(s AWSSpec, token string) string {
	h := sha256.New()
	for _, p := range []string{s.Account, s.Address, s.Namespace, s.mount(), s.Role, token} {
		_, _ = fmt.Fprintf(h, "%d:%s|", len(p), p)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (b *AWSBroker) client(ctx context.Context, s AWSSpec, token string) (*vaultapi.Client, error) {
	tun := b.tunables(ctx)
	secretmask.Add(token)
	hc, err := guardedClient(s.Address, tun.AllowPrivate, tun.Timeout)
	if err != nil {
		return nil, err
	}
	c, err := vaultapi.NewClient(&vaultapi.Config{Address: s.Address, HttpClient: hc})
	if err != nil {
		return nil, fmt.Errorf("vault: build client: %w", err)
	}
	if s.Namespace != "" {
		c.SetNamespace(s.Namespace)
	}
	c.SetToken(token)
	return c, nil
}

// Credentials returns live credentials for the spec, reading a new lease only when none is cached or
// the cached one has passed its renewal point. token is the Vault token resolved from the secret source.
func (b *AWSBroker) Credentials(ctx context.Context, s AWSSpec, token string) (AWSCredentials, error) {
	if token == "" || s.Role == "" {
		return AWSCredentials{}, errors.New("vault: token and role are required")
	}
	key := leaseKey(s, token)
	b.mu.Lock()
	cur, ok := b.leases[key]
	b.mu.Unlock()
	if ok && b.now().Before(cur.renewAt) {
		return cur.creds, nil
	}
	c, err := b.client(ctx, s, token)
	if err != nil {
		return AWSCredentials{}, err
	}
	ttl := awsTTL()
	sec, err := c.Logical().ReadWithDataWithContext(ctx, s.mount()+"/creds/"+s.Role, map[string][]string{"ttl": {strconv.Itoa(int(ttl.Seconds())) + "s"}})
	if err != nil {
		return AWSCredentials{}, fmt.Errorf("vault: read aws creds: %w", err)
	}
	if sec == nil || sec.Data == nil {
		return AWSCredentials{}, ErrNoAWSCredentials
	}
	creds := AWSCredentials{AccessKeyID: str(sec.Data["access_key"]), SecretAccessKey: str(sec.Data["secret_key"]), SessionToken: str(sec.Data["security_token"])}
	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return AWSCredentials{}, ErrNoAWSCredentials
	}
	secretmask.Add(creds.SecretAccessKey)
	secretmask.Add(creds.SessionToken)
	lease := time.Duration(sec.LeaseDuration) * time.Second
	if lease <= 0 {
		lease = ttl
	}
	next := awsLease{creds: creds, leaseID: sec.LeaseID, spec: s, renewAt: b.now().Add(lease * time.Duration(awsRenewPercent()) / 100)}
	b.mu.Lock()
	b.leases[key] = next
	b.mu.Unlock()
	if ok && cur.leaseID != "" && cur.leaseID != next.leaseID {
		b.revokeLease(ctx, c, cur.leaseID)
	}
	return creds, nil
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func (b *AWSBroker) revokeLease(ctx context.Context, c *vaultapi.Client, id string) {
	// Best effort: an unreachable Vault lets the lease expire on its own TTL.
	_ = c.Sys().RevokeWithContext(ctx, id)
}

// Revoke ends every cached lease of the account and drops them from memory. A failed revoke is
// reported, the cache is cleared either way.
func (b *AWSBroker) Revoke(ctx context.Context, account, token string) error {
	b.mu.Lock()
	var mine []awsLease
	for k, l := range b.leases {
		if l.spec.Account == account {
			mine = append(mine, l)
			delete(b.leases, k)
		}
	}
	b.mu.Unlock()
	var firstErr error
	for _, l := range mine {
		if l.leaseID == "" || token == "" {
			continue
		}
		c, err := b.client(ctx, l.spec, token)
		if err == nil {
			err = c.Sys().RevokeWithContext(ctx, l.leaseID)
		}
		if err != nil && firstErr == nil {
			firstErr = fmt.Errorf("vault: revoke lease: %w", err)
		}
	}
	return firstErr
}
