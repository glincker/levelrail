package api

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type benchWatchLine struct {
	text string
	at   time.Time
}

func benchOpenWatch(ctx context.Context, b *testing.B, url string, cookie *http.Cookie) <-chan benchWatchLine {
	b.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/v2/apps?watch=1", nil)
	if err != nil {
		b.Fatal(err)
	}
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		b.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		b.Fatalf("watch status %d", resp.StatusCode)
	}
	out := make(chan benchWatchLine, 4096)
	go func() {
		defer func() { _ = resp.Body.Close() }()
		defer close(out)
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			if line := sc.Text(); strings.HasPrefix(line, "data:") {
				select {
				case out <- benchWatchLine{text: line, at: time.Now()}:
				default:
				}
			}
		}
	}()
	return out
}

// BenchmarkV2WatchFanout measures write-to-delivery delay with N concurrent poll-based streams over a seeded store.
func BenchmarkV2WatchFanout(b *testing.B) {
	if os.Getenv(envWatchPollInterval) == "" {
		b.Setenv(envWatchPollInterval, "25ms")
	}
	for _, streams := range []int{1, 16, 64} {
		b.Run(fmt.Sprintf("streams=%d", streams), func(b *testing.B) {
			rt, db := newTestRouter(b)
			cookie, err := loginTestSessionSlow(rt, db)
			if err != nil {
				b.Fatal(err)
			}
			h := rt.Handler()
			benchSeedApps(b, db, 200)
			srv := httptest.NewServer(h)
			defer srv.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			chans := make([]<-chan benchWatchLine, streams)
			for i := range chans {
				chans[i] = benchOpenWatch(ctx, b, srv.URL, cookie)
			}
			var sumMS, maxMS float64
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				name := fmt.Sprintf("bench-evt-%d", i)
				start := time.Now()
				svc := store.DesiredService{Name: name, Image: "nginx:1", Port: 80}
				if err := db.SaveDesiredService(context.Background(), svc); err != nil {
					b.Fatal(err)
				}
				var worst time.Duration
				for _, ch := range chans {
					d, ok := benchAwaitEvent(ch, name, start)
					if !ok {
						b.Fatalf("stream closed or timed out waiting for %s", name)
					}
					sumMS += float64(d) / float64(time.Millisecond)
					worst = max(worst, d)
				}
				maxMS = max(maxMS, float64(worst)/float64(time.Millisecond))
			}
			b.StopTimer()
			b.ReportMetric(sumMS/float64(b.N*streams), "ms/event-mean")
			b.ReportMetric(maxMS, "ms/event-max")
		})
	}
}

func benchAwaitEvent(ch <-chan benchWatchLine, name string, start time.Time) (time.Duration, bool) {
	timeout := time.After(15 * time.Second)
	for {
		select {
		case l, ok := <-ch:
			if !ok {
				return 0, false
			}
			if strings.Contains(l.text, `"`+name+`"`) {
				return l.at.Sub(start), true
			}
		case <-timeout:
			return 0, false
		}
	}
}
