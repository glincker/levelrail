package compose

import (
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// TestFixtures_Validate table-drives real, realistic compose file
// fixtures (testdata/*.yaml) through Parse and Validate, the shape a
// pasted-file "apps deploy-compose" import actually sees: valid files
// must pass cleanly, the unsupported-keys file must fail with every
// unsupported key named, not just the first.
func TestFixtures_Validate(t *testing.T) {
	tests := []struct {
		file         string
		wantServices []string
		wantErrs     []string // every substring that must appear somewhere in Validate()'s error
	}{
		{
			file:         "web_db.yaml",
			wantServices: []string{"db", "web"},
		},
		{
			file:         "web_redis_worker.yaml",
			wantServices: []string{"redis", "web", "worker"},
		},
		{
			file: "unsupported_keys.yaml",
			wantErrs: []string{
				"deploy.restart_policy is not supported",
				`service "web": secrets: is not supported yet`,
				`service "web": configs: is not supported yet`,
				"top-level secrets: is not supported yet",
				"top-level configs: is not supported yet",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			data, err := os.ReadFile("testdata/" + tt.file)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			f, err := Parse(data)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			err = f.Validate()
			if len(tt.wantErrs) == 0 {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				got := sortedServiceNames(f)
				if !equalStrings(got, tt.wantServices) {
					t.Errorf("services = %v, want %v", got, tt.wantServices)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() error = nil, want errors containing %v", tt.wantErrs)
			}
			for _, want := range tt.wantErrs {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Validate() error = %v, want it to contain %q", err, want)
				}
			}
		})
	}
}

// TestFixtures_DeployReplicas confirms deploy.replicas: N in a real
// compose file validates cleanly and maps onto the right
// store.DesiredService.Replicas per service, including a service that
// omits deploy: entirely (0, resolved to store.DefaultReplicas later by
// SaveDesiredService).
func TestFixtures_DeployReplicas(t *testing.T) {
	data, err := os.ReadFile("testdata/deploy_replicas.yaml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	f, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if err := f.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}

	services, _, err := ToDesiredServices("app", f)
	if err != nil {
		t.Fatalf("ToDesiredServices() error = %v", err)
	}

	want := map[string]int{"app-web": 3, "app-worker": 1, "app-cache": 0}
	got := make(map[string]int, len(services))
	for _, svc := range services {
		got[svc.Name] = svc.Replicas
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("replicas by service = %+v, want %+v", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestFixtures_ExpandBuildService exercises the git-build dispatch path
// (internal/deploy.Pipeline.DeploySpec's own expandComposeServices) end
// to end against the web_db.yaml fixture: web's depends_on: [db] must
// survive translation into spec.Service.DependsOn keyed by the exact
// same service names DeploySpec will fan out under.
func TestFixtures_ExpandBuildService(t *testing.T) {
	sourceDir := t.TempDir()
	data, err := os.ReadFile("testdata/web_db.yaml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(sourceDir+"/docker-compose.yaml", data, 0o600); err != nil { //nolint:gosec // sourceDir is t.TempDir(), not user input
		t.Fatalf("write fixture into source dir: %v", err)
	}

	services, _, err := ExpandBuildService(spec.Service{
		Build: spec.Build{Type: spec.BuildCompose, Path: "docker-compose.yaml"},
	}, sourceDir)
	if err != nil {
		t.Fatalf("ExpandBuildService() error = %v", err)
	}

	web, ok := services["web"]
	if !ok {
		t.Fatal(`services["web"] missing`)
	}
	if len(web.DependsOn) != 1 || web.DependsOn[0] != "db" {
		t.Errorf("web.DependsOn = %v, want [db]", web.DependsOn)
	}
	if _, ok := services["db"]; !ok {
		t.Error(`services["db"] missing`)
	}
}
