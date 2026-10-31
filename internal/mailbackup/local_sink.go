package mailbackup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DefaultLocalCapMB applies only when APP_MAIL_BACKUP_LOCAL_CAP_MB is unset.
const DefaultLocalCapMB = 2048

// CapFromEnv reads the local folder cap in bytes from the environment.
func CapFromEnv(getenv func(string) string) (int64, error) {
	raw := getenv("APP_MAIL_BACKUP_LOCAL_CAP_MB")
	if raw == "" {
		return DefaultLocalCapMB << 20, nil
	}
	mb, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || mb <= 0 {
		return 0, fmt.Errorf("APP_MAIL_BACKUP_LOCAL_CAP_MB must be a positive integer, got %q", raw)
	}
	return mb << 20, nil
}

// DefaultMinKeep applies only when APP_MAIL_BACKUP_MIN_KEEP is unset.
const DefaultMinKeep = 3

// MinKeepFromEnv reads how many newest verified backups pruning always keeps.
func MinKeepFromEnv(getenv func(string) string) (int, error) {
	raw := getenv("APP_MAIL_BACKUP_MIN_KEEP")
	if raw == "" {
		return DefaultMinKeep, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return 0, fmt.Errorf("APP_MAIL_BACKUP_MIN_KEEP must be an integer of at least 1, got %q", raw)
	}
	return n, nil
}

// LocalSink stores backups in one folder under a size cap. MinKeep newest
// verified backups survive pruning even when that leaves the folder over the cap.
type LocalSink struct {
	Dir      string
	CapBytes int64
	MinKeep  int
}

func (s *LocalSink) path(name string) (string, error) {
	if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") {
		return "", fmt.Errorf("invalid sink name %q", name)
	}
	return filepath.Join(s.Dir, name), nil
}

type capWriter struct {
	w    io.Writer
	n    int64
	capB int64
}

func (c *capWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	if c.capB > 0 && c.n > c.capB {
		return 0, &CapError{CapBytes: c.capB, NeedBytes: c.n}
	}
	return c.w.Write(p)
}

// Put streams r to a temp file and renames it, so readers never see a
// partial backup and a cap failure leaves existing backups untouched.
func (s *LocalSink) Put(_ context.Context, name string, r io.Reader) error {
	dst, err := s.path(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.Dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	_, err = io.Copy(&capWriter{w: tmp, capB: s.CapBytes}, r)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

// Open implements Sink.
func (s *LocalSink) Open(_ context.Context, name string) (io.ReadCloser, error) {
	p, err := s.path(name)
	if err != nil {
		return nil, err
	}
	return os.Open(p) //nolint:gosec // name is validated by path()
}

// List implements Sink.
func (s *LocalSink) List(_ context.Context) ([]string, error) {
	ents, err := os.ReadDir(s.Dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// Delete implements Sink.
func (s *LocalSink) Delete(_ context.Context, name string) error {
	p, err := s.path(name)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

type localEntry struct {
	id       string
	created  time.Time
	size     int64
	verified bool
	names    []string
}

func (s *LocalSink) entries() ([]*localEntry, error) {
	names, err := s.List(context.Background())
	if err != nil {
		return nil, err
	}
	byID := map[string]*localEntry{}
	for _, n := range names {
		var id string
		switch {
		case strings.HasSuffix(n, ".tar.zst.age"):
			id = strings.TrimSuffix(strings.TrimPrefix(n, "mail-"), ".tar.zst.age")
		case strings.HasSuffix(n, ".manifest.json"):
			id = strings.TrimSuffix(strings.TrimPrefix(n, "mail-"), ".manifest.json")
		default:
			continue
		}
		e := byID[id]
		if e == nil {
			e = &localEntry{id: id}
			byID[id] = e
		}
		info, err := os.Stat(filepath.Join(s.Dir, n))
		if err != nil {
			return nil, err
		}
		e.size += info.Size()
		e.names = append(e.names, n)
		if e.created.IsZero() {
			e.created = info.ModTime()
		}
		if strings.HasSuffix(n, ".manifest.json") {
			var m Manifest
			if raw, err := os.ReadFile(filepath.Join(s.Dir, n)); err == nil && json.Unmarshal(raw, &m) == nil { //nolint:gosec // n comes from listing s.Dir
				e.created, e.verified = m.CreatedAt, m.Verified
			}
		}
	}
	out := make([]*localEntry, 0, len(byID))
	for _, e := range byID {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].created.Before(out[j].created) })
	return out, nil
}

// Prune removes oldest backups until the folder fits the cap. The newest
// MinKeep (at least one) verified backups are never removed; when they alone
// exceed the cap, a CapError is returned instead.
func (s *LocalSink) Prune(ctx context.Context) ([]string, error) {
	if s.CapBytes <= 0 {
		return nil, nil
	}
	es, err := s.entries()
	if err != nil {
		return nil, err
	}
	var total int64
	keep := max(s.MinKeep, 1)
	protected := map[int]bool{}
	for i := len(es) - 1; i >= 0; i-- {
		total += es[i].size
		if es[i].verified && len(protected) < keep {
			protected[i] = true
		}
	}
	var removed []string
	for i, e := range es {
		if total <= s.CapBytes {
			break
		}
		if protected[i] {
			continue
		}
		for _, n := range e.names {
			if err := s.Delete(ctx, n); err != nil {
				return removed, err
			}
		}
		total -= e.size
		removed = append(removed, e.id)
	}
	if total > s.CapBytes {
		return removed, &CapError{CapBytes: s.CapBytes, NeedBytes: total}
	}
	return removed, nil
}
