package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testRoot = "testdata/registry/templates"

func rules(ps []Problem) map[string]int {
	m := map[string]int{}
	for _, p := range ps {
		m[p.Rule]++
	}
	return m
}

func TestValidateFolder(t *testing.T) {
	cats := knownCategories()
	if ps := validateFolder(filepath.Join(testRoot, "good"), "good", cats); len(ps) != 0 {
		t.Fatalf("good template reported problems: %v", ps)
	}
	if got := rules(validateFolder(filepath.Join(testRoot, "sock-ok"), "sock-ok", cats)); got[ruleSecurity] == 0 {
		t.Fatalf("docker socket template must be rejected, got %v", got)
	}
	got := rules(validateFolder(filepath.Join(testRoot, "bad-one"), "bad-one", cats))
	for _, r := range []string{ruleID, ruleCategory, ruleIcon, ruleImage, ruleHealth, ruleSecurity, rulePorts} {
		if got[r] == 0 {
			t.Errorf("bad-one: rule %s not reported, got %v", r, got)
		}
	}
	if got[ruleSecurity] < 3 {
		t.Errorf("bad-one: want privileged, host network and socket reported, got %d", got[ruleSecurity])
	}
	bad := rules(validateFolder(filepath.Join(testRoot, "Bad_Id"), "Bad_Id", cats))
	if bad[ruleID] == 0 || bad[ruleFields] == 0 || bad[ruleImage] == 0 {
		t.Errorf("Bad_Id: want id, required-field and image-pin, got %v", bad)
	}
}

func TestUnpinned(t *testing.T) {
	for img, wantBad := range map[string]bool{
		"nginx": true, "nginx:latest": true, "nginx:1.27": false,
		"localhost:5000/app": true, "localhost:5000/app:2": false,
		"app@sha256:abc": false, "app:${V:-latest}": true, "app:${V:-3}": false,
	} {
		if got := unpinned(img) != ""; got != wantBad {
			t.Errorf("unpinned(%q) = %v, want %v", img, got, wantBad)
		}
	}
}

func TestFixedHostPort(t *testing.T) {
	for p, want := range map[any]bool{"3000": false, "3000/udp": false, "8080:80": true, "${P:-80}:80": true, "${P:-80}": false} {
		if got := fixedHostPort(p); got != want {
			t.Errorf("fixedHostPort(%v) = %v, want %v", p, got, want)
		}
	}
}

func TestRunChangedOnlyAndLegacy(t *testing.T) {
	list := filepath.Join(t.TempDir(), "changed.txt")
	if err := os.WriteFile(list, []byte("testdata/registry/templates/good/compose.yaml\ndocs/x.md\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := run(testRoot, list, false, false, &out, &errOut); code != 0 {
		t.Fatalf("changed good folder should pass, code %d: %s %s", code, out.String(), errOut.String())
	}
	if err := os.WriteFile(list, []byte("testdata/registry/templates/bad-one/compose.yaml\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := run(testRoot, list, false, false, &out, &errOut); code != 1 {
		t.Fatalf("changed bad folder should fail, code %d", code)
	}
	if !strings.Contains(out.String(), "bad-one: ") {
		t.Errorf("want one line per problem, got %q", out.String())
	}
	out.Reset()
	if code := run(testRoot, "", false, false, &out, &errOut); code != 0 {
		t.Fatalf("legacy mode must not fail, code %d", code)
	}
	if !strings.Contains(out.String(), "legacy summary") {
		t.Errorf("want legacy summary, got %q", out.String())
	}
}
