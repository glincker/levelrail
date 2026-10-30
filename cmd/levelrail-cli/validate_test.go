package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateOffline(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.yaml")
	bad := filepath.Join(dir, "bad.yaml")
	goodDoc := "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: \"ghcr.io/acme/web:1.2.3\"}\n    port: 3000\n"
	if err := os.WriteFile(good, []byte(goodDoc), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte("version: 1\nkind: App\nmetadata: {name: Web_Bad}\nspec:\n  service: {port: nope}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	if code := run("levelrail-cli-test", []string{"validate", good}, &out, &errBuf, envMap()); code != 0 {
		t.Fatalf("good: code %d stderr %s", code, errBuf.String())
	}
	out.Reset()
	errBuf.Reset()
	if code := run("levelrail-cli-test", []string{"validate", "--json", bad}, &out, &errBuf, envMap()); code != 1 {
		t.Fatalf("bad: code %d", code)
	}
	var rep validateReport
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil || rep.Valid || len(rep.Issues) == 0 {
		t.Fatalf("report %s err %v", out.String(), err)
	}
	if rep.Issues[0].Line == 0 || !strings.Contains(rep.Issues[0].File, "bad.yaml") {
		t.Fatalf("issue lacks position: %+v", rep.Issues[0])
	}
}
