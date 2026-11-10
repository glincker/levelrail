package platformimport

import (
	"context"
	"strings"
	"testing"
)

func TestCoolifyKeyDBDragonflyClickHouseImported(t *testing.T) {
	srv := readOnlyServer(t, map[string]string{
		"/api/v1/projects":                 "coolify/projects.json",
		"/api/v1/projects/p1/environments": "coolify/environments.json",
		"/api/v1/projects/p1/production":   "coolify/env_resources_extra_dbs.json",
	}, "", "Authorization", "Bearer tok-fixture")
	src, err := NewCoolify(srv.URL, "tok-fixture", testOpts)
	if err != nil {
		t.Fatal(err)
	}
	d, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range d.Unsupported {
		if u.Kind == "database" {
			t.Errorf("database still skipped: %+v", u)
		}
	}
	got := map[string]string{}
	for _, db := range d.Databases {
		got[db.Engine] = db.Version
	}
	if v, ok := got["keydb"]; !ok || v != "" {
		t.Errorf("keydb: %q %v", v, ok)
	}
	if got["dragonfly"] != "v1.27.1" || got["clickhouse"] != "24.8" {
		t.Errorf("versions: %+v", got)
	}
	p := BuildPlan(d, PlanOptions{LatestDBVersions: map[string]string{"keydb": "6.3"}})
	if len(p.Databases) != 3 {
		t.Fatalf("planned databases: %+v", p.Databases)
	}
	var copyHints int
	for _, it := range p.Report.Items {
		if it.Kind != "database" {
			continue
		}
		if strings.ContainsFunc(it.Target, func(r rune) bool { return r < 0x20 }) {
			t.Errorf("control character kept in target %q", it.Target)
		}
		for _, m := range it.Manual {
			if strings.Contains(m, "migrate db-copy "+it.Target) {
				copyHints++
			}
		}
	}
	if copyHints != 3 {
		t.Errorf("want a db-copy command per database, got %d", copyHints)
	}
}

func TestEngineFromImageDragonflyKeepsFullTag(t *testing.T) {
	for img, want := range map[string]string{
		"eqalpha/keydb:x86_64_v6.3.4":                         "x86_64_v6.3.4",
		"docker.io/eqalpha/keydb:6.3.4":                       "6.3.4",
		"other/keydb:6.3.4":                                   "",
		"eqalpha/keydb:6.3":                                   "",
		"docker.dragonflydb.io/dragonflydb/dragonfly:v1.27.1": "v1.27.1",
		"docker.dragonflydb.io/dragonflydb/dragonfly:latest":  "",
		"docker.dragonflydb.io/dragonflydb/dragonfly:v1.27":   "",
	} {
		if e, v := engineFromImage(img); (e != "dragonfly" && e != "keydb") || v != want {
			t.Errorf("%s: %s %q", img, e, v)
		}
	}
}
