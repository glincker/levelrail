package gitlabapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestListCommitStatuses_PrefixPaginationAndAuth(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.EscapedPath())
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		n := statusPageSize
		if page == 2 {
			n = 1
		}
		_, _ = w.Write([]byte("["))
		for i := 0; i < n; i++ {
			if i > 0 {
				_, _ = w.Write([]byte(","))
			}
			_, _ = fmt.Fprintf(w, `{"name":"job-%d-%d","status":"success"}`, page, i)
		}
		_, _ = w.Write([]byte("]"))
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	got, err := c.ListCommitStatuses(context.Background(), srv.URL+"/gitlab/", "tok", "grp/sub/web", "abc123")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(got) != statusPageSize+1 {
		t.Fatalf("len = %d", len(got))
	}
	want := "/gitlab/api/v4/projects/grp%2Fsub%2Fweb/repository/commits/abc123/statuses"
	if len(paths) != 2 || paths[0] != want {
		t.Errorf("paths = %v", paths)
	}
}

func TestListCommitStatuses_ErrorsFailClosed(t *testing.T) {
	for _, code := range []int{401, 404, 500, 302} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", "http://other.invalid/")
			w.WriteHeader(code)
		}))
		c := &Client{HTTP: srv.Client()}
		_, err := c.ListCommitStatuses(context.Background(), srv.URL, "tok", "a/b", "sha")
		srv.Close()
		var apiErr *apiError
		if !errors.As(err, &apiErr) || apiErr.StatusCode != code {
			t.Errorf("status %d: err = %v", code, err)
		}
	}
}

func TestListCommitStatuses_TruncatedListIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("["))
		for i := 0; i < statusPageSize; i++ {
			if i > 0 {
				_, _ = w.Write([]byte(","))
			}
			_, _ = w.Write([]byte(`{"name":"j","status":"success"}`))
		}
		_, _ = w.Write([]byte("]"))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client()}
	if _, err := c.ListCommitStatuses(context.Background(), srv.URL, "t", "a/b", "s"); !errors.Is(err, ErrTooManyStatuses) {
		t.Errorf("err = %v", err)
	}
}
