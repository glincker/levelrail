package catalog

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/compose"
)

func TestTemplates_ComposeParsesAndValidates(t *testing.T) {
	seen := make(map[string]bool, len(Templates))
	for _, tpl := range Templates {
		tpl := tpl
		t.Run(tpl.ID, func(t *testing.T) {
			if tpl.ID == "" {
				t.Fatal("template has an empty ID")
			}
			if seen[tpl.ID] {
				t.Fatalf("duplicate template ID %q", tpl.ID)
			}
			seen[tpl.ID] = true

			if tpl.Name == "" {
				t.Errorf("template %q: empty Name", tpl.ID)
			}
			if tpl.Slogan == "" {
				t.Errorf("template %q: empty Slogan", tpl.ID)
			}
			if tpl.Category == "" {
				t.Errorf("template %q: empty Category", tpl.ID)
			}
			if tpl.DocumentationURL == "" {
				t.Errorf("template %q: empty DocumentationURL", tpl.ID)
			}

			if tpl.RecommendedMemoryBytes <= 0 {
				t.Errorf("template %q: RecommendedMemoryBytes must be set to a positive value", tpl.ID)
			}

			f, err := compose.Parse([]byte(tpl.Compose))
			if err != nil {
				t.Fatalf("template %q: compose.Parse() error = %v", tpl.ID, err)
			}
			if err := f.Validate(); err != nil {
				t.Fatalf("template %q: (*compose.File).Validate() error = %v", tpl.ID, err)
			}
			if _, _, err := compose.ToDesiredServices("test", f); err != nil {
				t.Fatalf("template %q: compose.ToDesiredServices() error = %v", tpl.ID, err)
			}

			if !anyServiceHasHealthcheck(f) {
				t.Errorf("template %q: no service declares a healthcheck: block", tpl.ID)
			}
		})
	}
}

func anyServiceHasHealthcheck(f *compose.File) bool {
	for _, svc := range f.Services {
		if svc.Healthcheck != nil && len(svc.Healthcheck.Test) > 0 {
			return true
		}
	}
	return false
}

func TestTemplates_MinimumCatalogSize(t *testing.T) {
	if len(Templates) < 180 {
		t.Fatalf("got %d templates, want at least 180", len(Templates))
	}
}

func templateByID(t *testing.T, id string) *compose.File {
	t.Helper()
	for _, tpl := range Templates {
		if tpl.ID != id {
			continue
		}
		f, err := compose.Parse([]byte(tpl.Compose))
		if err != nil {
			t.Fatalf("template %q: compose.Parse() error = %v", id, err)
		}
		return f
	}
	t.Fatalf("no template with ID %q", id)
	return nil
}

// TestTemplate_Keycloak_ProductionMode guards the #615 follow-up: Keycloak
// must boot in production mode (start, not start-dev) with a hostname the
// server accepts and proxy headers trusted from this platform's own Caddy
// ingress, which terminates TLS in front of the container.
func TestTemplate_Keycloak_ProductionMode(t *testing.T) {
	f := templateByID(t, "keycloak")
	svc, ok := f.Services["keycloak"]
	if !ok {
		t.Fatal(`no "keycloak" service in the keycloak template`)
	}

	if got, want := svc.Command, "start"; len(got) != 1 || got[0] != want {
		t.Errorf("command = %v, want [%q] (production mode, not start-dev)", got, want)
	}
	if got, want := svc.Environment["KC_HOSTNAME"], "${SERVICE_FQDN_KEYCLOAK:-http://localhost:8080}"; got != want {
		t.Errorf("KC_HOSTNAME = %q, want %q", got, want)
	}
	if got, want := svc.Environment["KC_PROXY_HEADERS"], "xforwarded"; got != want {
		t.Errorf("KC_PROXY_HEADERS = %q, want %q", got, want)
	}
	if got, want := svc.Environment["KC_LEGACY_OBSERVABILITY_INTERFACE"], "true"; got != want {
		t.Errorf("KC_LEGACY_OBSERVABILITY_INTERFACE = %q, want %q (still needed in start, not just start-dev)", got, want)
	}
}

// TestTemplate_Vikunja_ProductionDefaults guards the #615 follow-up:
// Vikunja's JWT secret must come from the platform's own generated-secret
// templating, never a static default, and its public URL should resolve
// to the assigned domain the same way the Outline and Pocket ID entries do.
func TestTemplate_Vikunja_ProductionDefaults(t *testing.T) {
	f := templateByID(t, "vikunja")
	svc, ok := f.Services["vikunja"]
	if !ok {
		t.Fatal(`no "vikunja" service in the vikunja template`)
	}

	if got, want := svc.Environment["VIKUNJA_SERVICE_JWTSECRET"], "$SERVICE_HEX_64_JWTSECRET"; got != want {
		t.Errorf("VIKUNJA_SERVICE_JWTSECRET = %q, want %q (a generated per-instance secret, not a static value)", got, want)
	}
	if got, want := svc.Environment["VIKUNJA_SERVICE_PUBLICURL"], "${SERVICE_FQDN_VIKUNJA:-http://localhost:3456}"; got != want {
		t.Errorf("VIKUNJA_SERVICE_PUBLICURL = %q, want %q", got, want)
	}

	for _, v := range svc.Volumes {
		if v.ContainerPath == "/app/vikunja/files" {
			if v.Name == "" {
				t.Error("vikunja files volume must be a named volume (persists across restarts), not a bind mount or anonymous volume")
			}
			return
		}
	}
	t.Fatal("vikunja service has no volume mounted at /app/vikunja/files")
}

// tcpOnlyTemplates keep an inert TCP healthcheck because no active probe
// could be confirmed for them; see the PR that converted the rest.
var tcpOnlyTemplates = map[string]bool{
	"libretranslate": true, // first boot downloads language models for many minutes
	"transmission":   true, // every HTTP path is behind the configured basic auth
	"invoice-ninja":  true, // the :5 image serves php-fpm, its HTTP front is unverified
	"grimmory":       true, // nightly image with no documented endpoint
	"databasus":      true, // no documented health endpoint
	"statusnook":     true, // no documented health endpoint
}

func TestTemplates_HealthchecksBecomeActiveProbes(t *testing.T) {
	for _, tpl := range Templates {
		t.Run(tpl.ID, func(t *testing.T) {
			f, err := compose.Parse([]byte(tpl.Compose))
			if err != nil {
				t.Fatalf("compose.Parse() error = %v", err)
			}
			services, warnings, err := compose.ToDesiredServices("test", f)
			if err != nil {
				t.Fatalf("compose.ToDesiredServices() error = %v", err)
			}
			if tcpOnlyTemplates[tpl.ID] {
				if len(warnings) == 0 {
					t.Errorf("listed as TCP-only but its healthcheck translated; drop it from tcpOnlyTemplates")
				}
				return
			}
			if len(warnings) != 0 {
				t.Fatalf("healthcheck not translated into an active probe: %v", warnings)
			}
			for _, svc := range services {
				if svc.Health == nil || svc.Health.Readiness == nil {
					continue
				}
				if err := svc.Health.Readiness.ProbeConfig().Validate(); err != nil {
					t.Errorf("service %q readiness probe invalid: %v", svc.Name, err)
				}
			}
		})
	}
}
