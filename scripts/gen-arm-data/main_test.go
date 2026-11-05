package main

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/catalog"
)

func compose(images ...string) string {
	s := "services:\n"
	for i, im := range images {
		s += "  s" + string(rune('a'+i)) + ":\n    image: " + im + "\n"
	}
	return s
}

func fixture() (document, map[string]templateRow) {
	plat := catalog.PlatformsFile{
		ScannedAt: "2026-11-02T00:00:00Z",
		Images: []catalog.ImagePlatforms{
			{Image: "a/both", Tag: "1", Platforms: []string{"linux/arm64", "linux/amd64"}, CheckedAt: "2026-11-01T00:00:00Z"},
			{Image: "a/amd", Tag: "1", Platforms: []string{"linux/amd64"}, CheckedAt: "2026-11-01T00:00:00Z"},
			{Image: "a/old", Tag: "1", Platforms: []string{"linux/arm64", "linux/amd64"}, CheckedAt: "2026-01-01T00:00:00Z"},
			{Image: "a/bad", Tag: "1", Error: "timeout", CheckedAt: "2026-11-01T00:00:00Z"},
		},
	}
	ts := []catalog.Template{
		{ID: "t-yes", Compose: compose("a/both:1", "a/old:1")},
		{ID: "t-no", Compose: compose("a/both:1", "a/amd:1")},
		{ID: "t-unk", Compose: compose("a/both:1", "a/bad:1")},
		{ID: "t-var", Compose: "services:\n  x:\n    image: ${IMG}\n"},
		{ID: "t-missing", Compose: compose("a/none:1")},
	}
	doc := build(ts, plat, 14*24*time.Hour)
	m := map[string]templateRow{}
	for _, r := range doc.Templates {
		m[r.ID] = r
	}
	return doc, m
}

func TestVerdictsAndSources(t *testing.T) {
	doc, m := fixture()
	want := map[string]string{"t-yes": yes, "t-no": no, "t-unk": unknown, "t-var": unknown, "t-missing": unknown}
	for id, w := range want {
		if m[id].ARM64 != w {
			t.Errorf("%s arm64 = %s, want %s", id, m[id].ARM64, w)
		}
	}
	if s := m["t-yes"].Images[1].Source; s != srcCache {
		t.Errorf("old image source = %s, want cache", s)
	}
	if s := m["t-yes"].Images[0].Source; s != srcManifest {
		t.Errorf("fresh image source = %s, want manifest", s)
	}
	if s := m["t-unk"].Images[0]; s.Source != srcUnknown || s.ARM64 != unknown {
		t.Errorf("errored image must stay unknown: %+v", s)
	}
	c := doc.Counts
	if c.Templates != 5 || c.ARM64Yes != 1 || c.ARM64No != 1 || c.ARM64Unknown != 3 {
		t.Errorf("counts %+v", c)
	}
	if doc.GeneratedAt != "2026-11-02T00:00:00Z" {
		t.Errorf("generated_at %s", doc.GeneratedAt)
	}
}

func TestDeterministic(t *testing.T) {
	a, _ := fixture()
	b, _ := fixture()
	if len(a.Templates) != len(b.Templates) || a.Templates[0].ID != "t-missing" {
		t.Fatalf("order not sorted by id: %v", a.Templates[0].ID)
	}
}
