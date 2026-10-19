package walship

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"sync"
	"time"

	_ "modernc.org/sqlite" // pure Go driver, same as the store
)

// DefaultCheckpointBytes is the WAL size at which the shipper checkpoints and lets SQLite restart the WAL.
const DefaultCheckpointBytes = 16 << 20

// Stats counts what a Shipper has done since it was created.
type Stats struct {
	Generations   int
	Segments      int
	SnapshotBytes int64
	SegmentBytes  int64
	FrameBytes    int64
	Checkpoints   int
}

// Shipper copies new WAL frames of one SQLite database to a Sink. It holds a
// read transaction between syncs so SQLite cannot restart the WAL under it.
type Shipper struct {
	path            string
	sink            Sink
	checkpointBytes int64
	db              *sql.DB

	mu       sync.Mutex
	gen      int
	hold     *sql.Conn
	haveHdr  bool
	hdr      walHeader
	offset   int64
	sum      [2]uint32
	stats    Stats
	frameLen int
}

// New opens a shipper for the database at path. checkpointBytes <= 0 uses DefaultCheckpointBytes.
func New(path string, sink Sink, checkpointBytes int64) (*Shipper, error) {
	if checkpointBytes <= 0 {
		checkpointBytes = DefaultCheckpointBytes
	}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "wal_autocheckpoint(0)")
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("walship: open %s: %w", path, err)
	}
	db.SetMaxOpenConns(4)
	return &Shipper{path: path, sink: sink, checkpointBytes: checkpointBytes, db: db}, nil
}

// Stats returns a copy of the counters.
func (s *Shipper) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

// Close releases the read hold and the connection pool.
func (s *Shipper) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releaseHold()
	return s.db.Close()
}

// Run syncs every interval until ctx ends.
func (s *Shipper) Run(ctx context.Context, interval time.Duration) error {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if err := s.Sync(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// Sync ships every committed frame not yet shipped, then checkpoints when the WAL is large.
func (s *Shipper) Sync(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hold == nil {
		if err := s.newGeneration(ctx); err != nil {
			return err
		}
	}
	if err := s.syncLocked(ctx); err != nil {
		return err
	}
	if s.hold == nil {
		return nil
	}
	if fi, err := os.Stat(s.path + "-wal"); err == nil && fi.Size() >= s.checkpointBytes { //nolint:gosec // path is the operator-configured database file
		return s.checkpoint(ctx)
	}
	return nil
}

func (s *Shipper) acquireHold(ctx context.Context) error {
	c, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("walship: hold conn: %w", err)
	}
	if _, err := c.ExecContext(ctx, "BEGIN"); err != nil {
		_ = c.Close()
		return fmt.Errorf("walship: hold begin: %w", err)
	}
	var n int
	if err := c.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master").Scan(&n); err != nil {
		_ = c.Close()
		return fmt.Errorf("walship: hold read: %w", err)
	}
	s.hold = c
	return nil
}

func (s *Shipper) releaseHold() {
	if s.hold != nil {
		_, _ = s.hold.ExecContext(context.Background(), "ROLLBACK")
		_ = s.hold.Close()
		s.hold = nil
	}
}

// newGeneration snapshots the DB file under a read hold; replaying the WAL
// from its first frame then repairs any page the primary checkpointed mid-copy.
func (s *Shipper) newGeneration(ctx context.Context) error {
	s.releaseHold()
	names, err := s.sink.List("gen-")
	if err != nil {
		return fmt.Errorf("walship: list sink: %w", err)
	}
	next := 1
	for _, n := range names {
		var g int
		if _, err := fmt.Sscanf(n, "gen-%d/", &g); err == nil && g >= next {
			next = g + 1
		}
	}
	s.gen = next
	if err := s.acquireHold(ctx); err != nil {
		return err
	}
	f, err := os.Open(s.path) //nolint:gosec // path is the operator-configured database file
	if err != nil {
		return fmt.Errorf("walship: open db file: %w", err)
	}
	defer func() { _ = f.Close() }()
	pr, pw := io.Pipe()
	go func() {
		zw := gzip.NewWriter(pw)
		_, cerr := io.Copy(zw, f)
		if cerr == nil {
			cerr = zw.Close()
		}
		pw.CloseWithError(cerr)
	}()
	cr := &countReader{r: pr}
	if err := s.sink.Put(snapshotName(s.gen), cr); err != nil {
		_ = pr.CloseWithError(err)
		return fmt.Errorf("walship: put snapshot: %w", err)
	}
	s.stats.Generations++
	s.stats.SnapshotBytes += cr.n
	s.haveHdr = false
	s.offset = 0
	return nil
}

func (s *Shipper) syncLocked(ctx context.Context) error {
	f, err := os.Open(s.path + "-wal") //nolint:gosec // path is the operator-configured database file
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("walship: open wal: %w", err)
	}
	defer func() { _ = f.Close() }()
	var hb [walHeaderSize]byte
	if _, err := f.ReadAt(hb[:], 0); err != nil {
		return nil
	}
	h, err := parseHeader(hb[:])
	if err != nil {
		return nil
	}
	if !s.haveHdr || h.salt1 != s.hdr.salt1 || h.salt2 != s.hdr.salt2 {
		if s.haveHdr && h.seq != s.hdr.seq+1 {
			return s.newGeneration(ctx)
		}
		if err := s.sink.Put(headerName(s.gen, h.seq), bytes.NewReader(h.raw[:])); err != nil {
			return fmt.Errorf("walship: put header: %w", err)
		}
		s.hdr, s.haveHdr, s.offset, s.sum = h, true, 0, h.cksum
		s.frameLen = frameHeaderLen + h.pageSize
	}
	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("walship: stat wal: %w", err)
	}
	avail := fi.Size() - walHeaderSize - s.offset
	if avail < int64(s.frameLen) {
		return nil
	}
	buf := make([]byte, avail-avail%int64(s.frameLen))
	if _, err := f.ReadAt(buf, walHeaderSize+s.offset); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("walship: read wal: %w", err)
	}
	n, sum := scanFrames(s.hdr, s.sum, buf)
	if n == 0 {
		return nil
	}
	var zb bytes.Buffer
	zw := gzip.NewWriter(&zb)
	_, _ = zw.Write(buf[:n])
	if err := zw.Close(); err != nil {
		return fmt.Errorf("walship: compress: %w", err)
	}
	size := int64(zb.Len())
	if err := s.sink.Put(segmentName(s.gen, s.hdr.seq, s.offset), &zb); err != nil {
		return fmt.Errorf("walship: put segment: %w", err)
	}
	s.offset += int64(n)
	s.sum = sum
	s.stats.Segments++
	s.stats.SegmentBytes += size
	s.stats.FrameBytes += int64(n)
	return nil
}

// checkpoint blocks writers, ships the tail, backfills the whole WAL, and
// swaps the hold without a gap so the next writer may restart the WAL.
func (s *Shipper) checkpoint(ctx context.Context) error {
	a, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("walship: checkpoint conn: %w", err)
	}
	defer func() { _ = a.Close() }()
	if _, err := a.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return nil
	}
	defer func() { _, _ = a.ExecContext(context.Background(), "ROLLBACK") }()
	if err := s.syncLocked(ctx); err != nil {
		return err
	}
	s.releaseHold()
	var busy, logf, ckpt int
	if err := s.db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)").Scan(&busy, &logf, &ckpt); err != nil {
		return fmt.Errorf("walship: checkpoint: %w", err)
	}
	s.stats.Checkpoints++
	if err := s.acquireHold(ctx); err != nil {
		return err
	}
	return nil
}
