package proxyinstall

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

const (
	dirMode  = 0o755
	fileMode = 0o644
)

// Docker is the slice of the Engine API the install needs.
type Docker interface {
	InspectByName(ctx context.Context, name string) (*docker.ContainerState, error)
	Create(ctx context.Context, spec docker.ContainerSpec) (string, error)
	Start(ctx context.Context, id string) error
	ListLocalContainers(ctx context.Context) ([]docker.LocalContainer, error)
}

// Installer plans and applies the install. Every host fact is injected.
type Installer struct {
	Docker Docker
	// PortHolder names what listens on a host TCP port, "" when free.
	PortHolder func(ctx context.Context, port int) string
	// OwnIngressPorts are the host ports the built-in ingress binds.
	OwnIngressPorts func() []int
	// InContainer is true when the control plane itself runs in Docker, so
	// its data dir is not a host path a new container could mount.
	InContainer bool
	MkdirAll    func(path string, perm os.FileMode) error
	WriteFile   func(path string, data []byte, perm os.FileMode) error
}

// Result is what Apply did.
type Result struct {
	Plan    Plan   `json:"plan"`
	Created bool   `json:"created"`
	Started bool   `json:"started"`
	Detail  string `json:"detail"`
}

func (in *Installer) fs() (func(string, os.FileMode) error, func(string, []byte, os.FileMode) error) {
	mk, wr := in.MkdirAll, in.WriteFile
	if mk == nil {
		mk = os.MkdirAll
	}
	if wr == nil {
		wr = os.WriteFile
	}
	return mk, wr
}

// Plan builds the plan and attaches a Refusal when the host cannot take it.
// A container this install already made is not a refusal: Apply is a no-op.
func (in *Installer) Plan(ctx context.Context, input Input) (Plan, error) {
	p, err := BuildPlan(input)
	var ref *Refusal
	if errors.As(err, &ref) {
		p.Refusal = ref
		return p, nil
	}
	if err != nil {
		return Plan{}, fmt.Errorf("proxyinstall: build plan: %w", err)
	}
	if in.InContainer {
		p.Refusal = &Refusal{Code: CodeInContainer, Message: "this control plane runs inside Docker, so it cannot tell which host folder to mount into the proxy. Start Traefik yourself, then use It runs somewhere else."}
		return p, nil
	}
	st, err := in.Docker.InspectByName(ctx, p.Container)
	if err != nil {
		return Plan{}, fmt.Errorf("proxyinstall: inspect %s: %w", p.Container, err)
	}
	if st != nil {
		ours, err := in.isOurs(ctx, p.Container)
		if err != nil {
			return Plan{}, err
		}
		if !ours {
			p.Refusal = &Refusal{Code: CodeNameTaken, Message: fmt.Sprintf("a container named %s already exists and was not created by this control plane, so it was left alone. Remove or rename it, or use It runs somewhere else.", p.Container)}
		}
		return p, nil
	}
	p.Refusal = in.portRefusal(ctx)
	return p, nil
}

func (in *Installer) isOurs(ctx context.Context, name string) (bool, error) {
	all, err := in.Docker.ListLocalContainers(ctx)
	if err != nil {
		return false, fmt.Errorf("proxyinstall: list containers: %w", err)
	}
	for _, c := range all {
		if c.Name != name {
			continue
		}
		for k, v := range c.Labels {
			if strings.HasSuffix(k, "."+LabelManaged) && v != "" {
				return true, nil
			}
		}
	}
	return false, nil
}

func (in *Installer) portRefusal(ctx context.Context) *Refusal {
	own := map[int]bool{}
	if in.OwnIngressPorts != nil {
		for _, port := range in.OwnIngressPorts() {
			own[port] = true
		}
	}
	for _, port := range []int{HTTPPort, HTTPSPort} {
		if own[port] {
			return &Refusal{Code: CodeOwnIngress, Message: fmt.Sprintf("port %d is held by this control plane's built-in Caddy. Traefik cannot start until it lets go, and switching ingress does not hand the ports over by itself. Keep the built-in Caddy, or move its ports in Settings, Ingress first.", port)}
		}
		if holder := in.PortHolder(ctx, port); holder != "" {
			return &Refusal{Code: CodePortBusy, Message: fmt.Sprintf("port %d is already in use on this server by %s. Stop that first (nothing was changed), or use It runs somewhere else.", port, holder)}
		}
	}
	return nil
}

// Apply writes the files and creates and starts the container. A container
// it already made is started if stopped and otherwise left alone.
func (in *Installer) Apply(ctx context.Context, input Input) (Result, error) {
	p, err := in.Plan(ctx, input)
	if err != nil {
		return Result{}, err
	}
	res := Result{Plan: p}
	if p.Refusal != nil {
		return res, p.Refusal
	}
	st, err := in.Docker.InspectByName(ctx, p.Container)
	if err != nil {
		return res, fmt.Errorf("proxyinstall: inspect %s: %w", p.Container, err)
	}
	if st != nil {
		res.Detail = "already installed"
		if st.Running {
			return res, nil
		}
		if err := in.Docker.Start(ctx, st.ID); err != nil {
			return res, fmt.Errorf("proxyinstall: start %s: %w", p.Container, err)
		}
		res.Started, res.Detail = true, "started the existing container"
		return res, nil
	}
	mk, wr := in.fs()
	for _, m := range p.Mounts {
		if filepath.Ext(m.Host) == "" {
			if err := mk(m.Host, dirMode); err != nil {
				return res, fmt.Errorf("proxyinstall: create %s: %w", m.Host, err)
			}
		}
	}
	for _, f := range p.Files {
		if err := mk(filepath.Dir(f.Path), dirMode); err != nil {
			return res, fmt.Errorf("proxyinstall: create %s: %w", filepath.Dir(f.Path), err)
		}
		if err := wr(f.Path, []byte(f.Content), fileMode); err != nil {
			return res, fmt.Errorf("proxyinstall: write %s: %w", f.Path, err)
		}
	}
	id, err := in.Docker.Create(ctx, p.Spec())
	if err != nil {
		return res, fmt.Errorf("proxyinstall: create %s: %w", p.Container, err)
	}
	res.Created = true
	if err := in.Docker.Start(ctx, id); err != nil {
		return res, fmt.Errorf("proxyinstall: start %s: %w", p.Container, err)
	}
	res.Started, res.Detail = true, "created and started"
	return res, nil
}
