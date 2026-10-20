package githubapp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func filesJSON(n int) string {
	items := make([]string, n)
	for i := range items {
		items[i] = `{"filename":"f.txt"}`
	}
	return strings.Join(items, ",")
}

func TestCompareChangedFilesPaginates(t *testing.T) {
	page := 0
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/repos/o/r/compare/b1...h1") {
			t.Errorf("path = %s", r.URL.Path)
		}
		page++
		if page == 1 {
			_, _ = w.Write([]byte(`{"files":[` + filesJSON(100) + `]}`))
			return
		}
		_, _ = w.Write([]byte(`{"files":[{"filename":"last.go","previous_filename":"was.go"}]}`))
	})
	got, err := c.CompareChangedFiles(context.Background(), "", "tok", "o", "r", "b1", "h1")
	if err != nil || len(got) != 102 || got[100] != "last.go" || got[101] != "was.go" {
		t.Fatalf("got %d files (err %v)", len(got), err)
	}
}

func TestChangedFilesTruncatedAtThePageCap(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/compare/") {
			_, _ = w.Write([]byte(`{"files":[` + filesJSON(100) + `]}`))
			return
		}
		_, _ = w.Write([]byte(`[` + filesJSON(100) + `]`))
	})
	if _, err := c.CompareChangedFiles(context.Background(), "", "tok", "o", "r", "b", "h"); !errors.Is(err, ErrChangedFilesTruncated) {
		t.Fatalf("compare err = %v, want ErrChangedFilesTruncated", err)
	}
	if _, err := c.PullRequestFiles(context.Background(), "", "tok", "o", "r", 9); !errors.Is(err, ErrChangedFilesTruncated) {
		t.Fatalf("pull request err = %v, want ErrChangedFilesTruncated", err)
	}
}

func TestPullRequestFiles(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/pulls/9/files" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"filename":"a.ts"}]`))
	})
	got, err := c.PullRequestFiles(context.Background(), "", "tok", "o", "r", 9)
	if err != nil || len(got) != 1 || got[0] != "a.ts" {
		t.Fatalf("got %v, err %v", got, err)
	}
}

func TestDeploymentRequests(t *testing.T) {
	var paths []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/deployments") {
			_, _ = w.Write([]byte(`{"id":5}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	})
	id, err := c.CreateDeployment(context.Background(), "", "tok", "o", "r", "sha", "production", "d", true)
	if err != nil || id != 5 {
		t.Fatalf("CreateDeployment = %d, %v", id, err)
	}
	if err := c.CreateDeploymentStatus(context.Background(), "", "tok", "o", "r", id, DeploymentSuccess, "https://x", "", "ok"); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "POST /repos/o/r/deployments" || paths[1] != "POST /repos/o/r/deployments/5/statuses" {
		t.Fatalf("paths = %v", paths)
	}
}

func TestCreateDeploymentWithoutIDFails(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) })
	if _, err := c.CreateDeployment(context.Background(), "", "tok", "o", "r", "sha", "production", "d", true); err == nil {
		t.Fatal("a response without an id was accepted")
	}
}
