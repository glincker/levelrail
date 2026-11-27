package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestClassifyAndRank(t *testing.T) {
	dir := t.TempDir()
	reg := filepath.Join(dir, "registry")
	write(t, filepath.Join(reg, "templates", "ackee", "template.yaml"), "id: ackee\nname: Ackee\nmanifest:\n  aliases: [ackee-analytics]\n")
	write(t, filepath.Join(reg, "templates", "actual-budget", "template.yaml"), "id: actual-budget\nname: Actual Budget\n")
	write(t, filepath.Join(reg, "baseline-ids.txt"), "# frozen\nold-app\n")
	dist := filepath.Join(dir, "dist")
	for _, f := range []string{"immich.js", "immich.cjs", "memos.js", "logto-badge.js"} {
		write(t, filepath.Join(dist, f), "")
	}
	list := filepath.Join(dir, "apps.json")
	write(t, list, `[
	{"name":"Ackee","stars":100},
	{"name":"Ackee Analytics","stars":90},
	{"name":"Actual","stars":50},
	{"name":"Immich","stars":900,"category":"Photos"},
	{"name":"Memos","stars":300},
	{"name":"Nolo Logo","stars":10},
	{"name":"Old App","stars":5}
	]`)

	apps, err := loadList(list)
	if err != nil {
		t.Fatal(err)
	}
	known, err := loadRegistry(reg)
	if err != nil {
		t.Fatal(err)
	}
	icons, err := loadIcons(dist)
	if err != nil {
		t.Fatal(err)
	}
	rep := classify(apps, known, icons)

	if len(rep.Covered) != 3 {
		t.Fatalf("covered = %d, want 3 (id, alias, baseline)", len(rep.Covered))
	}
	if len(rep.Near) != 1 || rep.Near[0].Match != "actual-budget" {
		t.Fatalf("near = %+v, want Actual near actual-budget", rep.Near)
	}
	if len(rep.Missing) != 3 || rep.Missing[0].Name != "Immich" || rep.Missing[1].Name != "Memos" {
		t.Fatalf("missing order wrong: %+v", rep.Missing)
	}
	if rep.Missing[0].Slug != "immich" || rep.Missing[2].HasLogo {
		t.Fatalf("icon resolution wrong: %+v", rep.Missing)
	}
	out := render(rep, 10, len(apps))
	if !strings.Contains(out, "Needs a thesvg PR") && !strings.Contains(out, "needs a thesvg PR") {
		t.Fatal("report lacks the missing logo section")
	}
}

func TestCSVAndRankScore(t *testing.T) {
	dir := t.TempDir()
	list := filepath.Join(dir, "apps.csv")
	write(t, list, "Name,Rank,Repo\nSecond,2,r2\nFirst,1,r1\n")
	apps, err := loadList(list)
	if err != nil {
		t.Fatal(err)
	}
	if apps[1].Score <= apps[0].Score {
		t.Fatalf("rank 1 must outscore rank 2: %+v", apps)
	}
}

func TestEditWithin1(t *testing.T) {
	cases := map[[2]string]bool{
		{"vaultwarden", "vaultwardn"}:  true,
		{"gitea", "gitee"}:             true,
		{"grafana", "grafana"}:         true,
		{"grafana", "graphite"}:        false,
		{"changedetection", "changed"}: false,
	}
	for in, want := range cases {
		if got := editWithin1(in[0], in[1]); got != want {
			t.Errorf("editWithin1(%q,%q) = %v, want %v", in[0], in[1], got, want)
		}
	}
}
