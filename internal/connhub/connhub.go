// Package connhub is a read model over the existing connection stores (git, registry, notify,
// secrets, DNS, cloud, webhooks). It never calls a provider and never carries a secret.
package connhub

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

// Kinds of connection. The ref prefix is the kind.
const (
	KindGit      = "git"
	KindRegistry = "registry"
	KindNotify   = "notify"
	KindSecret   = "secret"
	KindDNS      = "dns"
	KindCloud    = "cloud"
	KindWebhook  = "webhook"
)

// Health values. They come from what each store already records.
const (
	HealthOK             = "ok"
	HealthDegraded       = "degraded"
	HealthNeedsReconnect = "needs_reconnect"
	HealthExpired        = "expired"
	HealthUnknown        = "unknown"
)

// Auth methods an adapter can report.
const (
	AuthAppInstall       = "app_install"
	AuthOAuth            = "oauth"
	AuthToken            = "token"
	AuthWorkloadIdentity = "workload_identity"
	AuthURLSecret        = "url_secret"
)

// Capabilities a connection offers.
const (
	CapSource        = "source"
	CapWebhook       = "webhook"
	CapStatus        = "status"
	CapDeployments   = "deployments"
	CapPreviews      = "previews"
	CapRegistryPull  = "registry_pull"
	CapNotify        = "notify"
	CapSecrets       = "secrets"
	CapDNS           = "dns"
	CapProvisionHost = "provision"
)

// Errors returned by the service.
var (
	ErrNotFound = errors.New("connhub: connection not found")
	ErrInvalid  = errors.New("connhub: invalid request")
)

// Connection is one projected connection. No field holds a credential value.
type Connection struct {
	Ref          string     `json:"ref"`
	Kind         string     `json:"kind"`
	Provider     string     `json:"provider"`
	Label        string     `json:"label"`
	Account      string     `json:"account,omitempty"`
	Host         string     `json:"host,omitempty"`
	ScopeType    string     `json:"scope_type"`
	ScopeID      string     `json:"scope_id,omitempty"`
	AuthMethod   string     `json:"auth_method"`
	Health       string     `json:"health"`
	HealthDetail string     `json:"health_detail,omitempty"`
	CheckedAt    *time.Time `json:"checked_at,omitempty"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	ExpiringSoon bool       `json:"expiring_soon"`
	RotateAfter  *time.Time `json:"rotate_after,omitempty"`
	Capabilities []string   `json:"capabilities"`
	BoundCount   int        `json:"bound_count"`
	CreatedAt    *time.Time `json:"created_at,omitempty"`
	IsDefault    bool       `json:"is_default,omitempty"`
}

// Filter narrows a listing. Empty fields match everything.
type Filter struct {
	Kind   string
	Scope  string
	Health string
}

// Binding is one link between a connection and a target. Origin is "legacy" for links derived
// from the old columns and "binding" for rows in connection_bindings.
type Binding struct {
	ID         string `json:"id,omitempty"`
	Ref        string `json:"ref"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Role       string `json:"role"`
	Rules      string `json:"rules,omitempty"`
	Origin     string `json:"origin"`
}

// Service projects connections from the configured sources.
type Service struct {
	src        Sources
	expiryWarn func(context.Context) time.Duration
	now        func() time.Time
}

// New builds a service. expiryWarn supplies the warning window, read live from runtime settings.
func New(src Sources, expiryWarn func(context.Context) time.Duration) *Service {
	return &Service{src: src, expiryWarn: expiryWarn, now: time.Now}
}

// List returns the projected connections matching f, ordered by kind then label.
func (s *Service) List(ctx context.Context, f Filter) ([]Connection, error) {
	meta, counts, err := s.sidecars(ctx)
	if err != nil {
		return nil, err
	}
	var out []Connection
	for _, a := range s.adapters() {
		if f.Kind != "" && f.Kind != a.kind {
			continue
		}
		list, err := a.list(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, list...)
	}
	filtered := out[:0]
	for _, c := range out {
		s.overlay(ctx, &c, meta, counts)
		if f.Scope != "" && f.Scope != c.ScopeType {
			continue
		}
		if f.Health != "" && f.Health != c.Health {
			continue
		}
		filtered = append(filtered, c)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Kind != filtered[j].Kind {
			return filtered[i].Kind < filtered[j].Kind
		}
		return strings.ToLower(filtered[i].Label) < strings.ToLower(filtered[j].Label)
	})
	return filtered, nil
}

// Get returns one connection by ref or ErrNotFound.
func (s *Service) Get(ctx context.Context, ref string) (Connection, error) {
	kind, _, ok := strings.Cut(ref, ":")
	if !ok {
		return Connection{}, ErrNotFound
	}
	list, err := s.List(ctx, Filter{Kind: kind})
	if err != nil {
		return Connection{}, err
	}
	for _, c := range list {
		if c.Ref == ref {
			return c, nil
		}
	}
	return Connection{}, ErrNotFound
}

// AppBindings returns the stored bindings of an app plus the links its legacy columns imply.
func (s *Service) AppBindings(ctx context.Context, app string) ([]Binding, error) {
	out := []Binding{}
	if s.src.Meta != nil {
		rows, err := s.src.Meta.ListConnectionBindingsByTarget(ctx, "app", app)
		if err != nil {
			return nil, err
		}
		for _, b := range rows {
			out = append(out, Binding{ID: b.ID, Ref: b.Ref, TargetType: b.TargetType, TargetID: b.TargetID, Role: b.Role, Rules: b.Rules, Origin: "binding"})
		}
	}
	legacy, err := s.legacyAppBindings(ctx, app)
	if err != nil {
		return nil, err
	}
	return append(out, legacy...), nil
}

func (s *Service) sidecars(ctx context.Context) (map[string]store.ConnectionMeta, map[string]int, error) {
	meta := map[string]store.ConnectionMeta{}
	counts := map[string]int{}
	if s.src.Meta == nil {
		return meta, counts, nil
	}
	rows, err := s.src.Meta.ListConnectionMeta(ctx)
	if err != nil {
		return nil, nil, err
	}
	for k, m := range rows {
		meta[k] = m
	}
	if counts, err = s.src.Meta.CountConnectionBindingsByRef(ctx); err != nil {
		return nil, nil, err
	}
	return meta, counts, nil
}

// overlay applies the sidecar: scope, last used, user entered expiry and rotation, and a stored
// health only when the native store says nothing.
func (s *Service) overlay(ctx context.Context, c *Connection, meta map[string]store.ConnectionMeta, counts map[string]int) {
	c.BoundCount += counts[c.Ref]
	if m, ok := meta[c.Ref]; ok {
		c.ScopeType, c.ScopeID = m.ScopeType, m.ScopeID
		c.LastUsedAt = ptrTime(m.LastUsedAt)
		c.RotateAfter = ptrTime(m.RotateAfter)
		if c.ExpiresAt == nil {
			c.ExpiresAt = ptrTime(m.ExpiresAt)
		}
		if c.Health == HealthUnknown && m.Health != "" && m.Health != HealthUnknown {
			c.Health, c.HealthDetail, c.CheckedAt = m.Health, m.HealthDetail, ptrTime(m.CheckedAt)
		}
	}
	if c.ScopeType == "" {
		c.ScopeType = "instance"
	}
	if c.ExpiresAt != nil {
		now := s.now()
		switch {
		case c.ExpiresAt.Before(now):
			c.Health, c.HealthDetail = HealthExpired, "credential expired"
		case s.expiryWarn != nil && c.ExpiresAt.Sub(now) <= s.expiryWarn(ctx):
			c.ExpiringSoon = true
		}
	}
}

func ptrTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	t = t.UTC()
	return &t
}
