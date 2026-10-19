package walship

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func cpuSeconds() float64 {
	var ru syscall.Rusage
	_ = syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return float64(ru.Utime.Sec+ru.Stime.Sec) + float64(ru.Utime.Usec+ru.Stime.Usec)/1e6
}

func rssKB() int64 {
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output() //nolint:gosec // fixed argv, own pid
	if err != nil {
		return 0
	}
	v, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	return v
}

func gzipSize(t *testing.T, path string) int64 {
	f, err := os.Open(path) //nolint:gosec // test path
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	cw := &countWriter{}
	zw := gzip.NewWriter(cw)
	_, _ = io.Copy(zw, f)
	_ = zw.Close()
	return cw.n
}

type countWriter struct{ n int64 }

func (c *countWriter) Write(p []byte) (int, error) { c.n += int64(len(p)); return len(p), nil }

func pct(d []time.Duration, p float64) time.Duration {
	if len(d) == 0 {
		return 0
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	return d[int(float64(len(d)-1)*p)]
}

func TestLiveReplicationLagAndBytes(t *testing.T) {
	if testing.Short() {
		t.Skip("live WAL shipping benchmark")
	}
	for _, interval := range []time.Duration{100 * time.Millisecond, time.Second} {
		dir := t.TempDir()
		path := filepath.Join(dir, "p.db")
		db := openPrimary(t, path)
		sink := DirSink{Dir: filepath.Join(dir, "replica")}
		sh, err := New(path, sink, 0)
		if err != nil {
			t.Fatal(err)
		}
		var mu sync.Mutex
		var commits []time.Time
		var lags []time.Duration
		stop := make(chan struct{})
		done := make(chan struct{})
		go func() {
			defer close(done)
			prev := 0
			for {
				mu.Lock()
				n := len(commits)
				mu.Unlock()
				if err := sh.Sync(context.Background()); err != nil {
					t.Error(err)
					return
				}
				end := time.Now()
				mu.Lock()
				for _, c := range commits[prev:n] {
					lags = append(lags, end.Sub(c))
				}
				mu.Unlock()
				prev = n
				select {
				case <-stop:
					return
				case <-time.After(interval):
				}
			}
		}()
		start := time.Now()
		logical, err := churn(db, newRand(7), 400, 2*time.Millisecond, func(ts time.Time) {
			mu.Lock()
			commits = append(commits, ts)
			mu.Unlock()
		})
		if err != nil {
			t.Fatal(err)
		}
		dur := time.Since(start)
		close(stop)
		<-done
		st := sh.Stats()
		_ = sh.Close()
		_ = db.Close()
		shipped := st.SnapshotBytes + st.SegmentBytes
		mb := float64(logical) / (1 << 20)
		fi, _ := os.Stat(path)
		model := gzipSize(t, path) * int64(dur/(5*time.Second)+1)
		mu.Lock()
		l := append([]time.Duration(nil), lags...)
		nc := len(commits)
		mu.Unlock()
		t.Logf("interval=%s commits=%d dur=%s logical=%.2fMB db=%.2fMB walFrames=%.2fMB shippedSeg=%.2fMB snap=%.2fMB gens=%d checkpoints=%d",
			interval, nc, dur.Round(time.Millisecond), mb, float64(fi.Size())/(1<<20), float64(st.FrameBytes)/(1<<20), float64(st.SegmentBytes)/(1<<20), float64(st.SnapshotBytes)/(1<<20), st.Generations, st.Checkpoints)
		t.Logf("interval=%s lag p50=%s p95=%s p99=%s max=%s | shipped per logical MB=%.2fMB (segments only %.2fMB) | 5s-snapshot model total=%.2fMB (%.2fMB per logical MB)",
			interval, pct(l, 0.5), pct(l, 0.95), pct(l, 0.99), pct(l, 1), float64(shipped)/(1<<20)/mb, float64(st.SegmentBytes)/(1<<20)/mb, float64(model)/(1<<20), float64(model)/(1<<20)/mb)
	}
}

func TestLiveOverhead(t *testing.T) {
	if testing.Short() {
		t.Skip("live WAL shipping benchmark")
	}
	run := func(ship bool) (cpu float64, peakRSS int64, wall time.Duration) {
		dir := t.TempDir()
		path := filepath.Join(dir, "p.db")
		db := openPrimary(t, path)
		defer func() { _ = db.Close() }()
		stop := make(chan struct{})
		var wg sync.WaitGroup
		var sh *Shipper
		if ship {
			var err error
			sh, err = New(path, DirSink{Dir: filepath.Join(dir, "replica")}, 0)
			if err != nil {
				t.Fatal(err)
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = sh.Run(contextUntil(stop), 100*time.Millisecond)
			}()
		}
		var peak int64
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if v := rssKB(); v > peak {
					peak = v
				}
				select {
				case <-stop:
					return
				case <-time.After(100 * time.Millisecond):
				}
			}
		}()
		runtime.GC()
		c0 := cpuSeconds()
		t0 := time.Now()
		if _, err := churn(db, newRand(9), 600, time.Millisecond, nil); err != nil {
			t.Fatal(err)
		}
		cpu = cpuSeconds() - c0
		wall = time.Since(t0)
		close(stop)
		wg.Wait()
		if sh != nil {
			_ = sh.Close()
		}
		return cpu, peak, wall
	}
	for i := 0; i < 3; i++ {
		c0, r0, w0 := run(false)
		c1, r1, w1 := run(true)
		t.Logf("round %d: baseline cpu=%.2fs rss=%dMB wall=%s | shipper cpu=%.2fs rss=%dMB wall=%s | delta cpu=%.2fs (%.1f%% of wall) rss=%+dMB",
			i, c0, r0/1024, w0.Round(time.Millisecond), c1, r1/1024, w1.Round(time.Millisecond), c1-c0, (c1-c0)/w1.Seconds()*100, (r1-r0)/1024)
	}
}

func TestLiveKillPrimaryMidWrite(t *testing.T) {
	if testing.Short() {
		t.Skip("live WAL shipping benchmark")
	}
	for trial := 0; trial < 5; trial++ {
		dir := t.TempDir()
		cmd := exec.Command(os.Args[0], "-test.run=^TestWalshipChild$") //nolint:gosec // re-exec of this test binary
		cmd.Env = append(os.Environ(), "WALSHIP_CHILD_DIR="+dir)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Duration(3000+trial*900) * time.Millisecond)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		dest := filepath.Join(dir, "restored.db")
		res, err := Restore(context.Background(), DirSink{Dir: filepath.Join(dir, "replica")}, dest)
		if err != nil {
			t.Fatalf("trial %d restore: %v", trial, err)
		}
		pd, pe, pcheck := counts(t, filepath.Join(dir, "p.db"))
		rd, re, rcheck := counts(t, dest)
		var maxID int64
		rdb := openPrimary(t, dest)
		_ = rdb.QueryRow(`SELECT coalesce(max(id),0) FROM deploys`).Scan(&maxID)
		_ = rdb.Close()
		t.Logf("trial %d: primary deploys=%d events=%d integrity=%s | replica deploys=%d events=%d integrity=%s contiguous=%v | lost deploys=%d events=%d | gen=%d epochs=%d",
			trial, pd, pe, pcheck, rd, re, rcheck, maxID == rd, pd-rd, pe-re, res.Generation, res.Epochs)
		if rcheck != "ok" || pcheck != "ok" || maxID != rd || rd > pd || re > pe {
			t.Fatalf("trial %d inconsistent restore", trial)
		}
	}
}

func TestWalshipChild(t *testing.T) {
	dir := os.Getenv("WALSHIP_CHILD_DIR")
	if dir == "" {
		t.Skip("child process only")
	}
	path := filepath.Join(dir, "p.db")
	db := openPrimary(t, path)
	sh, err := New(path, DirSink{Dir: filepath.Join(dir, "replica")}, 256<<10)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = sh.Run(context.Background(), 100*time.Millisecond) }()
	r := newRand(time.Now().UnixNano())
	for {
		if _, err := churn(db, r, 50, time.Millisecond, nil); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return
		}
	}
}

func contextUntil(stop <-chan struct{}) context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-stop; cancel() }()
	return ctx
}
