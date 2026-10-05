package bindmount

import "testing"

func TestValidateHostPath(t *testing.T) {
	tests := []struct {
		name     string
		hostPath string
		wantErr  bool
	}{
		{name: "valid absolute path", hostPath: "/srv/myapp/data", wantErr: false},
		{name: "relative path rejected", hostPath: "data", wantErr: true},
		{name: "root rejected", hostPath: "/", wantErr: true},
		{name: "exact forbidden path rejected", hostPath: "/etc", wantErr: true},
		{name: "forbidden path prefix rejected", hostPath: "/etc/passwd", wantErr: true},
		{name: "docker socket rejected", hostPath: "/var/run/docker.sock", wantErr: true},
		{name: "var run prefix rejected", hostPath: "/var/run/anything", wantErr: true},
		{name: "run symlink target rejected", hostPath: "/run/docker.sock", wantErr: true},
		{name: "ancestor of docker dir rejected", hostPath: "/var/lib", wantErr: true},
		{name: "ancestor var rejected", hostPath: "/var", wantErr: true},
		{name: "control plane data dir rejected", hostPath: "/var/lib/levelrail-data/secrets", wantErr: true},
		{name: "dev rejected", hostPath: "/dev", wantErr: true},
		{name: "traversal normalised", hostPath: "/srv/../etc/shadow", wantErr: true},
		{name: "var www allowed", hostPath: "/var/www/site", wantErr: false},
		{name: "similar but distinct path allowed", hostPath: "/etcetera/data", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostPath(tt.hostPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHostPath(%q) error = %v, wantErr %v", tt.hostPath, err, tt.wantErr)
			}
		})
	}
}
