package dataexport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// spool mirrors a destination's in-memory log queue on disk so lines
// waiting for a down receiver survive a control plane restart. It is a
// JSON lines file plus a tiny file holding how many leading lines are done.
// Lines are removed by counting, in order, which matches the queue: it is
// FIFO and drops its oldest entries first.
//
// A crash between a successful send and its ack replays that batch, so
// delivery is at least once.
type spool struct {
	mu      sync.Mutex
	path    string
	ackPath string
	f       *os.File
	w       *bufio.Writer
	written int // lines in the file, acked or not
	acked   int // leading lines already delivered or dropped
}

const spoolCompactAfter = 2000

func spoolFileName(id string) string {
	return strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(id)
}

// openSpool loads whatever a previous run left behind and returns the
// records still owed to the receiver.
func openSpool(dir, id string) (*spool, []LogRecord, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // operator-controlled data directory
		return nil, nil, fmt.Errorf("dataexport: create spool dir: %w", err)
	}
	name := spoolFileName(id)
	s := &spool{path: filepath.Join(dir, name+".spool"), ackPath: filepath.Join(dir, name+".ack")}
	if b, err := os.ReadFile(s.ackPath); err == nil { //nolint:gosec // path built from a validated id under the data dir
		s.acked, _ = strconv.Atoi(strings.TrimSpace(string(b)))
	}
	pending, lines, err := s.readPending()
	if err != nil {
		return nil, nil, err
	}
	s.written = lines
	if s.acked > lines {
		s.acked = lines
	}
	if err := s.reopen(); err != nil {
		return nil, nil, err
	}
	return s, pending, nil
}

func (s *spool) readPending() (pending []LogRecord, lines int, err error) {
	raw, err := os.ReadFile(s.path) //nolint:gosec // path built from a validated id under the data dir
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("dataexport: read spool: %w", err)
	}
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		lines++
		if lines <= s.acked {
			continue
		}
		var r LogRecord
		if json.Unmarshal(line, &r) != nil {
			continue // a torn last line from a crash
		}
		pending = append(pending, r)
	}
	return pending, lines, nil
}

func (s *spool) reopen() error {
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // path built from a validated id under the data dir
	if err != nil {
		return fmt.Errorf("dataexport: open spool: %w", err)
	}
	s.f = f
	s.w = bufio.NewWriter(f)
	return nil
}

// append records r. It is buffered; flush makes it durable.
func (s *spool) append(r LogRecord) {
	b, err := json.Marshal(r)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.w == nil {
		return
	}
	_, _ = s.w.Write(b)
	_ = s.w.WriteByte('\n')
	s.written++
}

// flush pushes buffered lines to the file.
func (s *spool) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.w != nil {
		_ = s.w.Flush()
	}
}

// ack marks n more leading lines as done. When nothing is owed the files are
// reset; when a lot is done the file is rewritten without it.
func (s *spool) ack(n int) {
	if n <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.acked += n
	if s.acked > s.written {
		s.acked = s.written
	}
	switch {
	case s.acked == s.written:
		s.resetLocked()
	case s.acked >= spoolCompactAfter && s.acked*2 > s.written:
		s.compactLocked()
	default:
		_ = os.WriteFile(s.ackPath, []byte(strconv.Itoa(s.acked)), 0o600)
	}
}

func (s *spool) resetLocked() {
	if s.w != nil {
		_ = s.w.Flush()
	}
	if s.f != nil {
		_ = s.f.Truncate(0)
		_, _ = s.f.Seek(0, 0)
	}
	s.written, s.acked = 0, 0
	_ = os.WriteFile(s.ackPath, []byte("0"), 0o600)
}

func (s *spool) compactLocked() {
	if s.w != nil {
		_ = s.w.Flush()
	}
	pending, _, err := s.readPending()
	if err != nil {
		return
	}
	var buf bytes.Buffer
	for _, r := range pending {
		b, merr := json.Marshal(r)
		if merr == nil {
			buf.Write(b)
			buf.WriteByte('\n')
		}
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return
	}
	if s.f != nil {
		_ = s.f.Close()
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = s.reopen()
		return
	}
	s.written, s.acked = len(pending), 0
	_ = os.WriteFile(s.ackPath, []byte("0"), 0o600)
	_ = s.reopen()
}

// close flushes and releases the file.
func (s *spool) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.w != nil {
		_ = s.w.Flush()
	}
	if s.f != nil {
		_ = s.f.Close()
		s.f, s.w = nil, nil
	}
}

// remove deletes a destination's spool files.
func (s *spool) remove() {
	s.close()
	_ = os.Remove(s.path)
	_ = os.Remove(s.ackPath)
}
