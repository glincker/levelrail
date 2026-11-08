package docker

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
)

// HealthOutputMaxBytes bounds the probe output line carried per container:
// it is part of the agent wire contract, not an operator tunable.
const HealthOutputMaxBytes = 256

// ErrDetailUnsupported means the runtime (or an older agent) cannot do a
// live detail inspect; callers fall back to their unavailable markers.
var ErrDetailUnsupported = errors.New("docker: live container inspect is not supported by this runtime")

// ErrDetailUnscoped refuses a detail query without a name prefix and an
// owner label, so a shared daemon never lists another tenant's containers.
var ErrDetailUnscoped = errors.New("docker: live container inspect needs a name prefix and an owner label")

// ContainerHealthDetail is the healthcheck summary of one container.
type ContainerHealthDetail struct {
	Status        string
	FailingStreak int
	LastOutput    string
}

// ContainerNetworkDetail is one attached network with its aliases.
type ContainerNetworkDetail struct {
	Name    string
	IP      string
	Aliases []string
}

// ContainerMountDetail is one live mount.
type ContainerMountDetail struct {
	Type     string
	Source   string
	Target   string
	ReadOnly bool
}

// ContainerDetail is an allowlist of inspect fields. It has no env,
// command or label map on purpose: nothing secret can be copied into it.
type ContainerDetail struct {
	ID           string
	Name         string
	Image        string
	ImageID      string
	RepoDigests  []string
	State        string
	Health       *ContainerHealthDetail
	RestartCount int
	StartedAt    time.Time
	FinishedAt   time.Time
	Ports        []PortBinding
	Networks     []ContainerNetworkDetail
	Mounts       []ContainerMountDetail
	NanoCPUs     int64
	MemoryBytes  int64
	// InstanceID is the owner label value, so callers can recheck scope.
	InstanceID string
}

// DetailQuery scopes a live inspect to one name prefix and one owner label.
type DetailQuery struct {
	NamePrefix string
	LabelKey   string
	LabelValue string
	Limit      int
	// IDs, when set, keeps only these containers, applied before Limit.
	IDs []string
}

// ContainerDetailInspector is optional, discovered by type assertion like
// StatsInspector, so existing runtime fakes need no new method.
type ContainerDetailInspector interface {
	InspectContainerDetails(ctx context.Context, q DetailQuery) ([]ContainerDetail, error)
}

var _ ContainerDetailInspector = (*Client)(nil)

// InspectContainerDetails implements ContainerDetailInspector. On a
// mid-list error it returns what it already read together with the error.
func (c *Client) InspectContainerDetails(ctx context.Context, q DetailQuery) ([]ContainerDetail, error) {
	if q.NamePrefix == "" || q.LabelKey == "" || q.LabelValue == "" {
		return nil, ErrDetailUnscoped
	}
	f := filters.NewArgs()
	f.Add("name", "^/"+regexp.QuoteMeta(q.NamePrefix))
	f.Add("label", q.LabelKey+"="+q.LabelValue)
	c.instanceLabelFilter(f)
	for _, id := range q.IDs {
		f.Add("id", id)
	}
	summaries, err := c.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: f})
	if err != nil {
		return nil, fmt.Errorf("docker: list containers for inspect: %w", err)
	}
	sort.Slice(summaries, func(i, j int) bool { return firstName(summaries[i].Names) < firstName(summaries[j].Names) })
	if q.Limit > 0 && len(summaries) > q.Limit {
		summaries = summaries[:q.Limit]
	}
	digests := map[string][]string{}
	out := make([]ContainerDetail, 0, len(summaries))
	for _, s := range summaries {
		resp, err := c.cli.ContainerInspect(ctx, s.ID)
		if err != nil {
			if cerrdefs.IsNotFound(err) {
				continue
			}
			return out, fmt.Errorf("docker: inspect %s: %w", s.ID, err)
		}
		d := detailFromInspect(resp, q.LabelKey)
		if d.InstanceID != q.LabelValue {
			continue
		}
		if rd, ok := digests[d.ImageID]; ok {
			d.RepoDigests = rd
		} else if d.ImageID != "" {
			if img, err := c.cli.ImageInspect(ctx, d.ImageID); err == nil {
				d.RepoDigests = img.RepoDigests
			}
			digests[d.ImageID] = d.RepoDigests
		}
		out = append(out, d)
	}
	return out, nil
}

func firstName(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return names[0]
}

// detailFromInspect copies only allowlisted fields out of a full inspect.
func detailFromInspect(resp container.InspectResponse, labelKey string) ContainerDetail {
	d := ContainerDetail{Ports: []PortBinding{}, Networks: []ContainerNetworkDetail{}, Mounts: []ContainerMountDetail{}}
	if b := resp.ContainerJSONBase; b != nil {
		d.ID, d.Name, d.ImageID, d.RestartCount = b.ID, strings.TrimPrefix(b.Name, "/"), b.Image, b.RestartCount
		if st := b.State; st != nil {
			d.State = st.Status
			d.StartedAt, _ = time.Parse(time.RFC3339Nano, st.StartedAt)
			if !st.Running {
				d.FinishedAt, _ = time.Parse(time.RFC3339Nano, st.FinishedAt)
			}
			if h := st.Health; h != nil {
				d.Health = &ContainerHealthDetail{Status: h.Status, FailingStreak: h.FailingStreak}
				if n := len(h.Log); n > 0 && h.Log[n-1] != nil {
					d.Health.LastOutput = lastLine(h.Log[n-1].Output, HealthOutputMaxBytes)
				}
			}
		}
		if hc := b.HostConfig; hc != nil {
			d.NanoCPUs, d.MemoryBytes = hc.NanoCPUs, hc.Memory
		}
	}
	if resp.Config != nil {
		d.Image = resp.Config.Image
		d.InstanceID = resp.Config.Labels[labelKey]
	}
	if ns := resp.NetworkSettings; ns != nil {
		for port, bindings := range ns.Ports {
			for _, pb := range bindings {
				hp, _ := strconv.Atoi(pb.HostPort)
				d.Ports = append(d.Ports, PortBinding{ContainerPort: port.Int(), HostPort: hp, HostIP: pb.HostIP, Protocol: port.Proto()})
			}
		}
		sort.Slice(d.Ports, func(i, j int) bool { return d.Ports[i].ContainerPort < d.Ports[j].ContainerPort })
		for name, ep := range ns.Networks {
			if ep == nil {
				continue
			}
			aliases := append([]string{}, ep.Aliases...)
			d.Networks = append(d.Networks, ContainerNetworkDetail{Name: name, IP: ep.IPAddress, Aliases: aliases})
		}
		sort.Slice(d.Networks, func(i, j int) bool { return d.Networks[i].Name < d.Networks[j].Name })
	}
	for _, m := range resp.Mounts {
		src := m.Source
		if m.Type == "volume" && m.Name != "" {
			src = m.Name
		}
		d.Mounts = append(d.Mounts, ContainerMountDetail{Type: string(m.Type), Source: src, Target: m.Destination, ReadOnly: !m.RW})
	}
	return d
}

// CutUTF8 cuts s to at most limit bytes without splitting a rune.
func CutUTF8(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	s = s[:limit]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// lastLine keeps the final non-empty line, cut to max bytes on a rune edge.
func lastLine(s string, limit int) string {
	s = strings.TrimRight(s, "\r\n\t ")
	if i := strings.LastIndexAny(s, "\r\n"); i >= 0 {
		s = s[i+1:]
	}
	return CutUTF8(s, limit)
}
