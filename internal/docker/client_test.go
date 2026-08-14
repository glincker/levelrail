package docker

import (
	"reflect"
	"testing"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/mount"
)

func TestToDockerPorts(t *testing.T) {
	tests := []struct {
		name    string
		ports   []PortBinding
		wantErr bool
	}{
		{name: "empty", ports: nil},
		{
			name: "single port, auto host port",
			ports: []PortBinding{
				{ContainerPort: 3000},
			},
		},
		{
			name: "single port, explicit host port",
			ports: []PortBinding{
				{ContainerPort: 3000, HostPort: 8080},
			},
		},
		{
			name: "protocol defaults to tcp when empty",
			ports: []PortBinding{
				{ContainerPort: 53, Protocol: ""},
			},
		},
		{
			name: "udp respected when set",
			ports: []PortBinding{
				{ContainerPort: 53, Protocol: "udp"},
			},
		},
		{
			name: "invalid port number",
			ports: []PortBinding{
				{ContainerPort: -1},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exposed, bindings, err := toDockerPorts(tt.ports)
			if (err != nil) != tt.wantErr {
				t.Fatalf("toDockerPorts() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(tt.ports) == 0 {
				if exposed != nil || bindings != nil {
					t.Errorf("expected nil/nil for no ports, got %v / %v", exposed, bindings)
				}
				return
			}
			if len(exposed) != len(tt.ports) {
				t.Errorf("exposed has %d entries, want %d", len(exposed), len(tt.ports))
			}
			if len(bindings) != len(tt.ports) {
				t.Errorf("bindings has %d entries, want %d", len(bindings), len(tt.ports))
			}
		})
	}
}

func TestToDockerPorts_HostPortZeroMeansAutoAssign(t *testing.T) {
	_, bindings, err := toDockerPorts([]PortBinding{{ContainerPort: 3000, HostPort: 0}})
	if err != nil {
		t.Fatalf("toDockerPorts() error = %v", err)
	}
	for _, bs := range bindings {
		for _, b := range bs {
			if b.HostPort != "" {
				t.Errorf("HostPort = %q, want empty string (auto-assign) when spec HostPort is 0", b.HostPort)
			}
		}
	}
}

func TestToDockerPorts_HostPortSetIsPassedThrough(t *testing.T) {
	_, bindings, err := toDockerPorts([]PortBinding{{ContainerPort: 3000, HostPort: 8080}})
	if err != nil {
		t.Fatalf("toDockerPorts() error = %v", err)
	}
	found := false
	for _, bs := range bindings {
		for _, b := range bs {
			if b.HostPort == "8080" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected a binding with HostPort=8080")
	}
}

func TestToDockerEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want []string
	}{
		{name: "empty", env: nil, want: nil},
		{
			name: "single var",
			env:  map[string]string{"FOO": "bar"},
			want: []string{"FOO=bar"},
		},
		{
			name: "multiple vars, sorted for determinism",
			env:  map[string]string{"ZOO": "z", "AAA": "a"},
			want: []string{"AAA=a", "ZOO=z"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toDockerEnv(tt.env)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("toDockerEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToDockerMounts(t *testing.T) {
	tests := []struct {
		name    string
		volumes []VolumeMount
		want    []mount.Mount
	}{
		{name: "empty", volumes: nil, want: nil},
		{
			name:    "single volume",
			volumes: []VolumeMount{{Name: "db-main-data", ContainerPath: "/var/lib/postgresql/data"}},
			want: []mount.Mount{
				{Type: mount.TypeVolume, Source: "db-main-data", Target: "/var/lib/postgresql/data"},
			},
		},
		{
			name: "multiple volumes preserve order",
			volumes: []VolumeMount{
				{Name: "a", ContainerPath: "/a"},
				{Name: "b", ContainerPath: "/b"},
			},
			want: []mount.Mount{
				{Type: mount.TypeVolume, Source: "a", Target: "/a"},
				{Type: mount.TypeVolume, Source: "b", Target: "/b"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toDockerMounts(tt.volumes)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("toDockerMounts() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestToContainerState(t *testing.T) {
	tests := []struct {
		name    string
		summary dockertypes.Container
		want    ContainerState
	}{
		{
			name: "leading slash trimmed from name",
			summary: dockertypes.Container{
				ID: "abc123", Names: []string{"/web-a1b2c3d4"}, Image: "myapp:sha", State: "running",
			},
			want: ContainerState{ID: "abc123", Name: "web-a1b2c3d4", Image: "myapp:sha", Running: true},
		},
		{
			name:    "no names at all: empty name, not a panic",
			summary: dockertypes.Container{ID: "abc123", Names: nil, State: "exited"},
			want:    ContainerState{ID: "abc123", Name: "", Running: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toContainerState(tt.summary)
			if got.ID != tt.want.ID || got.Name != tt.want.Name || got.Image != tt.want.Image || got.Running != tt.want.Running {
				t.Errorf("toContainerState() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMapAction(t *testing.T) {
	tests := []struct {
		name string
		in   events.Action
		want EventAction
	}{
		{name: "start", in: "start", want: EventStart},
		{name: "die", in: "die", want: EventDie},
		{name: "stop", in: "stop", want: EventStop},
		{name: "unrecognized action mapped to empty, filtered out by caller", in: "exec_create", want: ""},
		{name: "empty input", in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mapAction(tt.in); got != tt.want {
				t.Errorf("mapAction(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestEventTime(t *testing.T) {
	tests := []struct {
		name string
		msg  events.Message
		want time.Time
	}{
		{
			name: "TimeNano set: preferred over Time",
			msg:  events.Message{Time: 1_700_000_000, TimeNano: 1_700_000_000_123_456_789},
			want: time.Unix(0, 1_700_000_000_123_456_789),
		},
		{
			name: "only Time set: falls back to whole-second precision",
			msg:  events.Message{Time: 1_700_000_000},
			want: time.Unix(1_700_000_000, 0),
		},
		{
			name: "neither set: zero value, not a fabricated time",
			msg:  events.Message{},
			want: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := eventTime(tt.msg); !got.Equal(tt.want) {
				t.Errorf("eventTime(%+v) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}

func TestObservedPorts(t *testing.T) {
	tests := []struct {
		name  string
		ports []dockertypes.Port
		want  []PortBinding
	}{
		{name: "no ports", ports: nil, want: nil},
		{
			name: "exposed but not published, skipped",
			ports: []dockertypes.Port{
				{PrivatePort: 3000, PublicPort: 0, Type: "tcp"},
			},
			want: nil,
		},
		{
			name: "published port kept",
			ports: []dockertypes.Port{
				{PrivatePort: 3000, PublicPort: 32768, Type: "tcp"},
			},
			want: []PortBinding{
				{ContainerPort: 3000, HostPort: 32768, Protocol: "tcp"},
			},
		},
		{
			name: "mix of published and unpublished",
			ports: []dockertypes.Port{
				{PrivatePort: 3000, PublicPort: 32768, Type: "tcp"},
				{PrivatePort: 9000, PublicPort: 0, Type: "tcp"},
			},
			want: []PortBinding{
				{ContainerPort: 3000, HostPort: 32768, Protocol: "tcp"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := observedPorts(tt.ports)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("observedPorts() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
