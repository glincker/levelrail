package api

import (
	"net/http"
	"strings"
	"testing"
)

type fakeBreaker struct{ held map[string]int }

func (f *fakeBreaker) Resume(name string) int {
	n := f.held[name]
	delete(f.held, name)
	return n
}

func TestResumeAppReleasesQuarantine(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.SetCrashBreaker(&fakeBreaker{held: map[string]int{"web": 2}})
	seedApp(t, db, "web")

	rec := doDB(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/resume", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"resumed":2`) {
		t.Fatalf("resume = %d %s", rec.Code, rec.Body.String())
	}
	rec = doDB(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/resume", "")
	if !strings.Contains(rec.Body.String(), `"resumed":0`) {
		t.Fatalf("second resume = %s, want 0", rec.Body.String())
	}
	if got := doDB(t, rt, cookie, http.MethodPost, "/api/v1/apps/nope/resume", "").Code; got != http.StatusNotFound {
		t.Fatalf("unknown app = %d, want 404", got)
	}
}
