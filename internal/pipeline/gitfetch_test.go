package pipeline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type testRepo struct {
	t    *testing.T
	dir  string
	repo *git.Repository
}

func newTestRepo(t *testing.T) *testRepo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available for the file transport")
	}
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	return &testRepo{t: t, dir: dir, repo: repo}
}

func (r *testRepo) write(rel, body string) {
	r.t.Helper()
	full := filepath.Join(r.dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		r.t.Fatal(err)
	}
}

func (r *testRepo) commit(msg string) string {
	r.t.Helper()
	wt, err := r.repo.Worktree()
	if err != nil {
		r.t.Fatal(err)
	}
	if _, err := wt.Add("."); err != nil {
		r.t.Fatal(err)
	}
	h, err := wt.Commit(msg, &git.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@example.test", When: time.Now()}})
	if err != nil {
		r.t.Fatal(err)
	}
	return h.String()
}

func TestGitFetcherReadsCommitSHA(t *testing.T) {
	r := newTestRepo(t)
	r.write(".pipelines/ci.yaml", syncYAMLA)
	first := r.commit("first")
	r.write(".pipelines/ci.yaml", syncYAMLB)
	second := r.commit("second")

	tests := []struct {
		name    string
		sha     string
		wantSHA string
		wantYML string
	}{
		{"older commit reads older files", first, first, syncYAMLA},
		{"newer commit reads newer files", second, second, syncYAMLB},
		{"empty sha reads the tip", "", second, syncYAMLB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GitFetcher{}.FetchFiles(context.Background(), r.dir, "", "", tt.sha, []string{".pipelines"})
			if err != nil {
				t.Fatal(err)
			}
			if got.SHA != tt.wantSHA || string(got.Files["ci.yaml"]) != tt.wantYML {
				t.Fatalf("got sha %s files %q, want sha %s files %q", got.SHA, got.Files["ci.yaml"], tt.wantSHA, tt.wantYML)
			}
		})
	}
}

func TestGitFetcherUnavailableSHA(t *testing.T) {
	r := newTestRepo(t)
	r.write(".pipelines/ci.yaml", syncYAMLA)
	r.commit("first")
	tests := []struct {
		name string
		sha  string
	}{
		{"unknown commit", strings.Repeat("a", 40)},
		{"not a hash", "main"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := GitFetcher{}.FetchFiles(context.Background(), r.dir, "", "", tt.sha, []string{".pipelines"})
			if !errors.Is(err, ErrSHAUnavailable) {
				t.Fatalf("err = %v, want ErrSHAUnavailable (must not fall back to the tip)", err)
			}
		})
	}
}

func TestGitFetcherEnforcesLimitsWhileReading(t *testing.T) {
	tests := []struct {
		name         string
		files        int
		bigFiles     []string
		wantFiles    int
		wantOversize []string
	}{
		{"at the cap", maxSyncFiles, nil, maxSyncFiles, nil},
		{"stops one past the cap", maxSyncFiles + 30, nil, maxSyncFiles + 1, nil},
		{"oversize file is not read", 3, []string{"f01.yaml"}, 2, []string{"f01.yaml"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRepo(t)
			for i := 0; i < tt.files; i++ {
				r.write(fmt.Sprintf(".pipelines/f%02d.yaml", i), syncYAMLA)
			}
			for _, name := range tt.bigFiles {
				r.write(".pipelines/"+name, "# "+strings.Repeat("x", maxSyncFileSize+1)+"\n")
			}
			r.commit("init")
			got, err := GitFetcher{}.FetchFiles(context.Background(), r.dir, "", "", "", []string{".pipelines"})
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Files) != tt.wantFiles {
				t.Fatalf("files = %d, want %d", len(got.Files), tt.wantFiles)
			}
			if len(got.Oversize) != len(tt.wantOversize) {
				t.Fatalf("oversize = %v, want %v", got.Oversize, tt.wantOversize)
			}
			for _, name := range tt.wantOversize {
				if _, ok := got.Oversize[name]; !ok {
					t.Fatalf("oversize = %v, want %s", got.Oversize, name)
				}
			}
		})
	}
}
