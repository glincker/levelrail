package catalog

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// The engine resolves only SERVICE_* variables, so any other ${...} left in an
// env value would reach the container as a literal string.
func TestMailMoxInstallLeavesNoUnresolvedEnv(t *testing.T) {
	tpl, ok := TemplateByID("mail-mox")
	if !ok || tpl.Manifest == nil {
		t.Fatal("mail-mox must be a manifest template")
	}
	body, err := tpl.Manifest.Render(InstallInput{Env: map[string]string{"MAIL_FIRST_DOMAIN": "example.com"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	f, err := compose.Parse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	stub := func(string, string, int) (string, error) { return "secret", nil }
	if _, unresolved, err := compose.ResolveMagicVars(f, stub, func(string, string, string) error { return nil }); err != nil || len(unresolved) > 0 {
		t.Fatalf("resolve: %v %v", err, unresolved)
	}
	for name, svc := range f.Services {
		for k, v := range svc.Environment {
			if strings.Contains(v, "${") {
				t.Errorf("%s env %s keeps %q", name, k, v)
			}
		}
	}
	cmd := strings.Join(f.Services["mox"].Command, "\n")
	if !strings.Contains(cmd, "mox localserve -initonly") {
		t.Error("mox start script must make its own self-signed certificate")
	}
	if _, ok := f.Services["mox-tls"]; ok || len(f.Services) != 1 {
		t.Errorf("mail must be one container, got %d services", len(f.Services))
	}
	if got := f.Services["mox"].Environment["MAIL_FIRST_DOMAIN"]; got != "example.com" {
		t.Fatalf("domain env = %q", got)
	}
	if _, err := tpl.Manifest.Render(InstallInput{}, false); err == nil {
		t.Fatal("install without a domain must fail")
	}
}
