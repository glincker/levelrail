package build

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

func TestValidatePublicRepoURL(t *testing.T) {
	tests := []struct {
		name    string
		repoURL string
		wantErr bool
	}{
		{name: "https", repoURL: "https://github.com/acme/app.git"},
		{name: "http", repoURL: "http://internal.example/app.git"},
		{name: "file scheme rejected", repoURL: "file:///etc/passwd", wantErr: true},
		{name: "ssh scheme rejected", repoURL: "ssh://git@github.com/acme/app.git", wantErr: true},
		{name: "unparseable", repoURL: "://not a url", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePublicRepoURL(tt.repoURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePublicRepoURL(%q) error = %v, wantErr %v", tt.repoURL, err, tt.wantErr)
			}
			if tt.wantErr && err != nil && !errors.Is(err, ErrRepoURLSchemeNotAllowed) {
				// The unparseable case fails at url.Parse, before the
				// scheme check, so it never wraps this sentinel; every
				// other rejection must.
				if tt.name != "unparseable" {
					t.Errorf("ValidatePublicRepoURL(%q) error = %v, want wrapping ErrRepoURLSchemeNotAllowed", tt.repoURL, err)
				}
			}
		})
	}
}

func TestDetect_RejectsNonHTTPScheme(t *testing.T) {
	_, err := Detect(context.Background(), DetectRequest{RepoURL: "file:///etc/passwd"})
	if !errors.Is(err, ErrRepoURLSchemeNotAllowed) {
		t.Fatalf("Detect() error = %v, want ErrRepoURLSchemeNotAllowed", err)
	}
}

// localRepoFrom builds a real, single-commit git repository in a fresh
// temp dir containing a copy of fixtureDir's files, so detectUnchecked
// can clone it exactly like a real remote (go-git resolves a plain
// filesystem path as a "file" transport endpoint, the same free-of-any-
// network-dependency tradeoff internal/api/git_branches_live_test.go's
// own TestListRemoteBranches_Live already documents), without a live
// GitHub dependency.
func localRepoFrom(t *testing.T, fixtureDir string) string {
	t.Helper()
	srcDir := t.TempDir()

	err := filepath.WalkDir(fixtureDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(fixtureDir, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		dest := filepath.Join(srcDir, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0o750)
		}
		// path is always a WalkDir-yielded child of the hardcoded
		// testdata fixture this test passed in, never external input.
		data, readErr := os.ReadFile(path) //nolint:gosec // fixed testdata fixture path, not user input
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(dest, data, 0o600) //nolint:gosec // dest is derived from t.TempDir() plus a WalkDir-relative path under the fixed testdata fixture, not user input
	})
	if err != nil {
		t.Fatalf("copy fixture %q: %v", fixtureDir, err)
	}

	repo, err := git.PlainInit(srcDir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := wt.Add("."); err != nil {
		t.Fatalf("Add: %v", err)
	}
	sig := &object.Signature{Name: "Levelrail Test", Email: "test@example.invalid", When: time.Now()}
	if _, err := wt.Commit("fixture commit", &git.CommitOptions{Author: sig}); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return srcDir
}

func TestDetectUnchecked(t *testing.T) {
	tests := []struct {
		name          string
		fixture       string
		wantProvider  string
		wantFramework string
	}{
		{name: "node", fixture: "testdata/railpack-node", wantProvider: "node", wantFramework: "Node.js"},
		{name: "go", fixture: "testdata/railpack-go", wantProvider: "golang", wantFramework: "Go"},
		// Railpack itself detects a real provider here (python), just one
		// outside supportedRailpackProviders: Provider still reports what
		// Railpack found, FrameworkName stays empty so the wizard falls
		// back to manual build-type selection rather than claiming a name
		// for a stack it can't actually build yet.
		{name: "unsupported provider reports raw provider, no framework name", fixture: "testdata/railpack-unsupported", wantProvider: "python", wantFramework: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoDir := localRepoFrom(t, tt.fixture)
			result, err := detectUnchecked(context.Background(), DetectRequest{RepoURL: repoDir})
			if err != nil {
				t.Fatalf("detectUnchecked() error = %v", err)
			}
			if result.Provider != tt.wantProvider {
				t.Errorf("Provider = %q, want %q", result.Provider, tt.wantProvider)
			}
			if result.FrameworkName != tt.wantFramework {
				t.Errorf("FrameworkName = %q, want %q", result.FrameworkName, tt.wantFramework)
			}
		})
	}
}

func TestDetectUnchecked_NonexistentRemote(t *testing.T) {
	_, err := detectUnchecked(context.Background(), DetectRequest{RepoURL: filepath.Join(t.TempDir(), "does-not-exist")})
	if err == nil {
		t.Fatal("detectUnchecked() error = nil, want a clone error for a nonexistent remote")
	}
}

func TestDirSize_StopsAtCap(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), make([]byte, 100), 0o600); err != nil {
		t.Fatalf("write a.bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.bin"), make([]byte, 100), 0o600); err != nil {
		t.Fatalf("write b.bin: %v", err)
	}

	size, err := dirSize(dir, 50)
	if err != nil {
		t.Fatalf("dirSize() error = %v", err)
	}
	if size <= 50 {
		t.Errorf("dirSize() = %d, want > cap (50) once exceeded", size)
	}
}

func TestDirSize_UnderCap(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), make([]byte, 10), 0o600); err != nil {
		t.Fatalf("write a.bin: %v", err)
	}

	size, err := dirSize(dir, 1000)
	if err != nil {
		t.Fatalf("dirSize() error = %v", err)
	}
	if size != 10 {
		t.Errorf("dirSize() = %d, want 10", size)
	}
}
