package build

import (
	"io"
	"strings"
	"testing"
)

func TestBuildSettingsValidate(t *testing.T) {
	ok := []Settings{{}, {Target: "runtime"}, {Target: "build-1.2_x", NoCache: true}, {Provider: "node"}}
	for _, s := range ok {
		if err := ValidateSettings(s, 64); err != nil {
			t.Errorf("ValidateSettings(%+v) = %v, want nil", s, err)
		}
	}
	bad := []Settings{
		{Target: "-flag"}, {Target: "a b"}, {Target: "x;rm -rf /"}, {Target: "$(id)"},
		{Target: strings.Repeat("a", 65)}, {Provider: "rust"}, {Provider: "NODE"},
	}
	for _, s := range bad {
		if err := ValidateSettings(s, 64); err == nil {
			t.Errorf("ValidateSettings(%+v) = nil, want an error", s)
		}
	}
}

func TestBuildSettingsReachSolveOpt(t *testing.T) {
	req := Request{ContextDir: "testdata", Tag: "levelrail-spike:latest"}
	Settings{Target: "runtime", NoCache: true}.Apply(&req)
	opt, err := newSolveOpt(req, CacheConfig{}, nopWriteCloser{io.Discard})
	if err != nil {
		t.Fatalf("newSolveOpt: %v", err)
	}
	if opt.FrontendAttrs["target"] != "runtime" {
		t.Errorf("target attr = %q, want runtime", opt.FrontendAttrs["target"])
	}
	if _, ok := opt.FrontendAttrs["no-cache"]; !ok {
		t.Error("no-cache attr missing")
	}
}

func TestBuildSettingsProviderPin(t *testing.T) {
	if err := checkPinnedProvider("", "node"); err != nil {
		t.Errorf("no pin: %v", err)
	}
	if err := checkPinnedProvider("node", "node"); err != nil {
		t.Errorf("matching pin: %v", err)
	}
	if err := checkPinnedProvider("python", "node"); err == nil {
		t.Error("mismatched pin passed")
	}
}

func TestBuildSettingsProviderReachesRemoteRequest(t *testing.T) {
	rr, err := NewRemoteRailpackRequest(RailpackRequest{SourceDir: "testdata", Tag: "x:1", Provider: "node"}, CacheConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if rr.RailpackProvider != "node" {
		t.Fatalf("RailpackProvider = %q, want node", rr.RailpackProvider)
	}
}
