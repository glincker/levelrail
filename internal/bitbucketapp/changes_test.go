package bitbucketapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPullRequestChangedFilesFollowsPages(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`{"values":[{"new":{"path":"b"},"old":{"path":"b"}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"values":[{"new":{"path":"a"},"old":{"path":"old-a"}},{"new":null,"old":{"path":"gone"}}],"next":"` + srv.URL + `/repositories/ws/repo/pullrequests/5/diffstat?page=2"}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), APIBaseURL: srv.URL}
	got, err := c.PullRequestChangedFiles(context.Background(), "tok", "ws/repo", 5)
	if err != nil || strings.Join(got, ",") != "a,old-a,gone,b" {
		t.Fatalf("files = %v, err %v", got, err)
	}
}

func TestCompareChangedFiles(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.EscapedPath(), "/diffstat/") {
			t.Errorf("path = %s", r.URL.EscapedPath())
		}
		_, _ = w.Write([]byte(`{"values":[{"new":{"path":"x"},"old":{"path":"x"}}]}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), APIBaseURL: srv.URL}
	got, err := c.CompareChangedFiles(context.Background(), "tok", "ws/repo", "h1", "b1")
	if err != nil || strings.Join(got, ",") != "x" {
		t.Fatalf("files = %v, err %v", got, err)
	}
}

func TestChangedFilesTruncatedWhenPagesNeverEnd(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"values":[{"new":{"path":"a"},"old":{"path":"a"}}],"next":"` + srv.URL + `/repositories/ws/repo/pullrequests/5/diffstat"}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), APIBaseURL: srv.URL}
	if _, err := c.PullRequestChangedFiles(context.Background(), "tok", "ws/repo", 5); !errors.Is(err, ErrChangedFilesTruncated) {
		t.Fatalf("err = %v, want ErrChangedFilesTruncated", err)
	}
}
