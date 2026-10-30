package placement

import (
	"errors"
	"strings"
	"testing"
)

func TestSpread(t *testing.T) {
	ssd := map[string]string{"disk": "ssd"}
	tests := []struct {
		name     string
		cands    []Candidate
		req      Request
		want     string
		wantErr  bool
		errMatch string
	}{
		{
			name:  "least loaded wins",
			cands: []Candidate{{ID: "a", Schedulable: true, Online: true, Load: 3}, {ID: "b", Schedulable: true, Online: true, Load: 1}},
			want:  "b",
		},
		{
			name: "empty node with unknown capacity beats a busy node with known headroom",
			cands: []Candidate{
				{ID: "node-a", Schedulable: true, Online: true},
				{ID: "node-b", Schedulable: true, Online: true, Load: 2, Capacity: Capacity{CPUMilli: Measured(4000), MemoryBytes: Measured(8 << 30)}},
			},
			want: "node-a",
		},
		{
			name:  "tie broken by id",
			cands: []Candidate{{ID: "c", Schedulable: true, Online: true}, {ID: "b", Schedulable: true, Online: true}},
			want:  "b",
		},
		{
			name:  "cordoned and offline skipped",
			cands: []Candidate{{ID: "a", Online: true}, {ID: "b", Schedulable: true}, {ID: "c", Schedulable: true, Online: true, Load: 9}},
			want:  "c",
		},
		{
			name:  "exclude the drained node",
			cands: []Candidate{{ID: "a", Schedulable: true, Online: true}, {ID: "b", Schedulable: true, Online: true, Load: 5}},
			req:   Request{Exclude: "a"},
			want:  "b",
		},
		{
			name:  "selector filters before load",
			cands: []Candidate{{ID: "a", Schedulable: true, Online: true}, {ID: "b", Schedulable: true, Online: true, Labels: ssd, Load: 4}},
			req:   Request{Selector: Selector{"disk": "ssd"}},
			want:  "b",
		},
		{
			name:     "nothing matches",
			cands:    []Candidate{{ID: "a", Schedulable: true, Online: true, Labels: map[string]string{"disk": "hdd"}}},
			req:      Request{Selector: Selector{"disk": "ssd"}},
			wantErr:  true,
			errMatch: "labels do not match disk=ssd",
		},
		{
			name:     "only the excluded node",
			cands:    []Candidate{{ID: "a", Schedulable: true, Online: true}},
			req:      Request{Exclude: "a"},
			wantErr:  true,
			errMatch: "no other node",
		},
		{name: "empty", wantErr: true, errMatch: "no nodes are registered"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Spread(tt.cands, tt.req)
			if tt.wantErr {
				if !errors.Is(err, ErrNoEligibleNode) || !strings.Contains(err.Error(), tt.errMatch) {
					t.Fatalf("Spread() error = %v, want ErrNoEligibleNode containing %q", err, tt.errMatch)
				}
				return
			}
			if err != nil || got.NodeID != tt.want {
				t.Fatalf("Spread() = %q, %v; want %q", got.NodeID, err, tt.want)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name string
		c    Candidate
		req  Request
		ok   bool
	}{
		{"eligible", Candidate{ID: "a", Schedulable: true, Online: true, Labels: map[string]string{"gpu": "yes"}}, Request{Selector: Selector{"gpu": "yes"}}, true},
		{"cordoned", Candidate{ID: "a", Online: true}, Request{}, false},
		{"label value differs", Candidate{ID: "a", Schedulable: true, Online: true, Labels: map[string]string{"gpu": "no"}}, Request{Selector: Selector{"gpu": "yes"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := Check(tt.c, tt.req); (err == nil) != tt.ok {
				t.Fatalf("Check() = %v, want ok=%v", err, tt.ok)
			}
		})
	}
}

func TestParseLabels(t *testing.T) {
	tests := []struct {
		in []string
		ok bool
	}{
		{[]string{"disk=ssd", "zone=eu-1"}, true},
		{[]string{"topology/zone=a"}, true},
		{[]string{"Disk=ssd"}, false},
		{[]string{"disk"}, false},
		{[]string{"disk="}, false},
		{[]string{"disk=has space"}, false},
	}
	for _, tt := range tests {
		_, err := ParseLabels(tt.in)
		if (err == nil) != tt.ok {
			t.Errorf("ParseLabels(%v) error = %v, want ok=%v", tt.in, err, tt.ok)
		}
	}
}
