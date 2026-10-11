package importer

import (
	"os"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/catalog"
	"github.com/GLINCKER/levelrail/internal/compose"
)

// The fixtures below are trimmed excerpts of Apache-2.0 upstream
// templates (attribution in internal/catalog/manifests/NOTICE).
const fixtureN8N = `# documentation: https://n8n.io
# slogan: n8n is an extendable workflow automation tool.
# category: automation
# port: 5678

services:
  n8n:
    image: n8nio/n8n:2.10.2
    environment:
      - SERVICE_URL_N8N_5678
      - N8N_EDITOR_BASE_URL=${SERVICE_URL_N8N}
      - N8N_HOST=${SERVICE_FQDN_N8N}
      - N8N_PROTOCOL=${N8N_PROTOCOL:-https}
      - N8N_RUNNERS_AUTH_TOKEN=${SERVICE_PASSWORD_N8N}
    volumes:
      - n8n-data:/home/node/.n8n
    healthcheck:
      test: ["CMD-SHELL", "wget -qO- http://127.0.0.1:5678/healthz"]
      interval: 5s
`

const fixtureGitea = `# documentation: https://docs.gitea.com
# slogan: Self-hosted Git service.
# category: git
# port: 3000

services:
  gitea:
    image: gitea/gitea:1.24
    environment:
      - SERVICE_URL_GITEA_3000
      - GITEA__database__NAME=${POSTGRESQL_DATABASE-gitea}
      - GITEA__database__USER=$SERVICE_USER_POSTGRESQL
      - GITEA__database__PASSWD=$SERVICE_PASSWORD_POSTGRESQL
    volumes:
      - gitea-data:/data
      - ./config:/etc/gitea
    depends_on:
      postgresql:
        condition: service_healthy
  postgresql:
    image: postgres:16-alpine
    environment:
      - POSTGRES_USER=${SERVICE_USER_POSTGRESQL}
      - POSTGRES_PASSWORD=${SERVICE_PASSWORD_POSTGRESQL}
      - POSTGRES_DB=${POSTGRESQL_DATABASE}
    volumes:
      - postgresql-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U $${POSTGRES_USER} -d $${POSTGRES_DB}"]
      interval: 5s
`

const fixtureRedis = `# documentation: https://redis.io
# slogan: In-memory store.
# category: database
# port: 8080

services:
  app:
    image: example/app:1.0
    environment:
      - SERVICE_URL_APP_8080
      - REDIS_URL=redis://:${SERVICE_PASSWORD_REDIS}@redis:6379
  redis:
    image: redis:7-alpine
    command: redis-server --requirepass ${SERVICE_PASSWORD_REDIS}
    healthcheck:
      test: ["CMD", "redis-cli", "-a", "${SERVICE_PASSWORD_REDIS}", "ping"]
`

func convertFixture(t *testing.T, id, src string) Output {
	t.Helper()
	return Convert(id, []byte(src), Options{SourcePath: "templates/compose/" + id + ".yaml", SourceURL: "https://example.test/upstream", License: "Apache-2.0"})
}

func parseOutCompose(t *testing.T, out Output) *compose.File {
	t.Helper()
	f, err := compose.Parse(out.Compose)
	if err != nil {
		t.Fatalf("emitted compose does not parse: %v\n%s", err, out.Compose)
	}
	return f
}

func TestConvert_MagicVariables(t *testing.T) {
	out := convertFixture(t, "n8n", fixtureN8N)
	if !out.Imported {
		t.Fatalf("skipped: %+v", out.Skip)
	}
	f := parseOutCompose(t, out)
	env := f.Services["n8n"].Environment
	tests := []struct{ key, want string }{
		{"N8N_EDITOR_BASE_URL", "${SERVICE_URL_N8N}"},
		{"N8N_HOST", "${SERVICE_HOST_N8N}"},
		{"N8N_PROTOCOL", "https"},
		{"N8N_RUNNERS_AUTH_TOKEN", "${N8N_PASSWORD}"},
	}
	for _, tt := range tests {
		if got := env[tt.key]; got != tt.want {
			t.Errorf("%s = %q, want %q", tt.key, got, tt.want)
		}
	}
	if _, ok := env["SERVICE_URL_N8N_5678"]; ok {
		t.Error("the address declaration should not survive as an env var")
	}
	m := out.Manifest
	if len(m.Domains) != 1 || m.Domains[0].Service != "n8n" || m.Domains[0].Port != 5678 {
		t.Errorf("domains = %+v", m.Domains)
	}
	if len(m.Env) != 1 || m.Env[0].Key != "N8N_PASSWORD" || m.Env[0].Generate != "password" || !m.Env[0].Secret {
		t.Errorf("env prompts = %+v", m.Env)
	}
	if m.Healthcheck == nil || m.Healthcheck.Port != 5678 {
		t.Errorf("healthcheck = %+v", m.Healthcheck)
	}
}

func TestConvert_SharedGeneratedSecretsAndPromotedDefaults(t *testing.T) {
	out := convertFixture(t, "gitea", fixtureGitea)
	if out.Skip != nil {
		t.Fatalf("skipped: %+v", out.Skip)
	}
	f := parseOutCompose(t, out)
	app := f.Services["gitea"].Environment
	db := f.Services["postgresql"].Environment
	if app["GITEA__database__PASSWD"] != "${POSTGRESQL_PASSWORD}" || db["POSTGRES_PASSWORD"] != "${POSTGRESQL_PASSWORD}" {
		t.Errorf("the generated password is not shared: app %q db %q", app["GITEA__database__PASSWD"], db["POSTGRES_PASSWORD"])
	}
	if app["GITEA__database__NAME"] != "${POSTGRESQL_DATABASE}" || db["POSTGRES_DB"] != "${POSTGRESQL_DATABASE}" {
		t.Errorf("a default and a bare use of one variable must share one setting: %q vs %q", app["GITEA__database__NAME"], db["POSTGRES_DB"])
	}
	var found bool
	for _, p := range out.Manifest.Env {
		if p.Key == "POSTGRESQL_DATABASE" {
			found = p.Default == "gitea"
		}
	}
	if !found {
		t.Errorf("POSTGRESQL_DATABASE should default to gitea: %+v", out.Manifest.Env)
	}
	test := f.Services["postgresql"].Healthcheck.Test
	if got := strings.Join(test, " "); !strings.Contains(got, "pg_isready -U $POSTGRES_USER -d $POSTGRES_DB") {
		t.Errorf("escaped shell variables lost: %q", got)
	}
}

func TestConvert_GeneratedSecretInCommandAndHealthcheck(t *testing.T) {
	out := convertFixture(t, "redisapp", fixtureRedis)
	if out.Skip != nil {
		t.Fatalf("skipped: %+v", out.Skip)
	}
	f := parseOutCompose(t, out)
	redis := f.Services["redis"]
	if got := strings.Join(redis.Command, " "); !strings.Contains(got, "--requirepass $GENERATED_REDIS_PASSWORD") {
		t.Errorf("command = %q", got)
	}
	if redis.Environment["GENERATED_REDIS_PASSWORD"] != "${REDIS_PASSWORD}" {
		t.Errorf("alias env = %v", redis.Environment)
	}
	hc := strings.Join(redis.Healthcheck.Test, " ")
	if !strings.HasPrefix(hc, "CMD-SHELL") || !strings.Contains(hc, `"$GENERATED_REDIS_PASSWORD"`) {
		t.Errorf("healthcheck = %q, want a shell form that expands the secret", hc)
	}
	if f.Services["app"].Environment["REDIS_URL"] != "redis://:${REDIS_PASSWORD}@redis:6379" {
		t.Errorf("REDIS_URL = %q", f.Services["app"].Environment["REDIS_URL"])
	}
}

func TestConvert_VolumesAndSkips(t *testing.T) {
	header := "# documentation: https://example.test\n# slogan: demo\n# category: tools\n# port: 80\n\n"
	tests := []struct {
		name       string
		body       string
		wantSkip   string
		wantVolume string
	}{
		{
			name:       "relative directory becomes a named volume",
			body:       "services:\n  web:\n    image: nginx:1.27\n    volumes:\n      - ./data/uploads:/var/www/uploads\n",
			wantVolume: "web-data-uploads:/var/www/uploads",
		},
		{
			name:     "relative file needs content",
			body:     "services:\n  web:\n    image: nginx:1.27\n    volumes:\n      - ./nginx.conf:/etc/nginx/nginx.conf:ro\n",
			wantSkip: ReasonBindNeedsContent,
		},
		{
			name:     "inline file content",
			body:     "services:\n  web:\n    image: nginx:1.27\n    volumes:\n      - type: bind\n        source: ./init\n        target: /docker-entrypoint-initdb.d\n        content: |\n          select 1;\n",
			wantSkip: ReasonBindNeedsContent,
		},
		{
			name:     "docker socket",
			body:     "services:\n  web:\n    image: nginx:1.27\n    volumes:\n      - /var/run/docker.sock:/var/run/docker.sock\n",
			wantSkip: ReasonHostBindMount,
		},
		{
			name:       "timezone host files are dropped",
			body:       "services:\n  web:\n    image: nginx:1.27\n    volumes:\n      - /etc/localtime:/etc/localtime:ro\n      - data:/data\n",
			wantVolume: "data:/data",
		},
		{
			name:     "privileged",
			body:     "services:\n  web:\n    image: nginx:1.27\n    privileged: true\n",
			wantSkip: ReasonHostAccess,
		},
		{
			name:     "udp port",
			body:     "services:\n  web:\n    image: nginx:1.27\n    ports:\n      - \"10000:10000/udp\"\n",
			wantSkip: ReasonUnsupportedProto,
		},
		{
			name:     "runtime key with no equivalent",
			body:     "services:\n  web:\n    image: nginx:1.27\n    user: \"1000:1000\"\n",
			wantSkip: ReasonUnsupportedKey,
		},
		{
			name:     "build from source",
			body:     "services:\n  web:\n    build: .\n",
			wantSkip: ReasonBuildFromSource,
		},
		{
			name:     "unknown magic variable without a default is a setting, address of nothing is not",
			body:     "services:\n  web:\n    image: nginx:1.27\n    environment:\n      - A=${SERVICE_URL_NOWHERE}\n    ports:\n      - \"80\"\n",
			wantSkip: ReasonUnknownDomainKey,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := convertFixture(t, "demo", header+tt.body)
			if tt.wantSkip != "" {
				if out.Skip == nil || out.Skip.Code != tt.wantSkip {
					t.Fatalf("skip = %+v, want code %s", out.Skip, tt.wantSkip)
				}
				return
			}
			if out.Skip != nil {
				t.Fatalf("skipped: %+v", out.Skip)
			}
			f := parseOutCompose(t, out)
			var got []string
			for _, v := range f.Services["web"].Volumes {
				got = append(got, v.Name+":"+v.ContainerPath)
			}
			if len(got) != 1 || got[0] != tt.wantVolume {
				t.Errorf("volumes = %v, want [%s]", got, tt.wantVolume)
			}
		})
	}
}

func TestConvert_ImageTagsAreRecorded(t *testing.T) {
	src := "# documentation: https://example.test\n# slogan: demo\n# category: tools\n# port: 80\n\nservices:\n  web:\n    image: ghcr.io/example/web\n    ports: [\"80\"]\n"
	out := convertFixture(t, "demo", src)
	if out.Skip != nil {
		t.Fatalf("skipped: %+v", out.Skip)
	}
	if len(out.Manifest.Images) != 1 || out.Manifest.Images[0].Image != "ghcr.io/example/web:latest" {
		t.Errorf("images = %+v", out.Manifest.Images)
	}
	var unpinned bool
	for _, n := range out.Notes {
		unpinned = unpinned || n.Code == NoteUnpinnedImage
	}
	if !unpinned {
		t.Error("a floating tag should be recorded as a note")
	}
	if out.Manifest.Source == nil || out.Manifest.Source.License != "Apache-2.0" {
		t.Errorf("source = %+v", out.Manifest.Source)
	}
}

func TestConvert_TCPHealthcheckBecomesHTTP(t *testing.T) {
	src := "# documentation: https://example.test\n# slogan: demo\n# category: tools\n# port: 9000\n\nservices:\n  web:\n    image: example/web:1\n    environment:\n      - SERVICE_URL_WEB_9000\n    healthcheck:\n      test: [\"CMD-SHELL\", \"bash -c ':> /dev/tcp/127.0.0.1/9000' || exit 1\"]\n"
	out := convertFixture(t, "demo", src)
	if out.Skip != nil {
		t.Fatalf("skipped: %+v", out.Skip)
	}
	f := parseOutCompose(t, out)
	got := strings.Join(f.Services["web"].Healthcheck.Test, " ")
	if !strings.Contains(got, "curl -fsS -o /dev/null http://127.0.0.1:9000/") {
		t.Errorf("healthcheck = %q", got)
	}
}

func TestConvert_RejectsBadIDsAndIgnored(t *testing.T) {
	if out := Convert("Bad_ID", []byte("services: {}"), Options{}); out.Skip == nil || out.Skip.Code != ReasonBadID {
		t.Errorf("bad id skip = %+v", out.Skip)
	}
	ignored := "# ignore: true\n# documentation: x\n# slogan: y\nservices:\n  a:\n    image: a:1\n"
	if out := Convert("a", []byte(ignored), Options{}); out.Skip == nil || out.Skip.Code != ReasonUpstreamIgnored {
		t.Errorf("ignored skip = %+v", out.Skip)
	}
}

// TestCommittedManifestsStillValidate loads every manifest on disk the
// way the control plane does and checks the imported ones keep their
// attribution and pinned images.
func TestCommittedManifestsStillValidate(t *testing.T) {
	root := "../manifests"
	ms, err := catalog.LoadManifests(os.DirFS(root), ".")
	if err != nil {
		t.Fatalf("LoadManifests() error = %v", err)
	}
	var imported int
	for _, m := range ms {
		if m.Source == nil {
			continue
		}
		imported++
		if m.Source.License != "Apache-2.0" || m.Source.Path == "" {
			t.Errorf("%s: source = %+v, want the upstream licence and path", m.ID, m.Source)
		}
		if len(m.Images) == 0 {
			t.Errorf("%s: records no pinned images", m.ID)
		}
		if _, err := m.Stack(); err != nil {
			t.Errorf("%s: Stack() error = %v", m.ID, err)
		}
	}
	if imported < 60 {
		t.Errorf("%d imported manifests committed, want at least 60", imported)
	}
	for _, name := range []string{"NOTICE", "LICENSE-APACHE-2.0"} {
		if _, err := os.Stat(root + "/" + name); err != nil {
			t.Errorf("attribution file %s missing: %v", name, err)
		}
	}
}

func TestPlainText(t *testing.T) {
	if got := plainText("Fast \u2014 simple \u2013 private "); got != "Fast, simple, private" {
		t.Errorf("plainText() = %q", got)
	}
}
