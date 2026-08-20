//go:build !windows && !plan9 && !js && !wasip1

package telemetry

import "testing"

func TestParseSyslogTarget(t *testing.T) {
	tests := []struct {
		name        string
		target      string
		wantNetwork string
		wantRaddr   string
		wantErr     bool
	}{
		{name: "empty means local", target: "", wantNetwork: "", wantRaddr: ""},
		{name: "explicit local", target: "local", wantNetwork: "", wantRaddr: ""},
		{name: "udp remote", target: "udp://logs.example.com:514", wantNetwork: "udp", wantRaddr: "logs.example.com:514"},
		{name: "tcp remote", target: "tcp://logs.example.com:601", wantNetwork: "tcp", wantRaddr: "logs.example.com:601"},
		{name: "missing scheme is an error", target: "logs.example.com:514", wantErr: true},
		{name: "missing host is an error", target: "udp://", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			network, raddr, err := parseSyslogTarget(tt.target)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseSyslogTarget(%q) error = nil, want an error", tt.target)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSyslogTarget(%q) error = %v", tt.target, err)
			}
			if network != tt.wantNetwork || raddr != tt.wantRaddr {
				t.Errorf("parseSyslogTarget(%q) = (%q, %q), want (%q, %q)", tt.target, network, raddr, tt.wantNetwork, tt.wantRaddr)
			}
		})
	}
}
