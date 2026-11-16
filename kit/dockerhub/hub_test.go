package dockerhub

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testUser   = "acme"
	testToken  = "dckr_pat_secret-value" //nolint:gosec // fake fixture
	testDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

// fakeJWT builds an unsigned token whose access claim the client reads.
func fakeJWT(access []Access) string {
	b64 := func(v any) string {
		raw, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	return b64(map[string]string{"alg": "none"}) + "." + b64(map[string]any{"access": access}) + ".sig"
}

type fakeHub struct {
	srv         *httptest.Server
	tokenCalls  atomic.Int32
	limitStatus int
	repoStatus  int
	tagsStatus  int
	remaining   string
}

func newFakeHub(t *testing.T) (*fakeHub, *Client) {
	t.Helper()
	f := &fakeHub{limitStatus: http.StatusOK, repoStatus: http.StatusOK, tagsStatus: http.StatusOK, remaining: "187;w=21600"}
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		f.tokenCalls.Add(1)
		u, p, ok := r.BasicAuth()
		if ok && (u != testUser || p != testToken) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("service") != "registry.docker.io" {
			t.Errorf("service = %q", r.URL.Query().Get("service"))
		}
		scope := r.URL.Query().Get("scope")
		name := strings.TrimSuffix(strings.TrimPrefix(scope, "repository:"), ":pull")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":      fakeJWT([]Access{{Type: "repository", Name: name, Actions: []string{"pull"}}}),
			"expires_in": 300,
		})
	})
	mux.HandleFunc("/v2/auth/token", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["identifier"] != testUser || body["secret"] != testToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"hub-bearer"}`))
	})
	mux.HandleFunc("/v2/namespaces/acme/repositories", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer hub-bearer" {
			t.Errorf("repo list authorization = %q", r.Header.Get("Authorization"))
		}
		if f.repoStatus != http.StatusOK {
			w.WriteHeader(f.repoStatus)
			return
		}
		_, _ = w.Write([]byte(`{"count":2,"next":null,"results":[
			{"name":"app","namespace":"acme","is_private":true,"description":"internal","star_count":0,"pull_count":12},
			{"name":"tool","namespace":"acme","is_private":false}]}`))
	})
	mux.HandleFunc("/v2/namespaces/acme/repositories/app/tags", func(w http.ResponseWriter, _ *http.Request) {
		if f.tagsStatus != http.StatusOK {
			w.WriteHeader(f.tagsStatus)
			return
		}
		_, _ = w.Write([]byte(`{"count":1,"next":"https://hub.docker.com/next","results":[
			{"name":"v1","digest":"` + testDigest + `","full_size":1234,"last_updated":"2026-01-01T00:00:00Z","tag_last_pushed":"2026-01-02T00:00:00Z",
			 "images":[{"architecture":"amd64","os":"linux","digest":"sha256:a","size":600},
			           {"architecture":"arm64","os":"linux","variant":"v8","digest":"sha256:b","size":634},
			           {"architecture":"unknown","os":"unknown","digest":"sha256:c","size":10}]}]}`))
	})
	mux.HandleFunc("/v2/ratelimitpreview/test/manifests/latest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method = %s, want HEAD", r.Method)
		}
		w.Header().Set("RateLimit-Limit", "200;w=21600")
		w.Header().Set("RateLimit-Remaining", f.remaining)
		w.Header().Set("Docker-RateLimit-Source", "203.0.113.5")
		w.WriteHeader(f.limitStatus)
	})
	mux.HandleFunc("/v2/acme/app/manifests/v1", func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept"), "application/vnd.oci.image.index.v1+json") {
			t.Errorf("accept = %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Docker-Content-Digest", testDigest)
		w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
	})
	mux.HandleFunc("/v2/acme/limited/manifests/v1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "42")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	mux.HandleFunc("/v2/acme/missing/manifests/v1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f, &Client{HTTP: f.srv.Client(), BaseURL: f.srv.URL, AuthURL: f.srv.URL, RegistryURL: f.srv.URL}
}

var goodCred = Credentials{Username: testUser, Token: testToken}

func TestCredentialsNeverPrintToken(t *testing.T) {
	for _, s := range []string{fmt.Sprintf("%v", goodCred), fmt.Sprintf("%+v", goodCred), fmt.Sprintf("%#v", goodCred), goodCred.String()} {
		if strings.Contains(s, "secret") {
			t.Fatalf("token leaked: %s", s)
		}
	}
	orgCred := Credentials{Username: "o", Token: "dckr_oat_x"} //nolint:gosec // fake fixture
	if orgCred.Kind() != KindOrganization || goodCred.Kind() != KindPersonal {
		t.Fatal("kind inference wrong")
	}
}

func TestPullLimitsReadsHeadersAndCaches(t *testing.T) {
	f, c := newFakeHub(t)
	lim, err := c.PullLimits(context.Background(), goodCred)
	if err != nil {
		t.Fatal(err)
	}
	if !lim.Known || lim.Limit != 200 || lim.Remaining != 187 || lim.WindowSeconds != 21600 || lim.Source != "203.0.113.5" || !lim.Authenticated {
		t.Fatalf("limits = %+v", lim)
	}
	_, _, cached, err := c.CachedPullLimits(context.Background(), goodCred, time.Minute, false)
	if err != nil || cached {
		t.Fatalf("first cached call cached=%v err=%v", cached, err)
	}
	_, _, cached, _ = c.CachedPullLimits(context.Background(), goodCred, time.Minute, false)
	if !cached {
		t.Fatal("second call should come from cache")
	}
	_ = f
}

func TestPullLimits429WithHeadersMeansZeroRemaining(t *testing.T) {
	f, c := newFakeHub(t)
	f.limitStatus, f.remaining = http.StatusTooManyRequests, "0;w=21600"
	lim, err := c.PullLimits(context.Background(), goodCred)
	if err != nil || lim.Remaining != 0 || !lim.Known {
		t.Fatalf("lim=%+v err=%v", lim, err)
	}
}

func TestPullLimitsUnknownWhenNoHeaders(t *testing.T) {
	if lim := ParseRateLimit(http.Header{}); lim.Known {
		t.Fatalf("empty headers should be unknown: %+v", lim)
	}
}

func TestBadTokenIsUnauthorized(t *testing.T) {
	_, c := newFakeHub(t)
	_, err := c.Validate(context.Background(), Credentials{Username: testUser, Token: "wrong"})
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if strings.Contains(fmt.Sprint(err), "wrong") {
		t.Fatal("error leaks the token")
	}
}

func TestValidateReportsScopesAndCapabilities(t *testing.T) {
	_, c := newFakeHub(t)
	v, err := c.Validate(context.Background(), goodCred)
	if err != nil {
		t.Fatal(err)
	}
	if v.Kind != KindPersonal || !v.PrivateReposVisible || len(v.Scopes) == 0 || v.Scopes[0] != "repository:ratelimitpreview/test:pull" {
		t.Fatalf("validation = %+v", v)
	}
	want := map[string]string{"registry_login": StatusOK, "hub_api": StatusOK, "repository_list": StatusOK, "tag_read": StatusOK, "image_pull": StatusOK}
	for _, cp := range v.Capabilities {
		if want[cp.Name] != cp.Status {
			t.Errorf("capability %s = %s", cp.Name, cp.Status)
		}
		delete(want, cp.Name)
	}
	if len(want) != 0 {
		t.Errorf("missing capabilities: %v", want)
	}
}

func TestValidateScopeMissingIsDenied(t *testing.T) {
	f, c := newFakeHub(t)
	f.repoStatus = http.StatusForbidden
	v, err := c.Validate(context.Background(), goodCred)
	if err != nil {
		t.Fatal(err)
	}
	for _, cp := range v.Capabilities {
		if cp.Name == "repository_list" && cp.Status != StatusDenied {
			t.Fatalf("repository_list = %s, want denied", cp.Status)
		}
	}
}

func TestListTagsPageFields(t *testing.T) {
	_, c := newFakeHub(t)
	page, err := c.ListTagsPage(context.Background(), goodCred, "acme", "app", 1, 25)
	if err != nil {
		t.Fatal(err)
	}
	if !page.HasNext || len(page.Tags) != 1 {
		t.Fatalf("page = %+v", page)
	}
	tg := page.Tags[0]
	if tg.Digest != testDigest || tg.FullSize != 1234 || tg.LastPushed != "2026-01-02T00:00:00Z" || len(tg.Images) != 2 ||
		strings.Join(tg.Architectures, ",") != "linux/amd64,linux/arm64/v8" {
		t.Fatalf("tag = %+v", tg)
	}
}

func TestListTagsPageScopeMissing(t *testing.T) {
	f, c := newFakeHub(t)
	f.tagsStatus = http.StatusForbidden
	oat := Credentials{Username: testUser, Token: testToken}
	_, err := c.ListTagsPage(context.Background(), oat, "acme", "app", 1, 25)
	// The personal token falls back to the legacy route, which the fake lacks.
	if err == nil {
		t.Fatal("want an error")
	}
}

func TestListRepositoriesPrivateFlag(t *testing.T) {
	_, c := newFakeHub(t)
	page, err := c.ListRepositories(context.Background(), goodCred, "acme", "", 1, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Repositories) != 2 || !page.Repositories[0].IsPrivate || page.Repositories[0].FullName != "acme/app" || page.HasNext {
		t.Fatalf("page = %+v", page)
	}
}

func TestResolveDigest(t *testing.T) {
	_, c := newFakeHub(t)
	m, err := c.ResolveDigest(context.Background(), goodCred, "acme", "app", "v1")
	if err != nil || m.Digest != testDigest || !strings.Contains(m.MediaType, "index") {
		t.Fatalf("m=%+v err=%v", m, err)
	}
	if _, err := c.ResolveDigest(context.Background(), goodCred, "acme", "missing", "v1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRateLimited429WithRetryAfter(t *testing.T) {
	_, c := newFakeHub(t)
	_, err := c.ResolveDigest(context.Background(), goodCred, "acme", "limited", "v1")
	var rl *RateLimitedError
	if !errors.Is(err, ErrRateLimited) || !errors.As(err, &rl) || rl.RetryAfter != 42*time.Second {
		t.Fatalf("err = %v", err)
	}
}

func TestRegistryTokenCachedPerScope(t *testing.T) {
	f, c := newFakeHub(t)
	for i := 0; i < 3; i++ {
		if _, err := c.PullLimits(context.Background(), goodCred); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.tokenCalls.Load(); n != 1 {
		t.Fatalf("token endpoint called %d times, want 1", n)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if parseRetryAfter("30", now) != 30*time.Second || parseRetryAfter("", now) != 0 || parseRetryAfter("junk", now) != 0 {
		t.Fatal("seconds form wrong")
	}
	if d := parseRetryAfter(now.Add(90*time.Second).Format(http.TimeFormat), now); d < 89*time.Second || d > 90*time.Second {
		t.Fatalf("date form = %s", d)
	}
}

func TestHostGuardBlocksOtherHosts(t *testing.T) {
	c := NewClient()
	c.BaseURL = "https://evil.example.com"
	if _, err := c.SearchRepositories(context.Background(), "x", 5); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("err = %v, want not allowed", err)
	}
}
