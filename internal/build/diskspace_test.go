package build

import (
	"errors"
	"io"
	"testing"
)

// withFakeDiskFree substitutes diskFreeFunc for the duration of a test,
// restoring the real diskspace.Free implementation afterward.
func withFakeDiskFree(t *testing.T, free map[string]int64) {
	t.Helper()
	orig := diskFreeFunc
	t.Cleanup(func() { diskFreeFunc = orig })
	diskFreeFunc = func(path string) (int64, error) {
		v, ok := free[path]
		if !ok {
			return 0, errors.New("diskspace: no such path")
		}
		return v, nil
	}
}

func TestCheckDiskSpace(t *testing.T) {
	t.Setenv(envMinDiskSpaceMB, "")

	tests := []struct {
		name    string
		free    map[string]int64
		dirs    []string
		wantErr bool
	}{
		{
			name:    "above threshold allows",
			free:    map[string]int64{"/ctx": 2 << 30},
			dirs:    []string{"/ctx"},
			wantErr: false,
		},
		{
			name:    "below threshold blocks",
			free:    map[string]int64{"/ctx": 100 << 20},
			dirs:    []string{"/ctx"},
			wantErr: true,
		},
		{
			name:    "empty dir is skipped, not checked",
			free:    map[string]int64{},
			dirs:    []string{""},
			wantErr: false,
		},
		{
			name:    "cache dir below threshold blocks even if context dir is fine",
			free:    map[string]int64{"/ctx": 2 << 30, "/cache": 10 << 20},
			dirs:    []string{"/ctx", "/cache"},
			wantErr: true,
		},
		{
			name:    "unreadable dir degrades to allow, not block",
			free:    map[string]int64{},
			dirs:    []string{"/does/not/exist"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFakeDiskFree(t, tt.free)
			err := checkDiskSpace(tt.dirs...)
			if tt.wantErr && err == nil {
				t.Fatal("checkDiskSpace(): want error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("checkDiskSpace(): want no error, got %v", err)
			}
		})
	}
}

func TestMinDiskSpaceBytes(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want int64
	}{
		{name: "unset uses default", env: "", want: defaultMinDiskSpaceMB << 20},
		{name: "valid override", env: "2048", want: 2048 << 20},
		{name: "invalid falls back to default", env: "not-a-number", want: defaultMinDiskSpaceMB << 20},
		{name: "zero falls back to default", env: "0", want: defaultMinDiskSpaceMB << 20},
		{name: "negative falls back to default", env: "-5", want: defaultMinDiskSpaceMB << 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envMinDiskSpaceMB, tt.env)
			if got := minDiskSpaceBytes(); got != tt.want {
				t.Fatalf("minDiskSpaceBytes() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestClient_Build_InsufficientDiskSpace(t *testing.T) {
	t.Setenv(envMinDiskSpaceMB, "")
	withFakeDiskFree(t, map[string]int64{"/ctx": 1 << 20})

	var c Client
	_, err := c.Build(t.Context(), Request{ContextDir: "/ctx", Tag: "app:sha"}, nil)
	if err == nil {
		t.Fatal("Build(): want error for insufficient disk space, got nil")
	}
}

func TestClient_BuildRailpack_InsufficientDiskSpace(t *testing.T) {
	t.Setenv(envMinDiskSpaceMB, "")
	withFakeDiskFree(t, map[string]int64{"/src": 1 << 20})

	var c Client
	_, err := c.BuildRailpack(t.Context(), RailpackRequest{SourceDir: "/src", Tag: "app:sha"}, nil)
	if err == nil {
		t.Fatal("BuildRailpack(): want error for insufficient disk space, got nil")
	}
}

func TestClient_SolveRemote_InsufficientDiskSpace(t *testing.T) {
	t.Setenv(envMinDiskSpaceMB, "")
	withFakeDiskFree(t, map[string]int64{"/ctx": 1 << 20})

	var c Client
	_, err := c.SolveRemote(t.Context(), RemoteRequest{Kind: RemoteKindDockerfile, ContextDir: "/ctx", Tag: "app:sha"}, io.Discard, nil)
	if err == nil {
		t.Fatal("SolveRemote(): want error for insufficient disk space, got nil")
	}
}
