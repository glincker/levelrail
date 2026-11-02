package appexport

import (
	"errors"
	"strings"
	"testing"
)

const projCanary = "CANARY-proj-5e1d90"

func projectSample() ProjectInput {
	return ProjectInput{Name: "Shop Stack", Databases: []string{"shop-db"}, Services: []Service{
		{
			Key: "shop-web", Image: "acme/web:1", Port: 3000, Domains: []string{"shop.example.com"},
			Env:          map[string]string{"NODE_ENV": "production", "SHARED": "a"},
			SecretEnv:    []string{"API_TOKEN"},
			SecretValues: map[string]string{"API_TOKEN": projCanary},
			Refs:         map[string]string{"DB_PASS": "database shop-db field password"},
			Volumes:      []Volume{{Name: "shop-web-data", ContainerPath: "/data"}},
			Health:       &Probe{Path: "/healthz"},
			DependsOn:    []string{"shop-api", "elsewhere-cache"},
			Unsupported:  []string{"GPU request (count 1)\nnot exported"},
		},
		{
			Key: "shop-api", Image: "acme/api:1", Port: 8080,
			Env:        map[string]string{"SHARED": "b"},
			SecretEnv:  []string{"API_TOKEN"},
			FilePaths:  []string{"/etc/api.toml"},
			BindMounts: []BindMount{{HostPath: "/srv/x", ContainerPath: "/x"}},
		},
	}}
}

func TestRenderProjectPlaceholdersByDefault(t *testing.T) {
	files, err := RenderProject(projectSample())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, f := range files {
		got[f.Name] = string(f.Content)
		if strings.Contains(string(f.Content), projCanary) {
			t.Fatalf("%s leaks a secret by default", f.Name)
		}
	}
	if _, ok := got[".env"]; ok {
		t.Fatal(".env must not exist without include secrets")
	}
	compose := got["docker-compose.yml"]
	for _, want := range []string{
		"# Not carried over", "shop-api: no readiness probe",
		"depends_on elsewhere-cache is outside this export", "managed databases are not exported",
		"managed config files", "GPU request (count 1) not exported", "x-domains",
		"Exported from Levelrail project",
	} {
		if !strings.Contains(compose, want) {
			t.Fatalf("compose is missing %q:\n%s", want, compose)
		}
	}
	if strings.Contains(compose, "elsewhere-cache:") {
		t.Fatal("dangling depends_on must be removed")
	}
	if !strings.Contains(got[".env.example"], "API_TOKEN=CHANGE_ME") && !strings.Contains(got[".env.example"], "SHOP_WEB_API_TOKEN=CHANGE_ME") {
		t.Fatalf("secret placeholder missing:\n%s", got[".env.example"])
	}
	if !strings.Contains(got["volumes.json"], `"compose_volume": "shop-stack_shop_web_data"`) {
		t.Fatalf("volume manifest wrong:\n%s", got["volumes.json"])
	}
}

func TestRenderProjectIncludeSecrets(t *testing.T) {
	in := projectSample()
	in.IncludeSecrets = true
	files, err := RenderProject(in)
	if err != nil {
		t.Fatal(err)
	}
	var env string
	for _, f := range files {
		if f.Name == ".env" {
			env = string(f.Content)
		} else if strings.Contains(string(f.Content), projCanary) {
			t.Fatalf("%s must stay placeholder-only", f.Name)
		}
	}
	if !strings.Contains(env, projCanary) {
		t.Fatalf(".env should hold the opted-in value:\n%s", env)
	}
}

func TestRenderProjectStableAndValidated(t *testing.T) {
	a, _ := RenderProject(projectSample())
	b, _ := RenderProject(projectSample())
	for i := range a {
		if string(a[i].Content) != string(b[i].Content) {
			t.Fatalf("%s is not byte-stable", a[i].Name)
		}
	}
	bad := projectSample()
	bad.Services[0].Key = "bad key"
	if _, err := RenderProject(bad); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("want ErrInvalidName, got %v", err)
	}
	if _, err := RenderProject(ProjectInput{Name: "x"}); err == nil {
		t.Fatal("empty project must be refused")
	}
}

func TestRenderProject_SkipsImagelessAppAndListsIt(t *testing.T) {
	t.Parallel()
	files, err := RenderProject(ProjectInput{Name: "shop", Services: []Service{
		{Key: "web", Image: "acme/web:1", Port: 80, Unsupported: []string{"SECURITY: egress"}},
		{Key: "draft"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var compose string
	for _, f := range files {
		if f.Name == "docker-compose.yml" {
			compose = string(f.Content)
		}
	}
	if !strings.Contains(compose, "draft: skipped") || strings.Contains(compose, "Nothing was left out") {
		t.Fatalf("header does not list the skipped app:\n%s", compose)
	}
}
