package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestHandleQueryLogs_Filters(t *testing.T) {
	rt, _, tdb, cookie := investigateFixture(t)
	now := time.Now().UTC().Truncate(time.Second)
	err := tdb.WriteLogBatch(context.Background(), []telemetry.LogEntry{
		{ResourceID: "service:web", ContainerID: "aaa111", Stream: "stdout", Timestamp: now.Add(-3 * time.Minute), Message: `{"status":200}`, Structured: true, FieldsJSON: `{"status":200}`},
		{ResourceID: "service:web", ContainerID: "aaa111", Stream: "stderr", Timestamp: now.Add(-2 * time.Minute), Message: `{"status":500,"route":"/pay"}`, Structured: true, FieldsJSON: `{"status":500,"route":"/pay"}`},
		{ResourceID: "service:web", ContainerID: "bbb222", Stream: "stdout", Timestamp: now.Add(-time.Minute), Message: "plain"},
	})
	if err != nil {
		t.Fatal(err)
	}
	base := url.Values{"from": {now.Add(-time.Hour).Format(time.RFC3339)}, "to": {now.Format(time.RFC3339)}}

	cases := []struct {
		name      string
		extra     url.Values
		wantLines int
		wantCode  int
	}{
		{"no filter", nil, 3, http.StatusOK},
		{"container prefix", url.Values{"container": {"bbb"}}, 1, http.StatusOK},
		{"stream", url.Values{"stream": {"stderr"}}, 1, http.StatusOK},
		{"field eq", url.Values{"field": {"status=500"}}, 1, http.StatusOK},
		{"field gt", url.Values{"field": {"status>=200"}}, 2, http.StatusOK},
		{"two fields", url.Values{"field": {"status=500", "route=/pay"}}, 1, http.StatusOK},
		{"bad field", url.Values{"field": {"nonsense"}}, 0, http.StatusBadRequest},
		{"bad stream", url.Values{"stream": {"tty"}}, 0, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := url.Values{}
			for k, v := range base {
				q[k] = v
			}
			for k, v := range tc.extra {
				q[k] = v
			}
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/logs?"+q.Encode(), ""))
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
			}
			if tc.wantCode != http.StatusOK {
				return
			}
			var got logsResponse
			getObs(t, rt, cookie, "/api/v1/apps/web/logs?"+q.Encode(), &got)
			if len(got.Entries) != tc.wantLines {
				t.Errorf("entries = %d, want %d", len(got.Entries), tc.wantLines)
			}
			if tc.name == "container prefix" && (len(got.Containers) != 2 || got.Entries[0].Container != "bbb222") {
				t.Errorf("containers = %+v entries = %+v", got.Containers, got.Entries)
			}
		})
	}
}

func TestHandleQueryMetrics_MaxPointsAndCompare(t *testing.T) {
	rt, _, tdb, cookie := investigateFixture(t)
	now := time.Now().UTC().Truncate(time.Minute)
	var samples []telemetry.Sample
	for ts := now.Add(-2 * time.Hour); ts.Before(now); ts = ts.Add(15 * time.Second) {
		v := 10.0
		if !ts.Before(now.Add(-time.Hour)) {
			v = 20
		}
		samples = append(samples, telemetry.Sample{ResourceID: "service:web", Metric: "cpu_percent", Timestamp: ts, Value: v})
	}
	if err := tdb.WriteSamples(context.Background(), samples); err != nil {
		t.Fatal(err)
	}
	q := url.Values{
		"metric": {"cpu_percent"}, "from": {now.Add(-time.Hour).Format(time.RFC3339)},
		"to": {now.Format(time.RFC3339)}, "max_points": {"60"}, "compare": {"previous"},
	}
	var got metricsResponse
	getObs(t, rt, cookie, "/api/v1/apps/web/metrics?"+q.Encode(), &got)

	if len(got.Points) == 0 || len(got.Points) > 60 {
		t.Fatalf("points = %d, want 1..60", len(got.Points))
	}
	if !got.Downsampled || got.StepSeconds < 60 {
		t.Errorf("downsampled = %v step = %v", got.Downsampled, got.StepSeconds)
	}
	if len(got.PreviousPoints) == 0 {
		t.Fatal("previous points missing")
	}
	if got.Points[0].Value != 20 || got.PreviousPoints[0].Value != 10 {
		t.Errorf("values current %v previous %v", got.Points[0].Value, got.PreviousPoints[0].Value)
	}
	if first := got.PreviousPoints[0].Timestamp; first.Before(now.Add(-time.Hour).Add(-time.Second)) {
		t.Errorf("previous series was not shifted onto the current window: %v", first)
	}

	rec := httptest.NewRecorder()
	bad := url.Values{"metric": {"cpu_percent"}, "max_points": {"zero"}}
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/metrics?"+bad.Encode(), ""))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad max_points status = %d", rec.Code)
	}
}
