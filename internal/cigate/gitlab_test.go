package cigate

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/gitlabapp"
)

// fakeGitLab answers statuses under prefix; the body is the only source of truth.
func fakeGitLab(t *testing.T, prefix string, code int, statuses string) *GitLab {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, prefix+"/api/v4/") || r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if page := r.URL.Query().Get("page"); page != "" && page != "1" {
			_, _ = w.Write([]byte("[]"))
			return
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte(statuses))
	}))
	t.Cleanup(srv.Close)
	return &GitLab{Client: &gitlabapp.Client{HTTP: srv.Client()}, InstanceURL: srv.URL + prefix, Token: "tok", ProjectPath: "grp/web"}
}

// gate runs Wait on a fake clock so timeouts take no real time.
func gate(t *testing.T, p Provider, pol Policy) Result {
	t.Helper()
	now := time.Unix(0, 0)
	cfg := Config{
		Timeout: 3 * time.Second, PollInterval: time.Second, NoChecksWait: time.Second,
		Now:   func() time.Time { return now },
		Sleep: func(_ context.Context, d time.Duration) error { now = now.Add(d); return nil },
	}
	res, err := Wait(context.Background(), p, "abc", pol, cfg)
	if err != nil {
		t.Fatalf("Wait error = %v", err)
	}
	return res
}

func TestGitLab_Verdicts(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		body     string
		pol      Policy
		want     Outcome
		failing  string
		prefix   string
		wantText string
	}{
		{name: "all pass", code: 200, body: `[{"name":"build","status":"success"},{"name":"test","status":"success"}]`, pol: Policy{Required: []string{"build", "test"}}, want: OutcomePass},
		{name: "one failed", code: 200, body: `[{"name":"build","status":"success"},{"name":"test","status":"failed"}]`, want: OutcomeFail, failing: "test"},
		{name: "allowed failure passes", code: 200, body: `[{"name":"lint","status":"failed","allow_failure":true}]`, want: OutcomePass},
		{name: "canceled fails", code: 200, body: `[{"name":"test","status":"canceled"}]`, want: OutcomeFail, failing: "test"},
		{name: "running never passes", code: 200, body: `[{"name":"build","status":"success"},{"name":"test","status":"running"}]`, want: OutcomeTimeout},
		{name: "unknown state never passes", code: 200, body: `[{"name":"test","status":"brand_new"}]`, want: OutcomeTimeout},
		{name: "manual blocks", code: 200, body: `[{"name":"deploy-gate","status":"manual"}]`, want: OutcomeTimeout},
		{name: "required missing", code: 200, body: `[{"name":"build","status":"success"}]`, pol: Policy{Required: []string{"build", "test"}}, want: OutcomeTimeout},
		{name: "unauthorized", code: 401, body: `{"message":"401 Unauthorized"}`, want: OutcomeTimeout},
		{name: "not found", code: 404, body: `{"message":"404 Not Found"}`, want: OutcomeTimeout},
		{name: "server error", code: 500, body: `oops`, want: OutcomeTimeout},
		{name: "self-managed prefix", code: 200, body: `[{"name":"build","status":"success"}]`, prefix: "/gitlab", want: OutcomePass},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := fakeGitLab(t, tt.prefix, tt.code, tt.body)
			res := gate(t, g, tt.pol)
			if res.Outcome != tt.want {
				t.Fatalf("outcome = %q, want %q (%+v)", res.Outcome, tt.want, res)
			}
			if tt.failing != "" && (len(res.Failing) != 1 || res.Failing[0] != tt.failing) {
				t.Errorf("failing = %v", res.Failing)
			}
		})
	}
}

func TestGitLab_PaginationReadsEveryPage(t *testing.T) {
	var gotPages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		gotPages = append(gotPages, page)
		n := 100
		if page == "2" {
			n = 1
		}
		var b strings.Builder
		b.WriteString("[")
		for i := 0; i < n; i++ {
			if i > 0 {
				b.WriteString(",")
			}
			st := "success"
			if page == "2" {
				st = "failed"
			}
			fmt.Fprintf(&b, `{"name":"job-%s-%d","status":"%s"}`, page, i, st)
		}
		b.WriteString("]")
		_, _ = w.Write([]byte(b.String()))
	}))
	defer srv.Close()
	g := &GitLab{Client: &gitlabapp.Client{HTTP: srv.Client()}, InstanceURL: srv.URL, Token: "tok", ProjectPath: "grp/web"}
	res := gate(t, g, Policy{})
	if res.Outcome != OutcomeFail || len(gotPages) != 2 || gotPages[1] != strconv.Itoa(2) {
		t.Errorf("res = %+v pages = %v", res, gotPages)
	}
}

// The webhook body is never an input: only the API answer for the SHA counts.
func TestGitLab_WebhookClaimingSuccessLosesToAPI(t *testing.T) {
	g := fakeGitLab(t, "", 200, `[{"name":"test","status":"failed"}]`)
	res := gate(t, g, Policy{Required: []string{"test"}})
	if res.Outcome != OutcomeFail {
		t.Fatalf("outcome = %q", res.Outcome)
	}
}

func TestGitLab_RedirectIsNotFollowed(t *testing.T) {
	var leaked bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		leaked = true
		_, _ = w.Write([]byte(`[{"name":"x","status":"success"}]`))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer srv.Close()
	g := &GitLab{Client: &gitlabapp.Client{HTTP: srv.Client()}, InstanceURL: srv.URL, Token: "tok", ProjectPath: "a/b"}
	if _, err := g.Checks(context.Background(), "abc"); err == nil || leaked {
		t.Fatalf("err = %v leaked = %v", err, leaked)
	}
}
