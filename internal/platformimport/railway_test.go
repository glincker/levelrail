package platformimport

import (
	"os"
	"strings"
	"testing"
)

func railwayFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/railway/" + name) //nolint:gosec // fixed fixture names
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRailwayJSONMapsDeploySettings(t *testing.T) {
	in, err := RailwayToEmit(railwayFixture(t, "railway.json"), "shop", "railway.json")
	if err != nil {
		t.Fatal(err)
	}
	a := in.Apps[0]
	if a.HealthPath != "/healthz" || a.Replicas != 2 || a.PreDeploy != "npm run migrate" {
		t.Fatalf("app %+v", a)
	}
	if len(a.Command) != 3 || a.Command[2] != "node dist/server.js" {
		t.Fatalf("command %v", a.Command)
	}
	notes := strings.Join(in.Notes, "\n")
	for _, want := range []string{"build.builder RAILPACK", "build.buildCommand", "build.watchPatterns has no equivalent", "deploy.region has no equivalent", "restartPolicyType ON_FAILURE (max retries 5)", "healthcheckTimeout 60s"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes missing %q:\n%s", want, notes)
		}
	}
}

func TestRailwayTOMLCronBecomesJobAndKeepsNamesOnly(t *testing.T) {
	in, err := RailwayToEmit(railwayFixture(t, "railway.toml"), "report", "railway.toml")
	if err != nil {
		t.Fatal(err)
	}
	a := in.Apps[0]
	if len(a.Jobs) != 1 || a.Jobs[0].Schedule != "0 3 * * *" || a.Command != nil {
		t.Fatalf("jobs %+v command %v", a.Jobs, a.Command)
	}
	if len(a.Secrets) != 1 || a.Secrets[0].Name != "REPORT_TOKEN" || len(a.Env) != 0 {
		t.Fatalf("secrets %+v env %v", a.Secrets, a.Env)
	}
	all := strings.Join(in.Notes, "\n") + a.Secrets[0].Hint
	if strings.Contains(all, "not-a-real-token") {
		t.Fatal("a variable value leaked into the output")
	}
	if !strings.Contains(all, "build.dockerfilePath docker/Dockerfile.report") {
		t.Fatalf("notes:\n%s", all)
	}
}

func TestRailwayRejectsBadInput(t *testing.T) {
	cases := map[string]string{
		"bad json":         `{"deploy": `,
		"cron no command":  `{"deploy":{"cronSchedule":"* * * * *"}}`,
		"deploy not table": `{"deploy": "x"}`,
	}
	for name, body := range cases {
		if _, err := RailwayToEmit([]byte(body), "x", "railway.json"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	if _, err := RailwayToEmit([]byte("[[deploy]]\n"), "x", "railway.toml"); err == nil {
		t.Error("arrays of tables should be rejected")
	}
}
