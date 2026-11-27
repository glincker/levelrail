package compose

import (
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dockerguard"
)

func TestHostAccessRefusedWithoutGrant(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		grants   []string
		wantErr  string
		wantCaps []string
	}{
		{
			name:    "host network no grant",
			body:    "services:\n  hub:\n    image: hub:1\n    network_mode: host\n",
			wantErr: "host_network grant",
		},
		{
			name:     "host network granted",
			body:     "services:\n  hub:\n    image: hub:1\n    network_mode: host\n",
			grants:   []string{dockerguard.HostNetworkGrantCap},
			wantCaps: []string{dockerguard.HostNetworkGrantCap},
		},
		{
			name:    "host network granted but second service",
			body:    "services:\n  hub:\n    image: hub:1\n    network_mode: host\n  db:\n    image: db:1\n",
			grants:  []string{dockerguard.HostNetworkGrantCap},
			wantErr: "project network",
		},
		{
			name:    "device no grant",
			body:    "services:\n  app:\n    image: app:1\n    devices:\n      - /dev/dri:/dev/dri\n",
			wantErr: "devices grant",
		},
		{
			name:     "device granted",
			body:     "services:\n  app:\n    image: app:1\n    devices:\n      - /dev/dri:/dev/dri\n",
			grants:   []string{dockerguard.DeviceGrantCap("/dev/dri")},
			wantCaps: []string{dockerguard.DeviceGrantCap("/dev/dri")},
		},
		{
			name:    "other device refused even with a grant token",
			body:    "services:\n  app:\n    image: app:1\n    devices:\n      - /dev/mem\n",
			grants:  []string{dockerguard.DeviceGrantCap("/dev/mem")},
			wantErr: "allowlist",
		},
		{
			name:    "remapped device refused",
			body:    "services:\n  app:\n    image: app:1\n    devices:\n      - /dev/video0:/dev/mem\n",
			grants:  []string{dockerguard.DeviceGrantCap("/dev/video0")},
			wantErr: "without a remap",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, err := Parse([]byte(c.body))
			if err != nil {
				t.Fatal(err)
			}
			if c.grants != nil {
				f.CapGrants = map[string][]string{}
				for k := range f.Services {
					f.CapGrants[k] = c.grants
				}
			}
			err = f.Validate()
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("want error containing %q, got %v", c.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("granted file refused: %v", err)
			}
			svcs, _, err := ToDesiredServices("app", f)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(svcs[0].GrantedCaps, ",") != strings.Join(c.wantCaps, ",") {
				t.Fatalf("granted caps = %v, want %v", svcs[0].GrantedCaps, c.wantCaps)
			}
		})
	}
}
