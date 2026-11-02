package appexport

import (
	"archive/tar"
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

const canary = "CANARY-s3cr3t-9f2c41"

func sample() Input {
	return Input{AppName: "shop", Services: []Service{
		{
			Key: "web", Image: "ghcr.io/acme/shop:1.4", Port: 3000, Domains: []string{"shop.example.com"},
			Env:       map[string]string{"NODE_ENV": "production", "LOG_LEVEL": "info"},
			SecretEnv: []string{"SESSION_SECRET", "API_TOKEN", "DATABASE_URL"},
			Refs:      map[string]string{"DB_PASS": "database db field password"}, //nolint:gosec // fixture, not a credential
			Volumes:   []Volume{{Name: "shop-uploads", ContainerPath: "/uploads"}},
			Health:    &Probe{Path: "/healthz", Interval: "10s", Timeout: "3s", Failures: 3},
			DependsOn: []string{"db"}, DependsOnHealthy: []string{"db"},
			Restart: "always", MemoryBytes: 536870912, NanoCPUs: 500000000,
		},
		{
			Key: "db", Image: "postgres:16.4", Port: 5432,
			Env:       map[string]string{"POSTGRES_DB": "app", "POSTGRES_USER": "app"},
			SecretEnv: []string{"SESSION_SECRET", "POSTGRES_PASSWORD"},
			Volumes:   []Volume{{Name: "shop-db", ContainerPath: "/var/lib/postgresql/data"}},
			Health:    &Probe{Exec: []string{"pg_isready", "-U", "app"}, Interval: "5s", Failures: 5},
		},
	}}
}

func TestRenderGoldenAndCanary(t *testing.T) {
	files, err := Render(sample())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.Contains(string(f.Content), canary) {
			t.Fatalf("%s leaks the canary secret", f.Name)
		}
		path := filepath.Join("testdata", f.Name+".golden")
		if *update {
			if err := os.MkdirAll("testdata", 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, f.Content, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(path) //nolint:gosec // fixed testdata path
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(want, f.Content) {
			t.Errorf("%s differs from golden:\n%s", f.Name, f.Content)
		}
	}
}

func TestRenderStableAndRoundTrip(t *testing.T) {
	a, _ := Render(sample())
	in := sample()
	in.Services[0], in.Services[1] = in.Services[1], in.Services[0]
	b, _ := Render(in)
	for i := range a {
		if !bytes.Equal(a[i].Content, b[i].Content) {
			t.Fatalf("%s depends on input order", a[i].Name)
		}
	}
	compose := string(a[2].Content)
	for _, want := range []string{"condition: service_healthy", "healthcheck:", `["CMD", "pg_isready"`, "mem_limit: 536870912", "cpus: 0.5", "x-domains"} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose lacks %q", want)
		}
	}
	env := string(a[0].Content)
	if !strings.Contains(env, "NODE_ENV=production") || !strings.Contains(env, "SESSION_SECRET=CHANGE_ME") {
		t.Errorf("env template wrong:\n%s", env)
	}
}

func TestWriteTarHasThreeFiles(t *testing.T) {
	files, _ := Render(sample())
	var buf bytes.Buffer
	if err := WriteTar(&buf, files); err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(&buf)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
	if strings.Join(names, ",") != ".env.example,README.md,docker-compose.yml" {
		t.Fatalf("names = %v", names)
	}
}

func TestRenderRejectsEmpty(t *testing.T) {
	if _, err := Render(Input{AppName: "x"}); err == nil {
		t.Fatal("want error")
	}
}

func TestRenderRejectsInjectedNames(t *testing.T) {
	for name, mut := range map[string]func(*Service){
		"env key":    func(s *Service) { s.Env = map[string]string{"A=1\n  evil: x": "v"} },
		"secret key": func(s *Service) { s.SecretEnv = []string{"BAD KEY"} },
		"service":    func(s *Service) { s.Key = "web\n  evil" },
		"depends":    func(s *Service) { s.DependsOn = []string{"db\nevil"} },
	} {
		svc := Service{Key: "web", Image: "a:1"}
		mut(&svc)
		if _, err := Render(Input{AppName: "x", Services: []Service{svc}}); !errors.Is(err, ErrInvalidName) {
			t.Errorf("%s: err = %v, want ErrInvalidName", name, err)
		}
	}
}

func TestHealthcheckUsesExecForm(t *testing.T) {
	files, err := Render(Input{AppName: "x", Services: []Service{{Key: "web", Image: "a:1", Port: 80, Health: &Probe{Path: "/h;touch /x"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(files[2].Content); !strings.Contains(got, `test: ["CMD", "curl", "-fsS", "http://localhost:80/h;touch /x"]`) {
		t.Fatalf("compose:\n%s", got)
	}
}

func TestDotenvQuoteBackslash(t *testing.T) {
	if got := dotenvQuote(`a b\`); got != `'a b\'` {
		t.Errorf("got %q", got)
	}
	if got := dotenvQuote(`it's\`); got != SecretPlaceholder {
		t.Errorf("got %q", got)
	}
}
