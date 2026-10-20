package walship

import (
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const maxRestoreBytes = 1 << 40

// RestoreResult describes what Restore replayed.
type RestoreResult struct {
	Generation int
	Epochs     int
	FrameBytes int64
	// Truncated is true when a missing segment stopped replay early.
	Truncated bool
}

// Restore rebuilds the database at dest from the newest generation in sink.
func Restore(ctx context.Context, sink Sink, dest string) (RestoreResult, error) {
	var res RestoreResult
	names, err := sink.List("gen-")
	if err != nil {
		return res, fmt.Errorf("walship: list: %w", err)
	}
	for _, n := range names {
		var g int
		if strings.HasSuffix(n, "/snapshot.db.gz") {
			if _, err := fmt.Sscanf(n, "gen-%d/", &g); err == nil && g > res.Generation {
				res.Generation = g
			}
		}
	}
	if res.Generation == 0 {
		return res, errors.New("walship: no snapshot in sink")
	}
	prefix := fmt.Sprintf("gen-%06d/", res.Generation)
	if err := gunzipTo(sink, snapshotName(res.Generation), dest); err != nil {
		return res, err
	}
	_ = os.Remove(dest + "-wal")
	_ = os.Remove(dest + "-shm")

	byEpoch := map[uint32][]string{}
	var epochs []uint32
	var hdrs = map[uint32]string{}
	for _, n := range names {
		if !strings.HasPrefix(n, prefix) {
			continue
		}
		var seq uint32
		var off int64
		base := strings.TrimPrefix(n, prefix)
		switch {
		case strings.HasSuffix(base, ".hdr"):
			if _, err := fmt.Sscanf(base, "e%d.hdr", &seq); err == nil {
				hdrs[seq] = n
				epochs = append(epochs, seq)
			}
		case strings.HasSuffix(base, ".seg.gz"):
			if _, err := fmt.Sscanf(base, "e%d-%d.seg.gz", &seq, &off); err == nil {
				byEpoch[seq] = append(byEpoch[seq], n)
			}
		}
	}
	for _, seq := range epochs {
		hr, err := sink.Get(hdrs[seq])
		if err != nil {
			return res, fmt.Errorf("walship: get header: %w", err)
		}
		wal, err := io.ReadAll(hr)
		_ = hr.Close()
		if err != nil {
			return res, fmt.Errorf("walship: read header: %w", err)
		}
		h, err := parseHeader(wal)
		if err != nil {
			return res, err
		}
		var next int64
		for _, seg := range byEpoch[seq] {
			var s2 uint32
			var off int64
			base := strings.TrimPrefix(seg, prefix)
			if _, err := fmt.Sscanf(base, "e%d-%d.seg.gz", &s2, &off); err != nil || off != next {
				res.Truncated = true
				break
			}
			r, err := sink.Get(seg)
			if err != nil {
				return res, fmt.Errorf("walship: get segment: %w", err)
			}
			zr, err := gzip.NewReader(r)
			if err != nil {
				_ = r.Close()
				return res, fmt.Errorf("walship: gunzip segment: %w", err)
			}
			b, err := io.ReadAll(zr)
			_ = r.Close()
			if err != nil {
				return res, fmt.Errorf("walship: read segment: %w", err)
			}
			if len(b)%(frameHeaderLen+h.pageSize) != 0 {
				res.Truncated = true
				break
			}
			wal = append(wal, b...)
			next += int64(len(b))
		}
		if next == 0 && len(byEpoch[seq]) == 0 {
			continue
		}
		if err := os.WriteFile(dest+"-wal", wal, 0o600); err != nil {
			return res, fmt.Errorf("walship: write wal: %w", err)
		}
		if err := foldWAL(ctx, dest); err != nil {
			return res, err
		}
		res.Epochs++
		res.FrameBytes += next
		if res.Truncated {
			break
		}
	}
	return res, nil
}

func foldWAL(ctx context.Context, dest string) error {
	db, err := sql.Open("sqlite", "file:"+dest+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("walship: open restored: %w", err)
	}
	defer func() { _ = db.Close() }()
	var busy, logf, ckpt int
	if err := db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logf, &ckpt); err != nil {
		return fmt.Errorf("walship: fold wal: %w", err)
	}
	if busy != 0 {
		return errors.New("walship: fold wal busy")
	}
	return nil
}

func gunzipTo(sink Sink, name, dest string) error {
	r, err := sink.Get(name)
	if err != nil {
		return fmt.Errorf("walship: get %s: %w", name, err)
	}
	defer func() { _ = r.Close() }()
	zr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("walship: gunzip %s: %w", name, err)
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) //nolint:gosec // dest is the caller restore target
	if err != nil {
		return fmt.Errorf("walship: create %s: %w", dest, err)
	}
	if _, err := io.Copy(f, io.LimitReader(zr, maxRestoreBytes)); err != nil {
		_ = f.Close()
		return fmt.Errorf("walship: write %s: %w", dest, err)
	}
	return f.Close()
}
