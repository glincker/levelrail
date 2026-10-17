package placement

import (
	"errors"
	"strings"
	"testing"
)

const gib = int64(1) << 30

func node(id string, memTotal, memReq int64, load int) Candidate {
	return Candidate{
		ID: id, Schedulable: true, Online: true, Load: load,
		Capacity: Capacity{
			CPUMilli: Measured(4000), MemoryBytes: Measured(memTotal),
			DiskTotalBytes: Measured(100 * gib), DiskFreeBytes: Measured(80 * gib),
		},
		Requested: Resources{MemoryBytes: memReq},
	}
}

func TestSpreadResources(t *testing.T) {
	unknown := Candidate{ID: "u", Schedulable: true, Online: true}
	tests := []struct {
		name     string
		cands    []Candidate
		req      Request
		want     string
		errMatch string
	}{
		{
			name:  "more headroom beats lower load",
			cands: []Candidate{node("a", 8*gib, 6*gib, 0), node("b", 8*gib, 1*gib, 5)},
			req:   Request{Needs: Resources{MemoryBytes: gib}},
			want:  "b",
		},
		{
			name:     "node that cannot fit is filtered",
			cands:    []Candidate{node("a", 2*gib, 1*gib, 0), node("b", 8*gib, 7*gib, 9)},
			req:      Request{Needs: Resources{MemoryBytes: 2 * gib}},
			errMatch: "short by",
		},
		{
			name:  "fits exactly",
			cands: []Candidate{node("a", 2*gib, 1*gib, 0)},
			req:   Request{Needs: Resources{MemoryBytes: gib}},
			want:  "a",
		},
		{
			name:  "equal headroom and load falls to id",
			cands: []Candidate{node("b", 8*gib, 0, 1), node("a", 8*gib, 0, 1)},
			want:  "a",
		},
		{
			name:  "overcommit lets a full node take more",
			cands: []Candidate{node("a", 2*gib, 2*gib, 0)},
			req:   Request{Needs: Resources{MemoryBytes: gib}, Policy: Policy{MemoryOvercommitPercent: 200, CPUOvercommitPercent: 100, MinFreeDiskPercent: 10}},
			want:  "a",
		},
		{
			name:  "unknown capacity ranks after a verified node",
			cands: []Candidate{unknown, node("z", 8*gib, 0, 9)},
			req:   Request{Needs: Resources{MemoryBytes: gib}},
			want:  "z",
		},
		{
			name:  "unknown capacity alone is still placed with a warning",
			cands: []Candidate{unknown},
			req:   Request{Needs: Resources{MemoryBytes: gib}},
			want:  "u",
		},
		{
			name:     "unknown capacity rejected by policy",
			cands:    []Candidate{unknown},
			req:      Request{Needs: Resources{MemoryBytes: gib}, Policy: Policy{RejectUnknown: true}},
			errMatch: "capacity unknown",
		},
		{
			name: "disk below the free floor is filtered",
			cands: []Candidate{func() Candidate {
				c := node("a", 8*gib, 0, 0)
				c.Capacity.DiskFreeBytes = Measured(5 * gib)
				return c
			}()},
			req:      Request{Policy: DefaultPolicy()},
			errMatch: "disk: needs 0 B, 0 B free",
		},
		{
			name:  "prefer wins when it fits",
			cands: []Candidate{node("a", 8*gib, 0, 0), node("b", 8*gib, 4*gib, 3)},
			req:   Request{Prefer: "b"},
			want:  "b",
		},
		{
			name:  "prefer is ignored when it does not fit",
			cands: []Candidate{node("a", 8*gib, 0, 0), node("b", 1*gib, 1*gib, 3)},
			req:   Request{Prefer: "b", Needs: Resources{MemoryBytes: gib}},
			want:  "a",
		},
		{
			name:     "cpu shortfall is reported per node",
			cands:    []Candidate{node("a", 8*gib, 0, 0)},
			req:      Request{Needs: Resources{CPUMilli: 8000}},
			errMatch: "cpu: needs 8 CPU",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := Spread(tt.cands, tt.req)
			if tt.errMatch != "" {
				if !errors.Is(err, ErrNoEligibleNode) || !strings.Contains(err.Error(), tt.errMatch) {
					t.Fatalf("error = %v, want ErrNoEligibleNode containing %q", err, tt.errMatch)
				}
				return
			}
			if err != nil || d.NodeID != tt.want {
				t.Fatalf("Spread() = %q, %v; want %q", d.NodeID, err, tt.want)
			}
			if d.Why == "" || len(d.Considered) != len(tt.cands) {
				t.Fatalf("decision not explained: %+v", d)
			}
		})
	}
}

func TestSpreadDeterministic(t *testing.T) {
	cands := []Candidate{node("c", 8*gib, 0, 0), node("a", 8*gib, 0, 0), node("b", 8*gib, 0, 0)}
	for i := 0; i < 20; i++ {
		d, err := Spread(cands, Request{})
		if err != nil || d.NodeID != "a" {
			t.Fatalf("run %d: %q, %v", i, d.NodeID, err)
		}
		cands[0], cands[2] = cands[2], cands[0]
	}
}

func TestUnknownIsNotZeroOrInfinite(t *testing.T) {
	c := Candidate{ID: "u", Schedulable: true, Online: true}
	e := Evaluate(c, Request{Needs: Resources{MemoryBytes: gib}})
	if !e.Eligible || !e.Unverified || e.Capacity {
		t.Fatalf("unknown node must be eligible and unverified, got %+v", e)
	}
	for _, d := range e.Dimensions {
		if d.Allocatable != nil || d.Free != nil {
			t.Fatalf("unknown dimension %s must not carry numbers", d.Name)
		}
	}
	if e.Headroom != nil {
		t.Fatalf("unknown node has no headroom score")
	}
}

func TestPartialReportVerifiesWhatItCan(t *testing.T) {
	c := node("a", 1*gib, 0, 0)
	c.Capacity.CPUMilli = Amount{}
	e := Evaluate(c, Request{Needs: Resources{MemoryBytes: 2 * gib}})
	if e.Eligible || !e.Capacity {
		t.Fatalf("known memory shortfall must reject despite unknown cpu: %+v", e)
	}
	ok := Evaluate(c, Request{Needs: Resources{MemoryBytes: gib / 2}})
	if !ok.Eligible || ok.Unverified {
		t.Fatalf("unrequested unknown cpu must not mark the node unverified: %+v", ok)
	}
}

func TestPinnedWarnsButHonors(t *testing.T) {
	cands := []Candidate{node("a", 1*gib, 1*gib, 0), node("b", 8*gib, 0, 0)}
	d := Pinned(cands, Request{Needs: Resources{MemoryBytes: gib}}, "a")
	if d.NodeID != "a" || len(d.Warnings) != 1 || !d.CapacityBlocked() {
		t.Fatalf("pin must be honored with a warning: %+v", d)
	}
}

func TestPolicyValidate(t *testing.T) {
	if err := DefaultPolicy().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []Policy{
		{CPUOvercommitPercent: 0, MemoryOvercommitPercent: 100},
		{CPUOvercommitPercent: 100, MemoryOvercommitPercent: 2000},
		{CPUOvercommitPercent: 100, MemoryOvercommitPercent: 100, MinFreeDiskPercent: 95},
	} {
		if p.Validate() == nil {
			t.Errorf("Validate(%+v) = nil, want error", p)
		}
	}
}
