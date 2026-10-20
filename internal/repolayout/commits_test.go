package repolayout

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCommitsAndBranchesPerProvider(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/app/commits", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("sha") != "main" || r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`[{"sha":"aaa","html_url":"u","commit":{"message":"fix: a\n\nbody","author":{"name":"Ann","date":"2026-01-02T03:04:05Z"}}}]`))
	})
	mux.HandleFunc("/repos/acme/app/branches", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"main"},{"name":"dev"}]`))
	})
	mux.HandleFunc("/projects/acme%2Fapp/repository/commits", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ref_name") != "main" {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`[{"id":"bbb","title":"feat: b","author_name":"Bob","authored_date":"2026-01-02T03:04:05Z","web_url":"w"}]`))
	})
	mux.HandleFunc("/projects/acme%2Fapp/repository/branches", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"main"}]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tests := []struct {
		name    string
		src     CommitSource
		wantSHA string
		author  string
	}{
		{"github", &githubSource{c: srv.Client(), api: srv.URL, repo: "acme/app", ref: "main", token: "tok"}, "aaa", "Ann"},
		{"gitlab", &gitlabSource{c: srv.Client(), api: srv.URL, repo: "acme/app", ref: "main"}, "bbb", "Bob"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			commits, err := tc.src.Commits(context.Background(), 10)
			if err != nil {
				t.Fatalf("Commits: %v", err)
			}
			if len(commits) != 1 || commits[0].SHA != tc.wantSHA || commits[0].Author != tc.author {
				t.Fatalf("commits = %+v", commits)
			}
			if commits[0].AuthoredAt.IsZero() {
				t.Fatal("authored_at not parsed")
			}
			branches, err := tc.src.Branches(context.Background())
			if err != nil || len(branches) == 0 {
				t.Fatalf("Branches = %v, %v", branches, err)
			}
		})
	}
}

func TestFirstLineTrimsBody(t *testing.T) {
	if got := firstLine("  subject \n\nbody"); got != "subject" {
		t.Fatalf("firstLine = %q", got)
	}
}
