package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestSilenceWatcherPingsOnlyWhenSilentPastGrace(t *testing.T) {
	var hits atomic.Int32
	var gotBody atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody.Store(string(b))
		hits.Add(1)
	}))
	defer srv.Close()

	cfg := SilenceConfig{Grace: 50 * time.Millisecond, Interval: 20 * time.Millisecond, MaxBackoff: 40 * time.Millisecond,
		Check: 5 * time.Millisecond, Timeout: time.Second, AllowPrivate: true}
	w := NewSilenceWatcher(cfg, "node-1", nil, nil)
	w.SetURL(srv.URL + "/fail")
	w.SetConnected(true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	time.Sleep(120 * time.Millisecond)
	if hits.Load() != 0 {
		t.Fatal("connected agent must not ping")
	}

	w.SetConnected(false)
	waitFor(t, func() bool { return hits.Load() > 0 }, "ping")
	var body map[string]any
	if err := json.Unmarshal([]byte(gotBody.Load().(string)), &body); err != nil {
		t.Fatal(err)
	}
	if body["node"] != "node-1" {
		t.Fatalf("node = %v", body["node"])
	}

	w.SetConnected(true)
	time.Sleep(30 * time.Millisecond)
	n := hits.Load()
	time.Sleep(100 * time.Millisecond)
	if hits.Load() != n {
		t.Fatal("pings must stop once contact resumes")
	}
}

func TestSilenceConfigFromEnvRejectsBadValues(t *testing.T) {
	_, err := SilenceConfigFromEnv(func(k string) (string, bool) {
		if k == EnvSilenceGrace {
			return "soon", true
		}
		return "", false
	})
	if err == nil {
		t.Fatal("want error for a bad duration")
	}
	c, err := SilenceConfigFromEnv(func(string) (string, bool) { return "", false })
	if err != nil || c.Grace != defaultSilenceGrace {
		t.Fatalf("defaults: %v %v", c, err)
	}
}

func TestSilenceWatcherRejectsPrivateURL(t *testing.T) {
	w := NewSilenceWatcher(SilenceConfig{Timeout: time.Second}, "n", nil, nil)
	w.SetURL("http://127.0.0.1/x")
	if _, raw := w.silentFor(); raw != "" {
		t.Fatal("private URL must be refused")
	}
}
