package diskhistory

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const gib = int64(1) << 30

func openStore(t *testing.T) *Store {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewStore(db.DB)
}

// series builds n daily samples ending at base, used growing by perDay bytes.
func series(n int, base time.Time, startUsed, perDay, total int64) []Sample {
	out := make([]Sample, 0, n)
	for i := 0; i < n; i++ {
		used := startUsed + int64(i)*perDay
		out = append(out, Sample{At: base.Add(-time.Duration(n-1-i) * 24 * time.Hour), TotalBytes: total, FreeBytes: total - used})
	}
	return out
}

func TestStoreRoundTripAndPrune(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	old := Sample{At: now.Add(-100 * 24 * time.Hour), FreeBytes: 5, TotalBytes: 10}
	cur := Sample{At: now, FreeBytes: 40 * gib, TotalBytes: 100 * gib, ImagesBytes: 7, ContainersBytes: 8, VolumesBytes: 9, BuildCacheBytes: 10}
	for _, sm := range []Sample{old, cur} {
		if err := s.Insert(ctx, sm); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Since(ctx, now.Add(-200*24*time.Hour))
	if err != nil || len(got) != 2 {
		t.Fatalf("Since = %v, %v", got, err)
	}
	if !got[1].At.Equal(cur.At) || got[1].ImagesBytes != 7 || got[1].BuildCacheBytes != 10 {
		t.Fatalf("round trip mismatch: %+v", got[1])
	}
	n, err := s.Prune(ctx, now.Add(-90*24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("Prune = %d, %v", n, err)
	}
	if got, _ = s.Since(ctx, time.Time{}); len(got) != 1 {
		t.Fatalf("after prune: %d samples", len(got))
	}
}

func TestForecastTable(t *testing.T) {
	base := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		samples []Sample
		wantNil bool
		want    float64
	}{
		{name: "empty", wantNil: true},
		{name: "six days is too few", samples: series(6, base, 10*gib, gib, 100*gib), wantNil: true},
		{name: "flat usage", samples: series(10, base, 10*gib, 0, 100*gib), wantNil: true},
		{name: "shrinking usage", samples: series(10, base, 50*gib, -gib, 100*gib), wantNil: true},
		{name: "seven days growing 1 GiB per day", samples: series(7, base, 10*gib, gib, 100*gib), want: 84},
		{name: "30 days growing 2 GiB per day", samples: series(30, base, 10*gib, 2*gib, 200*gib), want: 66},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Forecast(c.samples)
			if c.wantNil {
				if got != nil {
					t.Fatalf("want nil, got %+v", got)
				}
				return
			}
			if got == nil || got.DaysUntilFull < c.want-0.5 || got.DaysUntilFull > c.want+0.5 {
				t.Fatalf("got %+v, want about %v days", got, c.want)
			}
		})
	}
}

func TestSamplerTickRecordsOncePerIntervalAndPrunes(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	now := time.Now().UTC()
	smp := NewSampler(s, Source{Stat: func() (int64, int64, error) { return 10 * gib, 100 * gib, nil }},
		Settings{Interval: func() time.Duration { return 24 * time.Hour }, Retention: func() time.Duration { return 7 * 24 * time.Hour }},
		time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	smp.now = func() time.Time { return now }
	_ = s.Insert(ctx, Sample{At: now.Add(-10 * 24 * time.Hour), FreeBytes: 1, TotalBytes: 2})
	for i := 0; i < 2; i++ {
		if err := smp.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := s.Since(ctx, time.Time{})
	if len(got) != 1 || got[0].FreeBytes != 10*gib {
		t.Fatalf("want one fresh sample after prune, got %+v", got)
	}
}
