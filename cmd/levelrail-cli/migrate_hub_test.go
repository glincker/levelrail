package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMigrateHubSessions_ShowJSONAndDelete(t *testing.T) {
	srv, seen, bodies := fakeRouteServer(t, map[string]struct {
		status int
		body   string
	}{
		"GET /api/v1/migration/hub/sessions/hs_1":           {200, `{"id":"hs_1","engine":"postgres","host":"db.internal","port":5432,"step":"inventory","items":[{"source_db":"shop","selected":true,"status":"ready"}],"summary":{"databases":1,"selected":1}}`},
		"PUT /api/v1/migration/hub/sessions/hs_1/selection": {200, `{"id":"hs_1","items":[]}`},
		"DELETE /api/v1/migration/hub/sessions/hs_1":        {204, ``},
	})
	base := []string{"--api-url", srv.URL, "--token", "t"}

	code, out, errb := runCLIAuthMigrate(append([]string{"migrate", "hub", "sessions", "show", "hs_1", "--json"}, base...))
	if code != exitOK {
		t.Fatalf("show: code %d err %q", code, errb)
	}
	var got struct {
		ID    string `json:"id"`
		Items []struct {
			SourceDB string `json:"source_db"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.ID != "hs_1" || len(got.Items) != 1 || got.Items[0].SourceDB != "shop" {
		t.Fatalf("show --json = %q (%v)", out, err)
	}
	if code, _, errb = runCLIAuthMigrate(append([]string{"migrate", "hub", "sessions", "select", "hs_1", "--deselect", "shop", "--target-name", "shop=shop2"}, base...)); code != exitOK {
		t.Fatalf("select: code %d err %q", code, errb)
	}
	if code, _, errb = runCLIAuthMigrate(append([]string{"migrate", "hub", "sessions", "delete", "hs_1"}, base...)); code == exitOK || !strings.Contains(errb, "--yes") {
		t.Fatalf("delete without --yes off a terminal must refuse: code %d err %q", code, errb)
	}
	if code, _, errb = runCLIAuthMigrate(append([]string{"migrate", "hub", "sessions", "delete", "hs_1", "--yes"}, base...)); code != exitOK {
		t.Fatalf("delete: code %d err %q", code, errb)
	}
	want := "GET /api/v1/migration/hub/sessions/hs_1,PUT /api/v1/migration/hub/sessions/hs_1/selection,DELETE /api/v1/migration/hub/sessions/hs_1"
	if g := strings.Join(seen(), ","); g != want {
		t.Fatalf("requests = %s, want %s", g, want)
	}
	if b := bodies()[1]; b != `{"items":[{"source_db":"shop","selected":false,"target_name":"shop2"}]}`+"\n" && b != `{"items":[{"source_db":"shop","selected":false,"target_name":"shop2"}]}` {
		t.Fatalf("selection body = %q", b)
	}
}

func TestMigrateChecklist_StepDone(t *testing.T) {
	srv, seen, bodies := fakeRouteServer(t, map[string]struct {
		status int
		body   string
	}{
		"POST /api/v1/migration/checklists/web/steps/dns_switched": {200, `{"app":"web","progress":3,"total":6,"current":"old_stopped","steps":[]}`},
	})
	base := []string{"--api-url", srv.URL, "--token", "t"}

	if code, _, errb := runCLIAuthMigrate(append([]string{"migrate", "checklist", "step", "web", "dns_switched"}, base...)); code == exitOK || !strings.Contains(errb, "--done") {
		t.Fatalf("step without --done or --reset must refuse: code %d err %q", code, errb)
	}
	code, out, errb := runCLIAuthMigrate(append([]string{"migrate", "checklist", "step", "web", "dns_switched", "--done", "--note", "flipped"}, base...))
	if code != exitOK || !strings.Contains(out, "web: 3/6, next old_stopped") {
		t.Fatalf("step: code %d out %q err %q", code, out, errb)
	}
	if g := strings.Join(seen(), ","); g != "POST /api/v1/migration/checklists/web/steps/dns_switched" {
		t.Fatalf("requests = %s", g)
	}
	if b := bodies()[0]; !strings.Contains(b, `"action":"complete"`) || !strings.Contains(b, `"note":"flipped"`) {
		t.Fatalf("step body = %q", b)
	}
}

func TestMigrateHubSessions_ReceiptOutAndPasswordStdin(t *testing.T) {
	srv, seen, _ := fakeRouteServer(t, map[string]struct {
		status int
		body   string
	}{
		"GET /api/v1/migration/hub/sessions/hs_1/receipt": {200, `{"session_id":"hs_1","databases":[]}`},
	})
	base := []string{"--api-url", srv.URL, "--token", "t"}
	dir := t.TempDir()
	out := filepath.Join(dir, "r.json")

	if code, _, errb := runCLIAuthMigrate(append([]string{"migrate", "hub", "sessions", "receipt", "hs_1", "--out", out}, base...)); code != exitOK {
		t.Fatalf("receipt --out: code %d err %q", code, errb)
	}
	if st, err := os.Stat(out); err != nil || (runtime.GOOS != "windows" && st.Mode().Perm() != 0o600) {
		t.Fatalf("receipt file: %v %v", st, err)
	}
	if code, _, errb := runCLIAuthMigrate(append([]string{"migrate", "hub", "sessions", "receipt", "hs_1", "--out", out}, base...)); code == exitOK || !strings.Contains(errb, "--force") {
		t.Fatalf("existing file must be refused without --force: code %d err %q", code, errb)
	}
	if code, _, errb := runCLIAuthMigrate(append([]string{"migrate", "hub", "sessions", "receipt", "hs_1", "--out", out, "--force"}, base...)); code != exitOK {
		t.Fatalf("--force: code %d err %q", code, errb)
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(dir, "link.json")
		if err := os.Symlink(filepath.Join(dir, "elsewhere"), link); err != nil {
			t.Fatal(err)
		}
		if code, _, errb := runCLIAuthMigrate(append([]string{"migrate", "hub", "sessions", "receipt", "hs_1", "--out", link, "--force"}, base...)); code == exitOK || !strings.Contains(errb, "not a regular file") {
			t.Fatalf("symlink target must be refused: code %d err %q", code, errb)
		}
	}
	if code, _, errb := runCLIAuthMigrate(append([]string{"migrate", "hub", "sessions", "apply", "hs_1", "--password-stdin", "--yes"}, base...)); code == exitOK || !strings.Contains(errb, "empty password") {
		t.Fatalf("empty piped password must be refused: code %d err %q", code, errb)
	}
	for _, r := range seen() {
		if strings.Contains(r, "/apply") {
			t.Fatalf("apply must not be sent: %v", seen())
		}
	}
}
