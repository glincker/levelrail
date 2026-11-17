package build

import "testing"

func TestEffectiveMemoryPercentMB(t *testing.T) {
	g := MemGuard{Percent: 60, ReserveMB: 512, NodeHeapPercent: 75}
	for name, c := range map[string]struct{ pct, host, want int }{
		"percent of ram":     {75, 8192, 6144},
		"zero is unlimited":  {0, 8192, 0},
		"detection failed":   {75, 0, 0},
		"reserve is kept":    {100, 4096, 3584},
		"small host floored": {75, 1024, 512},
	} {
		if got := EffectiveMemoryPercentMB(c.pct, c.host, g); got != c.want {
			t.Errorf("%s: got %d, want %d", name, got, c.want)
		}
	}
}

func TestEffectiveCPUMillis(t *testing.T) {
	for name, c := range map[string]struct{ fixed, pct, cpus, want int }{
		"percent of cores":   {0, 75, 8, 6000},
		"zero is unlimited":  {0, 0, 8, 0},
		"detection failed":   {0, 75, 0, 0},
		"env seed wins":      {2500, 75, 8, 2500},
		"tiny share floored": {0, 5, 1, 100},
	} {
		if got := EffectiveCPUMillis(c.fixed, c.pct, c.cpus); got != c.want {
			t.Errorf("%s: got %d, want %d", name, got, c.want)
		}
	}
}
