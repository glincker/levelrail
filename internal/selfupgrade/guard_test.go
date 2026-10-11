package selfupgrade

import (
	"strings"
	"testing"
)

func TestDowngradeMessage(t *testing.T) {
	tests := []struct {
		name string
		in   GuardInput
		want []string
		not  []string
	}{
		{
			name: "names the release that last ran",
			in: GuardInput{BinaryVersion: "v0.3.0", BinarySchema: 120, DBSchema: 125, DBVersion: "v0.4.1",
				Program: "levelrail", InstallerCommand: "curl -fsSL https://example.test/install.sh | sudo"},
			want: []string{"v0.3.0, schema 120", "schema 125", "last run by v0.4.1", "forward-only", "data is intact",
				"curl -fsSL https://example.test/install.sh | sudo LEVELRAIL_VERSION=v0.4.1 sh -s upgrade", "sudo levelrail restore-snapshot --list"},
		},
		{
			name: "unknown last version points at the newest release",
			in:   GuardInput{BinaryVersion: "v0.3.0", BinarySchema: 120, DBSchema: 125, Program: "levelrail", InstallerCommand: "curl | sudo"},
			want: []string{"the newest release", "curl | sudo sh -s upgrade"},
			not:  []string{"LEVELRAIL_VERSION="},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := DowngradeMessage(tt.in)
			for _, w := range tt.want {
				if !strings.Contains(msg, w) {
					t.Errorf("message lacks %q:\n%s", w, msg)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(msg, n) {
					t.Errorf("message has %q:\n%s", n, msg)
				}
			}
		})
	}
}
