package bitbucketserver

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

func newTestClient(t *testing.T, h http.HandlerFunc) (*Client, string) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &Client{HTTP: srv.Client()}, srv.URL
}

func TestNewClientBlocksLoopback(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer ts.Close()
	_, err := NewClient().CurrentUser(context.Background(), ts.URL, "tok")
	if !errors.Is(err, netguard.ErrBlockedAddress) {
		t.Fatalf("err = %v, want ErrBlockedAddress", err)
	}
}

func TestSplitRepo(t *testing.T) {
	cases := []struct {
		in, project, slug string
		wantErr           bool
	}{
		{"PROJ/app", "PROJ", "app", false},
		{"~jane/app", "~jane", "app", false},
		{"/PROJ/app/", "PROJ", "app", false},
		{"PROJ", "", "", true},
		{"PROJ/a/b", "", "", true},
		{"", "", "", true},
	}
	for _, tc := range cases {
		p, s, err := SplitRepo(tc.in)
		if (err != nil) != tc.wantErr || p != tc.project || s != tc.slug {
			t.Errorf("SplitRepo(%q) = %q, %q, %v", tc.in, p, s, err)
		}
	}
}

func TestCurrentUser(t *testing.T) {
	cases := []struct {
		name       string
		whoamiCode int
		whoami     string
		wantErr    bool
		wantLogin  string
	}{
		{"ok", http.StatusOK, "jane\n", false, "jane"},
		{"unauthorized", http.StatusUnauthorized, "", true, ""},
		{"empty body", http.StatusOK, "", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, base := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer tok" {
					t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
				}
				switch r.URL.Path {
				case "/plugins/servlet/applinks/whoami":
					w.WriteHeader(tc.whoamiCode)
					_, _ = io.WriteString(w, tc.whoami)
				case "/rest/api/1.0/users/jane":
					_, _ = io.WriteString(w, `{"id":42,"slug":"jane"}`)
				default:
					http.NotFound(w, r)
				}
			})
			acct, err := c.CurrentUser(context.Background(), base, "tok")
			if (err != nil) != tc.wantErr || acct.Login != tc.wantLogin {
				t.Fatalf("acct = %+v, err = %v", acct, err)
			}
			if err == nil && acct.ID != "42" {
				t.Errorf("ID = %q, want 42", acct.ID)
			}
		})
	}
}

func TestListReposPagesAndStripsUserinfo(t *testing.T) {
	c, base := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/1.0/repos" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("start") == "0" {
			_, _ = io.WriteString(w, `{"isLastPage":false,"nextPageStart":1,"values":[{"slug":"a","name":"A","public":false,"project":{"key":"P"},
				"links":{"self":[{"href":"https://bb/projects/P/repos/a/browse"}],"clone":[{"name":"ssh","href":"ssh://git@bb/p/a.git"},{"name":"http","href":"https://jane@bb/scm/p/a.git"}]}}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"isLastPage":true,"values":[{"slug":"b","name":"B","public":true,"project":{"key":"P"},"links":{"clone":[]}},{"slug":"old","archived":true,"project":{"key":"P"}}]}`)
	})
	repos, res, err := c.ListRepos(context.Background(), base, "tok", gitprovider.ListOptions{MaxItems: 100})
	if err != nil || res.Truncated {
		t.Fatalf("err = %v, truncated = %v", err, res.Truncated)
	}
	if len(repos) != 2 {
		t.Fatalf("repos = %+v, want 2 (archived skipped)", repos)
	}
	if repos[0].FullName != "P/a" || repos[0].CloneURL != "https://bb/scm/p/a.git" || !repos[0].Private || repos[0].WebURL == "" {
		t.Errorf("repo 0 = %+v", repos[0])
	}
	if repos[1].Private {
		t.Errorf("public repo reported private: %+v", repos[1])
	}
}

func TestGetRepoDefaultBranch(t *testing.T) {
	cases := []struct {
		name       string
		branchCode int
		want       string
		wantErr    bool
	}{
		{"set", http.StatusOK, "trunk", false},
		{"empty repository", http.StatusNotFound, "", false},
		{"server error", http.StatusInternalServerError, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, base := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/rest/api/1.0/projects/P/repos/a":
					_, _ = io.WriteString(w, `{"slug":"a","project":{"key":"P"},"links":{"clone":[{"name":"http","href":"https://bb/scm/p/a.git"}]}}`)
				case "/rest/api/1.0/projects/P/repos/a/default-branch":
					w.WriteHeader(tc.branchCode)
					if tc.branchCode == http.StatusOK {
						_, _ = io.WriteString(w, `{"displayId":"trunk","id":"refs/heads/trunk"}`)
					}
				default:
					http.NotFound(w, r)
				}
			})
			repo, err := c.GetRepo(context.Background(), base, "tok", "P/a")
			if (err != nil) != tc.wantErr || repo.DefaultBranch != tc.want {
				t.Fatalf("repo = %+v, err = %v", repo, err)
			}
		})
	}
}

func TestListBranches(t *testing.T) {
	c, base := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"isLastPage":true,"values":[{"displayId":"main","latestCommit":"abc"},{"displayId":"dev","latestCommit":"def"}]}`)
	})
	bs, err := c.ListBranches(context.Background(), base, "tok", "P/a")
	if err != nil || len(bs) != 2 || bs[1].Name != "dev" || bs[1].CommitSHA != "def" {
		t.Fatalf("branches = %+v, err = %v", bs, err)
	}
	if _, err := c.ListBranches(context.Background(), base, "tok", "bad"); err == nil {
		t.Error("want error for malformed repo")
	}
}

func TestWebhookCreateAndDelete(t *testing.T) {
	var created webhookRequest
	var deleted string
	c, base := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		const hooks = "/rest/api/1.0/projects/P/repos/a/webhooks"
		switch {
		case r.Method == http.MethodPost && r.URL.Path == hooks:
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Errorf("decode: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodGet && r.URL.Path == hooks:
			_, _ = io.WriteString(w, `{"isLastPage":true,"values":[{"id":7,"name":"other","url":"https://cp/h"},{"id":9,"name":"levelrail","url":"https://cp/h"}]}`)
		case r.Method == http.MethodDelete:
			deleted = r.URL.Path
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	ctx := context.Background()
	if err := c.CreateRepoWebhook(ctx, base, "tok", "P/a", "https://cp/h", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if created.URL != "https://cp/h" || created.Configuration["secret"] != "s3cret" || len(created.Events) != 1 || created.Events[0] != "repo:refs_changed" || !created.Active {
		t.Errorf("created = %+v", created)
	}
	if err := c.DeleteRepoWebhook(ctx, base, "tok", "P/a", "https://cp/h"); err != nil {
		t.Fatal(err)
	}
	if deleted != "/rest/api/1.0/projects/P/repos/a/webhooks/9" {
		t.Errorf("deleted = %q, want only our hook (id 9)", deleted)
	}
	deleted = ""
	if err := c.DeleteRepoWebhook(ctx, base, "tok", "P/a", "https://cp/other"); err != nil || deleted != "" {
		t.Errorf("unknown url: err = %v, deleted = %q", err, deleted)
	}
}
