package provision

import (
	"strings"
	"testing"
)

func mirrorParams() CloudInitParams {
	return CloudInitParams{
		ControlPlaneAddr: "cp:8443", JoinToken: "t", CAFingerprint: "f", NodeName: "n",
		AgentName: "levelrail-agent", DisplayName: "Levelrail", AgentVersion: "v1.2.3",
	}
}

func TestRenderCloudInitRegistryMirror(t *testing.T) {
	p := mirrorParams()
	p.RegistryMirrorPrefix = "m.daocloud.io"
	out, err := RenderCloudInit(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "m.daocloud.io/ghcr.io/glincker/levelrail-agent:v1.2.3") {
		t.Errorf("mirrored image missing:\n%s", out)
	}
	p.AgentImage = "ghcr.io/x/y@sha256:abc"
	if got := p.agentImage(); got != p.AgentImage {
		t.Errorf("explicit image rewritten: %q", got)
	}
}

func TestRenderCloudInitDockerMirror(t *testing.T) {
	cases := []struct {
		mirror  string
		want    string
		wantErr bool
	}{
		{"", "(curl -fsSL https://get.docker.com | sh)", false},
		{"Aliyun", "sh -s -- --mirror Aliyun", false},
		{"AzureChinaCloud", "--mirror AzureChinaCloud", false},
		{"Evil; rm -rf /", "", true},
	}
	for _, c := range cases {
		t.Setenv("LEVELRAIL_DOCKER_MIRROR", "")
		p := mirrorParams()
		p.DockerMirror = c.mirror
		out, err := RenderCloudInit(p)
		if (err != nil) != c.wantErr {
			t.Fatalf("%q: err = %v", c.mirror, err)
		}
		if !c.wantErr && !strings.Contains(out, c.want) {
			t.Errorf("%q: %q not in output", c.mirror, c.want)
		}
	}
	t.Setenv("LEVELRAIL_DOCKER_MIRROR", "Aliyun")
	out, err := RenderCloudInit(mirrorParams())
	if err != nil || !strings.Contains(out, "--mirror Aliyun") {
		t.Errorf("env fallback failed: %v", err)
	}
}
