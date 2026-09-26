package importplan

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type dirFiles struct{ root string }

func (d dirFiles) ReadFile(_ context.Context, _, _, path string) ([]byte, bool, error) {
	for _, name := range []string{path, path + ".txt"} {
		b, err := os.ReadFile(filepath.Join(d.root, name)) //nolint:gosec // test fixture path
		if err == nil {
			return b, true, nil
		}
	}
	return nil, false, nil
}

func TestTokenize(t *testing.T) {
	cases := []struct {
		in   string
		want []string
		err  bool
	}{
		{`a b  c`, []string{"a", "b", "c"}, false},
		{`a "b c" 'd e'`, []string{"a", "b c", "d e"}, false},
		{"docker run \\\n  -d \\\n  nginx", []string{"docker", "run", "-d", "nginx"}, false},
		{`-e A="x y" b`, []string{"-e", "A=x y", "b"}, false},
		{`echo $(rm -rf /) \$HOME`, []string{"echo", "$(rm", "-rf", "/)", "$HOME"}, false},
		{`"unterminated`, nil, true},
		{`a # comment`, []string{"a"}, false},
		{`a\ b`, []string{"a b"}, false},
		{`""`, []string{""}, false},
	}
	for _, c := range cases {
		got, err := Tokenize(c.in)
		if (err != nil) != c.err {
			t.Fatalf("%q: err = %v", c.in, err)
		}
		if !c.err && !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %q want %q", c.in, got, c.want)
		}
	}
}

func TestDockerRunRealWorld(t *testing.T) {
	cases := []struct {
		name, cmd string
		image     string
		port      int
		vols      int
		warnCodes []string
		warnText  string
	}{
		{"postgres", `docker run --name pg -e POSTGRES_PASSWORD=mysecret -p 5432:5432 -v pgdata:/var/lib/postgresql/data -d postgres:16`, "postgres:16", 5432, 1, nil, ""},
		{"redis", `docker run -d --name redis -p 6379:6379 redis:7-alpine redis-server --appendonly yes`, "redis:7-alpine", 6379, 0, nil, ""},
		{"nginx", `docker run -d -p 8080:80 -v /srv/site:/usr/share/nginx/html:ro nginx`, "nginx", 80, 1, []string{"bind_mount", "unpinned_tag"}, ""},
		{"minio", `docker run -p 9000:9000 -p 9001:9001 --name minio -e "MINIO_ROOT_USER=admin" -e "MINIO_ROOT_PASSWORD=s3cretpass" -v /mnt/data:/data quay.io/minio/minio server /data --console-address ":9001"`, "quay.io/minio/minio", 9000, 1, []string{"extra_ports", "bind_mount", "unpinned_tag"}, ""},
		{"portainer", `docker run -d -p 8000:8000 -p 9443:9443 --name portainer --restart=always -v /var/run/docker.sock:/var/run/docker.sock -v portainer_data:/data portainer/portainer-ce:latest`, "portainer/portainer-ce:latest", 8000, 2, []string{"docker_socket", "bind_mount"}, ""},
		{"n8n", `docker run -it --rm --name n8n -p 5678:5678 -v n8n_data:/home/node/.n8n docker.n8n.io/n8nio/n8n`, "docker.n8n.io/n8nio/n8n", 5678, 1, nil, ""},
		{"vaultwarden", `docker run -d --name vaultwarden -e SIGNUPS_ALLOWED=false -v /vw-data/:/data/ -p 80:80 vaultwarden/server:latest`, "vaultwarden/server:latest", 80, 1, []string{"bind_mount"}, ""},
		{"jellyfin", `docker run -d --name jellyfin --user 1000:1000 --net=host -v /config:/config -v /media:/media:ro --restart unless-stopped jellyfin/jellyfin`, "jellyfin/jellyfin", 8096, 2, []string{"unsupported_flag"}, "--network host"},
		{"watchtower", `docker run -d --name watchtower -v /var/run/docker.sock:/var/run/docker.sock containrrr/watchtower --interval 300`, "containrrr/watchtower", 0, 1, []string{"docker_socket"}, ""},
		{"multiline", "docker run -d \\\n  --name app \\\n  -p 3000:3000 \\\n  -e NODE_ENV=production \\\n  myorg/app:1.0", "myorg/app:1.0", 3000, 0, nil, ""},
		{"sudo", `sudo docker run -p 80:80 nginx:1.25`, "nginx:1.25", 80, 0, nil, ""},
		{"container run", `docker container run -p 80:80 nginx:1.25`, "nginx:1.25", 80, 0, nil, ""},
		{"privileged", `docker run --privileged --cap-add=SYS_ADMIN --device /dev/fuse -p 22:22 ghcr.io/o/x:1`, "ghcr.io/o/x:1", 22, 0, []string{"unsupported_flag"}, "--privileged"},
		{"pid host", `docker run --pid=host -p 9100:9100 prom/node-exporter:v1`, "prom/node-exporter:v1", 9100, 0, []string{"unsupported_flag"}, "--pid host"},
		{"memory cpus", `docker run -m 512m --cpus 1.5 -p 80:80 nginx:1`, "nginx:1", 80, 0, nil, ""},
		{"short attached", `docker run -p8080:80 -e FOO=bar nginx:1`, "nginx:1", 80, 0, nil, ""},
		{"ip bound port", `docker run -p 127.0.0.1:8080:80/tcp nginx:1`, "nginx:1", 80, 0, nil, ""},
		{"entrypoint", `docker run --entrypoint "/bin/sh -c" -p 80:80 alpine:3 'echo hi'`, "alpine:3", 80, 0, nil, ""},
		{"env from host", `docker run -e API_KEY -p 80:80 myorg/x:2`, "myorg/x:2", 80, 0, []string{"env_from_host"}, ""},
		{"env file", `docker run --env-file ./.env -p 80:80 myorg/x:2`, "myorg/x:2", 80, 0, []string{"env_file"}, ""},
		{"tmpfs ulimit", `docker run --tmpfs /tmp --ulimit nofile=1024 -p 80:80 myorg/x:2`, "myorg/x:2", 80, 0, []string{"unsupported_flag"}, "--tmpfs"},
		{"network custom", `docker run --network mynet -p 80:80 myorg/x:2`, "myorg/x:2", 80, 0, []string{"network"}, ""},
		{"gpus", `docker run --gpus all -p 11434:11434 ollama/ollama:0.3`, "ollama/ollama:0.3", 11434, 0, []string{"unsupported_flag"}, "--gpus"},
		{"labels", `docker run -l a=b --label c=d -p 80:80 myorg/x:2`, "myorg/x:2", 80, 0, []string{"unsupported_flag"}, "--label"},
		{"grafana", `docker run -d -p 3000:3000 --name=grafana -v grafana-storage:/var/lib/grafana grafana/grafana-oss:11.0.0`, "grafana/grafana-oss:11.0.0", 3000, 1, nil, ""},
		{"uptime kuma", `docker run -d --restart=always -p 3001:3001 -v uptime-kuma:/app/data --name uptime-kuma louislam/uptime-kuma:1`, "louislam/uptime-kuma:1", 3001, 1, nil, ""},
		{"digest pinned", `docker run -p 80:80 nginx@sha256:` + strings.Repeat("a", 64), "nginx@sha256:" + strings.Repeat("a", 64), 80, 0, nil, ""},
		{"no port known", `docker run -d myorg/worker:3`, "myorg/worker:3", 0, 0, []string{"port_unknown"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := Plan(context.Background(), Input{Text: c.cmd}, Deps{})
			if err != nil {
				t.Fatal(err)
			}
			if p.Source != SourceDockerRun {
				t.Fatalf("source = %s", p.Source)
			}
			s := p.Services[0]
			if s.Image != c.image || s.Port != c.port || len(s.Volumes) != c.vols {
				t.Errorf("image=%q port=%d vols=%d", s.Image, s.Port, len(s.Volumes))
			}
			codes := map[string]bool{}
			var all strings.Builder
			for _, w := range p.Warnings {
				codes[w.Code] = true
				all.WriteString(w.Message + "\n")
			}
			for _, w := range c.warnCodes {
				if !codes[w] {
					t.Errorf("missing warning %q in %v", w, p.Warnings)
				}
			}
			if c.warnText != "" && !strings.Contains(all.String(), c.warnText) {
				t.Errorf("warnings missing %q: %s", c.warnText, all.String())
			}
		})
	}
}

func TestDockerRunDetails(t *testing.T) {
	p, err := Plan(context.Background(), Input{Text: `docker run --name pg -e POSTGRES_PASSWORD=hunter2 -e TZ=UTC -e TOKEN -m 1g --cpus 0.5 --restart always -v pgdata:/data postgres:16`}, Deps{})
	if err != nil {
		t.Fatal(err)
	}
	s := p.Services[0]
	if s.MemoryBytes != 1<<30 || s.NanoCPUs != 500000000 || s.Restart != "always" || p.SuggestedName != "pg" {
		t.Errorf("unexpected: %+v", s)
	}
	if p.Deploy != DeployCompose {
		t.Errorf("deploy = %s", p.Deploy)
	}
	for _, e := range s.Env {
		switch e.Key {
		case "POSTGRES_PASSWORD":
			if e.Value != "" || !e.Secret || !e.Required {
				t.Errorf("secret default leaked: %+v", e)
			}
		case "TZ":
			if e.Value != "UTC" || e.Secret {
				t.Errorf("TZ: %+v", e)
			}
		}
	}
	if !reflect.DeepEqual(p.MissingRequiredEnv, []string{"POSTGRES_PASSWORD", "TOKEN"}) {
		t.Errorf("missing = %v", p.MissingRequiredEnv)
	}
	if strings.Contains(p.ComposeYAML, "hunter2") {
		t.Errorf("compose yaml echoes a secret default")
	}
	p2, _ := Plan(context.Background(), Input{Text: `docker run -e TOKEN -p 80:80 x:1`, Env: map[string]string{"TOKEN": "abc"}, Name: "mine", Port: 81}, Deps{})
	if len(p2.MissingRequiredEnv) != 0 || p2.SuggestedName != "mine" || p2.Services[0].Port != 81 {
		t.Errorf("overrides not applied: %+v", p2)
	}
}

func TestDockerRunErrors(t *testing.T) {
	for _, in := range []string{`docker run`, `docker run -p abc:80 x`, `docker run -e 1BAD=x img`, `docker run -v rel:relative img`, `docker run -m zz img`} {
		if _, err := Plan(context.Background(), Input{Text: in}, Deps{}); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in   string
		want SourceKind
		err  bool
	}{
		{"https://github.com/o/r", SourceRepo, false},
		{"github.com/o/r", SourceRepo, false},
		{"git@github.com:o/r.git", SourceRepo, false},
		{"https://gitea.example.com/o/r.git", SourceRepo, false},
		{"nginx", SourceImage, false},
		{"ghcr.io/o/app:1.2", SourceImage, false},
		{"docker run -p 80:80 nginx", SourceDockerRun, false},
		{"services:\n  a:\n    image: x\n", SourceCompose, false},
		{"FROM alpine\nRUN echo hi\n", SourceDockerfile, false},
		{"", "", true},
		{"hello world", "", true},
	}
	for _, c := range cases {
		got, err := Classify(Input{Text: c.in})
		if (err != nil) != c.err || got != c.want {
			t.Errorf("%q: got %q err %v", c.in, got, err)
		}
	}
}

func TestImagePlan(t *testing.T) {
	p, _ := Plan(context.Background(), Input{Text: "postgres"}, Deps{})
	if p.Services[0].Port != 5432 || len(p.Warnings) != 1 || p.Warnings[0].Code != "unpinned_tag" || p.Deploy != DeployApp {
		t.Errorf("%+v", p)
	}
	p, _ = Plan(context.Background(), Input{Text: "myorg/thing:1.0"}, Deps{})
	if p.Warnings[0].Code != "port_unknown" {
		t.Errorf("%+v", p.Warnings)
	}
}

func TestDockerfilePlan(t *testing.T) {
	b, _ := os.ReadFile("testdata/dockerfile/Dockerfile")
	p, err := Plan(context.Background(), Input{Text: string(b)}, Deps{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Source != SourceDockerfile || p.Services[0].Port != 8080 || p.Deploy != DeployNone {
		t.Errorf("%+v", p)
	}
	keys := map[string]string{}
	for _, e := range p.Services[0].Env {
		keys[e.Key] = e.Value
	}
	if keys["NODE_ENV"] != "production" || keys["LOG_LEVEL"] != "info" {
		t.Errorf("env = %v", keys)
	}
}

func TestParseEnvFile(t *testing.T) {
	b, _ := os.ReadFile("testdata/nextjs/.env.example")
	got := map[string]EnvVar{}
	for _, e := range ParseEnvFile(string(b), ".env.example") {
		got[e.Key] = e
	}
	check := func(k string, required, hasDef, secret bool, value string) {
		e := got[k]
		if e.Required != required || e.HasDefault != hasDef || e.Secret != secret || e.Value != value {
			t.Errorf("%s: %+v", k, e)
		}
	}
	check("DATABASE_URL", false, true, true, "")
	check("NEXTAUTH_SECRET", true, false, true, "")
	check("SENTRY_DSN", true, false, false, "")
	check("NEXT_PUBLIC_SITE", false, true, false, "https://example.com")
	check("API_TOKEN", true, false, true, "")
	check("DEBUG", false, true, false, "false")
}

func TestRepoPlans(t *testing.T) {
	cases := []struct {
		dir    string
		build  BuildMethod
		port   int
		deploy DeployKind
		health string
	}{
		{"nextjs", BuildRailpack, 3000, DeployBuild, "/"},
		{"dockerfile", BuildDockerfile, 8080, DeployBuild, ""},
		{"gomod", BuildRailpack, 8080, DeployBuild, "/"},
		{"static", BuildStatic, 0, DeployBuild, "/"},
		{"compose_repo", BuildImage, 80, DeployCompose, ""},
	}
	for _, c := range cases {
		t.Run(c.dir, func(t *testing.T) {
			p, err := Plan(context.Background(), Input{Text: "https://github.com/acme/" + c.dir}, Deps{Files: dirFiles{filepath.Join("testdata", c.dir)}})
			if err != nil {
				t.Fatal(err)
			}
			s := p.Services[0]
			if s.Build != c.build || s.Port != c.port || p.Deploy != c.deploy || s.HealthPath != c.health {
				t.Errorf("build=%s port=%d deploy=%s health=%q", s.Build, s.Port, p.Deploy, s.HealthPath)
			}
			if p.RepoURL != "https://github.com/acme/"+c.dir {
				t.Errorf("repo url %q", p.RepoURL)
			}
		})
	}
}

func TestRepoNextEnv(t *testing.T) {
	p, _ := Plan(context.Background(), Input{Text: "github.com/acme/nextjs"}, Deps{Files: dirFiles{"testdata/nextjs"}})
	want := []string{"API_TOKEN", "NEXTAUTH_SECRET", "SENTRY_DSN"}
	if !reflect.DeepEqual(p.MissingRequiredEnv, want) {
		t.Errorf("missing = %v", p.MissingRequiredEnv)
	}
}

func TestRepoComposeEnvAndInjection(t *testing.T) {
	p, _ := Plan(context.Background(), Input{Text: "https://github.com/acme/compose_repo"}, Deps{Files: dirFiles{"testdata/compose_repo"}})
	if !reflect.DeepEqual(p.MissingRequiredEnv, []string{"SECRET_KEY"}) {
		t.Errorf("missing = %v", p.MissingRequiredEnv)
	}
	if p.ComposeYAML == "" {
		t.Error("compose yaml empty")
	}
	b, _ := os.ReadFile("testdata/compose_repo/docker-compose.yml")
	p2, err := Plan(context.Background(), Input{Text: string(b), Env: map[string]string{"SECRET_KEY": "k"}}, Deps{})
	if err != nil || len(p2.MissingRequiredEnv) != 0 || !strings.Contains(p2.ComposeYAML, "SECRET_KEY: k") {
		t.Errorf("err=%v %+v\n%s", err, p2.MissingRequiredEnv, p2.ComposeYAML)
	}
}

func TestComposePasteInvalid(t *testing.T) {
	p, err := Plan(context.Background(), Input{Text: "services:\n  a:\n    build: .\n"}, Deps{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Deploy != DeployNone || len(p.Warnings) == 0 {
		t.Errorf("%+v", p)
	}
}

func TestRepoNoInspection(t *testing.T) {
	p, _ := Plan(context.Background(), Input{Text: "https://github.com/a/b"}, Deps{})
	if p.Deploy != DeployNone || p.Warnings[0].Code != "no_inspection" {
		t.Errorf("%+v", p)
	}
	if _, err := Plan(context.Background(), Input{Text: "file:///etc/passwd", Kind: SourceRepo}, Deps{}); err == nil {
		t.Error("file scheme must be rejected")
	}
}

func TestRawURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/o/r":        "https://raw.githubusercontent.com/o/r/HEAD/Dockerfile",
		"https://gitlab.com/g/s/r":      "https://gitlab.com/g/s/r/-/raw/HEAD/Dockerfile",
		"https://bitbucket.org/o/r":     "https://bitbucket.org/o/r/raw/HEAD/Dockerfile",
		"https://gitea.example.com/o/r": "https://gitea.example.com/o/r/raw/HEAD/Dockerfile",
	}
	for in, want := range cases {
		got, err := rawURL(in, "", "Dockerfile")
		if err != nil || got != want {
			t.Errorf("%s: %s %v", in, got, err)
		}
	}
}

func TestRepoParts(t *testing.T) {
	u, repo, err := repoParts("https://gitlab.com/grp/sub/proj.git")
	if err != nil || u != "https://gitlab.com/grp/sub/proj" || repo != "proj" {
		t.Errorf("%s %s %v", u, repo, err)
	}
	if _, _, err := repoParts("https://github.com/onlyowner"); err == nil {
		t.Error("expected error")
	}
}
