package resourceadvisor

import (
	"strings"
	"testing"
	"time"
)

func smallPostgres(name string, mem float64) Resource {
	return Resource{Ref: db(name), NodeID: "n1", Engine: "postgres", Memory: series(2*day, 5*time.Minute, 0.05, flat(mem))}
}

func TestConsolidateFinding(t *testing.T) {
	th := DefaultThresholds()
	node := Node{ID: "n1", Name: "vps"}
	six := []Resource{smallPostgres("a", 60*mib), smallPostgres("b", 70*mib), smallPostgres("c", 50*mib), smallPostgres("d", 120*mib), smallPostgres("e", 80*mib), smallPostgres("f", 90*mib)}
	tests := []struct {
		name string
		dbs  []Resource
		want int
	}{
		{"six small postgres", six, 1},
		{"three is below the count", six[:3], 0},
		{"large ones do not count", []Resource{smallPostgres("a", 600*mib), smallPostgres("b", 700*mib), smallPostgres("c", 500*mib), smallPostgres("d", 900*mib)}, 0},
		{"mixed engines are grouped apart", append(append([]Resource{}, six[:2]...), Resource{Ref: db("r"), Engine: "redis", Memory: six[0].Memory}, Resource{Ref: db("s"), Engine: "redis", Memory: six[0].Memory}), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := consolidateFinding(node, tc.dbs, th, testNow)
			if len(got) != tc.want {
				t.Fatalf("got %d findings, want %d", len(got), tc.want)
			}
			if tc.want == 1 {
				f := got[0]
				if f.Action.Appliable() || !strings.Contains(f.Impact.Risk, "failure domain") {
					t.Fatalf("consolidation must be manual with tradeoffs: %+v", f)
				}
				if f.Impact.SavingMemoryBytes <= 0 {
					t.Fatal("expected a saving estimate")
				}
			}
		})
	}
}

func TestDiskFindings(t *testing.T) {
	th := DefaultThresholds()
	grow := func(from, to float64) func(float64) float64 {
		return func(x float64) float64 { return from + (to-from)*x }
	}
	tests := []struct {
		name string
		node Node
		want []Kind
	}{
		{"fills within horizon", Node{ID: "n1", DiskTotal: 100 * gib, DiskUsed: series(week, time.Hour, 0.001, grow(50*gib, 85*gib))}, []Kind{KindDiskForecast}},
		{"flat disk", Node{ID: "n1", DiskTotal: 100 * gib, DiskUsed: series(week, time.Hour, 0.001, flat(50*gib))}, nil},
		{"slow growth beyond horizon", Node{ID: "n1", DiskTotal: 1000 * gib, DiskUsed: series(week, time.Hour, 0, grow(50*gib, 51*gib))}, nil},
		{"reclaimable docker data", Node{ID: "n1", Reclaimable: &Reclaimable{ImagesBytes: 3 * gib, BuildCacheBytes: gib}}, []Kind{KindReclaimableDisk}},
		{"little reclaimable", Node{ID: "n1", Reclaimable: &Reclaimable{ImagesBytes: 100 * mib}}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := diskFindings(tc.node, th, testNow)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i, k := range tc.want {
				if got[i].Kind != k {
					t.Fatalf("got %s, want %s", got[i].Kind, k)
				}
			}
		})
	}
}

func TestHostFindings(t *testing.T) {
	th := DefaultThresholds()
	consumers := []Resource{
		{Ref: db("askverdict"), Memory: series(2*time.Hour, time.Minute, 0, flat(900*mib))},
		{Ref: db("glinr"), Memory: series(2*time.Hour, time.Minute, 0, flat(600*mib))},
		{Ref: app("web"), Memory: series(2*time.Hour, time.Minute, 0, flat(500*mib))},
		{Ref: db("n8n"), Memory: series(2*time.Hour, time.Minute, 0, flat(50*mib))},
		{Ref: db("geokit"), Memory: series(2*time.Hour, time.Minute, 0, flat(60*mib))},
	}
	tests := []struct {
		name string
		node Node
		res  []Resource
		want []Kind
	}{
		{"healthy node", Node{ID: "n1", MemoryTotal: 8 * gib, MemoryAvailable: series(2*time.Hour, time.Minute, 0, flat(6*gib))}, nil, nil},
		{"pressure", Node{ID: "n1", MemoryTotal: 8 * gib, MemoryAvailable: series(2*time.Hour, time.Minute, 0, flat(400*mib))}, nil, []Kind{KindHostMemoryPressure}},
		{"capacity unknown", Node{ID: "local", NoIdentity: true}, nil, []Kind{KindCapacityUnknown}},
		{"swap in use", Node{ID: "n1", MemoryTotal: 8 * gib, SwapUsed: series(2*time.Hour, time.Minute, 0, flat(gib))}, nil, []Kind{KindSwapInUse}},
		{"top consumers", Node{ID: "n1", MemoryTotal: 8 * gib}, consumers, []Kind{KindTopConsumers}},
		{"evenly spread is not flagged", Node{ID: "n1", MemoryTotal: 8 * gib}, []Resource{consumers[3], consumers[4], {Ref: app("a"), Memory: consumers[3].Memory}, {Ref: app("b"), Memory: consumers[4].Memory}, {Ref: app("c"), Memory: consumers[3].Memory}}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hostFindings(tc.node, tc.res, th, testNow)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d %v, want %v", len(got), got, tc.want)
			}
			for i, k := range tc.want {
				if got[i].Kind != k {
					t.Fatalf("got %s, want %s", got[i].Kind, k)
				}
			}
		})
	}
}
