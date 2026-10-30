package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
)

func stackPlanRaw(t *testing.T, rt *Router, cookie *http.Cookie, docs string, extra map[string]any) string {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/plan", stackBody(t, docs, extra)))
	if rec.Code != http.StatusOK {
		t.Fatalf("plan = %d %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func TestStackSecretSurvivesReapplyAndRestartAndNeverLeaks(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	logs := &lockedBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	rt := NewRouter(logger, testBrand(), db, WithSecretSetter(secrets.NewManager(db, mk)))
	cookie := stackSeed(t, rt, db)

	const secret = "s3cr3t-graduation-0xC0FFEE" //nolint:gosec // fixture, not a credential
	docs := func(image string) string {
		return strings.Replace(stackTestDocs(image), "      LOG_LEVEL: info\n", "      LOG_LEVEL: info\n      DB_PASSWORD: {secretRef: DB_PASSWORD}\n", 1)
	}
	supplied := map[string]any{"secrets": map[string]string{"web/DB_PASSWORD": secret}}
	resolve := func(m *secrets.Manager) string {
		t.Helper()
		v, err := m.Resolve(t.Context(), "web", "DB_PASSWORD")
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		return v
	}

	if code, out := stackApply(t, rt, cookie, docs("ghcr.io/acme/web:1.0.0"), supplied); code != http.StatusOK || out.Revision == nil || out.Revision.Status != "applied" {
		t.Fatalf("first apply = %d %+v %+v", code, out.Revision, out.Issues)
	}
	if got := resolve(secrets.NewManager(db, mk)); got != secret {
		t.Fatal("secret was not stored by the first apply")
	}
	planOut := stackPlanRaw(t, rt, cookie, docs("ghcr.io/acme/web:1.1.0"), supplied)

	// A re-apply that supplies no value keeps the stored one.
	if code, out := stackApply(t, rt, cookie, docs("ghcr.io/acme/web:1.1.0"), nil); code != http.StatusOK || out.Revision == nil || out.Revision.Status != "applied" {
		t.Fatalf("re-apply = %d %+v %+v", code, out.Revision, out.Issues)
	}
	if got := resolve(secrets.NewManager(db, mk)); got != secret {
		t.Fatal("secret did not survive a re-apply")
	}

	// A restart is a new router and manager over the same store and master key.
	restarted := NewRouter(logger, testBrand(), db, WithSecretSetter(secrets.NewManager(db, mk)))
	if got := resolve(secrets.NewManager(db, mk)); got != secret {
		t.Fatal("secret did not survive a restart")
	}
	planOut += stackPlanRaw(t, restarted, cookie, docs("ghcr.io/acme/web:1.2.0"), nil)
	if code, out := func() (int, stackApplyResponse) {
		var o stackApplyResponse
		req := authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/apply", stackBody(t, docs("ghcr.io/acme/web:1.2.0"), nil))
		return iacCall(t, restarted, req, &o), o
	}(); code != http.StatusOK || out.Revision == nil {
		t.Fatalf("apply after restart = %d %+v", code, out.Revision)
	}
	if got := resolve(secrets.NewManager(db, mk)); got != secret {
		t.Fatal("secret was lost by an apply after restart")
	}

	if strings.Contains(planOut, secret) {
		t.Fatal("plan output holds the secret value")
	}
	if strings.Contains(logs.String(), secret) {
		t.Fatal("logs hold the secret value")
	}
}

func TestStackConcurrentAppliesExactlyOneWinsOtherGetsClearConflict(t *testing.T) {
	rt, db, cookie := stackFixture(t)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	stackAfterLease = func() {
		entered <- struct{}{}
		<-release
	}
	t.Cleanup(func() { stackAfterLease = nil })

	type outcome struct {
		code int
		body string
	}
	results := make(chan outcome, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/stacks/shop-prod/apply", stackBody(t, stackTestDocs("ghcr.io/acme/web:4.0.0"), nil)))
			results <- outcome{rec.Code, rec.Body.String()}
		}()
	}
	<-entered
	loser := <-results
	var errBody struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal([]byte(loser.body), &errBody)
	if loser.code != http.StatusConflict || !strings.Contains(strings.ToLower(errBody.Error), "lock") {
		t.Fatalf("loser = %d %q, want 409 naming the lock", loser.code, loser.body)
	}
	close(release)
	wg.Wait()
	if winner := <-results; winner.code != http.StatusOK {
		t.Fatalf("winner = %d %s", winner.code, winner.body)
	}
	s, _ := db.GetStackByName(t.Context(), "shop-prod")
	revs, err := db.ListStackRevisions(t.Context(), s.ID, 0)
	if err != nil || len(revs) != 1 {
		t.Fatalf("revisions = %d %v, want exactly one", len(revs), err)
	}
}
