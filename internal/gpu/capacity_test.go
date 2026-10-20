package gpu

import (
	"errors"
	"testing"
)

func devs(n int) []Device {
	out := make([]Device, n)
	for i := range out {
		out[i] = Device{Index: i, UUID: "GPU-" + string(rune('a'+i))}
	}
	return out
}

func TestLedgerFit(t *testing.T) {
	usable := Info{Present: true, RuntimeInstalled: true, Devices: devs(2)}
	tests := []struct {
		name    string
		info    Info
		claims  []Claim
		claim   Claim
		want    error
		free    int
		reserve int
	}{
		{"empty node fits count", usable, nil, Claim{Count: 1}, nil, 2, 0},
		{"exactly full node fits last GPU", usable, []Claim{{Name: "a", Count: 1}}, Claim{Count: 1}, nil, 1, 1},
		{"exactly full node rejects one more", usable, []Claim{{Name: "a", Count: 1}, {Name: "b", Count: 1}}, Claim{Count: 1}, ErrInsufficient, 0, 2},
		{"all claim takes everything", usable, []Claim{{Name: "a", Count: -1}}, Claim{Count: 1}, ErrInsufficient, 0, 2},
		{"all claim needs an empty node", usable, []Claim{{Name: "a", Count: 1}}, Claim{Count: -1}, ErrInsufficient, 1, 1},
		{"zero count means all", usable, nil, Claim{}, nil, 2, 0},
		{"device id overlap by index", usable, []Claim{{Name: "a", DeviceIDs: []string{"0"}}}, Claim{DeviceIDs: []string{"0"}}, ErrDeviceTaken, 1, 1},
		{"device id overlap by uuid", usable, []Claim{{Name: "a", DeviceIDs: []string{"0"}}}, Claim{DeviceIDs: []string{"gpu-a"}}, ErrDeviceTaken, 1, 1},
		{"disjoint device ids fit", usable, []Claim{{Name: "a", DeviceIDs: []string{"0"}}}, Claim{DeviceIDs: []string{"1"}}, nil, 1, 1},
		{"unknown device id", usable, nil, Claim{DeviceIDs: []string{"5"}}, ErrUnknownDevice, 2, 0},
		{"pinned plus anon leaves nothing for pinned", usable, []Claim{{Name: "a", DeviceIDs: []string{"0"}}, {Name: "b", Count: 1}}, Claim{DeviceIDs: []string{"1"}}, ErrInsufficient, 0, 2},
		{"reserved never exceeds total", usable, []Claim{{Name: "a", Count: 3}}, Claim{Count: 1}, ErrInsufficient, 0, 2},
		{"runtime missing", Info{Present: true, Devices: devs(2)}, nil, Claim{Count: 1}, ErrRuntimeMissing, 2, 0},
		{"no gpu", Info{}, nil, Claim{Count: 1}, ErrNoGPU, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewLedger(tt.info, tt.claims)
			if err := l.Fit(tt.claim); !errors.Is(err, tt.want) {
				t.Fatalf("Fit = %v, want %v", err, tt.want)
			}
			if l.Free() != tt.free || l.Reserved() != tt.reserve {
				t.Fatalf("free/reserved = %d/%d, want %d/%d", l.Free(), l.Reserved(), tt.free, tt.reserve)
			}
		})
	}
}
