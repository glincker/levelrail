package docker

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/go-connections/nat"
)

// BrowserSpec describes a short-lived, locked-down container that exposes
// one debugging port on the host loopback interface.
type BrowserSpec struct {
	Name    string
	Image   string
	Command []string
	Env     map[string]string
	Labels  map[string]string
	// Network is the only network the container joins; empty means none.
	Network     string
	User        string
	MemoryBytes int64
	NanoCPUs    int64
	PidsLimit   int64
	ShmBytes    int64
	// Tmpfs maps a container path to its tmpfs mount options.
	Tmpfs map[string]string
	// DebugPort is the container port published to 127.0.0.1 on a random host port.
	DebugPort int
}

// Browser is a running one-shot browser container.
type Browser struct {
	ID string
	// Addr is the host loopback address the debug port is published on.
	Addr string
	c    *Client
}

// LabeledContainer is a container found by label, in any state.
type LabeledContainer struct {
	ID      string
	Name    string
	Created time.Time
	Labels  map[string]string
}

func browserHostConfig(spec BrowserSpec, port nat.Port) *container.HostConfig {
	hc := &container.HostConfig{
		AutoRemove:     true,
		ReadonlyRootfs: true,
		CapDrop:        []string{"ALL"},
		SecurityOpt:    []string{securityOptNoNewPrivileges},
		RestartPolicy:  container.RestartPolicy{Name: container.RestartPolicyDisabled},
		Tmpfs:          spec.Tmpfs,
		PortBindings:   nat.PortMap{port: []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: ""}}},
		Resources: container.Resources{
			Memory:     spec.MemoryBytes,
			MemorySwap: spec.MemoryBytes,
			NanoCPUs:   spec.NanoCPUs,
		},
		NetworkMode: container.NetworkMode("none"),
	}
	if spec.PidsLimit > 0 {
		pids := spec.PidsLimit
		hc.PidsLimit = &pids
	}
	if spec.ShmBytes > 0 {
		hc.ShmSize = spec.ShmBytes
	}
	if spec.Network != "" {
		hc.NetworkMode = container.NetworkMode(spec.Network)
	}
	return hc
}

// StartBrowser creates and starts spec's container and reports where its
// debug port is published. The image must already exist locally. The caller
// must Close the Browser; the container also removes itself once stopped.
func (c *Client) StartBrowser(ctx context.Context, spec BrowserSpec) (*Browser, error) {
	port, err := nat.NewPort("tcp", fmt.Sprint(spec.DebugPort))
	if err != nil {
		return nil, fmt.Errorf("docker: browser debug port %d: %w", spec.DebugPort, err)
	}
	cfg := &container.Config{
		Image:        spec.Image,
		Cmd:          spec.Command,
		Env:          toDockerEnv(spec.Env),
		Labels:       c.withInstanceLabel(spec.Labels),
		User:         spec.User,
		ExposedPorts: nat.PortSet{port: struct{}{}},
	}
	resp, err := c.cli.ContainerCreate(ctx, cfg, browserHostConfig(spec, port), nil, nil, spec.Name)
	if err != nil {
		return nil, fmt.Errorf("docker: create browser container %q: %w", spec.Name, err)
	}
	b := &Browser{ID: resp.ID, c: c}
	if err := c.cli.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		b.Close()
		return nil, fmt.Errorf("docker: start browser container %q: %w", spec.Name, err)
	}
	info, err := c.cli.ContainerInspect(ctx, resp.ID)
	if err != nil {
		b.Close()
		return nil, fmt.Errorf("docker: inspect browser container %q: %w", spec.Name, err)
	}
	bindings := info.NetworkSettings.Ports[port]
	if len(bindings) == 0 || bindings[0].HostPort == "" {
		b.Close()
		return nil, fmt.Errorf("docker: browser container %q published no debug port", spec.Name)
	}
	b.Addr = "127.0.0.1:" + bindings[0].HostPort
	return b, nil
}

// Endpoint is the host loopback address of the published debug port.
func (b *Browser) Endpoint() string { return b.Addr }

// Close force-removes the container; safe to call more than once.
func (b *Browser) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = b.c.cli.ContainerRemove(ctx, b.ID, container.RemoveOptions{Force: true}) // best effort; the orphan sweep is the backstop
}

// EnsureImageID makes ref available locally and returns its image ID. pulled
// reports whether this call had to pull it.
func (c *Client) EnsureImageID(ctx context.Context, ref string) (id string, pulled bool, err error) {
	if inspected, ierr := c.cli.ImageInspect(ctx, ref); ierr == nil {
		return inspected.ID, false, nil
	} else if !cerrdefs.IsNotFound(ierr) {
		return "", false, fmt.Errorf("docker: inspect image %q: %w", ref, ierr)
	}
	if err := c.ensureImage(ctx, ref, nil, false); err != nil {
		return "", false, err
	}
	inspected, err := c.cli.ImageInspect(ctx, ref)
	if err != nil {
		return "", false, fmt.Errorf("docker: inspect pulled image %q: %w", ref, err)
	}
	return inspected.ID, true, nil
}

// ListContainersByLabel lists containers in any state carrying label
// (key=value), scoped to this Client's instance label when configured.
func (c *Client) ListContainersByLabel(ctx context.Context, label string) ([]LabeledContainer, error) {
	f := filters.NewArgs()
	f.Add("label", label)
	c.instanceLabelFilter(f)
	list, err := c.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: f})
	if err != nil {
		return nil, fmt.Errorf("docker: list containers by label %q: %w", label, err)
	}
	out := make([]LabeledContainer, 0, len(list))
	for _, s := range list {
		name := ""
		if len(s.Names) > 0 {
			name = strings.TrimPrefix(s.Names[0], "/")
		}
		out = append(out, LabeledContainer{ID: s.ID, Name: name, Created: time.Unix(s.Created, 0).UTC(), Labels: s.Labels})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out, nil
}

// ImageInUse reports whether any container, in any state, was created from imageID.
func (c *Client) ImageInUse(ctx context.Context, imageID string) (bool, error) {
	f := filters.NewArgs()
	f.Add("ancestor", imageID)
	list, err := c.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: f, Limit: 1})
	if err != nil {
		return false, fmt.Errorf("docker: list containers of image %q: %w", imageID, err)
	}
	return len(list) > 0, nil
}

// RemoveImageByID removes an image without force, so Docker itself refuses
// while anything still uses it. A missing image is not an error.
func (c *Client) RemoveImageByID(ctx context.Context, imageID string) error {
	_, err := c.cli.ImageRemove(ctx, imageID, image.RemoveOptions{Force: false, PruneChildren: true})
	if err != nil && !cerrdefs.IsNotFound(err) {
		return fmt.Errorf("docker: remove image %q: %w", imageID, err)
	}
	return nil
}
