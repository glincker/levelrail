package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/functions"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestFunctionWebhookAndDeadLetters(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.gitSourceSecrets = newFakeGitSourceSecrets()
	cfg := functions.TriggerConfigFromEnv(func(string) (string, bool) { return "", false })
	WithFunctionTriggers(db, cfg, nil)(rt)
	if err := db.SaveDesiredService(context.Background(), store.DesiredService{Name: "fn", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	do := func(req *http.Request) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec
	}

	rec := do(authedRequest(t, cookie, http.MethodPut, "/api/v1/functions/fn/triggers/webhook", `{"path":"/hook"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("enable = %d: %s", rec.Code, rec.Body.String())
	}
	var hook functionWebhookView
	if err := json.Unmarshal(rec.Body.Bytes(), &hook); err != nil || hook.Secret == "" {
		t.Fatalf("enable must return the secret once: %+v %v", hook, err)
	}

	body := []byte(`{"x":1}`)
	signed := func(secret string, ts time.Time, payload []byte) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/functions/fn", bytes.NewReader(payload))
		req.Header.Set(functions.HeaderTimestamp, strconv.FormatInt(ts.Unix(), 10))
		req.Header.Set(functions.HeaderSignature, functions.Sign(secret, ts.Unix(), payload))
		return req
	}
	if rec := do(signed("wrong", time.Now(), body)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad secret = %d, want 401", rec.Code)
	}
	if rec := do(signed(hook.Secret, time.Now().Add(-time.Hour), body)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("stale = %d, want 401", rec.Code)
	}
	good := time.Now()
	if rec := do(signed(hook.Secret, good, body)); rec.Code != http.StatusAccepted {
		t.Fatalf("valid = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(signed(hook.Secret, good, body)); rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("duplicate")) {
		t.Fatalf("replay = %d: %s", rec.Code, rec.Body.String())
	}

	inv, ok, err := db.ClaimFunctionInvocation(context.Background(), time.Now().Add(time.Second))
	if err != nil || !ok || inv.Path != "/hook" || inv.Source != "webhook" {
		t.Fatalf("queued call = %+v ok=%v err=%v", inv, ok, err)
	}
	if err := db.SettleFunctionInvocation(context.Background(), inv.ID, store.FunctionInvocationDead, time.Now(), 500, "boom", time.Now()); err != nil {
		t.Fatal(err)
	}

	rec = do(authedRequest(t, cookie, http.MethodGet, "/api/v1/function-invocations?status=dead", ""))
	var list struct {
		Invocations []functionInvocationView `json:"invocations"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Invocations) != 1 {
		t.Fatalf("dead list = %s", rec.Body.String())
	}
	if rec := do(authedRequest(t, cookie, http.MethodPost, "/api/v1/function-invocations/"+inv.ID+"/retry", "")); rec.Code != http.StatusAccepted {
		t.Fatalf("retry = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := do(authedRequest(t, cookie, http.MethodPost, "/api/v1/function-invocations/"+inv.ID+"/retry", "")); rec.Code != http.StatusConflict {
		t.Fatalf("second retry = %d, want 409", rec.Code)
	}
}
