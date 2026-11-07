package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/cutoververify"
)

func writeFixture(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func findingIDs(rep nextDoctorReport) map[string]string {
	m := map[string]string{}
	for _, f := range rep.Findings {
		m[f.ID] = f.Severity
	}
	return m
}

func TestNextDoctor_ReportsFindingsAndHidesValues(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "package.json", `{"dependencies":{"next":"^13.5.0","react":"18"}}`)
	writeFixture(t, dir, "next.config.js", "module.exports = {}\n")
	writeFixture(t, dir, "Dockerfile", "FROM node:20\nRUN npm run build\n")
	writeFixture(t, dir, ".env.production", "NEXT_PUBLIC_API_SECRET=hunter2value\nNEXT_PUBLIC_SITE=x\n")

	rep := checkNextProject(dir, 14, 1024)
	ids := findingIDs(rep)
	for id, sev := range map[string]string{
		"old-next":                             nextSevError,
		"no-standalone":                        nextSevWarn,
		"no-cache-handler":                     nextSevWarn,
		"no-deployment-id-arg":                 nextSevWarn,
		"no-sharp":                             nextSevWarn,
		"no-dockerignore":                      nextSevWarn,
		"public-secret-next_public_api_secret": nextSevError,
	} {
		if ids[id] != sev {
			t.Errorf("finding %s: got %q want %q (all: %v)", id, ids[id], sev, ids)
		}
	}
	raw, _ := json.Marshal(rep)
	if strings.Contains(string(raw), "hunter2value") {
		t.Fatal("report leaked an env value")
	}
}

func TestNextDoctor_CleanProjectExitsZero(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "package.json", `{"dependencies":{"next":"15.1.0","sharp":"0.33.0"}}`)
	writeFixture(t, dir, "next.config.mjs", "export default { output: 'standalone', cacheHandler: './c.js' }\n")
	writeFixture(t, dir, ".dockerignore", "node_modules\n.next\n")
	var out, errb bytes.Buffer
	code := runNextDoctor("lr", []string{dir, "--json"}, &out, &errb, func(string) (string, bool) { return "", false })
	if code != exitOK {
		t.Fatalf("code %d out %s err %s", code, out.String(), errb.String())
	}
	var rep nextDoctorReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil || len(rep.Findings) != 0 {
		t.Fatalf("expected no findings: %v %s", err, out.String())
	}
}

func TestNextDoctor_ErrorExitsOneAndLargeNodeModules(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "package.json", `{"dependencies":{"next":"15.0.0"}}`)
	if err := os.Mkdir(filepath.Join(dir, "node_modules"), 0o750); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(dir, "node_modules"), "big.bin", strings.Repeat("x", 4096))
	rep := checkNextProject(dir, 14, 1024)
	if findingIDs(rep)["large-node_modules"] != nextSevWarn {
		t.Fatalf("expected large node_modules finding: %v", findingIDs(rep))
	}

	empty := t.TempDir()
	var out, errb bytes.Buffer
	if code := runNextDoctor("lr", []string{empty}, &out, &errb, func(string) (string, bool) { return "", false }); code != exitCheckFailed {
		t.Fatalf("want exitCheckFailed, got %d", code)
	}
}

func TestAppsSkewView(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	v := buildSkewView("web", map[string]string{"NEXT_DEPLOYMENT_ID": "abcdef1234567890"}, "redis", now.Add(90*time.Second).Format(time.RFC3339), now)
	if !v.NextJS || v.HeldSecondsLeft != 90 {
		t.Fatalf("unexpected view %+v", v)
	}
	var out bytes.Buffer
	printSkew(&out, "lr", v)
	if !strings.Contains(out.String(), "abcdef123456") || strings.Contains(out.String(), "abcdef1234567890") {
		t.Fatalf("id should be shortened: %s", out.String())
	}
	out.Reset()
	printSkew(&out, "lr", buildSkewView("api", map[string]string{"PORT": "80"}, "", "", now))
	if !strings.Contains(out.String(), "does not look like a Next.js app") {
		t.Fatalf("got %s", out.String())
	}
}

type fakeVerifier struct{}

func (fakeVerifier) DomainVerify(_ context.Context, domain string, _ []string) (apiclient.DomainVerifyReport, error) {
	pass := cutoververify.Line{Verdict: "PASS"}
	rep := apiclient.DomainVerifyReport{Domain: domain}
	for _, id := range []string{"dns", "certificate", "response"} {
		l := pass
		l.ID = id
		if domain == "bad.example.com" && id == "dns" {
			l.Verdict, l.Next = "FAIL", "point the A record here"
		}
		rep.Lines = append(rep.Lines, l)
	}
	return rep, nil
}

func TestCheckDomainsLive(t *testing.T) {
	targets := []apiclient.DomainResource{{Domain: "b.example.com"}, {Domain: "bad.example.com"}, {Domain: "a.example.com"}}
	rows := checkDomainsLive(fakeVerifier{}, targets, 2, time.Second)
	if len(rows) != 3 || rows[0].Domain != "a.example.com" || !rows[0].Live {
		t.Fatalf("rows %+v", rows)
	}
	if rows[2].Live || rows[2].DNS != "fail" || rows[2].Next != "point the A record here" {
		t.Fatalf("bad row %+v", rows[2])
	}
}
