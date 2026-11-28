// Package awsxtest gives service tests a fake AWS endpoint.
package awsxtest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/awsx"
)

// Call is one request the fake server received.
type Call struct {
	Target, Body, Auth string
	Method, Path       string
}

// Reply is what the fake sends back.
type Reply struct {
	Status int
	Body   string
	Header map[string]string
}

// Recorder keeps received calls in order.
type Recorder struct {
	mu    sync.Mutex
	calls []Call
}

// Calls returns a copy of the recorded calls.
func (r *Recorder) Calls() []Call {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Call(nil), r.calls...)
}

// New starts a fake server and returns a client pointed at it. Sleeps are skipped.
func New(t *testing.T, h func(c Call, n int) Reply) (*awsx.Client, *Recorder) {
	t.Helper()
	rec := &Recorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c := Call{Target: r.Header.Get("X-Amz-Target"), Body: string(b), Auth: r.Header.Get("Authorization"), Method: r.Method, Path: r.URL.RequestURI()}
		rec.mu.Lock()
		rec.calls = append(rec.calls, c)
		n := len(rec.calls)
		rec.mu.Unlock()
		rep := h(c, n)
		for k, v := range rep.Header {
			w.Header().Set(k, v)
		}
		if rep.Status == 0 {
			rep.Status = 200
		}
		w.WriteHeader(rep.Status)
		_, _ = w.Write([]byte(rep.Body))
	}))
	t.Cleanup(srv.Close)
	o := awsx.DefaultOptions(awsx.Credentials{AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "test-secret-key"})
	o.BaseURL = srv.URL
	o.Now = func() time.Time { return time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC) }
	o.Sleep = func(_ context.Context, _ time.Duration) error { return nil }
	c, err := awsx.New(o)
	if err != nil {
		t.Fatal(err)
	}
	return c, rec
}
