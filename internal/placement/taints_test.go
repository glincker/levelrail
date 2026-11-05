package placement

import (
	"errors"
	"strings"
	"testing"
)

func capNode(id string, requestedCPU int64) Candidate {
	return Candidate{
		ID: id, Schedulable: true, Online: true,
		Capacity:  Capacity{CPUMilli: Measured(4000), MemoryBytes: Measured(8 << 30), DiskTotalBytes: Measured(100 << 30), DiskFreeBytes: Measured(90 << 30)},
		Requested: Resources{CPUMilli: requestedCPU},
	}
}

func TestTolerationTolerates(t *testing.T) {
	gpu := Taint{Key: "gpu", Value: "a100", Effect: EffectNoSchedule}
	tests := []struct {
		name string
		tol  Toleration
		want bool
	}{
		{"equal match", Toleration{Key: "gpu", Value: "a100"}, true},
		{"equal wrong value", Toleration{Key: "gpu", Value: "t4"}, false},
		{"equal wrong key", Toleration{Key: "ssd", Value: "a100"}, false},
		{"exists on key", Toleration{Key: "gpu", Operator: TolerationExists}, true},
		{"exists matches every taint", Toleration{Operator: TolerationExists}, true},
		{"effect must match", Toleration{Key: "gpu", Value: "a100", Effect: EffectNoExecute}, false},
		{"effect matches", Toleration{Key: "gpu", Value: "a100", Effect: EffectNoSchedule}, true},
		{"unknown operator tolerates nothing", Toleration{Key: "gpu", Operator: "Gt"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tol.Tolerates(gpu); got != tt.want {
				t.Fatalf("Tolerates = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSpreadHonorsTaints(t *testing.T) {
	tainted := capNode("gpu-node", 0)
	tainted.Taints = []Taint{{Key: "gpu", Value: "a100", Effect: EffectNoSchedule}}
	plain := capNode("plain", 3000)

	d, err := Spread([]Candidate{tainted, plain}, Request{})
	if err != nil || d.NodeID != "plain" {
		t.Fatalf("untolerated taint must keep the workload off: %+v %v", d, err)
	}
	var reason string
	for _, r := range d.Rejected {
		if r.NodeID == "gpu-node" {
			reason = r.Reason
		}
	}
	if !strings.Contains(reason, "gpu=a100:NoSchedule") {
		t.Fatalf("rejection should name the taint, got %q", reason)
	}

	d, err = Spread([]Candidate{tainted, plain}, Request{Tolerations: []Toleration{{Key: "gpu", Operator: TolerationExists}}})
	if err != nil || d.NodeID != "gpu-node" {
		t.Fatalf("tolerated node has the most headroom: %+v %v", d, err)
	}

	only := Candidate{ID: "only", Schedulable: true, Online: true, Taints: []Taint{{Key: "maint", Effect: EffectNoExecute}}}
	if _, err := Spread([]Candidate{only}, Request{}); !errors.Is(err, ErrNoEligibleNode) {
		t.Fatalf("NoExecute must filter like NoSchedule, got %v", err)
	}
}

func TestPreferNoScheduleOnlyLowersRank(t *testing.T) {
	soft := capNode("soft", 0)
	soft.Taints = []Taint{{Key: "spot", Effect: EffectPreferNoSchedule}}
	busy := capNode("busy", 2000)

	d, err := Spread([]Candidate{soft, busy}, Request{})
	if err != nil || d.NodeID != "busy" {
		t.Fatalf("an untolerated soft taint ranks the node last even with more headroom: %+v %v", d, err)
	}
	d, err = Spread([]Candidate{soft}, Request{})
	if err != nil || d.NodeID != "soft" {
		t.Fatalf("a soft taint must not filter the only node: %+v %v", d, err)
	}
	d, err = Spread([]Candidate{soft, busy}, Request{Tolerations: []Toleration{{Key: "spot", Operator: TolerationExists}}})
	if err != nil || d.NodeID != "soft" {
		t.Fatalf("a tolerated soft taint no longer counts: %+v %v", d, err)
	}
}

func TestBinpackPrefersTheFullestNodeThatFits(t *testing.T) {
	empty, half, full := capNode("empty", 0), capNode("half", 2000), capNode("full", 3500)
	cands := []Candidate{empty, half, full}
	need := Resources{CPUMilli: 400}

	d, err := Spread(cands, Request{Needs: need})
	if err != nil || d.NodeID != "empty" {
		t.Fatalf("default stays spread: %+v %v", d, err)
	}
	d, err = Spread(cands, Request{Needs: need, Policy: Policy{ScoreMode: ScoreBinpack}})
	if err != nil || d.NodeID != "full" {
		t.Fatalf("binpack fills the fullest node: %+v %v", d, err)
	}
	if !strings.HasPrefix(d.Why, "bin-packed") {
		t.Fatalf("decision should explain the mode, got %q", d.Why)
	}
	d, err = Spread(cands, Request{Needs: Resources{CPUMilli: 1000}, Policy: Policy{ScoreMode: ScoreBinpack}})
	if err != nil || d.NodeID != "half" {
		t.Fatalf("binpack still respects the fit check: %+v %v", d, err)
	}
}

func TestValidateTaintsAndPolicy(t *testing.T) {
	if err := ValidateTaints([]Taint{{Key: "gpu", Value: "a100", Effect: EffectNoSchedule}}); err != nil {
		t.Fatal(err)
	}
	bad := [][]Taint{
		{{Key: "Bad Key", Effect: EffectNoSchedule}},
		{{Key: "gpu", Effect: "Sometimes"}},
		{{Key: "gpu", Value: "bad value!", Effect: EffectNoSchedule}},
		{{Key: "gpu", Effect: EffectNoSchedule}, {Key: "gpu", Effect: EffectNoSchedule}},
	}
	for i, taints := range bad {
		if err := ValidateTaints(taints); err == nil {
			t.Errorf("case %d should fail", i)
		}
	}
	if err := ValidateTolerations([]Toleration{{Key: "gpu", Operator: TolerationExists}, {Key: "a", Value: "b"}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTolerations([]Toleration{{Operator: TolerationEqual}}); err == nil {
		t.Error("Equal without a key should fail")
	}
	p := DefaultPolicy()
	p.ScoreMode = "random"
	if err := p.Validate(); err == nil {
		t.Error("unknown score mode should fail")
	}
}
