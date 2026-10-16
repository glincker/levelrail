package docker

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
)

// ImageDetails is what a deploy trace reports about a local image.
type ImageDetails struct {
	ID             string
	Arch, OS       string
	SizeBytes      int64
	Entrypoint     []string
	Cmd            []string
	ExposedPorts   []string
	HasHealthcheck bool
	Healthcheck    []string
}

// NodePlatform is the platform the daemon runs containers on.
type NodePlatform struct {
	Arch, OS string
}

// ImageDetailInspector is an optional Runtime capability (type-asserted, like
// ExitStateInspector) used only to enrich a deploy trace.
type ImageDetailInspector interface {
	InspectImageDetails(ctx context.Context, ref string) (*ImageDetails, error)
	NodePlatform(ctx context.Context) (NodePlatform, error)
}

// InspectImageDetails implements ImageDetailInspector.
func (c *Client) InspectImageDetails(ctx context.Context, ref string) (*ImageDetails, error) {
	info, err := c.cli.ImageInspect(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("docker: inspect image %q: %w", ref, err)
	}
	d := &ImageDetails{ID: info.ID, Arch: info.Architecture, OS: info.Os, SizeBytes: info.Size}
	if cfg := info.Config; cfg != nil {
		d.Entrypoint, d.Cmd = cfg.Entrypoint, cfg.Cmd
		for p := range cfg.ExposedPorts {
			d.ExposedPorts = append(d.ExposedPorts, string(p))
		}
		sort.Strings(d.ExposedPorts)
		if hc := cfg.Healthcheck; hc != nil && len(hc.Test) > 0 && hc.Test[0] != "NONE" {
			d.HasHealthcheck, d.Healthcheck = true, hc.Test
		}
	}
	return d, nil
}

// NodePlatform implements ImageDetailInspector.
func (c *Client) NodePlatform(ctx context.Context) (NodePlatform, error) {
	info, err := c.cli.Info(ctx)
	if err != nil {
		return NodePlatform{}, fmt.Errorf("docker: node info: %w", err)
	}
	return NodePlatform{Arch: NormalizeArch(info.Architecture), OS: info.OSType}, nil
}

// NormalizeArch maps the daemon's uname style architecture to the OCI name
// an image reports.
func NormalizeArch(a string) string {
	switch strings.ToLower(a) {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	case "armv7l", "armhf":
		return "arm"
	default:
		return strings.ToLower(a)
	}
}

// LifecycleEvent is one daemon event for a watched container.
type LifecycleEvent struct {
	// Action is create, start, die, oom, kill, stop or health_status.
	Action    string
	Health    string
	ExitCode  int
	HasExit   bool
	OOMKilled bool
	Time      time.Time
}

// LifecycleWatcher is an optional Runtime capability: the daemon's own event
// stream for one container, so a trace shows what the daemon actually did.
type LifecycleWatcher interface {
	WatchContainer(ctx context.Context, name string) (<-chan LifecycleEvent, <-chan error)
}

// WatchContainer implements LifecycleWatcher.
func (c *Client) WatchContainer(ctx context.Context, name string) (<-chan LifecycleEvent, <-chan error) {
	f := filters.NewArgs()
	f.Add("type", string(events.ContainerEventType))
	f.Add("container", name)
	raw, rawErrs := c.cli.Events(ctx, events.ListOptions{Filters: f, Since: strconv.FormatInt(time.Now().Unix(), 10)})

	out := make(chan LifecycleEvent, 32)
	outErr := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(outErr)
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-rawErrs:
				if ok && err != nil {
					outErr <- err
				}
				return
			case msg, ok := <-raw:
				if !ok {
					return
				}
				if ev, keep := lifecycleFrom(msg); keep {
					select {
					case out <- ev:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return out, outErr
}

func lifecycleFrom(msg events.Message) (LifecycleEvent, bool) {
	action := string(msg.Action)
	ev := LifecycleEvent{Action: action, Time: eventTime(msg)}
	switch {
	case strings.HasPrefix(action, "health_status"):
		ev.Action = "health_status"
		ev.Health = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(action, "health_status"), ":"))
	case action == "die":
		if code, err := strconv.Atoi(msg.Actor.Attributes["exitCode"]); err == nil {
			ev.ExitCode, ev.HasExit = code, true
		}
		ev.OOMKilled = msg.Actor.Attributes["oomKilled"] == "true"
	case action == "create", action == "start", action == "oom", action == "kill", action == "stop":
		ev.OOMKilled = action == "oom"
	default:
		return LifecycleEvent{}, false
	}
	return ev, true
}

// RegistryHost returns the registry domain a reference pulls from.
func RegistryHost(ref string) string {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return ""
	}
	return reference.Domain(named)
}
