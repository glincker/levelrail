package egress

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Environment variable names for the sidecar policy channel.
const (
	EnvPolicySocket = "APP_PLUGIN_EGRESS_POLICY_SOCKET"
	EnvPolicyCache  = "APP_PLUGIN_EGRESS_POLICY_CACHE"
	EnvListen       = "APP_PLUGIN_EGRESS_LISTEN"
)

const maxPolicyBytes = 1 << 20

// PolicyServer applies pushed policies to a store and caches the last good one.
type PolicyServer struct {
	store     *PolicyStore
	cachePath string
}

// NewPolicyServer returns a server. An empty cachePath disables the cache.
func NewPolicyServer(store *PolicyStore, cachePath string) *PolicyServer {
	return &PolicyServer{store: store, cachePath: cachePath}
}

// LoadCache installs the cached policy when the store has none yet. A missing
// cache is not an error; a corrupt one is, and leaves the store denying all.
func (s *PolicyServer) LoadCache() error {
	if s.cachePath == "" || s.store.Ready() {
		return nil
	}
	b, err := os.ReadFile(s.cachePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read policy cache: %w", err)
	}
	p, err := decodeBytes(b)
	if err != nil {
		return fmt.Errorf("policy cache: %w", err)
	}
	return s.store.Set(p)
}

func (s *PolicyServer) writeCache(b []byte) error {
	if s.cachePath == "" {
		return nil
	}
	dir := filepath.Dir(s.cachePath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".policy-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.cachePath)
}

// Handler serves PUT /policy and GET /healthz.
func (s *PolicyServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		if !s.store.Ready() {
			http.Error(w, "no policy", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "ok")
	})
	mux.HandleFunc("PUT /policy", func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPolicyBytes))
		if err != nil {
			http.Error(w, "policy too large or unreadable", http.StatusBadRequest)
			return
		}
		p, err := decodeBytes(b)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := s.writeCache(b); err != nil {
			http.Error(w, "policy cache write failed", http.StatusInternalServerError)
			return
		}
		if err := s.store.Set(p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	return mux
}

// ListenPolicySocket creates the socket directory (0700) and the socket (0600).
func ListenPolicySocket(path string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create socket dir: %w", err)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("remove stale socket: %w", err)
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen policy socket: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("chmod policy socket: %w", err)
	}
	return ln, nil
}

// PushPolicy sends a policy to a sidecar over its Unix socket.
func PushPolicy(ctx context.Context, socketPath string, p Policy) error {
	body, err := EncodePolicy(p)
	if err != nil {
		return err
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
			DisableKeepAlives: true,
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://policy/policy", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("push policy: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("push policy: status %d: %s", resp.StatusCode, string(msg))
	}
	return nil
}
