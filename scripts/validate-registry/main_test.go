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

func TestSplitLegacy(t *testing.T) {
	ps := []Problem{{Template: "old", Rule: rulePorts}, {Template: "old", Rule: ruleSecurity}, {Template: "new", Rule: rulePorts}}
	kept, waived := splitLegacy(ps, map[string]bool{"old": true})
	if waived != 1 || len(kept) != 2 {
		t.Fatalf("want 1 waived and 2 kept, got %d waived, %d kept", waived, len(kept))
	}
}

func TestValidateMarketplace(t *testing.T) {
	cases := []struct {
		name string
		m    meta
		want int
	}{
		{"complete", meta{SourceRepo: "https://github.com/a/b", License: "MIT", LicenseClass: "permissive"}, 0},
		{"licenseref", meta{SourceRepo: "https://github.com/a/b", License: "LicenseRef-x-BSD", LicenseClass: "permissive"}, 0},
		{"unknown class", meta{SourceRepo: "https://github.com/a/b", License: "LicenseRef-x", LicenseClass: "unknown"}, 0},
		{"missing all", meta{}, 2},
		{"http repo", meta{SourceRepo: "http://github.com/a/b", License: "MIT", LicenseClass: "permissive"}, 1},
		{"no class", meta{SourceRepo: "https://github.com/a/b", License: "MIT"}, 1},
		{"bad spdx", meta{SourceRepo: "https://github.com/a/b", License: "Whatever", LicenseClass: "permissive"}, 1},
		{"class mismatch", meta{SourceRepo: "https://github.com/a/b", License: "GPL-3.0", LicenseClass: "permissive"}, 1},
		{"bad class", meta{SourceRepo: "https://github.com/a/b", License: "MIT", LicenseClass: "nice"}, 1},
	}
	for _, c := range cases {
		if got := len(validateMarketplace("x", c.m)); got != c.want {
			t.Errorf("%s: %d problems, want %d", c.name, got, c.want)
		}
	}
}

func TestMarketplaceRuleExemptions(t *testing.T) {
	ps := validateFolder(filepath.Join(testRoot, "sock-ok"), "sock-ok", knownCategories())
	if rules(ps)[ruleMarket] == 0 {
		t.Fatalf("new folder without marketplace fields must fail, got %v", rules(ps))
	}
	kept, waived := splitLegacy([]Problem{{Template: "old", Rule: ruleMarket}}, map[string]bool{"old": true})
	if len(kept) != 0 || waived != 1 {
		t.Fatalf("legacy id must waive the marketplace rule, kept %v waived %d", kept, waived)
	}
	if !inCatalog("2fauth") || inCatalog("dagu") {
		t.Error("inCatalog must be true for a compiled template and false for a registry-only one")
	}
}
