package configtranslate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVercelCronCommandHasNoSecretValue(t *testing.T) {
	res := Translate(map[string][]byte{"vercel.json": []byte(`{"crons":[{"path":"/api/tick","schedule":"0 * * * *"}]}`)})
	jobs := res.Fragment.Services["web"].Jobs
	if len(jobs) != 1 {
		t.Fatalf("jobs = %+v", res.Fragment.Services)
	}
	all := strings.Join(jobs[0].Command, " ")
	if !strings.Contains(all, "$CRON_SECRET") || strings.Contains(all, "s3cret") {
		t.Fatalf("command must reference the variable only: %q", all)
	}
	if jobs[0].Command[len(jobs[0].Command)-1] != "http://127.0.0.1:3000/api/tick" {
		t.Fatalf("url arg = %v", jobs[0].Command)
	}
}

// runCron runs the script with a fake curl that prints its arguments.
func runCron(t *testing.T, secret string) string {
	t.Helper()
	dir := t.TempDir()
	fake := filepath.Join(dir, "curl")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\necho \"$@\"\n"), 0o755); err != nil { //nolint:gosec // test stub
		t.Fatal(err)
	}
	cmd := exec.Command("sh", cronCommand(3000, "/api/tick")[1:]...) //nolint:gosec // fixed test script
	cmd.Env = []string{"PATH=" + dir + ":/usr/bin:/bin"}
	if secret != "" {
		cmd.Env = append(cmd.Env, "CRON_SECRET="+secret)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v: %s", err, out)
	}
	return string(out)
}

func TestVercelCronScriptSendsBearerOnlyWithSecret(t *testing.T) {
	if got := runCron(t, "s3cret"); !strings.Contains(got, "Authorization: Bearer s3cret") || !strings.Contains(got, "/api/tick") {
		t.Fatalf("with secret: %q", got)
	}
	if got := runCron(t, ""); strings.Contains(got, "Authorization") || !strings.Contains(got, "/api/tick") {
		t.Fatalf("without secret: %q", got)
	}
}

func TestVercelEdgeSourceWarning(t *testing.T) {
	files := map[string][]byte{
		"vercel.json":           []byte(`{}`),
		"app/api/og/route.ts":   []byte("export const runtime = 'edge'\n"),
		"middleware.ts":         []byte(`export const runtime = "edge";`),
		"app/api/node/route.ts": []byte("export const runtime = 'nodejs'\n"),
		"app/page.tsx":          []byte("export default function P() { return null }"),
	}
	res := Translate(files)
	var found *Finding
	for i := range res.Findings {
		if res.Findings[i].Code == "vercel.runtime.edge.source" {
			found = &res.Findings[i]
		}
	}
	if found == nil || found.Severity != SeverityWarn {
		t.Fatalf("findings = %+v", res.Findings)
	}
	if !strings.Contains(found.Subject, "middleware.ts") || !strings.Contains(found.Subject, "app/api/og/route.ts") || strings.Contains(found.Subject, "node/route") {
		t.Fatalf("subject = %q", found.Subject)
	}
}
