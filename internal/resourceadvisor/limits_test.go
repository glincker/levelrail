package resourceadvisor

import (
	"testing"
	"time"
)

func TestLimitFindings(t *testing.T) {
	th := DefaultThresholds()
	tests := []struct {
		name      string
		res       Resource
		want      []Kind
		wantValue map[Kind]int64
	}{
		{
			name:      "no memory limit, stable usage suggests p95 plus headroom",
			res:       Resource{Ref: app("web"), Memory: series(week, 5*time.Minute, 0.1, flat(100*mib))},
			want:      []Kind{KindMemoryLimitMissing},
			wantValue: map[Kind]int64{KindMemoryLimitMissing: 192 * mib},
		},
		{
			name: "tiny cpu without limit floors at a tenth of a core",
			res: Resource{Ref: app("web"), MemoryLimit: 512 * mib,
				Memory:     series(week, 5*time.Minute, 0.05, flat(300*mib)),
				CPUPercent: series(week, 5*time.Minute, 0.5, flat(1))},
			want:      []Kind{KindCPULimitMissing},
			wantValue: map[Kind]int64{KindCPULimitMissing: 100_000_000},
		},
		{
			name: "usage near limit is tight",
			res:  Resource{Ref: app("web"), MemoryLimit: 512 * mib, Memory: series(week, 5*time.Minute, 0.02, flat(490*mib))},
			want: []Kind{KindMemoryLimitTight},
		},
		{
			name: "oom kill marks the limit tight even at moderate usage",
			res: Resource{Ref: app("web"), MemoryLimit: 512 * mib, Memory: series(week, 5*time.Minute, 0.05, flat(250*mib)),
				OOM: &OOM{At: testNow.Add(-2 * time.Hour), Excerpt: "Out of memory"}},
			want:      []Kind{KindMemoryLimitTight},
			wantValue: map[Kind]int64{KindMemoryLimitTight: 768 * mib},
		},
		{
			name: "restarts near the limit are tight",
			res:  Resource{Ref: app("web"), MemoryLimit: 512 * mib, Restarts: 4, Memory: series(week, 5*time.Minute, 0.05, flat(420*mib))},
			want: []Kind{KindMemoryLimitTight},
		},
		{
			name: "limit far above p99 for a full week is over provisioned",
			res:  Resource{Ref: app("web"), MemoryLimit: 2 * gib, Memory: series(week, 5*time.Minute, 0.1, flat(100*mib))},
			want: []Kind{KindMemoryOverProvisioned},
		},
		{
			name: "over provisioned needs the full window",
			res:  Resource{Ref: app("web"), MemoryLimit: 2 * gib, Memory: series(3*day, 5*time.Minute, 0.1, flat(100*mib))},
			want: nil,
		},
		{
			name: "comfortable limit gives nothing",
			res:  Resource{Ref: app("web"), MemoryLimit: 512 * mib, Memory: series(week, 5*time.Minute, 0.1, flat(300*mib))},
			want: nil,
		},
		{
			name: "cpu throttled at its cap",
			res: Resource{Ref: app("web"), MemoryLimit: 512 * mib, NanoCPUs: 500_000_000,
				Memory: series(week, 5*time.Minute, 0.05, flat(300*mib)), CPUPercent: series(week, 5*time.Minute, 0.02, flat(48))},
			want: []Kind{KindCPULimitTight},
		},
		{
			name: "cpu cap far above use for a full week",
			res: Resource{Ref: app("web"), MemoryLimit: 512 * mib, NanoCPUs: 4_000_000_000,
				Memory: series(week, 5*time.Minute, 0.05, flat(300*mib)), CPUPercent: series(week, 5*time.Minute, 0.3, flat(10))},
			want: []Kind{KindCPUOverProvisioned},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := kinds(limitFindings(tc.res, th, testNow))
			if len(got) != len(tc.want) {
				t.Fatalf("got %d findings %v, want %v", len(got), got, tc.want)
			}
			for _, k := range tc.want {
				f, ok := got[k]
				if !ok {
					t.Fatalf("missing %s in %v", k, got)
				}
				if f.Action == nil || !f.Action.Appliable() {
					t.Fatalf("%s should carry an appliable action", k)
				}
				if want, ok := tc.wantValue[k]; ok && f.Action.Value != want {
					t.Fatalf("%s value = %d, want %d", k, f.Action.Value, want)
				}
				if f.Evidence.Samples == 0 || f.Evidence.Window != "last 7 days" || len(f.Evidence.Series) == 0 {
					t.Fatalf("%s evidence incomplete: %+v", k, f.Evidence)
				}
			}
		})
	}
}

func TestTightLimitAlwaysRaises(t *testing.T) {
	r := Resource{Ref: app("web"), MemoryLimit: 512 * mib, Memory: series(week, 5*time.Minute, 0.02, flat(500*mib))}
	f := kinds(limitFindings(r, DefaultThresholds(), testNow))[KindMemoryLimitTight]
	if f.Action.Value <= r.MemoryLimit || f.Action.Current != r.MemoryLimit {
		t.Fatalf("tight suggestion %d does not raise %d", f.Action.Value, r.MemoryLimit)
	}
}

func TestOverProvisionedSaving(t *testing.T) {
	r := Resource{Ref: app("web"), MemoryLimit: 2 * gib, Memory: series(week, 5*time.Minute, 0.1, flat(100*mib))}
	f := kinds(limitFindings(r, DefaultThresholds(), testNow))[KindMemoryOverProvisioned]
	if f.Impact.SavingMemoryBytes != r.MemoryLimit-f.Action.Value || f.Impact.SavingMemoryBytes <= 0 {
		t.Fatalf("saving = %d for value %d", f.Impact.SavingMemoryBytes, f.Action.Value)
	}
}
