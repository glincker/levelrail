package models

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Errors returned by Service, distinguishable by callers to pick an
// HTTP status.
var (
	ErrInvalid            = errors.New("models: invalid request")
	ErrSecretsUnavailable = errors.New("models: no secrets master key is configured")
	ErrNodeNotFound       = errors.New("models: node not found")
)

var domainRe = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

// ServiceStore is the store surface Service uses. *store.DB satisfies it.
type ServiceStore interface {
	SaveModel(ctx context.Context, m store.Model) error
	GetModel(ctx context.Context, name string) (*store.Model, error)
	ListModels(ctx context.Context) ([]store.Model, error)
	DeleteModel(ctx context.Context, name string) error
	MarkModelDeleting(ctx context.Context, name string) error
	RotateModelAPIKey(ctx context.Context, name, hash, prefix string) error
	RestartModel(ctx context.Context, name string) error
	SetModelHFTokenSet(ctx context.Context, name string, set bool) error
	GetConditions(ctx context.Context, controllerName string) ([]reconcile.Condition, error)
	GetConditionsForControllers(ctx context.Context, controllerNames []string) (map[string][]reconcile.Condition, error)
	GetNode(ctx context.Context, id string) (*store.Node, error)
	GetNodeGPU(ctx context.Context, nodeID string) (store.NodeGPU, bool, error)
	ListNodes(ctx context.Context) ([]store.Node, error)
	ListNodeGPUs(ctx context.Context) (map[string]store.NodeGPU, error)
	ListDesiredServices(ctx context.Context) ([]store.DesiredService, error)
}

// SecretWriter stores the HuggingFace token.
type SecretWriter interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	DeleteAll(ctx context.Context, serviceName string) error
}

// Service implements model management for the API, CLI and MCP surfaces.
type Service struct {
	store   ServiceStore
	secrets SecretWriter
	hosts   *HostResolver
	localID string
}

// NewService builds a Service. secrets may be nil (HuggingFace tokens
// are then rejected). localNodeID identifies the control plane's own node
// so its GPU snapshot is found under the local key.
func NewService(st ServiceStore, secrets SecretWriter, hosts *HostResolver, localNodeID string) *Service {
	return &Service{store: st, secrets: secrets, hosts: hosts, localID: localNodeID}
}

// CreateInput is a request to deploy a model.
type CreateInput struct {
	Spec    Spec
	NodeID  string
	Domain  string
	HFToken string
}

// Created is the result of Create; APIKey is the plaintext, shown once.
type Created struct {
	Model  store.Model
	APIKey string
}

// NodeGPU returns the GPU snapshot of nodeID ("" or the local node ID is
// the control plane's host). known is false when the node has not
// reported yet.
func (s *Service) NodeGPU(ctx context.Context, nodeID string) (info gpu.Info, known bool, err error) {
	return s.nodeGPU(ctx, nodeID)
}

func (s *Service) nodeGPU(ctx context.Context, nodeID string) (gpu.Info, bool, error) {
	key := nodeID
	if nodeID == "" || nodeID == s.localID {
		key = store.LocalNodeGPUKey
	}
	g, ok, err := s.store.GetNodeGPU(ctx, key)
	return g.Info, ok, err
}

// SetLocalNodeID tells the service which node ID is the control plane's
// own host. Call once at startup before serving requests.
func (s *Service) SetLocalNodeID(id string) { s.localID = id }

// Create validates and stores a new model, returning its one-time API key.
func (s *Service) Create(ctx context.Context, in CreateInput) (Created, error) {
	if err := in.Spec.Validate(); err != nil {
		return Created{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if in.Spec.GPUCount == 0 {
		in.Spec.GPUCount = -1
	}
	domain := strings.ToLower(strings.TrimSpace(in.Domain))
	if domain != "" && !domainRe.MatchString(domain) {
		return Created{}, fmt.Errorf("%w: domain %q is not a valid hostname", ErrInvalid, in.Domain)
	}
	if in.HFToken != "" && s.secrets == nil {
		return Created{}, ErrSecretsUnavailable
	}
	if in.NodeID == s.localID {
		in.NodeID = ""
	}
	if in.NodeID != "" {
		if _, err := s.store.GetNode(ctx, in.NodeID); errors.Is(err, store.ErrNodeNotFound) {
			return Created{}, ErrNodeNotFound
		} else if err != nil {
			return Created{}, fmt.Errorf("models: look up node: %w", err)
		}
	}
	if info, known, err := s.nodeGPU(ctx, in.NodeID); err != nil {
		return Created{}, fmt.Errorf("models: read node gpu: %w", err)
	} else if known && !info.Present {
		return Created{}, fmt.Errorf("%w: the selected node reports no NVIDIA GPU", ErrInvalid)
	}

	key, hash, prefix, err := NewAPIKey()
	if err != nil {
		return Created{}, err
	}
	m := store.Model{
		Name: in.Spec.Name, Engine: in.Spec.Engine, ModelRef: in.Spec.ModelRef, NodeID: in.NodeID,
		GPUCount: in.Spec.GPUCount, GPUDeviceIDs: in.Spec.GPUDeviceIDs, ContextLength: in.Spec.ContextLength,
		Quantization: in.Spec.Quantization, Domain: domain, APIKeyHash: hash, APIKeyPrefix: prefix,
		HFTokenSet: in.HFToken != "",
	}
	if err := s.store.SaveModel(ctx, m); err != nil {
		return Created{}, err
	}
	if in.HFToken != "" {
		if err := s.secrets.SetValue(ctx, store.ModelSecretsKey(m.Name), hfTokenKey, in.HFToken); err != nil {
			_ = s.store.DeleteModel(ctx, m.Name)
			return Created{}, fmt.Errorf("models: store huggingface token: %w", err)
		}
	}
	saved, err := s.store.GetModel(ctx, m.Name)
	if err != nil {
		return Created{}, fmt.Errorf("models: reload model: %w", err)
	}
	return Created{Model: *saved, APIKey: key}, nil
}

// Delete tombstones the model; its controller removes the container and
// row.
func (s *Service) Delete(ctx context.Context, name string) error {
	return s.store.MarkModelDeleting(ctx, name)
}

// Restart recreates the model's container.
func (s *Service) Restart(ctx context.Context, name string) error {
	return s.store.RestartModel(ctx, name)
}

// RotateKey issues a new API key, invalidating the old one.
func (s *Service) RotateKey(ctx context.Context, name string) (string, error) {
	key, hash, prefix, err := NewAPIKey()
	if err != nil {
		return "", err
	}
	if err := s.store.RotateModelAPIKey(ctx, name, hash, prefix); err != nil {
		return "", err
	}
	return key, nil
}

// SetHFToken stores a new HuggingFace token and restarts the model so it
// takes effect.
func (s *Service) SetHFToken(ctx context.Context, name, token string) error {
	if s.secrets == nil {
		return ErrSecretsUnavailable
	}
	if _, err := s.store.GetModel(ctx, name); err != nil {
		return err
	}
	if err := s.secrets.SetValue(ctx, store.ModelSecretsKey(name), hfTokenKey, token); err != nil {
		return fmt.Errorf("models: store huggingface token: %w", err)
	}
	if err := s.store.SetModelHFTokenSet(ctx, name, true); err != nil {
		return err
	}
	return s.store.RestartModel(ctx, name)
}

// View is a model with its reconcile status and endpoint.
type View struct {
	Model   store.Model
	Ready   bool
	Reason  string
	Message string
	BaseURL string
}

func (s *Service) view(m store.Model, conds []reconcile.Condition) View {
	v := View{Model: m, BaseURL: s.hosts.BaseURL(m), Reason: "Pending"}
	if m.Deleting {
		v.Reason = "Deleting"
		return v
	}
	for _, c := range conds {
		if c.Type == "Ready" {
			v.Ready = c.Status == reconcile.ConditionTrue
			v.Reason, v.Message = c.Reason, c.Message
		}
	}
	return v
}

// List returns every model with status.
func (s *Service) List(ctx context.Context) ([]View, error) {
	list, err := s.store.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(list))
	for i, m := range list {
		names[i] = ControllerName(m.Name)
	}
	conds, err := s.store.GetConditionsForControllers(ctx, names)
	if err != nil {
		return nil, fmt.Errorf("models: read conditions: %w", err)
	}
	out := make([]View, len(list))
	for i, m := range list {
		out[i] = s.view(m, conds[ControllerName(m.Name)])
	}
	return out, nil
}

// Get returns one model with status.
func (s *Service) Get(ctx context.Context, name string) (View, error) {
	m, err := s.store.GetModel(ctx, name)
	if err != nil {
		return View{}, err
	}
	conds, err := s.store.GetConditions(ctx, ControllerName(name))
	if err != nil {
		return View{}, fmt.Errorf("models: read conditions: %w", err)
	}
	return s.view(*m, conds), nil
}
