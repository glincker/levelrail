package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func lokiGet(t *testing.T, rt *Router, path string, mutate func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	mutate(req)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func TestLokiQueryRange(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:1", Port: 3000}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	const plaintext = "loki-read-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_loki", Name: "grafana", TokenHash: hashToken(plaintext), Abilities: []string{AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}
	now := time.Now().UTC()
	if err := tdb.WriteLogBatch(ctx, []telemetry.LogEntry{
		{ResourceID: "service:web", ContainerID: "c1", Stream: "stdout", Timestamp: now.Add(-2 * time.Minute), Message: "listening on 3000"},
		{ResourceID: "service:web", ContainerID: "c1", Stream: "stderr", Timestamp: now.Add(-time.Minute), Message: "ERROR: boom happened"},
	}); err != nil {
		t.Fatalf("seed logs: %v", err)
	}

	type streamsResp struct {
		Status string `json:"status"`
		Data   struct {
			ResultType string       `json:"resultType"`
			Result     []lokiStream `json:"result"`
		} `json:"data"`
	}
	q := url.Values{"query": {`{app="web"} |= "boom"`}}.Encode()

	t.Run("session cookie", func(t *testing.T) {
		rec := lokiGet(t, rt, lokiBasePath+"/query_range?"+q, func(r *http.Request) { r.AddCookie(cookie) })
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		var got streamsResp
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Data.ResultType != "streams" || len(got.Data.Result) != 1 {
			t.Fatalf("unexpected result: %s", rec.Body)
		}
		s := got.Data.Result[0]
		if s.Stream["app"] != "web" || s.Stream["stream"] != "stderr" || len(s.Values) != 1 || s.Values[0][1] != "ERROR: boom happened" {
			t.Errorf("unexpected stream: %+v", s)
		}
	})

	t.Run("basic auth password is the token, grafana path", func(t *testing.T) {
		rec := lokiGet(t, rt, lokiGrafanaBasePath+"/query_range?"+q, func(r *http.Request) { r.SetBasicAuth("grafana", plaintext) })
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
	})

	t.Run("bearer instant query", func(t *testing.T) {
		rec := lokiGet(t, rt, lokiBasePath+"/query?query="+url.QueryEscape(`{app=~"w.*"}`), func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+plaintext)
		})
		if rec.Code != http.StatusOK || strings.Count(rec.Body.String(), `"values":[["`) != 2 {
			t.Fatalf("status %d body %s", rec.Code, rec.Body)
		}
	})

	t.Run("unauthenticated", func(t *testing.T) {
		rec := lokiGet(t, rt, lokiBasePath+"/query_range?"+q, func(*http.Request) {})
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status %d, want 401", rec.Code)
		}
	})

	t.Run("unsupported query is 400 in loki shape", func(t *testing.T) {
		rec := lokiGet(t, rt, lokiBasePath+"/query_range?query="+url.QueryEscape(`rate({app="web"}[5m])`), func(r *http.Request) { r.AddCookie(cookie) })
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"errorType":"bad_data"`) {
			t.Errorf("status %d body %s", rec.Code, rec.Body)
		}
	})

	t.Run("labels and values", func(t *testing.T) {
		rec := lokiGet(t, rt, lokiBasePath+"/label/app/values", func(r *http.Request) { r.AddCookie(cookie) })
		if !strings.Contains(rec.Body.String(), `"web"`) {
			t.Errorf("app values: %s", rec.Body)
		}
		rec = lokiGet(t, rt, lokiBasePath+"/labels", func(r *http.Request) { r.AddCookie(cookie) })
		if !strings.Contains(rec.Body.String(), `"stream"`) {
			t.Errorf("labels: %s", rec.Body)
		}
		rec = lokiGet(t, rt, lokiBasePath+"/ready", func(r *http.Request) { r.AddCookie(cookie) })
		if strings.TrimSpace(rec.Body.String()) != "ready" {
			t.Errorf("ready: %q", rec.Body)
		}
	})
}
