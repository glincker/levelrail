package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// scriptedRuntime answers every Exec through handle, so a test can play an
// engine's API and file system call by call. Everything else comes from
// fakeExecRuntime.
type scriptedRuntime struct {
	*fakeExecRuntime
	mu      sync.Mutex
	handle  func(cmd []string) (string, error)
	calls   [][]string
	uploads []string
	running bool
	starts  int
	stops   int
}

func newScriptedRuntime(handle func(cmd []string) (string, error)) *scriptedRuntime {
	return &scriptedRuntime{
		fakeExecRuntime: &fakeExecRuntime{inspectState: &docker.ContainerState{ID: "c1", Name: "db-search", Running: true}},
		handle:          handle,
		running:         true,
	}
}

func (s *scriptedRuntime) Exec(_ context.Context, _ string, cmd []string) (io.ReadCloser, error) {
	s.mu.Lock()
	s.calls = append(s.calls, cmd)
	s.mu.Unlock()
	out, err := s.handle(cmd)
	if err != nil {
		return io.NopCloser(&failingReader{data: out, err: err}), nil
	}
	return io.NopCloser(strings.NewReader(out)), nil
}

func (s *scriptedRuntime) ExecWithInput(_ context.Context, _ string, cmd []string, stdin io.Reader) (io.ReadCloser, error) {
	b, err := io.ReadAll(stdin)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.calls = append(s.calls, cmd)
	s.uploads = append(s.uploads, string(b))
	s.mu.Unlock()
	out, herr := s.handle(cmd)
	if herr != nil {
		return io.NopCloser(&failingReader{data: out, err: herr}), nil
	}
	return io.NopCloser(strings.NewReader(out)), nil
}

func (s *scriptedRuntime) InspectByName(context.Context, string) (*docker.ContainerState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &docker.ContainerState{ID: "c1", Name: "db-search", Running: s.running}, nil
}

func (s *scriptedRuntime) Stop(context.Context, string, time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stops++
	s.running = false
	return nil
}

func (s *scriptedRuntime) Start(context.Context, string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.starts++
	s.running = true
	return nil
}

func (s *scriptedRuntime) commandLog() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.calls))
	for _, c := range s.calls {
		if m, p, ok := httpRequestOf(c); ok {
			out = append(out, m+" "+p)
			continue
		}
		out = append(out, strings.Join(c, " "))
	}
	return out
}

// failingReader yields data, then err, like an exec whose command exited non-zero.
type failingReader struct {
	data string
	err  error
	done bool
}

func (f *failingReader) Read(p []byte) (int, error) {
	if !f.done && f.data != "" {
		n := copy(p, f.data)
		f.data = f.data[n:]
		return n, nil
	}
	f.done = true
	return 0, f.err
}

// httpRequestOf recognizes searchHTTPCmd's commands.
func httpRequestOf(cmd []string) (method, path string, ok bool) {
	if len(cmd) >= 5 && (cmd[0] == "sh" || cmd[0] == "bash") && strings.Contains(cmd[2], "127.0.0.1") {
		return cmd[3], cmd[4], true
	}
	return "", "", false
}

func httpOK(body string) string {
	return "HTTP/1.0 200 OK\r\nContent-Type: application/json\r\n\r\n" + body
}

func httpStatus(code int, body string) string {
	return fmt.Sprintf("HTTP/1.0 %d Whatever\r\nContent-Type: application/json\r\n\r\n%s", code, body)
}

var errExit1 = errors.New("exit status 1")
