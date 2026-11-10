package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

const importFixtures = "../../internal/platformimport/testdata/"

func readImportFixture(t *testing.T, name string, v any) {
	t.Helper()
	raw, err := os.ReadFile(importFixtures + name) //nolint:gosec // fixed test fixture names
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatal(err)
	}
}

func TestCoolifyComposeBuildPackStagesComposeApp(t *testing.T) {
	var app coolifyApplication
	readImportFixture(t, "coolify/compose_app.json", &app)
	got := mapCoolifyApplication(app, []coolifyEnvVar{{Key: "TZ", Value: "UTC"}}, false)
	if got.Blocking || got.Compose == nil {
		t.Fatalf("blocking=%v compose=%v issues=%+v", got.Blocking, got.Compose != nil, got.Issues)
	}
	if len(got.Compose.Generated) == 0 || strings.Contains(got.Compose.Yaml, "SERVICE_PASSWORD") {
		t.Fatalf("generated %v, yaml still has a magic var", got.Compose.Generated)
	}
	if len(got.Service.Domains) != 2 {
		t.Fatalf("domains %v", got.Service.Domains)
	}
	out, _ := json.Marshal(got)
	if bytes.Contains(out, []byte("services:")) {
		t.Fatal("report JSON carries the compose text")
	}
	stageComposeApp(&got, t.TempDir())
	if got.CreateError != "" || got.Compose.StagedPath == "" {
		t.Fatalf("stage: %q", got.CreateError)
	}
	staged, err := os.ReadFile(got.Compose.StagedPath)
	if err != nil || strings.Contains(string(staged), "SERVICE_PASSWORD") || !strings.Contains(string(staged), "mysql-data") {
		t.Fatalf("staged file %v", err)
	}
}

func TestCoolifyComposeBuildPackBlockedStaysBlocked(t *testing.T) {
	app := coolifyApplication{Name: "root", BuildPack: "dockercompose",
		DockerComposeRaw: "services:\n  a:\n    image: busybox\n    privileged: true\n    volumes:\n      - /var/run/docker.sock:/var/run/docker.sock\n"}
	got := mapCoolifyApplication(app, nil, false)
	var reasons []string
	for _, is := range got.Issues {
		if is.Severity == issueBlocking {
			reasons = append(reasons, is.Detail)
		}
	}
	joined := strings.Join(reasons, "\n")
	if !got.Blocking || !strings.Contains(joined, "privileged") || !strings.Contains(joined, "Docker socket") {
		t.Fatalf("blocking=%v reasons=%v", got.Blocking, reasons)
	}
}

func TestCoolifyFileStoragesBecomeAppFiles(t *testing.T) {
	var st coolifyStorageList
	readImportFixture(t, "coolify/storages_files.json", &st)
	if files, _ := mapFileStorages(st, false, nil); len(files) != 0 {
		t.Fatalf("content copied without --include-secret-values: %+v", files)
	}
	files, issues := mapFileStorages(st, true, nil)
	if len(files) != 2 || files[0].Path != "/etc/nginx/conf.d/site.conf" || files[0].Secret || !files[1].Secret {
		t.Fatalf("files %+v", files)
	}
	if !bytes.Equal(files[0].Content, []byte(st.Files[0].Content)) {
		t.Fatal("content changed")
	}
	var refused bool
	for _, is := range issues {
		refused = refused || (is.Severity == issueDropped && strings.Contains(is.Detail, "parent segment"))
	}
	if !refused {
		t.Fatalf("traversal not refused: %+v", issues)
	}
	a := mappedApp{ServiceName: "web", Files: files[:1]}
	dir := t.TempDir()
	if err := writeAppFiles(dir, &a); err != nil {
		t.Fatal(err)
	}
	staged, err := os.ReadFile(filepath.Join(dir, "web.files", "etc", "nginx", "conf.d", "site.conf")) //nolint:gosec // path under the test temp dir
	if err != nil || !bytes.Equal(staged, files[0].Content) {
		t.Fatalf("staged %v", err)
	}
}

func TestDokployComposeStackMapsToComposeApp(t *testing.T) {
	c := platformimport.NewComposeApp("c1", "Shop Stack", "p", "services:\n  web:\n    image: nginx\n",
		[]platformimport.Env{{Key: "MODE", Value: "prod"}, {Key: "API_TOKEN", Value: "fake", Secret: true}}, []string{"https://user:pw@shop.example.test"})
	got := mapDokployCompose(c, false)
	if got.Blocking || got.Compose == nil || len(got.Service.EnvLiteral) != 0 || len(got.Service.EnvKeys) != 2 {
		t.Fatalf("values staged without the flag: %+v", got)
	}
	if len(got.Service.Domains) != 0 {
		t.Fatalf("userinfo host kept: %v", got.Service.Domains)
	}
	if got := mapDokployCompose(c, true); got.Service.EnvLiteral["MODE"] != "prod" || got.Service.EnvLiteral["API_TOKEN"] != "fake" {
		t.Fatalf("values missing with the flag: %d", len(got.Service.EnvLiteral))
	}
}

func TestMigrationFilesSurviveNestedAndMultilineValues(t *testing.T) {
	st := coolifyStorageList{Files: []coolifyFileStorage{{MountPath: "/a", Content: "x"}, {MountPath: "/a/b", Content: "y"}, {MountPath: "/a", Content: "z"}}}
	files, issues := mapFileStorages(st, true, nil)
	if len(files) != 1 || files[0].Path != "/a" || len(issues) != 2 {
		t.Fatalf("files %+v issues %+v", files, issues)
	}
	dir := t.TempDir()
	skipped, err := writeSecretsEnvFile(filepath.Join(dir, "s.env"), map[string]string{"OK": "v", "BAD": "v\nINJECTED=1"})
	if err != nil || len(skipped) != 1 || skipped[0] != "BAD" {
		t.Fatalf("skipped %v err %v", skipped, err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, "s.env")) //nolint:gosec // path under the test temp dir
	if strings.Contains(string(body), "INJECTED") {
		t.Fatal("a multi-line value forged an env line")
	}
}
