package models

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	endpointCredentialKey = "ENDPOINT_API_KEY" //nolint:gosec // secrets key name, not a credential
	maxLatencyBudget      = 10 * time.Minute
)

// EndpointStore is the store surface external endpoints need. *store.DB satisfies it.
type EndpointStore interface {
	SaveInferenceEndpoint(ctx context.Context, m store.Model, e store.InferenceEndpoint) error
	GetInferenceEndpoint(ctx context.Context, name string) (*store.InferenceEndpoint, error)
	ListInferenceEndpoints(ctx context.Context) ([]store.InferenceEndpoint, error)
	SetInferenceEndpointRouting(ctx context.Context, name, fallback, fallbackRef string, budget time.Duration) error
	SetInferenceEndpointCredential(ctx context.Context, name string, set bool) error
	RecordInferenceEndpointHealth(ctx context.Context, name string, h store.EndpointHealth) error
	DeleteModel(ctx context.Context, name string) error
}

// CredentialStore reads and writes an endpoint credential in the secrets manager.
type CredentialStore interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// ExternalInput registers an external endpoint through Create.
type ExternalInput struct {
	BaseURL    string
	APIType    string
	Credential string
	// FallbackModel is empty to leave the routing policy off.
	FallbackModel string
	FallbackRef   string
	LatencyBudget time.Duration
}

// EndpointView is an external endpoint as the API reports it.
type EndpointView struct {
	store.InferenceEndpoint
	Ready bool
}

// ProbeInput is a test request for an endpoint that may not be saved yet.
type ProbeInput struct {
	BaseURL    string
	APIType    string
	Credential string
}

// EndpointPlumbing wires external endpoint support into a Service.
type EndpointPlumbing struct {
	Store       EndpointStore
	Credentials CredentialStore
	Checker     EndpointChecker
	Lookup      HostLookup
}

// SetEndpoints enables external endpoints on the service.
func (s *Service) SetEndpoints(p EndpointPlumbing) {
	if p.Checker == nil {
		p.Checker = NewEndpointProber(EndpointSlowThreshold())
	}
	s.ep = &p
}

func (s *Service) endpoints() (*EndpointPlumbing, error) {
	if s.ep == nil {
		return nil, fmt.Errorf("%w: external endpoints are not configured", ErrInvalid)
	}
	return s.ep, nil
}

func validAPIType(t string) (string, error) {
	switch t {
	case "", store.EndpointAPIOpenAI:
		return store.EndpointAPIOpenAI, nil
	case store.EndpointAPIOllama:
		return t, nil
	}
	return "", fmt.Errorf("%w: api type %q is not supported (openai or ollama)", ErrInvalid, t)
}

func (s *Service) validateRouting(ctx context.Context, self, fallback string, budget time.Duration) error {
	if budget < 0 || budget > maxLatencyBudget {
		return fmt.Errorf("%w: latency budget must be between 0 and %s", ErrInvalid, maxLatencyBudget)
	}
	if fallback == "" {
		if budget > 0 {
			return fmt.Errorf("%w: a latency budget needs a fallback model", ErrInvalid)
		}
		return nil
	}
	if fallback == self {
		return fmt.Errorf("%w: an endpoint cannot fall back to itself", ErrInvalid)
	}
	m, err := s.store.GetModel(ctx, fallback)
	if errors.Is(err, store.ErrModelNotFound) {
		return fmt.Errorf("%w: fallback model %q does not exist", ErrInvalid, fallback)
	}
	if err != nil {
		return fmt.Errorf("models: look up fallback model: %w", err)
	}
	if m.Deleting {
		return fmt.Errorf("%w: fallback model %q is being deleted", ErrInvalid, fallback)
	}
	return nil
}

func (s *Service) createExternal(ctx context.Context, in CreateInput) (Created, error) {
	p, err := s.endpoints()
	if err != nil {
		return Created{}, err
	}
	ext := in.External
	if !nameRe.MatchString(in.Spec.Name) {
		return Created{}, fmt.Errorf("%w: name must be 1-42 lowercase letters, digits or hyphens, starting and ending alphanumeric", ErrInvalid)
	}
	apiType, err := validAPIType(ext.APIType)
	if err != nil {
		return Created{}, err
	}
	base, err := NormalizeEndpointURL(ctx, ext.BaseURL, p.Lookup)
	if err != nil {
		return Created{}, err
	}
	if ext.Credential != "" && p.Credentials == nil {
		return Created{}, ErrSecretsUnavailable
	}
	if err := s.validateRouting(ctx, in.Spec.Name, ext.FallbackModel, ext.LatencyBudget); err != nil {
		return Created{}, err
	}
	domain := strings.ToLower(strings.TrimSpace(in.Domain))
	if domain != "" && !domainRe.MatchString(domain) {
		return Created{}, fmt.Errorf("%w: domain %q is not a valid hostname", ErrInvalid, in.Domain)
	}
	key, hash, prefix, err := NewAPIKey()
	if err != nil {
		return Created{}, err
	}
	m := store.Model{
		Name: in.Spec.Name, Engine: EngineExternal, ModelRef: in.Spec.ModelRef, Domain: domain,
		APIKeyHash: hash, APIKeyPrefix: prefix, Residency: store.ResidencyAlways, Compute: ComputeExternal,
	}
	probe := p.Checker.Probe(ctx, base, apiType, ext.Credential)
	e := store.InferenceEndpoint{
		Name: m.Name, BaseURL: base, APIType: apiType, CredentialSet: ext.Credential != "",
		FallbackModel: ext.FallbackModel, FallbackRef: ext.FallbackRef, LatencyBudget: ext.LatencyBudget,
	}
	h := probe.ToHealth(time.Now().UTC())
	e.HealthState, e.HealthLatency, e.HealthError, e.HealthCheckedAt = h.State, h.Latency, h.Error, h.CheckedAt
	e.Discovered, e.DiscoveryError = h.Models, h.DiscoveryError
	if h.State == store.EndpointOnline || h.State == store.EndpointSlow {
		e.LastOKAt = h.CheckedAt
	}
	if err := p.Store.SaveInferenceEndpoint(ctx, m, e); err != nil {
		return Created{}, err
	}
	if ext.Credential != "" {
		if err := p.Credentials.SetValue(ctx, store.ModelSecretsKey(m.Name), endpointCredentialKey, ext.Credential); err != nil {
			_ = p.Store.DeleteModel(ctx, m.Name)
			return Created{}, fmt.Errorf("models: store endpoint credential: %w", err)
		}
	}
	saved, err := s.store.GetModel(ctx, m.Name)
	if err != nil {
		return Created{}, fmt.Errorf("models: reload model: %w", err)
	}
	s.keysChanged()
	return Created{Model: *saved, APIKey: key}, nil
}

// TestEndpointInput probes an endpoint that is not saved yet.
func (s *Service) TestEndpointInput(ctx context.Context, in ProbeInput) (EndpointProbe, error) {
	p, err := s.endpoints()
	if err != nil {
		return EndpointProbe{}, err
	}
	apiType, err := validAPIType(in.APIType)
	if err != nil {
		return EndpointProbe{}, err
	}
	base, err := NormalizeEndpointURL(ctx, in.BaseURL, p.Lookup)
	if err != nil {
		return refusalResult(err)
	}
	return p.Checker.Probe(ctx, base, apiType, in.Credential), nil
}

func refusalResult(err error) (EndpointProbe, error) {
	switch {
	case errors.Is(err, ErrEndpointBlocked):
		return EndpointProbe{State: store.EndpointError, Code: DiagBlocked, Message: err.Error()}, nil
	case errors.Is(err, ErrEndpointPrivate):
		return EndpointProbe{State: store.EndpointError, Code: DiagPrivateDisabled, Message: err.Error()}, nil
	}
	return EndpointProbe{}, err
}

// TestEndpoint re-probes a saved endpoint, records the result and returns it.
func (s *Service) TestEndpoint(ctx context.Context, name string) (EndpointProbe, error) {
	p, err := s.endpoints()
	if err != nil {
		return EndpointProbe{}, err
	}
	e, err := p.Store.GetInferenceEndpoint(ctx, name)
	if err != nil {
		return EndpointProbe{}, err
	}
	return s.probeAndRecord(ctx, p, *e), nil
}

func (s *Service) probeAndRecord(ctx context.Context, p *EndpointPlumbing, e store.InferenceEndpoint) EndpointProbe {
	cred := ""
	if e.CredentialSet && p.Credentials != nil {
		c, err := p.Credentials.Resolve(ctx, store.ModelSecretsKey(e.Name), endpointCredentialKey)
		if err != nil {
			slog.Warn("models: endpoint credential unreadable", slog.String("model", e.Name), slog.String("error", err.Error()))
		}
		cred = c
	}
	res := p.Checker.Probe(ctx, e.BaseURL, e.APIType, cred)
	if err := p.Store.RecordInferenceEndpointHealth(ctx, e.Name, res.ToHealth(time.Now().UTC())); err != nil {
		slog.Warn("models: record endpoint health failed", slog.String("model", e.Name), slog.String("error", err.Error()))
	}
	return res
}

// GetEndpoint returns one endpoint with its health.
func (s *Service) GetEndpoint(ctx context.Context, name string) (EndpointView, error) {
	p, err := s.endpoints()
	if err != nil {
		return EndpointView{}, err
	}
	e, err := p.Store.GetInferenceEndpoint(ctx, name)
	if err != nil {
		return EndpointView{}, err
	}
	return endpointView(*e), nil
}

// ListEndpoints returns every endpoint keyed by model name.
func (s *Service) ListEndpoints(ctx context.Context) (map[string]EndpointView, error) {
	out := map[string]EndpointView{}
	if s.ep == nil {
		return out, nil
	}
	list, err := s.ep.Store.ListInferenceEndpoints(ctx)
	if err != nil {
		return nil, fmt.Errorf("models: list endpoints: %w", err)
	}
	for _, e := range list {
		out[e.Name] = endpointView(e)
	}
	return out, nil
}

func endpointView(e store.InferenceEndpoint) EndpointView {
	return EndpointView{InferenceEndpoint: e, Ready: e.HealthState == store.EndpointOnline || e.HealthState == store.EndpointSlow}
}

// SetEndpointRouting sets or clears the fallback policy; an empty fallback
// turns it off.
func (s *Service) SetEndpointRouting(ctx context.Context, name, fallback, fallbackRef string, budget time.Duration) error {
	p, err := s.endpoints()
	if err != nil {
		return err
	}
	if _, err := p.Store.GetInferenceEndpoint(ctx, name); err != nil {
		return err
	}
	if err := s.validateRouting(ctx, name, fallback, budget); err != nil {
		return err
	}
	if fallback == "" {
		fallbackRef, budget = "", 0
	}
	if err := p.Store.SetInferenceEndpointRouting(ctx, name, fallback, fallbackRef, budget); err != nil {
		return err
	}
	s.keysChanged()
	return nil
}

// SetEndpointCredential stores a new credential and re-probes. An empty
// credential removes it.
func (s *Service) SetEndpointCredential(ctx context.Context, name, credential string) (EndpointProbe, error) {
	p, err := s.endpoints()
	if err != nil {
		return EndpointProbe{}, err
	}
	if p.Credentials == nil {
		return EndpointProbe{}, ErrSecretsUnavailable
	}
	e, err := p.Store.GetInferenceEndpoint(ctx, name)
	if err != nil {
		return EndpointProbe{}, err
	}
	if credential == "" {
		if err := p.Credentials.DeleteAll(ctx, store.ModelSecretsKey(name)); err != nil {
			return EndpointProbe{}, fmt.Errorf("models: remove endpoint credential: %w", err)
		}
	} else if err := p.Credentials.SetValue(ctx, store.ModelSecretsKey(name), endpointCredentialKey, credential); err != nil {
		return EndpointProbe{}, fmt.Errorf("models: store endpoint credential: %w", err)
	}
	if err := p.Store.SetInferenceEndpointCredential(ctx, name, credential != ""); err != nil {
		return EndpointProbe{}, err
	}
	e.CredentialSet = credential != ""
	s.keysChanged()
	return s.probeAndRecord(ctx, p, *e), nil
}

// deleteExternal removes an external model at once; there is no container
// for a controller to tear down.
func (s *Service) deleteExternal(ctx context.Context, name string) error {
	if err := s.ep.Store.DeleteModel(ctx, name); err != nil {
		return err
	}
	if s.ep.Credentials != nil {
		if err := s.ep.Credentials.DeleteAll(ctx, store.ModelSecretsKey(name)); err != nil {
			slog.Warn("models: remove endpoint credential failed", slog.String("model", name), slog.String("error", err.Error()))
		}
	}
	s.keysChanged()
	return nil
}

// ProbeAll re-probes every endpoint once.
func (s *Service) ProbeAll(ctx context.Context) {
	if s.ep == nil {
		return
	}
	list, err := s.ep.Store.ListInferenceEndpoints(ctx)
	if err != nil {
		slog.Warn("models: list endpoints for health check failed", slog.String("error", err.Error()))
		return
	}
	for _, e := range list {
		pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		s.probeAndRecord(pctx, s.ep, e)
		cancel()
	}
	s.keysChanged()
}

// RunEndpointHealth probes every endpoint each interval until ctx ends.
func (s *Service) RunEndpointHealth(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		s.ProbeAll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
