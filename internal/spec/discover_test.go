package spec

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverPath(t *testing.T) {
	tests := []struct {
		name        string
		files       []string // files to create in the temp dir
		brandedName string
		wantFile    string // empty means "expect an error"
	}{
		{
			name:     "finds app.yaml",
			files:    []string{"app.yaml"},
			wantFile: "app.yaml",
		},
		{
			name:     "finds deploy.yaml when app.yaml absent",
			files:    []string{"deploy.yaml"},
			wantFile: "deploy.yaml",
		},
		{
			name:        "prefers app.yaml over the branded name, so a rebrand never breaks an existing repo",
			files:       []string{"app.yaml", "levelrail.yaml"},
			brandedName: "levelrail",
			wantFile:    "app.yaml",
		},
		{
			name:        "falls back to the branded name when nothing generic exists",
			files:       []string{"levelrail.yaml"},
			brandedName: "levelrail",
			wantFile:    "levelrail.yaml",
		},
		{
			name:  "nothing found",
			files: []string{"README.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte("version: 1"), 0o600); err != nil {
					t.Fatalf("write fixture %s: %v", f, err)
				}
			}

			got, err := DiscoverPath(dir, tt.brandedName)
			if tt.wantFile == "" {
				if err == nil {
					t.Fatalf("DiscoverPath() = %q, want an error (nothing should match)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("DiscoverPath() error = %v", err)
			}
			want := filepath.Join(dir, tt.wantFile)
			if got != want {
				t.Errorf("DiscoverPath() = %q, want %q", got, want)
			}
		})
	}
}
