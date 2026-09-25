package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultPrefix      = "platform"
	stopTimeout        = 30 * time.Second
	hfTokenKey         = "HF_TOKEN"
	controllerNamePref = "model/"
)

// ControllerName is the reconcile_status name for model name.
func ControllerName(name string) string { return controllerNamePref + name }

// Store is the narrow store surface the controller needs.
type Store interface {
	GetModel(ctx context.Context, name string) (*store.Model, error)
	SetModelEndpoint(ctx context.Context, name, dial string) error
	DeleteModel(ctx context.Context, name string) error
}

// Secrets is the narrow secrets surface the controller needs.
type Secrets interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
	DeleteAll(ctx context.Context, serviceName string) error
}

// NodeInfo is what the controller needs to know about a model's node.
type NodeInfo struct {
	GPU gpu.Info
	// BindIP is the address the engine's port is published on.
	BindIP string
	// DialHost is the address the control plane reaches BindIP at. Empty
	// means the node is remote and not on the mesh.
	DialHost string
}

// Nodes resolves a model's node. nodeID "" is the local node.
type Nodes interface {
	NodeInfo(ctx context.Context, nodeID string) (NodeInfo, error)
}

// Controller converges one model row to a running engine container.
type Controller struct {
	name    string
	store   Store
	nodes   Nodes
	runtime docker.Runtime
	prober  Prober
	secrets Secrets
	prefix  string
	images  map[string]string
}

// Option configures a Controller.
type Option func(*Controller)

// WithContainerPrefix sets the container and volume name prefix
// (typically brand.ShortName).
func WithContainerPrefix(p string) Option { return func(c *Controller) { c.prefix = p } }

// WithImages overrides engine images by engine name.
func WithImages(images map[string]string) Option { return func(c *Controller) { c.images = images } }

// WithSecrets enables resolving the model's HuggingFace token and
// deleting its secrets on teardown. Without it, a model with a token set
// stays blocked.
func WithSecrets(s Secrets) Option { return func(c *Controller) { c.secrets = s } }

// New builds the Controller for model name.
func New(name string, st Store, nodes Nodes, rt docker.Runtime, prober Prober, opts ...Option) *Controller {
	c := &Controller{name: name, store: st, nodes: nodes, runtime: rt, prober: prober}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Name implements reconcile.Controller.
func (c *Controller) Name() string { return ControllerName(c.name) }

// ContainerPrefix is the name prefix shared by every container of model
// name, across config revisions.
func ContainerPrefix(prefix, name string) string {
	if prefix == "" {
		prefix = defaultPrefix
	}
	return prefix + "-model-" + name + "."
}

// VolumeName is the persistent model-cache volume of model name.
func VolumeName(prefix, name string) string {
	if prefix == "" {
		prefix = defaultPrefix
	}
	return prefix + "-model-" + name + "-cache"
}

// SpecFromModel converts a stored model into its Spec.
func SpecFromModel(m *store.Model) Spec {
	return Spec{Name: m.Name, Engine: m.Engine, ModelRef: m.ModelRef, GPUCount: m.GPUCount,
		GPUDeviceIDs: m.GPUDeviceIDs, ContextLength: m.ContextLength, Quantization: m.Quantization}
}

func (c *Controller) imageFor(engine string) string {
	if img := c.images[engine]; img != "" {
		return img
	}
	return engines[engine].DefaultImage
}

func (c *Controller) containerName(m *store.Model, image string, hfSet bool) string {
	h := sha256.New()
	_, _ = fmt.Fprint(h, m.Engine, "|", m.ModelRef, "|", m.GPUCount, "|", strings.Join(m.GPUDeviceIDs, ","), "|",
		m.ContextLength, "|", m.Quantization, "|", m.RestartNonce, "|", image, "|", hfSet, "|", m.NodeID)
	return ContainerPrefix(c.prefix, m.Name) + hex.EncodeToString(h.Sum(nil))[:8]
}

// Reconcile implements reconcile.Controller.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	m, err := c.store.GetModel(ctx, c.name)
	if errors.Is(err, store.ErrModelNotFound) {
		return unknown("NoDesiredState", ""), nil
	}
	if err != nil {
		return notReady("StoreError", err.Error()), fmt.Errorf("models/%s: get model: %w", c.name, err)
	}
	if m.Deleting {
		return c.teardown(ctx, m)
	}

	node, err := c.nodes.NodeInfo(ctx, m.NodeID)
	if err != nil {
		return notReady("NodeLookupFailed", err.Error()), fmt.Errorf("models/%s: node info: %w", c.name, err)
	}
	if blocked := gpuBlock(m, node); blocked != nil {
		return *blocked, nil
	}

	hfToken, blocked := c.resolveHFToken(ctx, m)
	if blocked != nil {
		return *blocked, nil
	}

	image := c.imageFor(m.Engine)
	target := c.containerName(m, image, hfToken != "")
	if err := c.removeStale(ctx, m, target); err != nil {
		return notReady("CleanupFailed", err.Error()), fmt.Errorf("models/%s: remove stale containers: %w", c.name, err)
	}

	state, err := c.runtime.InspectByName(ctx, target)
	if err != nil {
		return notReady("InspectFailed", err.Error()), fmt.Errorf("models/%s: inspect %q: %w", c.name, target, err)
	}
	if state == nil {
		return c.create(ctx, m, node, target, image, hfToken)
	}
	if !state.Running {
		if err := c.runtime.Start(ctx, state.ID); err != nil {
			return notReady("StartFailed", err.Error()), fmt.Errorf("models/%s: start %q: %w", c.name, target, err)
		}
		return notReady("Starting", "engine container is starting"), nil
	}
	return c.observe(ctx, m, node, state)
}

func gpuBlock(m *store.Model, node NodeInfo) *reconcile.Result {
	if !node.GPU.Present {
		res := notReady("NoGPUOnNode", "the model's node reports no NVIDIA GPU; pick a GPU node")
		return &res
	}
	if !node.GPU.RuntimeInstalled {
		res := notReady("GPURuntimeMissing", "the node has a GPU but Docker has no nvidia runtime. "+gpu.InstallHint)
		return &res
	}
	if len(m.GPUDeviceIDs) == 0 && m.GPUCount > node.GPU.Count() {
		res := notReady("InsufficientGPUs", fmt.Sprintf("model needs %d GPUs, node has %d", m.GPUCount, node.GPU.Count()))
		return &res
	}
	return nil
}

func (c *Controller) resolveHFToken(ctx context.Context, m *store.Model) (string, *reconcile.Result) {
	if !m.HFTokenSet {
		return "", nil
	}
	if c.secrets == nil {
		res := notReady("HFTokenUnavailable", "a HuggingFace token is set but no secrets master key is configured")
		return "", &res
	}
	tok, err := c.secrets.Resolve(ctx, store.ModelSecretsKey(m.Name), hfTokenKey)
	if err != nil {
		res := notReady("HFTokenUnavailable", "could not read the HuggingFace token: "+err.Error())
		return "", &res
	}
	return tok, nil
}

func resolvedGPUs(m *store.Model, node NodeInfo) int {
	switch {
	case len(m.GPUDeviceIDs) > 0:
		return len(m.GPUDeviceIDs)
	case m.GPUCount > 0:
		return m.GPUCount
	}
	return node.GPU.Count()
}

func (c *Controller) create(ctx context.Context, m *store.Model, node NodeInfo, target, image, hfToken string) (reconcile.Result, error) {
	eng := engines[m.Engine]
	vol := VolumeName(c.prefix, m.Name)
	if err := c.runtime.EnsureVolume(ctx, vol); err != nil {
		return notReady("VolumeFailed", err.Error()), fmt.Errorf("models/%s: ensure volume %q: %w", c.name, vol, err)
	}
	spec := SpecFromModel(m)
	gpuReq := &docker.GPURequest{Count: m.GPUCount, DeviceIDs: m.GPUDeviceIDs}
	id, err := c.runtime.Create(ctx, docker.ContainerSpec{
		Name:         target,
		Image:        image,
		Ports:        []docker.PortBinding{{ContainerPort: eng.Port, HostIP: node.BindIP}},
		Env:          Env(spec, hfToken),
		Volumes:      []docker.VolumeMount{{Name: vol, ContainerPath: eng.CachePath}},
		GPU:          gpuReq,
		Command:      Command(spec, resolvedGPUs(m, node)),
		ShmSizeBytes: ShmBytes(m.Engine),
	})
	if err != nil {
		return notReady("CreateFailed", err.Error()), fmt.Errorf("models/%s: create %q: %w", c.name, target, err)
	}
	if err := c.runtime.Start(ctx, id); err != nil {
		return notReady("StartFailed", err.Error()), fmt.Errorf("models/%s: start %q: %w", c.name, target, err)
	}
	return notReady("Starting", "engine container created and starting"), nil
}

func (c *Controller) observe(ctx context.Context, m *store.Model, node NodeInfo, state *docker.ContainerState) (reconcile.Result, error) {
	eng := engines[m.Engine]
	hostPort := 0
	for _, p := range state.Ports {
		if p.ContainerPort == eng.Port && p.HostPort > 0 {
			hostPort = p.HostPort
			break
		}
	}
	if hostPort == 0 {
		return notReady("Starting", "waiting for the engine port to be published"), nil
	}
	if node.DialHost == "" {
		return notReady("EndpointUnreachable", "the node is not on the WireGuard mesh, so the control plane cannot reach the engine; join the mesh to serve this model"), nil
	}
	dial := node.DialHost + ":" + strconv.Itoa(hostPort)
	if m.EndpointDial != dial {
		if err := c.store.SetModelEndpoint(ctx, m.Name, dial); err != nil {
			return notReady("StoreError", err.Error()), fmt.Errorf("models/%s: set endpoint: %w", c.name, err)
		}
	}

	st := c.prober.Probe(ctx, m.Engine, dial, m.ModelRef)
	switch st.Phase {
	case PhaseReady:
		return reconcile.Result{Conditions: []reconcile.Condition{{
			Type: "Ready", Status: reconcile.ConditionTrue, Reason: "ModelLoaded", Message: m.ModelRef + " is loaded and serving",
		}}}, nil
	case PhaseDownloading:
		return notReady("Downloading", st.Detail), nil
	case PhaseFailed:
		return notReady("DownloadFailed", st.Detail), nil
	case PhaseUnreachable:
		return notReady("Starting", st.Detail), nil
	}
	return notReady("Loading", st.Detail), nil
}

func (c *Controller) removeStale(ctx context.Context, m *store.Model, keep string) error {
	all, err := c.runtime.ListByPrefix(ctx, ContainerPrefix(c.prefix, m.Name))
	if err != nil {
		return fmt.Errorf("list containers: %w", err)
	}
	for _, s := range all {
		if s.Name == keep {
			continue
		}
		if err := c.removeContainer(ctx, s); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) removeContainer(ctx context.Context, s docker.ContainerState) error {
	if s.Running {
		if err := c.runtime.Stop(ctx, s.ID, stopTimeout); err != nil {
			return fmt.Errorf("stop %q: %w", s.Name, err)
		}
	}
	if err := c.runtime.Remove(ctx, s.ID, true); err != nil {
		return fmt.Errorf("remove %q: %w", s.Name, err)
	}
	return nil
}

// teardown removes the model's containers, then its secrets, then its
// row. Each step is idempotent so a failure part way retries cleanly.
// The weights volume is kept.
func (c *Controller) teardown(ctx context.Context, m *store.Model) (reconcile.Result, error) {
	all, err := c.runtime.ListByPrefix(ctx, ContainerPrefix(c.prefix, m.Name))
	if err != nil {
		return notReady("DeleteFailed", err.Error()), fmt.Errorf("models/%s: delete: list containers: %w", c.name, err)
	}
	for _, s := range all {
		if err := c.removeContainer(ctx, s); err != nil {
			return notReady("DeleteFailed", err.Error()), fmt.Errorf("models/%s: delete: %w", c.name, err)
		}
	}
	if c.secrets != nil {
		if err := c.secrets.DeleteAll(ctx, store.ModelSecretsKey(m.Name)); err != nil {
			return notReady("DeleteFailed", err.Error()), fmt.Errorf("models/%s: delete secrets: %w", c.name, err)
		}
	}
	if err := c.store.DeleteModel(ctx, m.Name); err != nil {
		return notReady("DeleteFailed", err.Error()), fmt.Errorf("models/%s: delete row: %w", c.name, err)
	}
	return unknown("Deleted", "model removed; the weights volume was kept"), nil
}

func notReady(reason, msg string) reconcile.Result {
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: "Ready", Status: reconcile.ConditionFalse, Reason: reason, Message: msg,
	}}}
}

func unknown(reason, msg string) reconcile.Result {
	return reconcile.Result{Conditions: []reconcile.Condition{{
		Type: "Ready", Status: reconcile.ConditionUnknown, Reason: reason, Message: msg,
	}}}
}
