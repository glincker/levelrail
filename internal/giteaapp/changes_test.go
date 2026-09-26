package giteaapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fileList(n int) string {
	items := make([]string, n)
	for i := range items {
		items[i] = `{"filename":"f.txt"}`
	}
	return "[" + strings.Join(items, ",") + "]"
}

func TestPullRequestChangedFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/repos/o/r/pulls/3/files" {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"filename":"a.go"},{"filename":"b.go","previous_filename":"old.go"}]`))
	}))
	defer srv.Close()
	got, err := (&Client{HTTP: srv.Client()}).PullRequestChangedFiles(context.Background(), srv.URL, "tok", "o/r", 3)
	if err != nil || strings.Join(got, ",") != "a.go,b.go,old.go" {
		t.Fatalf("files = %v, err %v", got, err)
	}
}

func TestPullRequestChangedFilesTruncated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(fileList(listPerPage)))
	}))
	defer srv.Close()
	_, err := (&Client{HTTP: srv.Client()}).PullRequestChangedFiles(context.Background(), srv.URL, "tok", "o/r", 3)
	if !errors.Is(err, ErrChangedFilesTruncated) {
		t.Fatalf("err = %v, want ErrChangedFilesTruncated", err)
	}
}

func TestCompareChangedFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/repos/o/r/compare/b1...h1") {
			t.Errorf("path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"commits":[{"files":[{"filename":"a"}]},{"files":[{"filename":"b"}]}]}`))
	}))
	defer srv.Close()
	got, err := (&Client{HTTP: srv.Client()}).CompareChangedFiles(context.Background(), srv.URL, "tok", "o/r", "b1", "h1")
	if err != nil || strings.Join(got, ",") != "a,b" {
		t.Fatalf("files = %v, err %v", got, err)
	}
}
