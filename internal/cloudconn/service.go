package cloudconn

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Errors the API maps to status codes.
var (
	ErrInvalid  = errors.New("cloudconn: invalid connection")
	ErrConflict = errors.New("cloudconn: connection name already in use")
	ErrNotFound = errors.New("cloudconn: connection not found")
)

// Connection is one named cloud account. It never carries a credential value.
type Connection struct {
	Name           string
	Provider       string
	TeamID         string
	AuthMethod     string
	Config         Config
	CredentialRef  string
	CreatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastCheckedAt  time.Time
	LastCheckClass string
}

// Input creates or updates a connection. On update empty strings keep the stored
// value and a zero Config keeps the stored settings; Provider and Name never change.
type Input struct {
	Provider      string
	TeamID        string
	AuthMethod    string
	Config        Config
	CredentialRef string
}

// TestResult is the outcome of a health probe. Reason comes from a fixed table, never from provider output.
type TestResult struct {
	OK     bool
	Class  string
	Reason string
}

// Rows is the connection table.
type Rows interface {
	ListCloudConnections(ctx context.Context) ([]store.CloudConnection, error)
	GetCloudConnection(ctx context.Context, name string) (store.CloudConnection, error)
	CreateCloudConnection(ctx context.Context, c store.CloudConnection) error
	UpdateCloudConnection(ctx context.Context, c store.CloudConnection) error
	SetCloudConnectionCheck(ctx context.Context, name, class string, at time.Time) error
	DeleteCloudConnection(ctx context.Context, name string) error
}

// Service owns the connection set and runs read-only probes.
type Service struct {
	Rows      Rows
	Providers Registry
	// Sources resolves secret:// references for a probe; nil means references cannot be resolved.
	Sources *secretsource.Resolver
	Now     func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) provider(name string) (Provider, error) {
	p, ok := s.Providers[name]
	if !ok {
		return nil, invalid("provider must be one of %v", s.Providers.Names())
	}
	return p, nil
}

func toConnection(r store.CloudConnection) Connection {
	var cfg Config
	_ = json.Unmarshal([]byte(r.Config), &cfg)
	return Connection{
		Name: r.Name, Provider: r.Provider, TeamID: r.TeamID, AuthMethod: r.AuthMethod, Config: cfg, CredentialRef: r.CredentialRef,
		CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt, LastCheckedAt: r.LastCheckedAt, LastCheckClass: r.LastCheckClass,
	}
}

// List returns every connection ordered by name.
func (s *Service) List(ctx context.Context) ([]Connection, error) {
	rows, err := s.Rows.ListCloudConnections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(rows))
	for _, r := range rows {
		out = append(out, toConnection(r))
	}
	return out, nil
}

// Get returns one connection or ErrNotFound.
func (s *Service) Get(ctx context.Context, name string) (Connection, error) {
	r, err := s.Rows.GetCloudConnection(ctx, name)
	if errors.Is(err, store.ErrCloudConnectionNotFound) {
		return Connection{}, ErrNotFound
	}
	if err != nil {
		return Connection{}, err
	}
	return toConnection(r), nil
}

// validate checks a full definition.
func (s *Service) validate(p Provider, in Input) error {
	if !validAuth[in.AuthMethod] {
		return invalid("auth_method must be oidc_federation, role_assumption, secret_ref or static")
	}
	allowed := false
	for _, a := range authAllowed[p.Name()] {
		allowed = allowed || a == in.AuthMethod
	}
	if !allowed {
		return invalid("%s does not support auth_method %s", p.Name(), in.AuthMethod)
	}
	if err := p.ValidateConfig(in.Config, in.AuthMethod); err != nil {
		return err
	}
	return validateCredentialRef(in.AuthMethod, in.CredentialRef)
}

// Create validates and stores a new connection.
func (s *Service) Create(ctx context.Context, actor, name string, in Input) (Connection, error) {
	if err := secretsource.ValidConnName(name, secretsource.CurrentLimits().MaxConn); err != nil {
		return Connection{}, invalid("name must be lowercase [a-z0-9_-], starting alphanumeric, within the length limit")
	}
	p, err := s.provider(in.Provider)
	if err != nil {
		return Connection{}, err
	}
	if err := s.validate(p, in); err != nil {
		return Connection{}, err
	}
	raw, _ := json.Marshal(in.Config)
	row := store.CloudConnection{Name: name, Provider: in.Provider, TeamID: in.TeamID, AuthMethod: in.AuthMethod, Config: string(raw), CredentialRef: in.CredentialRef, CreatedBy: actor}
	if err := s.Rows.CreateCloudConnection(ctx, row); err != nil {
		if errors.Is(err, store.ErrCloudConnectionExists) {
			return Connection{}, ErrConflict
		}
		return Connection{}, err
	}
	return s.Get(ctx, name)
}

// Update merges in over the stored connection, validates the result and saves it.
func (s *Service) Update(ctx context.Context, name string, in Input) (Connection, error) {
	cur, err := s.Get(ctx, name)
	if err != nil {
		return Connection{}, err
	}
	if in.Provider != "" && in.Provider != cur.Provider {
		return Connection{}, invalid("provider cannot change; create a new connection")
	}
	p, err := s.provider(cur.Provider)
	if err != nil {
		return Connection{}, err
	}
	merged := Input{Provider: cur.Provider, TeamID: cur.TeamID, AuthMethod: cur.AuthMethod, Config: cur.Config, CredentialRef: cur.CredentialRef}
	if in.TeamID != "" {
		merged.TeamID = in.TeamID
	}
	if in.AuthMethod != "" && in.AuthMethod != cur.AuthMethod {
		// A new auth method needs its own credential; the old reference does not carry over.
		merged.AuthMethod, merged.CredentialRef = in.AuthMethod, in.CredentialRef
	} else if in.CredentialRef != "" {
		merged.CredentialRef = in.CredentialRef
	}
	if in.Config != (Config{}) {
		merged.Config = in.Config
	}
	if err := s.validate(p, merged); err != nil {
		return Connection{}, err
	}
	raw, _ := json.Marshal(merged.Config)
	row := store.CloudConnection{Name: name, TeamID: merged.TeamID, AuthMethod: merged.AuthMethod, Config: string(raw), CredentialRef: merged.CredentialRef}
	if err := s.Rows.UpdateCloudConnection(ctx, row); err != nil {
		if errors.Is(err, store.ErrCloudConnectionNotFound) {
			return Connection{}, ErrNotFound
		}
		return Connection{}, err
	}
	return s.Get(ctx, name)
}

// Delete removes a connection.
func (s *Service) Delete(ctx context.Context, name string) error {
	err := s.Rows.DeleteCloudConnection(ctx, name)
	if errors.Is(err, store.ErrCloudConnectionNotFound) {
		return ErrNotFound
	}
	return err
}

var reasons = map[string]string{ //nolint:gosec // fixed class texts, not credentials
	ClassOK:                   "read-only check passed",
	ClassUnsupported:          "this build has no live check for the provider yet",
	ClassConfigInvalid:        "the stored settings no longer validate",
	ClassCredentialUnresolved: "the credential reference could not be resolved",
	ClassAuthFailed:           "the provider rejected the credential",
	ClassUnreachable:          "the provider endpoint could not be reached",
	ClassUnknown:              "the check failed for an unclassified reason",
}

// Test runs one read-only probe and records its class. Provider text is never returned.
func (s *Service) Test(ctx context.Context, name string) (TestResult, error) {
	c, err := s.Get(ctx, name)
	if err != nil {
		return TestResult{}, err
	}
	class := s.probe(ctx, c)
	if err := s.Rows.SetCloudConnectionCheck(ctx, name, class, s.now()); err != nil && !errors.Is(err, store.ErrCloudConnectionNotFound) {
		return TestResult{}, err
	}
	return TestResult{OK: class == ClassOK, Class: class, Reason: reasons[class]}, nil
}

func (s *Service) probe(ctx context.Context, c Connection) string {
	p, ok := s.Providers[c.Provider]
	if !ok {
		return ClassConfigInvalid
	}
	if p.ValidateConfig(c.Config, c.AuthMethod) != nil {
		return ClassConfigInvalid
	}
	t := Target{Connection: c}
	if c.AuthMethod == AuthSecretRef || c.AuthMethod == AuthRoleAssumption {
		ref, err := secretsource.ParseRef(c.CredentialRef)
		if err != nil || s.Sources == nil {
			return ClassCredentialUnresolved
		}
		v, err := s.Sources.Resolve(ctx, ref)
		if err != nil {
			return ClassCredentialUnresolved
		}
		t.Credential = v
	}
	class, err := p.Check(ctx, t)
	if errors.Is(err, ErrUnsupported) {
		return ClassUnsupported
	}
	if err != nil && class == "" {
		return ClassUnknown
	}
	if _, known := reasons[class]; !known {
		return ClassUnknown
	}
	return class
}
