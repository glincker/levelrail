package pages

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestApplyKeepsUserHeaders(t *testing.T) {
	l := spec.DefaultStaticLimits()
	user := spec.StaticHeaderBlock{Source: "_headers", Path: "/*", Set: []spec.StaticHeaderKV{{Name: "Cache-Control", Value: "no-store"}}}
	if err := spec.ValidateStaticHeaderBlock(&user, l); err != nil {
		t.Fatal(err)
	}
	user.Source = "_headers"
	rs := &spec.StaticRuleset{Headers: []spec.StaticHeaderBlock{user}}
	if err := Apply(rs, "spa", l); err != nil {
		t.Fatal(err)
	}
	for _, b := range rs.Headers[1:] {
		if !strings.HasPrefix(b.Source, SourcePrefix) {
			t.Fatalf("unexpected source %q", b.Source)
		}
		for _, kv := range b.Set {
			if kv.Name == "Cache-Control" {
				t.Fatalf("preset overrode the user's Cache-Control on %s", b.Path)
			}
		}
	}
	if err := Apply(rs, "", l); err != nil || len(rs.Headers) != 1 {
		t.Fatalf("clear left %d blocks, err %v", len(rs.Headers), err)
	}
	if err := Apply(rs, "nope", l); err == nil {
		t.Fatal("unknown preset accepted")
	}
}

func TestPresetsLoad(t *testing.T) {
	all, err := All()
	if err != nil || len(all) != 4 {
		t.Fatalf("presets: %d, %v", len(all), err)
	}
	for _, p := range all {
		rs := &spec.StaticRuleset{}
		if err := Apply(rs, p.ID, spec.DefaultStaticLimits()); err != nil {
			t.Fatalf("%s: %v", p.ID, err)
		}
	}
}
