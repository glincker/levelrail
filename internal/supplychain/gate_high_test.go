package supplychain

import "testing"

func TestDecideBlockOnHigh(t *testing.T) {
	tests := []struct {
		name     string
		mode     GateMode
		counts   Counts
		override string
		want     string
	}{
		{"high blocks block_on_high", GateBlockOnHigh, Counts{High: 1}, "", ActionBlock},
		{"critical blocks block_on_high", GateBlockOnHigh, Counts{Critical: 1}, "", ActionBlock},
		{"medium passes block_on_high", GateBlockOnHigh, Counts{Medium: 9}, "", ActionAllow},
		{"high passes block_on_critical", GateBlockOnCritical, Counts{High: 4}, "", ActionAllow},
		{"override lets block_on_high through", GateBlockOnHigh, Counts{High: 1}, "hotfix", ActionOverride},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Decide(tc.mode, &ScanSummary{Counts: tc.counts}, tc.override)
			if got.Action != tc.want {
				t.Fatalf("action = %q (%s), want %q", got.Action, got.Reason, tc.want)
			}
		})
	}
	if m, err := ParseGateMode("block_on_high"); err != nil || m != GateBlockOnHigh {
		t.Fatalf("ParseGateMode = %q, %v", m, err)
	}
}
