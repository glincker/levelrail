package dbupgrade

import (
	"reflect"
	"testing"
	"time"
)

const testCatalog = `{
  "updated": "2026-01-01",
  "engines": {
    "postgres": {
      "levels": ["major", "patch"], "auto_max": "patch", "image_revert": true,
      "versions": ["16.9", "16.11", "17.7", "18.1"],
      "lines": [{"line": "16", "eol": "2028-11-09"}, {"line": "17", "eol": "2029-11-08"}, {"line": "14", "eol": "2026-11-12"}, {"line": "13", "eol": "2025-11-13"}],
      "advisories": [
        {"ids": ["CVE-A"], "fixed": ["16.10", "17.6"]},
        {"ids": ["CVE-B"], "fixed": ["16.11", "17.7", "18.1"]}
      ]
    },
    "redis": {
      "levels": ["major", "minor", "patch"], "auto_max": "minor", "image_revert": true,
      "versions": ["7.2.10", "7.2.11", "7.4.6", "8.0.4"],
      "lines": [],
      "advisories": [{"ids": ["CVE-R"], "fixed": ["7.2.11", "7.4.6", "8.0.4"]}]
    },
    "mysql": {
      "levels": ["major", "major", "patch"], "auto_max": "none", "image_revert": false,
      "manual_reason": "manual", "versions": ["8.0.43", "8.4.6"], "lines": [], "advisories": []
    },
    "dragonfly": {
      "levels": ["major", "minor", "patch"], "auto_max": "patch", "image_revert": true,
      "versions": ["v1.27.1", "v1.27.4", "v1.28.0"], "lines": [], "advisories": []
    }
  }
}`

func testAdvisor(t *testing.T) Advisor {
	t.Helper()
	c, err := parseCatalog([]byte(testCatalog))
	if err != nil {
		t.Fatalf("parseCatalog() error = %v", err)
	}
	return Advisor{Catalog: c, EOLWarn: 180 * 24 * time.Hour}
}

type wantTarget struct {
	version  string
	kind     string
	security bool
	auto     bool
}

func TestAdvise(t *testing.T) {
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		engine     string
		current    string
		wantTarget []wantTarget
		support    string
		floating   bool
		advisories []string
	}{
		{
			name: "postgres patch with security and newer majors", engine: "postgres", current: "16.9",
			wantTarget: []wantTarget{{"16.11", KindPatch, true, true}, {"18.1", KindMajor, true, false}, {"17.7", KindMajor, true, false}},
			support:    SupportSupported, advisories: []string{"CVE-A", "CVE-B"},
		},
		{
			name: "postgres latest patch only majors left", engine: "postgres", current: "16.11",
			wantTarget: []wantTarget{{"18.1", KindMajor, false, false}, {"17.7", KindMajor, false, false}},
			support:    SupportSupported,
		},
		{
			name: "floating major tag counts as oldest of its line", engine: "postgres", current: "16",
			wantTarget: []wantTarget{{"16.11", KindPatch, true, true}, {"18.1", KindMajor, true, false}, {"17.7", KindMajor, true, false}},
			support:    SupportSupported, floating: true, advisories: []string{"CVE-A", "CVE-B"},
		},
		{
			name: "postgres line past eol", engine: "postgres", current: "13.5",
			wantTarget: []wantTarget{{"18.1", KindMajor, false, false}, {"17.7", KindMajor, false, false}, {"16.11", KindMajor, false, false}},
			support:    SupportEOL,
		},
		{
			name: "postgres line inside eol warning", engine: "postgres", current: "14.2",
			wantTarget: []wantTarget{{"18.1", KindMajor, false, false}, {"17.7", KindMajor, false, false}, {"16.11", KindMajor, false, false}},
			support:    SupportEOLSoon,
		},
		{
			name: "suffix is carried to the target", engine: "postgres", current: "16.9-alpine",
			wantTarget: []wantTarget{{"16.11-alpine", KindPatch, true, true}, {"18.1-alpine", KindMajor, true, false}, {"17.7-alpine", KindMajor, true, false}},
			support:    SupportSupported, advisories: []string{"CVE-A", "CVE-B"},
		},
		{
			name: "redis patch and minor", engine: "redis", current: "7.2.10",
			wantTarget: []wantTarget{{"7.2.11", KindPatch, true, true}, {"7.4.6", KindMinor, true, true}, {"8.0.4", KindMajor, true, false}},
			support:    SupportUnknown, advisories: []string{"CVE-R"},
		},
		{
			name: "mysql never automatic", engine: "mysql", current: "8.0.40",
			wantTarget: []wantTarget{{"8.0.43", KindPatch, false, false}, {"8.4.6", KindMajor, false, false}},
			support:    SupportUnknown,
		},
		{
			name: "dragonfly v prefix", engine: "dragonfly", current: "v1.27.1",
			wantTarget: []wantTarget{{"v1.27.4", KindPatch, false, true}, {"v1.28.0", KindMinor, false, false}},
			support:    SupportUnknown,
		},
		{name: "latest is not comparable", engine: "postgres", current: "latest", support: SupportUnknown},
		{name: "pgvector has no pinned targets", engine: "postgres", current: "17-pgvector", support: SupportUnknown},
		{name: "unknown engine", engine: "cockroach", current: "23.1", support: SupportUnknown},
	}
	ad := testAdvisor(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ad.Advise(tt.engine, tt.current, now)
			var targets []wantTarget
			for _, tg := range got.Targets {
				targets = append(targets, wantTarget{tg.Version, tg.Kind, tg.Security, tg.Automatic})
			}
			if !reflect.DeepEqual(targets, tt.wantTarget) {
				t.Errorf("targets = %+v, want %+v", targets, tt.wantTarget)
			}
			if got.Support != tt.support {
				t.Errorf("support = %q, want %q", got.Support, tt.support)
			}
			if got.Floating != tt.floating {
				t.Errorf("floating = %v, want %v", got.Floating, tt.floating)
			}
			if !reflect.DeepEqual(got.Advisories, tt.advisories) {
				t.Errorf("advisories = %v, want %v", got.Advisories, tt.advisories)
			}
		})
	}
}

func TestBestAutomatic(t *testing.T) {
	ad := testAdvisor(t)
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		engine, current, level, want string
	}{
		{"redis", "7.2.10", KindPatch, "7.2.11"},
		{"redis", "7.2.10", KindMinor, "7.4.6"},
		{"postgres", "16.9", KindMinor, "16.11"},
		{"mysql", "8.0.40", KindPatch, ""},
		{"postgres", "16.11", KindPatch, ""},
	}
	for _, tt := range tests {
		got, ok := ad.Advise(tt.engine, tt.current, now).BestAutomatic(tt.level)
		if (tt.want == "") == ok || (ok && got.Version != tt.want) {
			t.Errorf("BestAutomatic(%s %s, %s) = %q, %v, want %q", tt.engine, tt.current, tt.level, got.Version, ok, tt.want)
		}
	}
}

func TestClassify(t *testing.T) {
	tests := []struct {
		levels   []string
		from, to string
		want     string
	}{
		{[]string{KindMajor, KindPatch}, "16.4", "16.5", KindPatch},
		{[]string{KindMajor, KindPatch}, "16.4", "17.0", KindMajor},
		{[]string{KindMajor, KindMinor, KindPatch}, "7.2.4", "7.4.0", KindMinor},
		{[]string{KindMajor, KindMajor, KindPatch}, "11.4.2", "11.8.1", KindMajor},
		{[]string{KindMajor, KindMajor, KindPatch, KindPatch}, "24.8.1.1", "24.8.14.39", KindPatch},
		{[]string{KindMajor, KindPatch}, "16", "16.0", ""},
	}
	for _, tt := range tests {
		from, _ := parseVersion(tt.from)
		to, _ := parseVersion(tt.to)
		if got := classify(tt.levels, from, to); got != tt.want {
			t.Errorf("classify(%v, %s, %s) = %q, want %q", tt.levels, tt.from, tt.to, got, tt.want)
		}
	}
}

func TestEmbeddedCatalogParses(t *testing.T) {
	c, err := LoadCatalog()
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	for _, id := range []string{"postgres", "redis", "mysql", "mongodb", "mariadb", "keydb", "dragonfly", "clickhouse"} {
		if _, ok := c.Engine(id); !ok {
			t.Errorf("catalog has no %q", id)
		}
	}
}

func TestUsableTags(t *testing.T) {
	ec := EngineCatalog{Levels: []string{KindMajor, KindPatch}, Versions: []string{"15.15", "16.11"}}
	got := usableTags(ec, []string{"16.12", "16", "16.12-alpine", "14.20", "18.2", "latest", "15.16", "bookworm"})
	want := []string{"16.12", "18.2", "15.16"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("usableTags() = %v, want %v", got, want)
	}
}
