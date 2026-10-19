package api

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
)

// shutdownState tracks what must wind down when the process gets SIGTERM:
// live SSE streams (which never end on their own) and detached queued deploys.
type shutdownState struct {
	mu       sync.Mutex
	draining bool
	next     uint64
	streams  map[uint64]context.CancelFunc
	deploys  sync.WaitGroup
}

func (s *shutdownState) isDraining() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.draining
}

func (s *shutdownState) register(cancel context.CancelFunc) (id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.draining {
		cancel()
		return 0
	}
	if s.streams == nil {
		s.streams = map[uint64]context.CancelFunc{}
	}
	s.next++
	s.streams[s.next] = cancel
	return s.next
}

func (s *shutdownState) unregister(id uint64) {
	if id == 0 {
		return
	}
	s.mu.Lock()
	delete(s.streams, id)
	s.mu.Unlock()
}

// BeginShutdown stops new queued deploys from starting and ends every live
// SSE stream so http.Server.Shutdown does not wait on them. Safe to call twice.
func (rt *Router) BeginShutdown() int {
	s := &rt.shutdown
	s.mu.Lock()
	s.draining = true
	n := len(s.streams)
	for id, cancel := range s.streams {
		cancel()
		delete(s.streams, id)
	}
	s.mu.Unlock()
	return n
}

// WaitDeploys blocks until every detached queued deploy has finished or ctx
// ends, and reports whether all of them finished.
func (rt *Router) WaitDeploys(ctx context.Context) bool {
	done := make(chan struct{})
	go func() {
		rt.shutdown.deploys.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-ctx.Done():
		return false
	}
}

// streamDrainMiddleware cancels an SSE request's context on BeginShutdown.
// Ordinary requests are untouched so in-flight mutations finish normally.
func (rt *Router) streamDrainMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()
		dw := &drainWriter{ResponseWriter: w, state: &rt.shutdown, cancel: cancel}
		defer dw.release()
		next.ServeHTTP(dw, r.WithContext(ctx))
	})
}

type drainWriter struct {
	http.ResponseWriter
	state  *shutdownState
	cancel context.CancelFunc
	id     uint64
	wrote  bool
}

func (d *drainWriter) WriteHeader(status int) {
	if !d.wrote {
		d.wrote = true
		if strings.HasPrefix(d.Header().Get("Content-Type"), "text/event-stream") {
			d.id = d.state.register(d.cancel)
		}
	}
	d.ResponseWriter.WriteHeader(status)
}

func (d *drainWriter) Write(b []byte) (int, error) {
	if !d.wrote {
		d.WriteHeader(http.StatusOK)
	}
	return d.ResponseWriter.Write(b)
}

func (d *drainWriter) Flush() {
	if f, ok := d.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (d *drainWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := d.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return h.Hijack()
}

func (d *drainWriter) Unwrap() http.ResponseWriter { return d.ResponseWriter }

func (d *drainWriter) release() { d.state.unregister(d.id) }
