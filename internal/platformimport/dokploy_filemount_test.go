package platformimport

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
)

func loadFileMountApp(t *testing.T) dokployApp {
	t.Helper()
	raw, err := os.ReadFile("testdata/dokploy/app_filemount.json")
	if err != nil {
		t.Fatal(err)
	}
	var a dokployApp
	if err := json.Unmarshal(raw, &a); err != nil {
		t.Fatal(err)
	}
	return a
}

func notesText(app App) string {
	var b strings.Builder
	for _, n := range app.Notes {
		b.WriteString(n.Reason + "\n")
	}
	return b.String()
}

func TestDokployFileMount(t *testing.T) {
	app, unsup := mapDokployApp("p", loadFileMountApp(t))
	if unsup != nil {
		t.Fatalf("unsupported: %+v", unsup)
	}
	if len(app.Files) != 1 || app.Files[0].Path != "/etc/nginx/nginx.conf" || app.Files[0].Content != "events {}\n" {
		t.Fatalf("files = %+v", app.Files)
	}
	if len(app.Volumes) != 1 {
		t.Errorf("volume mount must still map: %+v", app.Volumes)
	}
	notes := notesText(app)
	for _, want := range []string{"no inline content", "not an absolute path"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes missing %q: %s", want, notes)
		}
	}
}

func TestDokployFileMountOversize(t *testing.T) {
	t.Setenv("APP_SERVICE_FILE_MAX_BYTES", strconv.Itoa(4))
	app, _ := mapDokployApp("p", loadFileMountApp(t))
	if len(app.Files) != 0 {
		t.Fatalf("oversize file must not import: %+v", app.Files)
	}
	if !strings.Contains(notesText(app), "larger than the app file size limit") {
		t.Errorf("missing oversize note: %s", notesText(app))
	}
}
