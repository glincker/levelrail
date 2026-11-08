package docker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
)

func TestDetailFromInspect_AllowlistOnly(t *testing.T) {
	long := strings.Repeat("x", 1000)
	resp := container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID: "c1", Name: "/web-1", Image: "sha256:img", RestartCount: 3,
			Args: []string{"--password=topsecret-arg"},
			State: &container.State{Status: "running", Running: true, StartedAt: "2026-11-07T10:00:00Z", Health: &container.Health{
				Status: "unhealthy", FailingStreak: 2, Log: []*container.HealthcheckResult{{Output: "first\nlast " + long + "\n"}},
			}},
			HostConfig: &container.HostConfig{Resources: container.Resources{NanoCPUs: 5e8, Memory: 1 << 29}},
		},
		Config: &container.Config{
			Image: "img:v1", Env: []string{"DB_PASSWORD=topsecret-env"}, Cmd: []string{"run", "--token=topsecret-cmd"},
			Labels: map[string]string{"owner": "cp-1", "extra.label": "topsecret-label"},
		},
		NetworkSettings: &container.NetworkSettings{Networks: map[string]*network.EndpointSettings{
			"lr": {IPAddress: "10.0.0.2", Aliases: []string{"web"}},
		}},
		Mounts: []container.MountPoint{{Type: "volume", Name: "web-data", Source: "/var/lib/x", Destination: "/data", RW: false}},
	}
	d := detailFromInspect(resp, "owner")
	raw, _ := json.Marshal(d)
	if strings.Contains(string(raw), "topsecret") {
		t.Fatalf("detail leaks: %s", raw)
	}
	if d.InstanceID != "cp-1" || d.Name != "web-1" || d.RestartCount != 3 || d.NanoCPUs != 5e8 || d.StartedAt.IsZero() {
		t.Errorf("detail = %+v", d)
	}
	if d.Health == nil || len(d.Health.LastOutput) != HealthOutputMaxBytes || !strings.HasPrefix(d.Health.LastOutput, "last ") {
		t.Errorf("health = %+v", d.Health)
	}
	if len(d.Mounts) != 1 || d.Mounts[0].Source != "web-data" || !d.Mounts[0].ReadOnly || d.Networks[0].Aliases[0] != "web" {
		t.Errorf("mounts/networks = %+v %+v", d.Mounts, d.Networks)
	}
}
