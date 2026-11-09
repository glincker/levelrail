package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Sections whose body carries a clock or uptime are checked for presence only.
var shellVolatileSections = map[string]bool{"system_status": true, "session": true}

type shellCaller struct {
	cookie *http.Cookie
	token  string
}

func (c shellCaller) get(h http.Handler, path string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, path, nil)
	if c.cookie != nil {
		r.AddCookie(c.cookie)
	}
	if c.token != "" {
		r.Header.Set("Authorization", "Bearer "+c.token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func compactJSON(t *testing.T, b []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, bytes.TrimSpace(b)); err != nil {
		t.Fatalf("compact: %v (%s)", err, b)
	}
	return buf.String()
}

// assertShellMatchesEndpoints proves each section is present exactly when the
// caller's own GET of that endpoint succeeds, and carries the same body.
func assertShellMatchesEndpoints(t *testing.T, h http.Handler, c shellCaller) map[string]json.RawMessage {
	t.Helper()
	rec := c.get(h, "/api/v1/shell")
	if rec.Code != http.StatusOK {
		t.Fatalf("shell = %d %s", rec.Code, rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cc)
	}
	if et := rec.Header().Get("ETag"); et != "" {
		t.Fatalf("ETag = %q, want none on a no-store body", et)
	}
	if v := rec.Header().Values("Vary"); !strings.Contains(strings.Join(v, ","), "Cookie") || !strings.Contains(strings.Join(v, ","), "Authorization") {
		t.Fatalf("Vary = %v, want Cookie and Authorization", v)
	}
	var got shellOverviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, sec := range shellSections {
		one := c.get(h, sec.path)
		body, present := got.Sections[sec.name]
		if (one.Code == http.StatusOK) != present {
			t.Errorf("section %s present=%v but GET %s = %d", sec.name, present, sec.path, one.Code)
			continue
		}
		if present && !shellVolatileSections[sec.name] && compactJSON(t, body) != compactJSON(t, one.Body.Bytes()) {
			t.Errorf("section %s differs from GET %s:\n shell %s\n route %s", sec.name, sec.path, body, one.Body.String())
		}
	}
	return got.Sections
}

func TestShellOverview_AdminMatchesEndpoints(t *testing.T) {
	rt, db := newTestRouter(t)
	admin := loginTestSession(t, rt, db)
	secs := assertShellMatchesEndpoints(t, rt.Handler(), shellCaller{cookie: admin})
	for _, k := range []string{"brand", "session", "projects", "teams"} {
		if _, ok := secs[k]; !ok {
			t.Errorf("admin shell missing %q", k)
		}
	}
}

func TestShellOverview_TeamMemberGetsOnlyOwnScope(t *testing.T) {
	f := newTeamFixture(t)
	h := f.rt.Handler()
	for _, role := range allTeamRoles {
		t.Run(role, func(t *testing.T) {
			secs := assertShellMatchesEndpoints(t, h, shellCaller{cookie: f.cookies[role]})
			for name, body := range secs {
				if strings.Contains(string(body), teamB) || strings.Contains(string(body), projB) {
					t.Errorf("section %s leaks team B: %s", name, body)
				}
			}
			if p, ok := secs["projects"]; ok && !strings.Contains(string(p), projA) {
				t.Errorf("projects section misses own project: %s", p)
			}
		})
	}
}

func TestShellOverview_ReadOnlyTokenSeesOnlyItsSections(t *testing.T) {
	rt, db := newTestRouter(t)
	_ = loginTestSession(t, rt, db)
	plain := "shell-overview-read-token-1"
	if err := db.SaveAPIToken(context.Background(), store.APIToken{ID: "tok_shell_read", Name: "shell", TokenHash: hashToken(plain), Abilities: []string{AbilityRead}}); err != nil {
		t.Fatal(err)
	}
	h := rt.Handler()
	assertShellMatchesEndpoints(t, h, shellCaller{token: plain})
	if rec := (shellCaller{}).get(h, "/api/v1/shell"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous shell = %d, want 401", rec.Code)
	}
}

func TestShellOverview_SlowSectionIsOmitted(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/demo" {
			<-release
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	s := &shellOverview{inner: inner, timeout: func(*http.Request) time.Duration { return 50 * time.Millisecond }}
	rec := httptest.NewRecorder()
	start := time.Now()
	s.handleShellOverview(rec, httptest.NewRequest(http.MethodGet, "/api/v1/shell", nil))
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("slow section held the response for %s", took)
	}
	var got shellOverviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Sections["demo"]; ok {
		t.Fatal("section past its deadline must be omitted")
	}
	if _, ok := got.Sections["brand"]; !ok {
		t.Fatalf("fast sections must stay: %s", rec.Body.String())
	}
}
