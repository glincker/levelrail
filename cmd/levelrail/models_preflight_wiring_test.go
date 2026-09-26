package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

type fakeRoot struct {
	dir string
	err error
}

func (f fakeRoot) DockerRootDir(context.Context) (string, error) { return f.dir, f.err }

func TestDockerDiskFacts(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var statted string
	usage := func(p string) (int64, int64, error) { statted = p; return 70, 100, nil }
	failing := func(string) (int64, int64, error) { return 0, 0, errors.New("no such path") }
	tests := []struct {
		name     string
		src      fakeRoot
		usage    func(string) (int64, int64, error)
		node     string
		wantOK   bool
		wantPath string
	}{
		{"local uses docker root", fakeRoot{dir: "/var/lib/docker"}, usage, "", true, "/var/lib/docker"},
		{"remote node is unknown", fakeRoot{dir: "/var/lib/docker"}, usage, "n1", false, ""},
		{"docker info fails", fakeRoot{err: errors.New("down")}, usage, "", false, ""},
		{"root not visible to control plane", fakeRoot{dir: "/var/lib/docker"}, failing, "", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			statted = ""
			free, total, ok := dockerDiskFacts(context.Background(), tt.src, tt.usage, tt.node, logger)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v, want %v", ok, tt.wantOK)
			}
			if tt.wantOK && statted != tt.wantPath {
				t.Fatalf("statted %q, want %q", statted, tt.wantPath)
			}
			if ok && (free != 70 || total != 100) {
				t.Fatalf("free=%d total=%d", free, total)
			}
		})
	}
}
