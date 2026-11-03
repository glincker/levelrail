package spec

import (
	"strings"
	"testing"
)

func TestParse_PlacementRules(t *testing.T) {
	base := "version: 1\nservices:\n  web:\n    build:\n      type: image\n      image: nginx:1\n    port: 80\n"
	tests := []struct {
		name    string
		extra   string
		wantErr string
	}{
		{name: "valid", extra: "    tolerations:\n      - {key: dedicated, operator: Equal, value: gpu, effect: NoSchedule}\n    affinity:\n      node_required:\n        - {key: disk, op: In, values: [ssd]}\n      apart:\n        - match: [{key: name, op: In, values: [db]}]\n          topology: zone\n"},
		{name: "none"},
		{name: "bad operator", extra: "    tolerations:\n      - {key: a, operator: Maybe}\n", wantErr: "operator"},
		{name: "too many terms", extra: "    affinity:\n      node_required:\n" + strings.Repeat("        - {key: a, op: Exists}\n", 9), wantErr: "at most"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Parse([]byte(base + tt.extra))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Parse() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if tt.extra != "" && (len(s.Services["web"].Tolerations) != 1 || s.Services["web"].Affinity == nil) {
				t.Fatalf("rules not parsed: %+v", s.Services["web"])
			}
		})
	}
}
