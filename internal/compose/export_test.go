package compose

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TestFromDesiredServices_NeverCapturesSecretValues is the load-bearing
// test for save-as-template: a service with a real-looking secret value
// anywhere on the source DesiredService must never have that value, or
// even its own env key spelled back unprefixed, appear literally in the
// exported document. store.DesiredService.SecretEnv/DatabaseEnv/VaultEnv
// never carry a value in the first place (only a name/reference), so
// this also guards against a future field addition accidentally
// starting to.
func TestFromDesiredServices_NeverCapturesSecretValues(t *testing.T) {
	const wantAbsent = "sk_live_super_secret_value_12345"
	services := []store.DesiredService{
		{
			Name:  "myapp-web",
			AppID: "myapp",
			Image: "myorg/web:1.0.0",
			Env:   map[string]string{"PLAIN_CONFIG": "ok"},
			SecretEnv: []store.SecretEnvRef{
				{Name: "API_KEY", Required: true},
				{Name: "PASSWORD_RESET_SALT", Required: false}, // starts with a generatable kind's own name on purpose
			},
			DatabaseEnv: map[string]store.DatabaseEnvRef{
				"DATABASE_URL": {Database: "main", Field: "url"},
			},
			VaultEnv: map[string]store.VaultEnvRef{
				"VAULT_TOKEN": {Path: "secret/data/myapp", Key: "token"},
			},
		},
	}

	out, err := FromDesiredServices("myapp", services)
	if err != nil {
		t.Fatalf("FromDesiredServices() error = %v", err)
	}

	if strings.Contains(out, wantAbsent) {
		t.Fatalf("exported compose contains a literal secret value:\n%s", out)
	}

	f, err := Parse([]byte(out))
	if err != nil {
		t.Fatalf("re-parse exported compose: %v\n%s", err, out)
	}
	env := f.Services["web"].Environment
	for _, key := range []string{"API_KEY", "PASSWORD_RESET_SALT", "DATABASE_URL", "VAULT_TOKEN"} {
		val, ok := env[key]
		if !ok {
			t.Errorf("exported env missing placeholder for %q", key)
			continue
		}
		if !strings.HasPrefix(val, "${SERVICE_SECRET_"+key) {
			t.Errorf("env[%q] = %q, want a ${SERVICE_SECRET_%s...} placeholder", key, val, key)
		}
	}
	if env["PLAIN_CONFIG"] != "ok" {
		t.Errorf("env[PLAIN_CONFIG] = %q, want the literal non-secret value preserved", env["PLAIN_CONFIG"])
	}

	// Every placeholder must resolve as "needs configuration", including
	// the one deliberately named to start with a generatable kind's own
	// prefix (PASSWORD_RESET_SALT): the SERVICE_SECRET_ wrapper must
	// shadow that, not let magicvars.go's own kind-sniffing treat it as
	// auto-generatable and silently fill in a random value instead of
	// stopping the deploy for operator input. failGenerate/failPersist
	// (resolve_test.go) fail this test outright if either is called,
	// proving none of the four placeholders was treated as generatable.
	_, unresolved, err := ResolveMagicVars(f, failGenerate(t), failPersist(t))
	if err != nil {
		t.Fatalf("ResolveMagicVars() error = %v", err)
	}
	if len(unresolved) != 4 {
		t.Fatalf("ResolveMagicVars() unresolved = %d, want 4 (one per captured secret/database/vault ref): %+v", len(unresolved), unresolved)
	}
}

func TestFromDesiredServices_RoundTripsOrdinaryFields(t *testing.T) {
	services := []store.DesiredService{
		{
			Name:       "myapp-web",
			AppID:      "myapp",
			Image:      "myorg/web:2.0.0",
			Port:       8080,
			Env:        map[string]string{"MODE": "prod"},
			Labels:     map[string]string{"team": "platform"},
			Command:    []string{"serve"},
			Entrypoint: []string{"/bin/web"},
			DependsOn:  []string{"db"},
			Replicas:   3,
			Volumes:    []store.ServiceVolume{{Name: "app-myapp-web-data", ContainerPath: "/data"}},
		},
		{
			Name:  "myapp-db",
			AppID: "myapp",
			Image: "postgres:16",
			Env:   map[string]string{"POSTGRES_DB": "app"},
		},
	}

	out, err := FromDesiredServices("myapp", services)
	if err != nil {
		t.Fatalf("FromDesiredServices() error = %v", err)
	}

	f, err := Parse([]byte(out))
	if err != nil {
		t.Fatalf("Parse(): %v\n%s", err, out)
	}
	got, warnings, err := ToDesiredServices("newapp", f)
	if err != nil {
		t.Fatalf("ToDesiredServices(): %v\n%s", err, out)
	}
	if len(warnings) != 0 {
		t.Errorf("ToDesiredServices() warnings = %v, want none", warnings)
	}
	if len(got) != 2 {
		t.Fatalf("ToDesiredServices() returned %d services, want 2", len(got))
	}

	byName := map[string]store.DesiredService{}
	for _, s := range got {
		byName[s.Name] = s
	}
	web, ok := byName["newapp-web"]
	if !ok {
		t.Fatalf("no newapp-web service in round-tripped result, got %+v", got)
	}
	if web.Image != "myorg/web:2.0.0" {
		t.Errorf("web.Image = %q, want myorg/web:2.0.0", web.Image)
	}
	if web.Port != 8080 {
		t.Errorf("web.Port = %d, want 8080", web.Port)
	}
	if web.Env["MODE"] != "prod" {
		t.Errorf("web.Env[MODE] = %q, want prod", web.Env["MODE"])
	}
	if web.Labels["team"] != "platform" {
		t.Errorf("web.Labels[team] = %q, want platform", web.Labels["team"])
	}
	if len(web.Command) != 1 || web.Command[0] != "serve" {
		t.Errorf("web.Command = %v, want [serve]", web.Command)
	}
	if len(web.DependsOn) != 1 || web.DependsOn[0] != "db" {
		t.Errorf("web.DependsOn = %v, want [db]", web.DependsOn)
	}
	if web.Replicas != 3 {
		t.Errorf("web.Replicas = %d, want 3", web.Replicas)
	}
	if len(web.Volumes) != 1 || web.Volumes[0].ContainerPath != "/data" {
		t.Errorf("web.Volumes = %+v, want one entry mounted at /data", web.Volumes)
	}
	if len(web.Domains) != 0 {
		t.Errorf("web.Domains = %v, want none (domains never carry over)", web.Domains)
	}
}

func TestFromDesiredServices_NoServices(t *testing.T) {
	if _, err := FromDesiredServices("myapp", nil); err == nil {
		t.Error("FromDesiredServices(nil) error = nil, want an error")
	}
}

func TestFromDesiredServices_GPUReservation(t *testing.T) {
	services := []store.DesiredService{
		{
			Name:      "myapp-worker",
			AppID:     "myapp",
			Image:     "myorg/worker:1.0.0",
			Resources: &store.ServiceResources{GPU: &store.ServiceGPU{Count: 2}},
		},
	}
	out, err := FromDesiredServices("myapp", services)
	if err != nil {
		t.Fatalf("FromDesiredServices() error = %v", err)
	}
	f, err := Parse([]byte(out))
	if err != nil {
		t.Fatalf("Parse(): %v\n%s", err, out)
	}
	got, _, err := ToDesiredServices("newapp", f)
	if err != nil {
		t.Fatalf("ToDesiredServices(): %v\n%s", err, out)
	}
	if len(got) != 1 || got[0].Resources == nil || got[0].Resources.GPU == nil {
		t.Fatalf("round-tripped service has no GPU reservation: %+v", got)
	}
	if got[0].Resources.GPU.Count != 2 {
		t.Errorf("GPU.Count = %d, want 2", got[0].Resources.GPU.Count)
	}
}
