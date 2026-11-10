package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func inlineBody(t *testing.T, dockerfile string, files map[string]string) string {
	t.Helper()
	in := inlineSourceInput{Dockerfile: dockerfile}
	for p, c := range files {
		in.Files = append(in.Files, inlineFileInput{Path: p, Content: c})
	}
	b, err := json.Marshal(triggerBuildRequest{Build: triggerBuildBuildInput{Type: "dockerfile"}, InlineSource: &in})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestTriggerBuild_InlineDockerfileBuildsThroughBuilder(t *testing.T) {
	rt, db, fb, cookie := newUploadRouter(t)
	fb.release = make(chan struct{})
	const dockerfile = "FROM scratch\nCOPY conf/app.conf /\n"
	_, resp := postTriggerBuildAccepted(t, rt, cookie, inlineBody(t, dockerfile, map[string]string{"conf/app.conf": "port=8080"}))
	req := fb.awaitCall(t)
	if b, err := os.ReadFile(filepath.Join(req.SourceDir, "Dockerfile")); err != nil || string(b) != dockerfile {
		t.Errorf("Dockerfile = %q, %v", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(req.SourceDir, "conf", "app.conf")); err != nil || string(b) != "port=8080" {
		t.Errorf("app.conf = %q, %v", b, err)
	}
	if fi, err := os.Stat(req.SourceDir); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("context dir mode = %v, %v, want 0700", fi.Mode(), err)
	}
	close(fb.release)
	a := awaitDeployAttemptFinished(t, db, resp.ID)
	if a.Source != store.DeployAttemptSourceUpload || len(a.CommitSHA) != 40 || !strings.Contains(a.CommitMessage, "Inline Dockerfile, 1 extra files") {
		t.Errorf("attempt source=%q commit=%q message=%q", a.Source, a.CommitSHA, a.CommitMessage)
	}
	// The cleanup defer can run just after the attempt row is finished.
	deadline := time.Now().Add(3 * time.Second)
	for _, err := os.Stat(req.SourceDir); !os.IsNotExist(err); _, err = os.Stat(req.SourceDir) {
		if time.Now().After(deadline) {
			t.Fatalf("build context %s left behind: %v", req.SourceDir, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	entries := inlineAuditEntries(t, db)
	if len(entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(entries))
	}
	if d := entries[0].Detail; strings.Contains(d, "FROM scratch") || strings.Contains(d, "port=8080") || !strings.Contains(d, `"sha256"`) {
		t.Errorf("audit detail leaks or lacks the hash: %s", d)
	}
}

func inlineAuditEntries(t *testing.T, db *store.DB) []store.AuditEntry {
	t.Helper()
	all, err := db.ListAuditEntries(context.Background(), 200, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var out []store.AuditEntry
	for _, e := range all {
		if e.Action == AuditActionInlineSourceDeploy {
			out = append(out, e)
		}
	}
	return out
}

func TestTriggerBuild_InlineConflictWritesNoAudit(t *testing.T) {
	rt, db, _, cookie := newUploadRouter(t)
	if err := db.SaveDeployAttempt(context.Background(), store.DeployAttempt{ID: "att_run", ServiceName: "web", Image: "web:1", Status: store.DeployAttemptStatusRunning, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", inlineBody(t, "FROM scratch", nil)))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
	if got := inlineAuditEntries(t, db); len(got) != 0 {
		t.Fatalf("refused deploy wrote %d inline audit entries", len(got))
	}
}

func TestQueue_InlineBuildReplaysWithCommitMessage(t *testing.T) {
	rt, db, cookie, gb := newQueueRouter(t)
	first := postBuild(t, rt, cookie, "web", "main")
	if gb.awaitStart(t) != first.ID {
		t.Fatal("first build did not start")
	}
	rec := serve(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", inlineBody(t, "FROM scratch\n", map[string]string{"a.txt": "x"})))
	var queued triggerBuildResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &queued); err != nil || rec.Code != http.StatusAccepted || queued.Status != store.DeployAttemptStatusQueued {
		t.Fatalf("inline build = %d %s, want queued", rec.Code, rec.Body.String())
	}
	if got := inlineAuditEntries(t, db); len(got) != 1 {
		t.Fatalf("queued inline deploy audit entries = %d, want 1", len(got))
	}
	gb.release <- struct{}{}
	if got := gb.awaitStart(t); got != queued.ID {
		t.Fatalf("next build = %s, want %s", got, queued.ID)
	}
	gb.release <- struct{}{}
	a := awaitAttemptStatus(t, db, queued.ID, store.DeployAttemptStatusSucceeded)
	if a.Source != store.DeployAttemptSourceUpload || !strings.Contains(a.CommitMessage, "Inline Dockerfile, 1 extra files") {
		t.Fatalf("replayed attempt source=%q message=%q", a.Source, a.CommitMessage)
	}
}

func TestTriggerBuild_InlineDockerfileRejects(t *testing.T) {
	rt, _, _, cookie := newUploadRouter(t)
	tooMany := map[string]string{}
	for i := range 21 {
		tooMany["f"+strings.Repeat("x", i)] = "x"
	}
	cases := []struct {
		name string
		body string
		want int
	}{
		{"over byte cap", inlineBody(t, "FROM scratch\n"+strings.Repeat("#", 300<<10), nil), http.StatusRequestEntityTooLarge},
		{"over file cap", inlineBody(t, "FROM scratch", tooMany), http.StatusRequestEntityTooLarge},
		{"traversal", inlineBody(t, "FROM scratch", map[string]string{"../../etc/x": "x"}), http.StatusBadRequest},
		{"absolute", inlineBody(t, "FROM scratch", map[string]string{"/etc/x": "x"}), http.StatusBadRequest},
		{"empty", inlineBody(t, "  ", nil), http.StatusBadRequest},
		{"with repo", `{"repo_url":"https://example.com/a.git","ref":"main","inline_source":{"dockerfile":"FROM scratch"}}`, http.StatusBadRequest},
		{"wrong type", `{"build":{"type":"railpack"},"inline_source":{"dockerfile":"FROM scratch"}}`, http.StatusBadRequest},
		{"forged inline url", `{"repo_url":"inline://abc","ref":"abc"}`, http.StatusBadRequest},
		{"case collision", inlineBody(t, "FROM scratch", map[string]string{"a.txt": "1", "A.txt": "2"}), http.StatusBadRequest},
		{"nfd collision", inlineBody(t, "FROM scratch", map[string]string{"café": "1", "café": "2"}), http.StatusBadRequest},
		{"file and dir", inlineBody(t, "FROM scratch", map[string]string{"a": "1", "a/b": "2"}), http.StatusBadRequest},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/builds", c.body))
		if rec.Code != c.want {
			t.Errorf("%s: status = %d, want %d; body = %s", c.name, rec.Code, c.want, rec.Body.String())
		}
	}
}
