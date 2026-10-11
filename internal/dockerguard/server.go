package dockerguard

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Server is a running guard socket.
type Server struct {
	Guard  *Guard
	Socket string
	srv    *http.Server
}

// Host is the docker host URL clients dial to reach the guard.
func (s *Server) Host() string { return "unix://" + s.Socket }

// UpstreamSocket turns a docker host URL into the Unix socket path the
// guard forwards to. Only unix:// hosts can be guarded.
func UpstreamSocket(host string) (string, error) {
	path, ok := strings.CutPrefix(host, "unix://")
	if !ok || path == "" {
		return "", fmt.Errorf("dockerguard: upstream %q is not a unix socket, only unix:// hosts can be guarded", host)
	}
	return path, nil
}

// Listen creates the guard socket at socket (mode 0600, parent 0700) and
// serves g on it until ctx ends or Close is called.
func Listen(ctx context.Context, g *Guard, socket string) (*Server, error) {
	if len(socket) > maxSocketPath {
		return nil, fmt.Errorf("dockerguard: socket path %s is %d bytes, over the %d a unix socket allows; set %s to a shorter path", socket, len(socket), maxSocketPath, EnvSocket)
	}
	if err := privateDir(filepath.Dir(socket)); err != nil {
		return nil, err
	}
	if err := removeStaleSocket(socket); err != nil {
		return nil, err
	}
	ln, err := listenPrivate(socket)
	if err != nil {
		return nil, err
	}
	s := &Server{
		Guard:  g,
		Socket: socket,
		srv: &http.Server{
			Handler:           g,
			ReadHeaderTimeout: g.tunables.HeaderTimeout,
			BaseContext:       func(net.Listener) context.Context { return context.WithoutCancel(ctx) },
		},
	}
	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("dockerguard: serve", slog.String("socket", socket), slog.String("error", err.Error()))
		}
	}()
	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()
	return s, nil
}

// listenPrivate binds then chmods to 0600; the 0700 parent keeps the
// moment between the two unreachable for other users.
func listenPrivate(socket string) (net.Listener, error) {
	ln, err := net.Listen("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("dockerguard: listen on %s: %w", socket, err)
	}
	if err := os.Chmod(socket, 0o600); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("dockerguard: chmod %s: %w", socket, err)
	}
	return ln, nil
}

// privateDir creates dir as 0700, or checks an existing one is not group
// or world accessible.
func privateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("dockerguard: create socket dir: %w", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("dockerguard: stat socket dir: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("dockerguard: socket dir %s is %s, want 0700 so only this user can reach the guard", dir, info.Mode().Perm())
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Geteuid() {
		return fmt.Errorf("dockerguard: socket dir %s is owned by uid %d, not this process", dir, st.Uid)
	}
	return nil
}

// maxSocketPath fits sockaddr_un on both Linux (108) and macOS (104).
const maxSocketPath = 103

// removeStaleSocket deletes a leftover socket from a previous run, and
// refuses to touch anything at that path that is not a socket.
func removeStaleSocket(socket string) error {
	info, err := os.Lstat(socket)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return fmt.Errorf("dockerguard: stat %s: %w", socket, err)
	case info.Mode()&fs.ModeSocket == 0:
		return fmt.Errorf("dockerguard: %s exists and is not a socket", socket)
	}
	if err := os.Remove(socket); err != nil {
		return fmt.Errorf("dockerguard: remove stale socket %s: %w", socket, err)
	}
	return nil
}

// Close stops accepting, waits briefly for in-flight requests, then drains
// the recorder. Hijacked streams are left to their own clients.
func (s *Server) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := s.srv.Shutdown(ctx)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		slog.Warn("dockerguard: shutdown", slog.String("error", err.Error()))
	}
	s.Guard.Close()
	_ = os.Remove(s.Socket)
	return nil
}
