package catalog

import (
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/store"
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

func renderMailServices(t *testing.T) []store.DesiredService {
	t.Helper()
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
	stub := func(string, string, int) (string, error) { return "mjtest-generated-pw-1234", nil }
	if _, _, err := compose.ResolveMagicVars(f, stub, func(string, string, string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	svcs, _, err := compose.ToDesiredServices("mjtest-mail", f)
	if err != nil || len(svcs) != 1 {
		t.Fatalf("desired services: %v %d", err, len(svcs))
	}
	return svcs
}

// The routed web listener answers 403 on /admin/ by design, so the derived
// readiness probe must target a path that listener serves.
func TestMailMoxDerivedReadinessProbe(t *testing.T) {
	svc := renderMailServices(t)[0]
	if svc.Health == nil || svc.Health.Readiness == nil {
		t.Fatal("mail must derive a readiness probe")
	}
	p := svc.Health.Readiness
	if svc.Port != 8081 {
		t.Errorf("routed port = %d, want the web listener 8081", svc.Port)
	}
	if !p.Platform || p.Path != "/webmail/" || len(p.Exec) != 0 || p.TCP {
		t.Errorf("probe = %+v, want a platform http probe on /webmail/", p)
	}
	if p.ExpectedStatus != "" && p.ExpectedStatus != "200-299" {
		t.Errorf("expected status = %q, want 2xx", p.ExpectedStatus)
	}
}

// Opt-in: MAIL_LIVE_DOCKER=1 starts the resolved image and requires the
// derived probe path to answer 2xx on the real web listener.
func TestLiveMailMoxReadinessProbe(t *testing.T) {
	if os.Getenv("MAIL_LIVE_DOCKER") != "1" {
		t.Skip("set MAIL_LIVE_DOCKER=1 to start the mail image and probe it")
	}
	svc := renderMailServices(t)[0]
	name := "mjtest-readiness"
	args := []string{"run", "-d", "--name", name, "-p", "127.0.0.1::8081", "--entrypoint", svc.Entrypoint[0]}
	env := map[string]string{"MAIL_ADMIN_PASSWORD": "mjtest-generated-pw-1234"}
	for k, v := range svc.Env {
		env[k] = v
	}
	for k, v := range env {
		if v != "" {
			args = append(args, "-e", k+"="+v)
		}
	}
	args = append(args, svc.Image)
	args = append(args, svc.Entrypoint[1:]...)
	args = append(args, svc.Command...)
	_ = exec.Command("docker", "rm", "-f", name).Run()
	if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil { //nolint:gosec // opt-in test, args built from the template
		t.Fatalf("docker run: %v %s", err, out)
	}
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := exec.Command("docker", "logs", "--tail", "15", name).CombinedOutput()
			t.Logf("container logs:\n%s", logs)
		}
		_ = exec.Command("docker", "rm", "-f", name).Run()
	})
	out, err := exec.Command("docker", "port", name, "8081/tcp").Output()
	if err != nil {
		t.Fatal(err)
	}
	url := "http://" + strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0]) + svc.Health.Readiness.Path
	deadline := time.Now().Add(40 * time.Second)
	for {
		resp, err := http.Get(url) //nolint:gosec // loopback URL from docker port
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode/100 == 2 {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("probe %s never answered 2xx (last err %v)", url, err)
		}
		time.Sleep(2 * time.Second)
	}
}
