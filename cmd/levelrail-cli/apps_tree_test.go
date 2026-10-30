package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestBuildAppTreeRenderAndJSON(t *testing.T) {
	in := appTreeInput{
		App: apiclient.AppResource{
			Name: "web", Image: "ghcr.io/x/web:1.2", Port: 8080, Replicas: 2,
			Domains: []string{"web.example.com"}, Env: map[string]string{"A": "supersecret"}, SecretEnv: []string{"B", "A"},
			Volumes: []apiclient.AppVolumeResource{{Name: "data", ContainerPath: "/data"}},
		},
		Status:    apiclient.AppStatus{Conditions: []apiclient.ConditionResource{{Type: "Ready", Status: "True"}}},
		Live:      &apiclient.DeploymentResource{ID: "d2", Status: "succeeded", Image: "web:1.2", ImageDigest: "sha256:0123456789abcdef0123"},
		Last:      &apiclient.DeploymentResource{ID: "d3", Status: "failed", Image: "web:1.3"},
		Databases: []apiclient.DatabaseLink{{App: "pg", EnvVar: "DATABASE_URL"}},
	}
	tr := buildAppTree(in)
	got := renderAppTree(tr)
	want := "web (ghcr.io/x/web:1.2, port 8080)\n" +
		"|-- deployment: d2 web:1.2 [succeeded] @0123456789ab\n" +
		"|-- last deploy: d3 web:1.3 [failed]\n" +
		"|-- replicas: 2\n|   |-- Ready: True\n" +
		"|-- domains: 1\n|   |-- web.example.com\n" +
		"|-- volumes: 1\n|   |-- data -> /data\n" +
		"|-- databases: 1\n|   |-- pg (DATABASE_URL)\n" +
		"`-- env: 2 keys\n"
	if got != want {
		t.Fatalf("render mismatch:\n%s\nwant:\n%s", got, want)
	}
	raw, err := json.Marshal(tr)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "supersecret") || !strings.Contains(string(raw), `"env_count":2`) || !strings.Contains(string(raw), `"digest":"0123456789ab"`) {
		t.Fatalf("bad json: %s", raw)
	}
}
