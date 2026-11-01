package build

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/moby/buildkit/client/llb"
	"github.com/moby/buildkit/solver/pb"
)

func TestParseNetworkMode(t *testing.T) {
	tests := []struct {
		in      string
		want    NetworkMode
		wantErr bool
	}{
		{"", NetworkOpen, false},
		{"open", NetworkOpen, false},
		{" NONE ", NetworkNone, false},
		{"block-private", NetworkBlockPrivate, false},
		{"bridge", "", true},
	}
	for _, tt := range tests {
		got, err := ParseNetworkMode(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("ParseNetworkMode(%q) = %q, %v", tt.in, got, err)
		}
	}
}

func TestNewSolveOptNetworkPolicy(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine\nRUN true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		mode    NetworkMode
		want    string
		wantErr error
	}{
		{"", "", nil},
		{NetworkOpen, "", nil},
		{NetworkNone, "none", nil},
		{NetworkBlockPrivate, "", ErrNetworkUnenforceable},
	}
	for _, tt := range tests {
		opt, err := newSolveOpt(Request{ContextDir: dir, Tag: "t:1", Network: tt.mode}, CacheConfig{}, nopWriteCloser{})
		if !errors.Is(err, tt.wantErr) {
			t.Fatalf("mode %q err = %v, want %v", tt.mode, err, tt.wantErr)
		}
		if err != nil {
			continue
		}
		if got := opt.FrontendAttrs["force-network-mode"]; got != tt.want {
			t.Errorf("mode %q force-network-mode = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestApplyRailpackNetwork(t *testing.T) {
	def, err := llb.Image("alpine:3.20").Run(llb.Shlex("echo one")).Root().Marshal(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	same, err := NetworkOpen.applyRailpack(def)
	if err != nil || same != def {
		t.Fatalf("open must return the definition untouched, got %v", err)
	}
	if _, err := NetworkBlockPrivate.applyRailpack(def); !errors.Is(err, ErrNetworkUnenforceable) {
		t.Fatalf("block-private err = %v", err)
	}
	got, err := NetworkNone.applyRailpack(def)
	if err != nil {
		t.Fatal(err)
	}
	execs := 0
	for _, raw := range got.Def {
		var op pb.Op
		if err := op.UnmarshalVT(raw); err != nil {
			t.Fatal(err)
		}
		if ex := op.GetExec(); ex != nil {
			execs++
			if ex.Network != pb.NetMode_NONE {
				t.Fatalf("exec network = %v, want NONE", ex.Network)
			}
		}
	}
	if execs != 1 {
		t.Fatalf("exec ops = %d, want 1", execs)
	}
}

func TestNetworkFacts(t *testing.T) {
	open := NetworkOpen.Facts()
	if open.Enforced || !strings.Contains(open.Summary, "no restriction") || open.NotProven == "" {
		t.Errorf("open facts must say nothing is restricted and not probed: %+v", open)
	}
	if none := NetworkNone.Facts(); !none.Enforced {
		t.Errorf("none must be enforced: %+v", none)
	}
	if bp := NetworkBlockPrivate.Facts(); bp.Enforced || !strings.Contains(bp.Summary, "cannot") && !strings.Contains(bp.Summary, "no build path") {
		t.Errorf("block-private must say it is unenforced: %+v", bp)
	}
}

func TestResolveNetworkPrefersStoredSettingOverEnv(t *testing.T) {
	t.Setenv(EnvBuildNetwork, "none")
	t.Cleanup(func() { SetNetworkResolver(nil) })
	ctx := context.Background()

	if got, err := resolveNetwork(ctx); err != nil || got != NetworkNone {
		t.Fatalf("env seed = %q, %v", got, err)
	}
	stored := "open"
	SetNetworkResolver(func(context.Context) string { return stored })
	if got, _ := resolveNetwork(ctx); got != NetworkOpen {
		t.Fatalf("stored open must win over env none, got %q", got)
	}
	stored = "none"
	if got, _ := resolveNetwork(ctx); got != NetworkNone {
		t.Fatalf("a changed stored value must apply on the next build, got %q", got)
	}
}

func TestNewSolveOptCustomFrontend(t *testing.T) {
	write := func(body string) string {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	evil := write("# syntax=evil/frontend:latest\nFROM alpine\nRUN true\n")
	stock := write("# syntax=docker/dockerfile:1\nFROM alpine\n")
	plain := write("FROM alpine\n")
	tests := []struct {
		name string
		dir  string
		mode NetworkMode
		args map[string]string
		want error
	}{
		{"none refuses directive", evil, NetworkNone, nil, ErrNetworkFrontend},
		{"none refuses build arg", plain, NetworkNone, map[string]string{"BUILDKIT_SYNTAX": "evil/frontend"}, ErrNetworkFrontend},
		{"none allows stock directive", stock, NetworkNone, nil, nil},
		{"none allows stock arg", plain, NetworkNone, map[string]string{"BUILDKIT_SYNTAX": "docker.io/docker/dockerfile:1"}, nil},
		{"none refuses lookalike", plain, NetworkNone, map[string]string{"BUILDKIT_SYNTAX": "localhost:5000/docker/dockerfile"}, ErrNetworkFrontend},
		{"open unchanged", evil, NetworkOpen, nil, nil},
		{"empty mode unchanged", evil, "", map[string]string{"BUILDKIT_SYNTAX": "evil/frontend"}, nil},
	}
	for _, tt := range tests {
		_, err := newSolveOpt(Request{ContextDir: tt.dir, Tag: "t:1", Network: tt.mode, BuildArgs: tt.args}, CacheConfig{}, nopWriteCloser{})
		if !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}
}
