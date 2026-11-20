package registry

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// consumerTokenModeNotes records how each reader of the built-in registry credential behaves in token mode.
// A new reader fails this test until it is classified here, so none is forgotten.
//
//nolint:gosec // classification notes, not credentials
var consumerTokenModeNotes = map[string]string{
	"cmd/levelrail-cli/registry.go":                "ok: reads settings through the API, no credential",
	"cmd/levelrail/boot_loaders.go":                "ok: cache ref only, the builder or Router attaches a cache scoped token",
	"cmd/levelrail/deploy_safety.go":               "ok: digest lookup mints a pull-only bearer for the built-in host in token mode",
	"cmd/levelrail/registry_transfer.go":           "ok: token exchange, pull and manifest check use minted bearers",
	"internal/api/apps_create.go":                  "ok: create-time digest lookup uses registryAuthFor, bearer in token mode",
	"internal/api/deploy_safety.go":                "ok: registryAuthFor mints a pull-only bearer for the built-in host in token mode",
	"internal/api/doctor_registry.go":              "ok: settings only, no credential",
	"internal/api/registry_catalog.go":             "ok: catalog and tag browse mint a bearer in token mode",
	"internal/api/registry_credentials.go":         "ok: host exemption check only, no credential use",
	"internal/api/registry_settings.go":            "ok: generates the password, which token mode ignores",
	"internal/api/standalone_domains_safety.go":    "ok: host comparison only",
	"internal/api/store_interfaces_settings.go":    "ok: interface declaration",
	"internal/apiclient/client.go":                 "ok: API client, no credential",
	"internal/build/client.go":                     "ok: option declaration, auth comes from the grant or CacheAuth",
	"internal/deploy/deploy.go":                    "ok: option plumbing, source classified under cmd/levelrail/deploy_safety.go",
	"internal/deploy/digest.go":                    "ok: interface, source classified under cmd/levelrail/deploy_safety.go",
	"internal/reconcile/application/controller.go": "ok: resolveRegistryAuth mints a pull-only bearer for the built-in host in token mode",
	"internal/reconcile/application/job_spec.go":   "ok: source classified under internal/reconcile/application/controller.go",
	"internal/reconcile/ingress/controller.go":     "ok: routes the registry host, no credential",
	"internal/reconcile/registry/controller.go":    "ok: switches the container between htpasswd and token auth",
}

var consumerPattern = regexp.MustCompile(`RegistryPasswordEnvKey|GetRegistrySettings\(|WithCacheRegistry\(|RegistryAuthSource|registryAuthFor\(|resolveRegistryAuth\(`)

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func TestRegistryCredentialConsumersAreClassified(t *testing.T) {
	root := moduleRoot(t)
	found := map[string]bool{}
	for _, top := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, "internal/store/") || rel == "internal/api/openapi_gen.go" {
				return nil
			}
			b, rerr := os.ReadFile(p) //nolint:gosec // repo source walked by a test
			if rerr == nil && consumerPattern.Match(b) {
				found[rel] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	var missing []string
	for f := range found {
		if _, ok := consumerTokenModeNotes[f]; !ok {
			missing = append(missing, f)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("new built-in registry credential consumers need a token mode decision in consumerTokenModeNotes: %v", missing)
	}
	for f := range consumerTokenModeNotes {
		if !found[f] {
			t.Errorf("consumerTokenModeNotes lists %s but it no longer reads the registry credential", f)
		}
	}
}
