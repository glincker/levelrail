package backup

import (
	"context"
	"io"
	"sync/atomic"
	"time"
)

// uploadKBPerSec returns the live upload cap in KB/s; 0 or nil means unlimited.
var uploadKBPerSec atomic.Pointer[func() int]

// SetUploadLimit installs the live upload speed cap getter, read on every upload.
func SetUploadLimit(get func() int) { uploadKBPerSec.Store(&get) }

func currentUploadLimit() int {
	if p := uploadKBPerSec.Load(); p != nil && *p != nil {
		return (*p)()
	}
	return 0
}

// throttledReader paces reads to kbps so a backup upload leaves bandwidth for the apps.
type throttledReader struct {
	ctx   context.Context
	r     io.Reader
	kbps  int
	start time.Time
	n     int64
	sleep func(context.Context, time.Duration) error
}

func newThrottledReader(ctx context.Context, r io.Reader, kbps int) io.Reader {
	if kbps <= 0 {
		return r
	}
	return &throttledReader{ctx: ctx, r: r, kbps: kbps, start: time.Now(), sleep: sleepCtx}
}

func (t *throttledReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	t.n += int64(n)
	// Sleep until the bytes sent so far fit the cap, measured from the start so bursts average out.
	due := time.Duration(float64(t.n) / float64(t.kbps*1024) * float64(time.Second))
	if wait := due - time.Since(t.start); wait > 0 {
		if serr := t.sleep(t.ctx, wait); serr != nil {
			return n, serr
		}
	}
	return n, err
}
