package supplychain

import "testing"

func TestDecideStrictMissingScan(t *testing.T) {
	tests := []struct {
		name   string
		mode   GateMode
		scan   *ScanSummary
		closed bool
		want   string
	}{
		{"missing scan allowed by default", GateBlockOnCritical, nil, false, ActionAllow},
		{"missing scan denied when strict", GateBlockOnCritical, nil, true, ActionBlock},
		{"missing scan allowed in warn mode", GateWarn, nil, true, ActionAllow},
		{"unknown severity ignored by default", GateBlockOnCritical, &ScanSummary{Counts: Counts{Unknown: 1}}, false, ActionAllow},
		{"unknown severity blocks when strict", GateBlockOnCritical, &ScanSummary{Counts: Counts{Unknown: 1}}, true, ActionBlock},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DecideStrict(tc.mode, tc.scan, "", tc.closed)
			if got.Action != tc.want {
				t.Fatalf("action = %q (%s), want %q", got.Action, got.Reason, tc.want)
			}
		})
	}
}
