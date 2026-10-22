package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/functions"
	"github.com/GLINCKER/levelrail/internal/ingress"
)

func TestMergeColdRecordsFoldsWakeHookIntoReplayedCall(t *testing.T) {
	t0 := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	recs := []ingress.AccessRecord{
		{Time: t0.Add(3 * time.Second), RequestID: "a", Status: 200, Millis: 20},
		{Time: t0, RequestID: "a", Status: 307, Millis: 3000, Cold: true},
		{Time: t0.Add(time.Minute), RequestID: "b", Status: 200, Millis: 5},
	}
	got := mergeColdRecords(recs)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if !got[0].Cold || got[0].Status != 200 || got[0].Millis != 3020 || !got[0].Time.Equal(t0) {
		t.Errorf("merged = %+v", got[0])
	}
	if got[1].Cold {
		t.Errorf("warm call flagged cold: %+v", got[1])
	}
}

func TestRunFunctionInvokeColdStoredAndReplayable(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.SetFunctionCapture(ctx, "fn", true); err != nil {
		t.Fatal(err)
	}
	var hits int
	var gotAuth, gotBody string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.Header().Set("Location", "/hello")
			w.WriteHeader(http.StatusTemporaryRedirect)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(srv.Close)

	res, err := rt.runFunctionInvoke(ctx, "fn", srv.URL, FunctionInvokeRequest{
		Method: "post", Path: "hello", Body: `{"a":1}`, Headers: map[string]string{"Authorization": "Bearer secret", "X-Test": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != 200 || res.Body != "ok" || !res.ColdStart || !res.Stored {
		t.Fatalf("result = %+v", res)
	}
	if gotAuth != "Bearer secret" || gotBody != `{"a":1}` {
		t.Errorf("function saw auth %q body %q", gotAuth, gotBody)
	}
	inputs, _ := rt.functionInputStore()
	in, ok := inputs.Get(res.RequestID)
	if !ok || in.Method != "POST" || in.Path != "/hello" || in.Headers.Get("Authorization") != "" || in.Headers.Get("X-Test") != "1" {
		t.Errorf("stored input = %+v, ok=%v", in, ok)
	}
}

func TestRunFunctionInvokeRejectsOversizedBody(t *testing.T) {
	rt, _ := newTestRouter(t)
	_, lim := rt.functionInputStore()
	_, err := rt.runFunctionInvoke(context.Background(), "fn", "https://example.invalid", FunctionInvokeRequest{Body: strings.Repeat("x", lim.RequestBytes+1)})
	if err == nil {
		t.Fatal("want an error for a body past the cap")
	}
}

func TestInputStoreRedactsAndBounds(t *testing.T) {
	s := functions.NewInputStore(functions.InvokeLimits{InputEntries: 2, InputBodyBytes: 4, InputRetention: time.Hour})
	h := http.Header{"Cookie": {"s=1"}, "X-Ok": {"1"}}
	now := time.Now()
	s.Put("a", functions.Input{Function: "f", Body: "abcdefgh", Headers: h, At: now})
	s.Put("b", functions.Input{Function: "f", At: now})
	s.Put("c", functions.Input{Function: "f", At: now})
	if _, ok := s.Get("a"); ok {
		t.Error("oldest entry should be evicted past the count cap")
	}
	s.Put("d", functions.Input{Function: "g", Body: "abcdefgh", Headers: h, At: now})
	in, ok := s.Get("d")
	if !ok || in.Body != "abcd" || in.Headers.Get("Cookie") != "" || in.Headers.Get("X-Ok") != "1" {
		t.Errorf("stored = %+v ok=%v", in, ok)
	}
}
