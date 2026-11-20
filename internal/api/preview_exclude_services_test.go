package api

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestWithoutExcludedServices(t *testing.T) {
	svcs := map[string]spec.Service{"web": {}, "worker": {}, "docs": {}}
	got := withoutExcludedServices(svcs, "worker, docs")
	if len(got) != 1 {
		t.Fatalf("want only web left, got %d services", len(got))
	}
	if _, ok := got["web"]; !ok {
		t.Fatal("web must stay")
	}
	if len(svcs) != 3 {
		t.Fatal("input map must not be mutated")
	}
	if all := withoutExcludedServices(svcs, "web,worker,docs"); len(all) != 3 {
		t.Fatalf("excluding everything must keep the input, got %d", len(all))
	}
	if same := withoutExcludedServices(svcs, ""); len(same) != 3 {
		t.Fatalf("empty list keeps all, got %d", len(same))
	}
}

func TestApplyPreviewExcludeFields(t *testing.T) {
	gs := &store.GitSource{Services: map[string]spec.Service{"web": {}, "worker": {}}}
	var s store.PreviewAppSettings
	keys := []string{"worker", "worker", " "}
	if msg := applyPreviewExcludeFields(gs, &s, setPreviewPolicyRequest{ExcludeServices: &keys}); msg != "" {
		t.Fatalf("unexpected rejection: %s", msg)
	}
	if s.ExcludeServices != "worker" {
		t.Fatalf("got %q", s.ExcludeServices)
	}
	bad := []string{"nope"}
	if msg := applyPreviewExcludeFields(gs, &s, setPreviewPolicyRequest{ExcludeServices: &bad}); msg == "" {
		t.Fatal("unknown service must be rejected")
	}
	all := []string{"web", "worker"}
	if msg := applyPreviewExcludeFields(gs, &s, setPreviewPolicyRequest{ExcludeServices: &all}); msg == "" {
		t.Fatal("excluding every service must be rejected")
	}
	none := []string{}
	if msg := applyPreviewExcludeFields(gs, &s, setPreviewPolicyRequest{ExcludeServices: &none}); msg != "" || s.ExcludeServices != "" {
		t.Fatalf("empty list must clear, msg=%q val=%q", msg, s.ExcludeServices)
	}
}
